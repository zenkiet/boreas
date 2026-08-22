package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type GrantStore struct{ pool *pgxpool.Pool }

func NewGrantStore(pool *pgxpool.Pool) *GrantStore { return &GrantStore{pool: pool} }

// Role reports the role granted on one task, or "" when nothing is granted.
func (s *GrantStore) Role(ctx context.Context, projectID, userID uuid.UUID, taskName string) (core.ProjectRole, error) {
	var role core.ProjectRole
	err := s.pool.QueryRow(ctx, `
		SELECT g.role FROM task_grants g
		JOIN tasks t ON t.id = g.task_id
		WHERE g.user_id = $2 AND t.project_id = $1 AND t.name = $3`,
		projectID, userID, taskName).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", mapError("get task grant", err)
	}
	return role, nil
}

// AnyInProject backs the project envelope: one grant is enough to see that the project exists.
func (s *GrantStore) AnyInProject(ctx context.Context, projectID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM task_grants g
			JOIN tasks t ON t.id = g.task_id
			WHERE g.user_id = $2 AND t.project_id = $1)`,
		projectID, userID).Scan(&exists)
	if err != nil {
		return false, mapError("check task grants", err)
	}
	return exists, nil
}

func (s *GrantStore) ListForTask(ctx context.Context, taskID uuid.UUID) ([]core.TaskGrant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.task_id, g.user_id, u.username, g.role, g.created_at
		FROM task_grants g
		JOIN users u ON u.id = g.user_id
		WHERE g.task_id = $1
		ORDER BY u.username`, taskID)
	if err != nil {
		return nil, mapError("list task grants", err)
	}
	grants, err := pgx.CollectRows(rows, scanGrant)
	if err != nil {
		return nil, mapError("scan task grants", err)
	}
	return grants, nil
}

func (s *GrantStore) Grant(ctx context.Context, grant core.TaskGrant) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_grants (task_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (task_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		grant.TaskID, grant.UserID, grant.Role)
	return mapError("grant task", err)
}

func (s *GrantStore) Revoke(ctx context.Context, taskID, userID uuid.UUID) error {
	return deleteRow(ctx, s.pool, "revoke task grant",
		`DELETE FROM task_grants WHERE task_id = $1 AND user_id = $2`, taskID, userID)
}

func scanGrant(row pgx.CollectableRow) (core.TaskGrant, error) {
	var g core.TaskGrant
	err := row.Scan(&g.TaskID, &g.UserID, &g.Username, &g.Role, &g.CreatedAt)
	return g, err
}
