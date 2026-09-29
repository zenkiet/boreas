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

const taskColumns = `id, project_id, name, description, note, image, status, dev_status, port,
	container_id, container_ip, labels, env, pending_recreate, error, created_at, updated_at`

func (s *TaskStore) List(ctx context.Context, projectID, userID uuid.UUID, allTasks bool) ([]core.Task, error) {
	return many(ctx, s.pool, scanTask, "list tasks", "scan tasks", `
		SELECT `+taskColumns+` FROM tasks t
		WHERE t.project_id = $1 AND ($3 OR EXISTS (
			SELECT 1 FROM task_grants g WHERE g.task_id = t.id AND g.user_id = $2))
		ORDER BY t.created_at, t.name`, projectID, userID, allTasks)
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
			container_id, container_ip, labels, env, pending_recreate, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+taskColumns,
		task.ProjectID, task.Name, task.Description, task.Note, task.Image, task.Status,
		cmp.Or(task.DevStatus, core.DevInProgress), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		task.PendingRecreate, task.Error)
}

// Update leaves updated_at to the database trigger and returns its value.
func (s *TaskStore) Update(ctx context.Context, task core.Task) (core.Task, error) {
	return one(ctx, s.pool, scanTask, "update task", `
		UPDATE tasks SET description = $2, note = $3, image = $4, status = $5, dev_status = $6, port = $7,
			container_id = $8, container_ip = $9, labels = $10, env = $11,
			pending_recreate = $12, error = $13
		WHERE id = $1
		RETURNING `+taskColumns,
		task.ID, task.Description, task.Note, task.Image, task.Status,
		cmp.Or(task.DevStatus, core.DevInProgress), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		task.PendingRecreate, task.Error)
}

func (s *TaskStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete task", `DELETE FROM tasks WHERE id = $1`, id)
}

func scanTask(row pgx.CollectableRow) (core.Task, error) {
	var t core.Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Description, &t.Note, &t.Image, &t.Status, &t.DevStatus, &t.Port,
		&t.ContainerID, &t.ContainerIP, &t.Labels, &t.Env, &t.PendingRecreate, &t.Error,
		&t.CreatedAt, &t.UpdatedAt)
	t.Labels, t.Env = nonNilMap(t.Labels), nonNilMap(t.Env)
	return t, err
}
