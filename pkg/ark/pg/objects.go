package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func LoadObject[T any](
	ctx context.Context, conn Conn, rowToFunc pgx.RowToFunc[T],
	query string, args ...any,
) (T, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		var zero T
		return zero, err
	}

	return pgx.CollectExactlyOneRow(rows, rowToFunc)
}
