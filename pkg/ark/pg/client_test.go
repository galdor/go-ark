package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientWithTx(t *testing.T) {
	require := require.New(t)

	ctx := context.Background()

	clientCfg := ClientCfg{
		URI:             "postgres://postgres:postgres@127.0.0.1:5432/service",
		ApplicationName: "test",
	}

	client, err := NewClient(ctx, &clientCfg)
	require.NoError(err)

	err = client.WithConn(ctx, func(conn Conn) error {
		query := `DROP TABLE IF EXISTS foo`
		if _, err := conn.Exec(ctx, query); err != nil {
			return err
		}

		query = `CREATE TABLE foo (i INT)`
		if _, err := conn.Exec(ctx, query); err != nil {
			return err
		}

		return nil
	})
	require.NoError(err)

	// Transactions are not committed if the function panics
	func() {
		defer func() {
			recover()
		}()

		err = client.WithTx(ctx, func(conn Conn) error {
			query := `INSERT INTO foo (i) VALUES (1)`
			if _, err := conn.Exec(ctx, query); err != nil {
				return err
			}

			panic("test")
		})
		require.NoError(err)
	}()

	var count int
	err = client.WithConn(ctx, func(conn Conn) error {
		query := `SELECT COUNT(*) FROM foo`
		err := conn.QueryRow(ctx, query).Scan(&count)
		if err != nil {
			return err
		}

		return nil
	})
	require.NoError(err)
	require.Equal(0, count)
}
