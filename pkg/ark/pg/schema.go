package pg

import (
	"context"
	"fmt"
)

const AdvisoryLockId1 uint32 = 0x0100

const (
	AdvisoryLockId2Migrations uint32 = 0x0001
)

func (c *Client) UpdateSchema(
	ctx context.Context, schema, dirPath string,
) error {
	c.Log.Info("updating schema %q using migrations from %q", schema, dirPath)

	var migrations Migrations
	if err := migrations.LoadDirectory(schema, dirPath); err != nil {
		return fmt.Errorf("cannot load migrations: %w", err)
	}

	if len(migrations) == 0 {
		c.Log.Info("no migration available")
		return nil
	}

	err := c.WithTx(ctx, func(conn Conn) error {
		// Take a lock to make sure only one application tries to update the
		// schema at the same time.
		err := TakeAdvisoryTxLock(ctx, conn,
			AdvisoryLockId1, AdvisoryLockId2Migrations)
		if err != nil {
			return fmt.Errorf("cannot take advisory lock: %w", err)
		}

		// Create the table if it does not exist. Note that we do not use the
		// current connection because we need each migration, which will be
		// executed in its own transaction (i.e. before the the end of the
		// main transaction), to see it.
		err = c.WithConn(ctx, func(conn Conn) error {
			return createSchemaVersionTable(ctx, conn)
		})
		if err != nil {
			return fmt.Errorf("cannot create schema version table: %w", err)
		}

		// Load currently applied versions and remove them from the set of
		// migrations.
		appliedVersions, err := loadSchemaVersions(ctx, conn, schema)
		if err != nil {
			return fmt.Errorf("cannot load schema versions: %w", err)
		}

		migrations.RejectVersions(appliedVersions)

		// Apply migrations in order
		migrations.Sort()

		for _, m := range migrations {
			c.Log.Info("applying migration %v", m)

			err := c.WithTx(ctx, func(conn Conn) error {
				return m.Apply(ctx, conn)
			})
			if err != nil {
				return fmt.Errorf("cannot apply migration %v: %w", m, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Close connections in case migrations created new types; this way these
	// types will be discovered by pgx during the next connections.
	conns := c.Pool.AcquireAllIdle(ctx)
	for _, conn := range conns {
		conn.Conn().Close(ctx)
		conn.Release()
	}

	return nil
}

func createSchemaVersionTable(ctx context.Context, conn Conn) error {
	query := `
CREATE TABLE IF NOT EXISTS schema_versions
  (schema VARCHAR NOT NULL,
   version VARCHAR NOT NULL,
   migration_date TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP),

   PRIMARY KEY (schema, version));
`
	_, err := conn.Exec(ctx, query)
	return err
}

func loadSchemaVersions(
	ctx context.Context, conn Conn, schema string,
) (map[string]struct{}, error) {
	query := `
SELECT version
  FROM schema_versions
  WHERE schema = $1;
`
	rows, err := conn.Query(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[string]struct{})

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		versions[version] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return versions, nil
}
