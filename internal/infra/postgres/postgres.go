// Package postgres implements the core persistence ports on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

// uniqueViolation is the SQLSTATE code Postgres reports for a duplicate key.
const uniqueViolation = "23505"

func mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return errors.Join(core.ErrAlreadyExists, fmt.Errorf("%s: %w", operation, err))
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func deleteRow(ctx context.Context, pool *pgxpool.Pool, operation, query string, args ...any) error {
	tag, err := pool.Exec(ctx, query, args...)
	if err != nil {
		return mapError(operation, err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}
