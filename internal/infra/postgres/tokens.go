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

const tokenColumns = `user_id, token_hash, expires_at, revoked_at`

func (s *TokenStore) Create(ctx context.Context, token core.AuthToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`,
		token.UserID, token.TokenHash, token.ExpiresAt)
	return mapError("create token", err)
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

func (s *TokenStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return mapError("revoke user tokens", err)
}

func scanToken(row pgx.CollectableRow) (core.AuthToken, error) {
	var t core.AuthToken
	err := row.Scan(&t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt)
	return t, err
}
