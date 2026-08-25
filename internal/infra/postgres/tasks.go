package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type TaskStore struct{ pool *pgxpool.Pool }

func NewTaskStore(pool *pgxpool.Pool) *TaskStore { return &TaskStore{pool: pool} }

const taskColumns = `id, project_id, name, description, image, status, dev_status, port,
	container_id, container_ip, labels, env, pending_recreate, error, created_at, updated_at`

func (s *TaskStore) List(ctx context.Context, projectID, userID uuid.UUID, allTasks bool) ([]core.Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+taskColumns+` FROM tasks t
		WHERE t.project_id = $1 AND ($3 OR EXISTS (
			SELECT 1 FROM task_grants g WHERE g.task_id = t.id AND g.user_id = $2))
		ORDER BY t.created_at, t.name`, projectID, userID, allTasks)
	if err != nil {
		return nil, mapError("list tasks", err)
	}
	return collectTasks(rows)
}

func (s *TaskStore) ListAll(ctx context.Context) ([]core.Task, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY created_at, name`)
	if err != nil {
		return nil, mapError("list all tasks", err)
	}
	return collectTasks(rows)
}

func (s *TaskStore) GetByName(ctx context.Context, projectID uuid.UUID, name string) (core.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE project_id = $1 AND name = $2`, projectID, name)
	if err != nil {
		return core.Task{}, mapError("get task", err)
	}
	task, err := pgx.CollectExactlyOneRow(rows, scanTask)
	if err != nil {
		return core.Task{}, mapError("get task", err)
	}
	return task, nil
}

func (s *TaskStore) Create(ctx context.Context, task core.Task) (core.Task, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO tasks (project_id, name, description, image, status, dev_status, port,
			container_id, container_ip, labels, env, pending_recreate, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+taskColumns,
		task.ProjectID, task.Name, task.Description, task.Image, task.Status,
		devStatusOrDefault(task.DevStatus), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		task.PendingRecreate, task.Error)
	if err != nil {
		return core.Task{}, mapError("create task", err)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanTask)
	if err != nil {
		return core.Task{}, mapError("create task", err)
	}
	return created, nil
}

// Update leaves updated_at to the database trigger and returns its value.
func (s *TaskStore) Update(ctx context.Context, task core.Task) (core.Task, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE tasks SET description = $2, image = $3, status = $4, dev_status = $5, port = $6,
			container_id = $7, container_ip = $8, labels = $9, env = $10,
			pending_recreate = $11, error = $12
		WHERE id = $1
		RETURNING `+taskColumns,
		task.ID, task.Description, task.Image, task.Status,
		devStatusOrDefault(task.DevStatus), task.Port,
		task.ContainerID, task.ContainerIP, nonNilMap(task.Labels), nonNilMap(task.Env),
		task.PendingRecreate, task.Error)
	if err != nil {
		return core.Task{}, mapError("update task", err)
	}
	updated, err := pgx.CollectExactlyOneRow(rows, scanTask)
	if err != nil {
		return core.Task{}, mapError("update task", err)
	}
	return updated, nil
}

func (s *TaskStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete task", `DELETE FROM tasks WHERE id = $1`, id)
}

func devStatusOrDefault(status core.DevStatus) core.DevStatus {
	if status == "" {
		return core.DevInProgress
	}
	return status
}

func scanTask(row pgx.CollectableRow) (core.Task, error) {
	var t core.Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Description, &t.Image, &t.Status, &t.DevStatus, &t.Port,
		&t.ContainerID, &t.ContainerIP, &t.Labels, &t.Env, &t.PendingRecreate, &t.Error,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return core.Task{}, err
	}
	t.Labels, t.Env = nonNilMap(t.Labels), nonNilMap(t.Env)
	return t, nil
}

func collectTasks(rows pgx.Rows) ([]core.Task, error) {
	tasks, err := pgx.CollectRows(rows, scanTask)
	if err != nil {
		return nil, mapError("scan tasks", err)
	}
	return tasks, nil
}
