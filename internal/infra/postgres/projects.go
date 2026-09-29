package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type ProjectStore struct{ pool *pgxpool.Pool }

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore { return &ProjectStore{pool: pool} }

const projectColumns = `id, slug, name, registry_credential_id,
	default_image, default_port, default_env, created_by, created_at, updated_at`

func (s *ProjectStore) List(ctx context.Context) ([]core.Project, error) {
	return many(ctx, s.pool, scanProject, "list projects", "scan projects",
		`SELECT `+projectColumns+` FROM projects ORDER BY slug`)
}

// ListForUser includes grant-only projects as viewers with default_env emptied: defaults may carry
// project secrets a grantee is not entitled to, and masking here covers every caller by construction.
func (s *ProjectStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]core.ProjectAccess, error) {
	scan := func(row pgx.CollectableRow) (core.ProjectAccess, error) {
		acc := core.ProjectAccess{UserID: userID}
		p := &acc.Project
		err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.RegistryCredentialID, &p.DefaultImage, &p.DefaultPort,
			&p.DefaultEnv, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &acc.Role, &acc.AllTasks)
		p.DefaultEnv = nonNilMap(p.DefaultEnv)
		return acc, err
	}
	return many(ctx, s.pool, scan, "list user projects", "scan user projects", `
		SELECT p.id, p.slug, p.name, p.registry_credential_id, p.default_image, p.default_port,
			CASE WHEN m.user_id IS NULL THEN '{}'::jsonb ELSE p.default_env END,
			p.created_by, p.created_at, p.updated_at, COALESCE(m.role, 'viewer'), m.user_id IS NOT NULL
		FROM projects p
		LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = $1
		WHERE m.user_id IS NOT NULL
		   OR EXISTS (SELECT 1 FROM task_grants g JOIN tasks t ON t.id = g.task_id
		              WHERE g.user_id = $1 AND t.project_id = p.id)
		ORDER BY p.slug`, userID)
}

func (s *ProjectStore) GetBySlug(ctx context.Context, slug string) (core.Project, error) {
	return one(ctx, s.pool, scanProject, "get project by slug",
		`SELECT `+projectColumns+` FROM projects WHERE slug = $1`, slug)
}

func (s *ProjectStore) Count(ctx context.Context) (int, error) {
	return one(ctx, s.pool, pgx.RowTo[int], "count projects", `SELECT count(*) FROM projects`)
}

func (s *ProjectStore) Create(ctx context.Context, project core.Project) (core.Project, error) {
	return one(ctx, s.pool, scanProject, "create project", `
		INSERT INTO projects (slug, name, registry_credential_id,
			default_image, default_port, default_env, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+projectColumns,
		project.Slug, project.Name, project.RegistryCredentialID,
		project.DefaultImage, project.DefaultPort, nonNilMap(project.DefaultEnv), project.CreatedBy)
}

func (s *ProjectStore) Update(ctx context.Context, project core.Project) (core.Project, error) {
	return one(ctx, s.pool, scanProject, "update project", `
		UPDATE projects SET name = $2, registry_credential_id = $3,
			default_image = $4, default_port = $5, default_env = $6
		WHERE id = $1
		RETURNING `+projectColumns,
		project.ID, project.Name, project.RegistryCredentialID,
		project.DefaultImage, project.DefaultPort, nonNilMap(project.DefaultEnv))
}

func (s *ProjectStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete project", `DELETE FROM projects WHERE id = $1`, id)
}

func (s *ProjectStore) ListMembers(ctx context.Context, projectID uuid.UUID) ([]core.ProjectMember, error) {
	return many(ctx, s.pool, scanMember, "list members", "scan members", `
		SELECT m.project_id, m.user_id, u.username, m.role, m.created_at
		FROM project_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1
		ORDER BY u.username`, projectID)
}

func (s *ProjectStore) GetMember(ctx context.Context, projectID, userID uuid.UUID) (core.ProjectMember, error) {
	return one(ctx, s.pool, scanMember, "get member", `
		SELECT m.project_id, m.user_id, u.username, m.role, m.created_at
		FROM project_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.user_id = $2`, projectID, userID)
}

func (s *ProjectStore) AddMember(ctx context.Context, member core.ProjectMember) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		member.ProjectID, member.UserID, member.Role)
	return mapError("add member", err)
}

func (s *ProjectStore) RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error {
	return deleteRow(ctx, s.pool, "remove member",
		`DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, userID)
}

func scanProject(row pgx.CollectableRow) (core.Project, error) {
	var p core.Project
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.RegistryCredentialID,
		&p.DefaultImage, &p.DefaultPort, &p.DefaultEnv, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	p.DefaultEnv = nonNilMap(p.DefaultEnv)
	return p, err
}

func scanMember(row pgx.CollectableRow) (core.ProjectMember, error) {
	var m core.ProjectMember
	err := row.Scan(&m.ProjectID, &m.UserID, &m.Username, &m.Role, &m.CreatedAt)
	return m, err
}
