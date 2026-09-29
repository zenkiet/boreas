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
	role, err := one(ctx, s.pool, pgx.RowTo[core.ProjectRole], "get task grant", `
		SELECT g.role FROM task_grants g
		JOIN tasks t ON t.id = g.task_id
		WHERE g.user_id = $2 AND t.project_id = $1 AND t.name = $3`,
		projectID, userID, taskName)
	if errors.Is(err, core.ErrNotFound) {
		return "", nil
	}
	return role, err
}

// AnyInProject backs the project envelope: one grant is enough to see that the project exists.
func (s *GrantStore) AnyInProject(ctx context.Context, projectID, userID uuid.UUID) (bool, error) {
	return one(ctx, s.pool, pgx.RowTo[bool], "check task grants", `
		SELECT EXISTS (
			SELECT 1 FROM task_grants g
			JOIN tasks t ON t.id = g.task_id
			WHERE g.user_id = $2 AND t.project_id = $1)`,
		projectID, userID)
}

func (s *GrantStore) ForUser(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]core.ProjectRole, error) {
	rows, err := s.pool.Query(ctx, `SELECT task_id, role FROM task_grants WHERE user_id = $1`, userID)
	if err != nil {
		return nil, mapError("list user grants", err)
	}
	roles := map[uuid.UUID]core.ProjectRole{}
	var (
		taskID uuid.UUID
		role   core.ProjectRole
	)
	if _, err := pgx.ForEachRow(rows, []any{&taskID, &role}, func() error {
		roles[taskID] = role
		return nil
	}); err != nil {
		return nil, mapError("scan user grants", err)
	}
	return roles, nil
}

func (s *GrantStore) ListForTask(ctx context.Context, taskID uuid.UUID) ([]core.TaskGrant, error) {
	return many(ctx, s.pool, scanGrant, "list task grants", "scan task grants", `
		SELECT g.task_id, g.user_id, u.username, g.role, g.created_at
		FROM task_grants g
		JOIN users u ON u.id = g.user_id
		WHERE g.task_id = $1
		ORDER BY u.username`, taskID)
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
