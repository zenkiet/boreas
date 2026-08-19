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
	rows, err := s.pool.Query(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY slug`)
	if err != nil {
		return nil, mapError("list projects", err)
	}
	projects, err := pgx.CollectRows(rows, scanProject)
	if err != nil {
		return nil, mapError("scan projects", err)
	}
	return projects, nil
}

func (s *ProjectStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]core.Project, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+projectColumns+` FROM projects
		WHERE id IN (SELECT project_id FROM project_members WHERE user_id = $1)
		ORDER BY slug`, userID)
	if err != nil {
		return nil, mapError("list user projects", err)
	}
	projects, err := pgx.CollectRows(rows, scanProject)
	if err != nil {
		return nil, mapError("scan user projects", err)
	}
	return projects, nil
}

func (s *ProjectStore) GetBySlug(ctx context.Context, slug string) (core.Project, error) {
	return s.one(ctx, "get project by slug", `SELECT `+projectColumns+` FROM projects WHERE slug = $1`, slug)
}

func (s *ProjectStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&count); err != nil {
		return 0, mapError("count projects", err)
	}
	return count, nil
}

func (s *ProjectStore) Create(ctx context.Context, project core.Project) (core.Project, error) {
	return s.one(ctx, "create project", `
		INSERT INTO projects (slug, name, registry_credential_id,
			default_image, default_port, default_env, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+projectColumns,
		project.Slug, project.Name, project.RegistryCredentialID,
		project.DefaultImage, project.DefaultPort, nonNilMap(project.DefaultEnv), project.CreatedBy)
}

func (s *ProjectStore) Update(ctx context.Context, project core.Project) (core.Project, error) {
	return s.one(ctx, "update project", `
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
	rows, err := s.pool.Query(ctx, `
		SELECT m.project_id, m.user_id, u.username, m.role, m.created_at
		FROM project_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1
		ORDER BY u.username`, projectID)
	if err != nil {
		return nil, mapError("list members", err)
	}
	members, err := pgx.CollectRows(rows, scanMember)
	if err != nil {
		return nil, mapError("scan members", err)
	}
	return members, nil
}

func (s *ProjectStore) GetMember(ctx context.Context, projectID, userID uuid.UUID) (core.ProjectMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.project_id, m.user_id, u.username, m.role, m.created_at
		FROM project_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.user_id = $2`, projectID, userID)
	if err != nil {
		return core.ProjectMember{}, mapError("get member", err)
	}
	member, err := pgx.CollectExactlyOneRow(rows, scanMember)
	if err != nil {
		return core.ProjectMember{}, mapError("get member", err)
	}
	return member, nil
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

func (s *ProjectStore) one(ctx context.Context, operation, query string, args ...any) (core.Project, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return core.Project{}, mapError(operation, err)
	}
	project, err := pgx.CollectExactlyOneRow(rows, scanProject)
	if err != nil {
		return core.Project{}, mapError(operation, err)
	}
	return project, nil
}

func scanProject(row pgx.CollectableRow) (core.Project, error) {
	var p core.Project
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.RegistryCredentialID,
		&p.DefaultImage, &p.DefaultPort, &p.DefaultEnv, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return core.Project{}, err
	}
	p.DefaultEnv = nonNilMap(p.DefaultEnv)
	return p, nil
}

func scanMember(row pgx.CollectableRow) (core.ProjectMember, error) {
	var m core.ProjectMember
	err := row.Scan(&m.ProjectID, &m.UserID, &m.Username, &m.Role, &m.CreatedAt)
	return m, err
}
