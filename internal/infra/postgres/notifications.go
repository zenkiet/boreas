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

func (s *NotificationStore) Create(ctx context.Context, n core.Notification) (core.Notification, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notifications (project_id, task_name, type, status, title, body, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		n.ProjectID, n.TaskName, n.Type, n.Status, n.Title, n.Body, n.UserID).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return core.Notification{}, mapError("create notification", err)
	}
	return n, nil
}

// visible keeps the notifications n that user $1 may read; $2 marks an administrator, who still
// never reads one addressed to someone else.
const visible = `(n.user_id = $1 OR n.user_id IS NULL AND ($2
	OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = n.project_id AND m.user_id = $1)
	OR EXISTS (SELECT 1 FROM task_grants g JOIN tasks t ON t.id = g.task_id
		WHERE g.user_id = $1 AND t.project_id = n.project_id AND t.name = n.task_name)))`

// selectNotifications takes $1 as the user and $2 as whether they are an admin.
const selectNotifications = `
	SELECT n.id, n.project_id, p.slug, n.task_name, n.type, n.status, n.title, n.body, n.created_at,
		EXISTS (SELECT 1 FROM notification_seen s WHERE s.notification_id = n.id AND s.user_id = $1)
	FROM notifications n JOIN projects p ON p.id = n.project_id
	WHERE ` + visible

func scanNotification(row pgx.CollectableRow) (core.Notification, error) {
	var n core.Notification
	err := row.Scan(&n.ID, &n.ProjectID, &n.Project, &n.TaskName, &n.Type, &n.Status, &n.Title, &n.Body, &n.CreatedAt, &n.Seen)
	return n, err
}

// List pages newest first. A nil projectID spans every project; before is the last id already read,
// and (created_at, id) ordering keeps rows sharing a timestamp from falling between pages.
func (s *NotificationStore) List(
	ctx context.Context, userID uuid.UUID, admin bool, projectID, before *uuid.UUID, limit int,
) ([]core.Notification, error) {
	return many(ctx, s.pool, scanNotification, "list notifications", "scan notifications", selectNotifications+`
		AND ($3::uuid IS NULL OR n.project_id = $3)
		AND ($4::uuid IS NULL OR (n.created_at, n.id) < (SELECT b.created_at, b.id FROM notifications b WHERE b.id = $4))
		ORDER BY n.created_at DESC, n.id DESC LIMIT $5`, userID, admin, projectID, before, limit)
}

// Get treats an id the user may not read as one that does not exist.
func (s *NotificationStore) Get(ctx context.Context, userID uuid.UUID, admin bool, id uuid.UUID) (core.Notification, error) {
	return one(ctx, s.pool, scanNotification, "get notification", selectNotifications+` AND n.id = $3`, userID, admin, id)
}

// MarkSeen skips ids outside the user's visibility, so a guessed id reveals nothing.
func (s *NotificationStore) MarkSeen(ctx context.Context, userID uuid.UUID, admin bool, ids []uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_seen (notification_id, user_id)
		SELECT n.id, $1 FROM notifications n WHERE n.id = ANY($3) AND `+visible+`
		ON CONFLICT DO NOTHING`, userID, admin, ids)
	return mapError("mark notifications seen", err)
}

func (s *NotificationStore) MarkUnseen(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM notification_seen WHERE notification_id = $1 AND user_id = $2`, id, userID)
	return mapError("mark notification unseen", err)
}

// LastDeploys maps task IDs to their newest deploy, skipping any older than the task so that a
// name reused after a delete starts clean.
func (s *NotificationStore) LastDeploys(ctx context.Context) (map[uuid.UUID]core.Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, d.status, d.created_at FROM tasks t
		CROSS JOIN LATERAL (
			SELECT n.status, n.created_at FROM notifications n
			WHERE n.project_id = t.project_id AND n.task_name = t.name
			  AND n.type IN ('deployed', 'deploy_failed') AND n.created_at >= t.created_at
			ORDER BY n.created_at DESC LIMIT 1) d`)
	if err != nil {
		return nil, mapError("list last deploys", err)
	}
	deploys := map[uuid.UUID]core.Notification{}
	var (
		taskID uuid.UUID
		n      core.Notification
	)
	if _, err := pgx.ForEachRow(rows, []any{&taskID, &n.Status, &n.CreatedAt}, func() error {
		deploys[taskID] = n
		return nil
	}); err != nil {
		return nil, mapError("scan last deploys", err)
	}
	return deploys, nil
}
