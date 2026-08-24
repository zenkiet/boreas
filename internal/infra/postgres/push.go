package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PushStore struct{ pool *pgxpool.Pool }

func NewPushStore(pool *pgxpool.Pool) *PushStore { return &PushStore{pool: pool} }

// Create claims a token for the caller, taking it over from whoever registered it
// before: a browser handed to a new user must not keep notifying the old one.
func (s *PushStore) Create(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_subscriptions (token, user_id) VALUES ($1, $2)
		ON CONFLICT (token) DO UPDATE SET user_id = EXCLUDED.user_id`, token, userID)
	return mapError("create push subscription", err)
}

// Delete scopes to the owner so one user cannot unregister another's browser.
func (s *PushStore) Delete(ctx context.Context, userID uuid.UUID, token string) error {
	return deleteRow(ctx, s.pool, "delete push subscription",
		`DELETE FROM push_subscriptions WHERE token = $1 AND user_id = $2`, token, userID)
}

// Tokens mirrors NotificationStore.List: administrators and project members see every
// task, grantees see only what they hold. Disabled accounts see nothing, so revoking a
// user stops their devices along with their login.
func (s *PushStore) Tokens(ctx context.Context, projectID uuid.UUID, taskName string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.token FROM push_subscriptions p
		JOIN users u ON u.id = p.user_id AND u.disabled_at IS NULL
		WHERE u.role = 'admin'
		   OR EXISTS (
			SELECT 1 FROM project_members m
			WHERE m.project_id = $1 AND m.user_id = p.user_id)
		   OR EXISTS (
			SELECT 1 FROM task_grants g
			JOIN tasks t ON t.id = g.task_id
			WHERE g.user_id = p.user_id AND t.project_id = $1 AND t.name = $2)`,
		projectID, taskName)
	if err != nil {
		return nil, mapError("list push subscriptions", err)
	}
	tokens, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, mapError("scan push subscriptions", err)
	}
	return tokens, nil
}
