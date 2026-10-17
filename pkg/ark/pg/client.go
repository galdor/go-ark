package pg

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.n16f.net/ark/pkg/ark/json"
	"go.n16f.net/ark/pkg/ark/log"
)

const (
	DefaultPoolSize                     = 5
	DefaultConnectionAcquisitionTimeout = 5000 // milliseconds
)

var (
	ErrNoConnectionAvailable = errors.New("no connection available")
)

type ClientCfg struct {
	URI             string `json:"uri"`
	ApplicationName string `json:"application_name,omitempty"`

	PoolSize int `json:"pool_size,omitempty"`

	ConnectionAcquisitionTimeout int `json:"connection_acquisition_timeout,omitempty"` // milliseconds

	SchemaDirectory string   `json:"schema_directory"`
	SchemaNames     []string `json:"schema_names"`

	Log *log.Logger `json:"-"`
}

type Client struct {
	Cfg *ClientCfg
	Log *log.Logger

	Pool *pgxpool.Pool

	connectionAcquisitionTimeout time.Duration

	stopChan chan struct{}
	wg       sync.WaitGroup
}

func (cfg *ClientCfg) ValidateJSON(v *json.Validator) {
	v.CheckURI("uri", cfg.URI)

	if cfg.PoolSize != 0 {
		// We need at least 2 connections for schema management
		v.CheckIntMinMax("pool_size", cfg.PoolSize, 2, 1000)
	}

	if cfg.ConnectionAcquisitionTimeout != 0 {
		v.CheckIntMin("connection_acquisition_timeout",
			cfg.ConnectionAcquisitionTimeout, 1)
	}

	v.WithChild("schema_names", func() {
		for i, name := range cfg.SchemaNames {
			v.CheckStringNotEmpty(i, name)
		}
	})
}

func NewClient(ctx context.Context, cfg *ClientCfg) (*Client, error) {
	if cfg.PoolSize == 0 {
		cfg.PoolSize = DefaultPoolSize
	}

	if cfg.ConnectionAcquisitionTimeout == 0 {
		cfg.ConnectionAcquisitionTimeout = DefaultConnectionAcquisitionTimeout
	}

	if cfg.Log == nil {
		cfg.Log = log.DefaultLogger()
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	if cfg.ApplicationName != "" {
		runtimeParams := poolCfg.ConnConfig.RuntimeParams
		runtimeParams["application_name"] = cfg.ApplicationName
	}

	poolCfg.MaxConns = int32(cfg.PoolSize)

	poolCfg.MaxConnIdleTime = 10 * time.Minute
	poolCfg.MaxConnLifetimeJitter = time.Second

	cfg.Log.Info("connecting to database %q at %s:%d as %q",
		poolCfg.ConnConfig.Database,
		poolCfg.ConnConfig.Host,
		poolCfg.ConnConfig.Port,
		poolCfg.ConnConfig.User)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to database: %w", err)
	}

	c := Client{
		Cfg: cfg,
		Log: cfg.Log,

		Pool: pool,

		stopChan: make(chan struct{}),
	}

	c.connectionAcquisitionTimeout =
		time.Duration(cfg.ConnectionAcquisitionTimeout) * time.Millisecond

	if c.Cfg.SchemaDirectory != "" {
		if err := c.updateSchemas(ctx); err != nil {
			c.Close()
			return nil, err
		}
	}

	return &c, nil
}

func (c *Client) updateSchemas(ctx context.Context) error {
	for _, name := range c.Cfg.SchemaNames {
		dirPath := path.Join(c.Cfg.SchemaDirectory, name)

		if err := c.UpdateSchema(ctx, name, dirPath); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Close() {
	close(c.stopChan)
	c.wg.Wait()

	c.Pool.Close()
}

func (c *Client) WithConn(ctx context.Context, fn func(Conn) error) error {
	return c.withConn(ctx, fn)
}

func (c *Client) WithTx(ctx context.Context, fn func(Conn) error) error {
	return c.withConn(ctx, func(conn Conn) error {
		if _, err := conn.Exec(ctx, "BEGIN"); err != nil {
			return fmt.Errorf("cannot begin transaction: %w", err)
		}

		if err := fn(conn); err != nil {
			if _, rbErr := conn.Exec(ctx, "ROLLBACK"); rbErr != nil {
				// There is nothing we can do here, and we do want to return the
				// function error, so we simply log the rollback error.
				//
				// Note that when the connection is released by withConn after a
				// rollback failure, the connection will not be in an idle
				// state, and Pgx will close it, as it should be.
				c.Log.Error("cannot rollback transaction: %v", rbErr)
			}

			return err
		}

		if _, err := conn.Exec(ctx, "COMMIT"); err != nil {
			return fmt.Errorf("cannot commit transaction: %w", err)
		}

		return nil
	})
}

func (c *Client) withConn(ctx context.Context, fn func(Conn) error) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, c.connectionAcquisitionTimeout)
	defer cancel()

	conn, err := c.Pool.Acquire(ctx)
	if err != nil {
		// We would like to detect connection errors to return them clearly
		// identified, but pgx is yet another one of those libraries hiding
		// their error types for inane reasons, making life harder for
		// everyone. Not much we can do here, so we will get incredibly
		// verbose error messages (including connection parameters) for each
		// connection failure.

		if errors.Is(err, context.DeadlineExceeded) {
			err = ErrNoConnectionAvailable
		}

		return fmt.Errorf("cannot acquire connection: %w", err)
	}
	defer conn.Release()

	return fn(conn)
}

func TakeAdvisoryTxLock(ctx context.Context, conn Conn, id1, id2 uint32) error {
	_, err := conn.Exec(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, id1, id2)
	return err
}
