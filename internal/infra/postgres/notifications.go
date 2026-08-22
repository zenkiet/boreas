package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type NotificationStore struct{ pool *pgxpool.Pool }

func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

const notificationColumns = `id, project_id, task_name, status, title, body, created_at`

func (s *NotificationStore) Create(ctx context.Context, n core.Notification) (core.Notification, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO notifications (project_id, task_name, status, title, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+notificationColumns,
		n.ProjectID, n.TaskName, n.Status, n.Title, n.Body)
	if err != nil {
		return core.Notification{}, mapError("create notification", err)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanNotification)
	if err != nil {
		return core.Notification{}, mapError("create notification", err)
	}
	return created, nil
}

func (s *NotificationStore) List(ctx context.Context, projectID uuid.UUID, limit int) ([]core.Notification, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+notificationColumns+` FROM notifications
		 WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, mapError("list notifications", err)
	}
	notifications, err := pgx.CollectRows(rows, scanNotification)
	if err != nil {
		return nil, mapError("scan notifications", err)
	}
	return notifications, nil
}

func scanNotification(row pgx.CollectableRow) (core.Notification, error) {
	var n core.Notification
	err := row.Scan(&n.ID, &n.ProjectID, &n.TaskName, &n.Status, &n.Title, &n.Body, &n.CreatedAt)
	return n, err
}
