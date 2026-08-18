package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type TokenStore struct{ pool *pgxpool.Pool }

func NewTokenStore(pool *pgxpool.Pool) *TokenStore { return &TokenStore{pool: pool} }

const tokenColumns = `id, user_id, name, kind, token_hash, valid_from, expires_at, revoked_at, created_at`

func (s *TokenStore) Create(ctx context.Context, token core.AuthToken) (core.AuthToken, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO auth_tokens (user_id, name, kind, token_hash, valid_from, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+tokenColumns,
		token.UserID, token.Name, token.Kind, token.TokenHash, token.ValidFrom, token.ExpiresAt)
	if err != nil {
		return core.AuthToken{}, mapError("create token", err)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanToken)
	if err != nil {
		return core.AuthToken{}, mapError("create token", err)
	}
	return created, nil
}

func (s *TokenStore) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tokenColumns+`
		FROM auth_tokens WHERE user_id = $1 AND kind = 'api' ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, mapError("list tokens", err)
	}
	tokens, err := pgx.CollectRows(rows, scanToken)
	if err != nil {
		return nil, mapError("scan tokens", err)
	}
	return tokens, nil
}

func (s *TokenStore) GetByHash(ctx context.Context, hash string) (core.AuthToken, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tokenColumns+` FROM auth_tokens WHERE token_hash = $1`, hash)
	if err != nil {
		return core.AuthToken{}, mapError("get token", err)
	}
	token, err := pgx.CollectExactlyOneRow(rows, scanToken)
	if err != nil {
		return core.AuthToken{}, mapError("get token", err)
	}
	return token, nil
}

func (s *TokenStore) Revoke(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	return mapError("revoke token", err)
}

func (s *TokenStore) RevokeByID(ctx context.Context, userID, tokenID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE auth_tokens SET revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1 AND user_id = $2 AND kind = 'api'`, tokenID, userID)
	if err != nil {
		return mapError("revoke API token", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *TokenStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return mapError("revoke user tokens", err)
}

func scanToken(row pgx.CollectableRow) (core.AuthToken, error) {
	var t core.AuthToken
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Kind, &t.TokenHash, &t.ValidFrom,
		&t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	return t, err
}
