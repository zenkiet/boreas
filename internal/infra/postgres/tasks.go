package postgres

import (
	"cmp"
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type TaskStore struct{ pool *pgxpool.Pool }

func NewTaskStore(pool *pgxpool.Pool) *TaskStore { return &TaskStore{pool: pool} }

// tasks.id, not id: inside the assignee subselect a bare id binds to users.
const taskColumns = `id, project_id, name, description, note, image, status, dev_status, port,
	container_id, container_ip, labels, env, volumes, routes, build, pending_recreate, error, created_at, updated_at,
	COALESCE((SELECT json_agg(json_build_object('user_id', g.user_id, 'username', u.username, 'avatar_at', u.avatar_at)
		ORDER BY u.username) FROM task_grants g JOIN users u ON u.id = g.user_id WHERE g.task_id = tasks.id), '[]')`

func (s *TaskStore) List(ctx context.Context, projectID, userID uuid.UUID, allTasks bool) ([]core.Task, error) {
	return many(ctx, s.pool, scanTask, "list tasks", "scan tasks", `
		SELECT `+taskColumns+` FROM tasks
		WHERE tasks.project_id = $1 AND ($3 OR EXISTS (
			SELECT 1 FROM task_grants g WHERE g.task_id = tasks.id AND g.user_id = $2))
		ORDER BY tasks.created_at, tasks.name`, projectID, userID, allTasks)
}

func (s *TaskStore) ListAll(ctx context.Context) ([]core.Task, error) {
	return many(ctx, s.pool, scanTask, "list all tasks", "scan tasks",
		`SELECT `+taskColumns+` FROM tasks ORDER BY created_at, name`)
}

func (s *TaskStore) GetByName(ctx context.Context, projectID uuid.UUID, name string) (core.Task, error) {
	return one(ctx, s.pool, scanTask, "get task",
		`SELECT `+taskColumns+` FROM tasks WHERE project_id = $1 AND name = $2`, projectID, name)
}

func (s *TaskStore) Create(ctx context.Context, task core.Task) (core.Task, error) {
	return one(ctx, s.pool, scanTask, "create task", `
		INSERT INTO tasks (project_id, name, description, note, image, status, dev_status, port,
			container_id, container_ip, labels, env, volumes, routes, pending_recreate, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+taskColumns,
		task.ProjectID, task.Name, task.Description, task.Note, task.Image, task.Status,
		cmp.Or(task.DevStatus, core.DevInProgress), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		nonNilMap(task.Volumes), nonNilRoutes(task.Routes), task.PendingRecreate, task.Error)
}

// Update leaves updated_at to the database trigger and returns its value.
func (s *TaskStore) Update(ctx context.Context, task core.Task) (core.Task, error) {
	return one(ctx, s.pool, scanTask, "update task", `
		UPDATE tasks SET description = $2, note = $3, image = $4, status = $5, dev_status = $6, port = $7,
			container_id = $8, container_ip = $9, labels = $10, env = $11,
			volumes = $12, routes = $13, pending_recreate = $14, error = $15
		WHERE id = $1
		RETURNING `+taskColumns,
		task.ID, task.Description, task.Note, task.Image, task.Status,
		cmp.Or(task.DevStatus, core.DevInProgress), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		nonNilMap(task.Volumes), nonNilRoutes(task.Routes), task.PendingRecreate, task.Error)
}

// SetBuild writes only build: Update rewrites the row from a copy read earlier, so it must never carry it.
func (s *TaskStore) SetBuild(ctx context.Context, id uuid.UUID, build core.Build) error {
	return deleteRow(ctx, s.pool, "set task build", `UPDATE tasks SET build = $2 WHERE id = $1`, id, build)
}

func (s *TaskStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete task", `DELETE FROM tasks WHERE id = $1`, id)
}

func scanTask(row pgx.CollectableRow) (core.Task, error) {
	var t core.Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Description, &t.Note, &t.Image, &t.Status, &t.DevStatus, &t.Port,
		&t.ContainerID, &t.ContainerIP, &t.Labels, &t.Env, &t.Volumes, &t.Routes, &t.Build, &t.PendingRecreate, &t.Error,
		&t.CreatedAt, &t.UpdatedAt, &t.Assignees)
	t.Labels, t.Env, t.Volumes, t.Routes = nonNilMap(t.Labels), nonNilMap(t.Env), nonNilMap(t.Volumes), nonNilRoutes(t.Routes)
	return t, err
}

func nonNilRoutes(routes []core.Route) []core.Route {
	if routes == nil {
		return []core.Route{}
	}
	return routes
}
