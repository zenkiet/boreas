package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type TokenStore struct{ pool *pgxpool.Pool }

func NewTokenStore(pool *pgxpool.Pool) *TokenStore { return &TokenStore{pool: pool} }

const tokenColumns = `id, user_id, name, kind, token_hash, device, valid_from, expires_at, revoked_at, last_used_at, created_at`

func (s *TokenStore) Create(ctx context.Context, token core.AuthToken) (core.AuthToken, error) {
	return one(ctx, s.pool, scanToken, "create token", `
		INSERT INTO auth_tokens (user_id, name, kind, token_hash, device, valid_from, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+tokenColumns,
		token.UserID, token.Name, token.Kind, token.TokenHash, token.Device, token.ValidFrom, token.ExpiresAt)
}

func (s *TokenStore) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	return many(ctx, s.pool, scanToken, "list tokens", "scan tokens", `SELECT `+tokenColumns+`
		FROM auth_tokens WHERE user_id = $1 AND kind = 'api' ORDER BY created_at DESC`, userID)
}

func (s *TokenStore) ListSessions(ctx context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	return many(ctx, s.pool, scanToken, "list sessions", "scan sessions", `SELECT `+tokenColumns+`
		FROM auth_tokens WHERE user_id = $1 AND kind = 'session' AND revoked_at IS NULL AND expires_at > now()
		ORDER BY COALESCE(last_used_at, created_at) DESC`, userID)
}

func (s *TokenStore) GetByHash(ctx context.Context, hash string) (core.AuthToken, error) {
	return one(ctx, s.pool, scanToken, "get token",
		`SELECT `+tokenColumns+` FROM auth_tokens WHERE token_hash = $1`, hash)
}

func (s *TokenStore) Touch(ctx context.Context, id uuid.UUID, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE auth_tokens SET last_used_at = now(), expires_at = $2
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute' OR expires_at <> $2)`,
		id, expiresAt)
	return mapError("touch token", err)
}

func (s *TokenStore) Revoke(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	return mapError("revoke token", err)
}

func (s *TokenStore) RevokeByID(ctx context.Context, userID, tokenID uuid.UUID, kind core.TokenKind) error {
	return deleteRow(ctx, s.pool, "revoke token", `
		UPDATE auth_tokens SET revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1 AND user_id = $2 AND kind = $3`, tokenID, userID, kind)
}

func (s *TokenStore) RevokeSessions(ctx context.Context, userID uuid.UUID, keepHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE auth_tokens SET revoked_at = now()
		WHERE user_id = $1 AND kind = 'session' AND revoked_at IS NULL AND token_hash <> $2`, userID, keepHash)
	return mapError("revoke sessions", err)
}

func (s *TokenStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return mapError("revoke user tokens", err)
}

func scanToken(row pgx.CollectableRow) (core.AuthToken, error) {
	var t core.AuthToken
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Kind, &t.TokenHash, &t.Device, &t.ValidFrom,
		&t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedAt)
	return t, err
}
