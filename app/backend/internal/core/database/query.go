package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Lister is what Select needs: a pool or a transaction.
type Lister interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Execer is what a write needs: a pool or a transaction.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Select reads every row of table matching where (SQL after WHERE, may be
// empty and may end with ORDER BY or LIMIT) into records of type T.
func Select[T any](ctx context.Context, q Lister, table, where string, args ...any) ([]T, error) {
	var zero T
	cols := SelectList(&zero)
	if cols == "" {
		return nil, fmt.Errorf("select %s: %T is not a struct", table, zero)
	}
	sqlText := "SELECT " + cols + " FROM " + table
	if where != "" {
		sqlText += " WHERE " + where
	}
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("select %s: %w", table, err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var v T
		if err := rows.Scan(ScanInto(&v)...); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
