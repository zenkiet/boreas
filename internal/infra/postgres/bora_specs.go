package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenkiet/boreas/internal/core"
)

type SpecStore struct{ pool *pgxpool.Pool }

func NewSpecStore(pool *pgxpool.Pool) *SpecStore { return &SpecStore{pool: pool} }

const visibleSpec = `($2 OR cardinality(s.projects) = (
	SELECT count(*) FROM project_members m WHERE m.user_id = $1 AND m.project_id = ANY(s.projects)))`

func (s *SpecStore) Get(ctx context.Context, userID uuid.UUID, admin bool, id uuid.UUID) (core.BoraSpec, error) {
	return one(ctx, s.pool, scanSpec, "get spec",
		`SELECT s.id, s.key, s.title, s.projects, s.slugs, s.state, s.body, s.created_by, s.created_at, s.updated_at
		FROM bora_specs s WHERE `+visibleSpec+` AND s.id = $3`, userID, admin, id)
}

func (s *SpecStore) List(ctx context.Context, userID uuid.UUID, admin bool, project, key string) ([]core.BoraSpec, error) {
	return many(ctx, s.pool, scanSpec, "list specs", "scan specs",
		`SELECT s.id, s.key, s.title, s.projects, s.slugs, s.state, s.body - 'ticket' - 'findings' - 'plans', s.created_by, s.created_at, s.updated_at
		FROM bora_specs s WHERE `+visibleSpec+` AND ($3 = '' OR $3 = ANY(s.slugs)) AND ($4 = '' OR lower(s.key) = lower($4))
		ORDER BY s.updated_at DESC, s.id DESC LIMIT 50`, userID, admin, project, key)
}

func (s *SpecStore) Save(ctx context.Context, b core.BoraSpec, from core.SpecState) error {
	if from == "" {
		_, err := s.pool.Exec(ctx, `INSERT INTO bora_specs (id, key, title, projects, slugs, state, body, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, b.ID, b.Key, b.Title, b.Projects, b.Slugs, b.State, b.Body, b.CreatedBy)
		return mapError("save spec", err)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE bora_specs SET title = $2, state = $3, body = $4, updated_at = now()
		WHERE id = $1 AND state = $5`, b.ID, b.Title, b.State, b.Body, from)
	if err == nil && tag.RowsAffected() == 0 {
		return core.ErrConflict
	}
	return mapError("save spec", err)
}

func (s *SpecStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete spec", `DELETE FROM bora_specs WHERE id = $1`, id)
}

func scanSpec(row pgx.CollectableRow) (core.BoraSpec, error) {
	var b core.BoraSpec
	err := row.Scan(&b.ID, &b.Key, &b.Title, &b.Projects, &b.Slugs, &b.State, &b.Body, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}
