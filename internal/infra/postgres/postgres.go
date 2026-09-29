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

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	// restrictViolation is what Postgres 18 reports instead for an ON DELETE RESTRICT reference.
	restrictViolation = "23001"
)

func mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case uniqueViolation:
			return errors.Join(core.ErrAlreadyExists, fmt.Errorf("%s: %w", operation, err))
		case foreignKeyViolation, restrictViolation:
			return errors.Join(core.ErrConflict, fmt.Errorf("%s: %w", operation, err))
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func one[T any](ctx context.Context, pool *pgxpool.Pool, scan pgx.RowToFunc[T], operation, query string, args ...any) (T, error) {
	var zero T
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return zero, mapError(operation, err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, scan)
	if err != nil {
		return zero, mapError(operation, err)
	}
	return row, nil
}

func many[T any](ctx context.Context, pool *pgxpool.Pool, scan pgx.RowToFunc[T], listOp, scanOp, query string, args ...any) ([]T, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapError(listOp, err)
	}
	items, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return nil, mapError(scanOp, err)
	}
	return items, nil
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
