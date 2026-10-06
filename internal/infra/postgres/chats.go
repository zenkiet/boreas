package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenkiet/boreas/internal/core"
)

type ChatStore struct{ pool *pgxpool.Pool }

func NewChatStore(pool *pgxpool.Pool) *ChatStore { return &ChatStore{pool: pool} }

const chatColumns = `c.id, c.project_id, p.slug, c.user_id, c.title, c.messages, c.created_at, c.updated_at`

func (s *ChatStore) Create(ctx context.Context, chat core.Chat) (core.Chat, error) {
	return one(ctx, s.pool, scanChat, "create chat", `
		WITH c AS (
			INSERT INTO chats (project_id, user_id, title, messages) VALUES ($1, $2, $3, $4) RETURNING *
		)
		SELECT `+chatColumns+` FROM c JOIN projects p ON p.id = c.project_id`,
		chat.ProjectID, chat.UserID, chat.Title, chat.Messages)
}

func (s *ChatStore) Get(ctx context.Context, id, userID uuid.UUID) (core.Chat, error) {
	return one(ctx, s.pool, scanChat, "get chat",
		`SELECT `+chatColumns+` FROM chats c JOIN projects p ON p.id = c.project_id WHERE c.id = $1 AND c.user_id = $2`, id, userID)
}

func (s *ChatStore) List(ctx context.Context, userID uuid.UUID) ([]core.Chat, error) {
	return many(ctx, s.pool, scanChat, "list chats", "scan chats", `
		SELECT c.id, c.project_id, p.slug, c.user_id, c.title, '[]'::jsonb, c.created_at, c.updated_at
		FROM chats c JOIN projects p ON p.id = c.project_id
		WHERE c.user_id = $1 ORDER BY c.updated_at DESC LIMIT 100`, userID)
}

func (s *ChatStore) Append(ctx context.Context, id uuid.UUID, messages []core.ChatMessage) error {
	return deleteRow(ctx, s.pool, "append chat messages",
		`UPDATE chats SET messages = messages || $2, updated_at = now() WHERE id = $1`, id, messages)
}

func (s *ChatStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete chat", `DELETE FROM chats WHERE id = $1 AND user_id = $2`, id, userID)
}

func scanChat(row pgx.CollectableRow) (core.Chat, error) {
	var c core.Chat
	err := row.Scan(&c.ID, &c.ProjectID, &c.ProjectSlug, &c.UserID, &c.Title, &c.Messages, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}
