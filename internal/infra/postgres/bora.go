package postgres

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenkiet/boreas/internal/core"
)

type BoraStore struct{ pool *pgxpool.Pool }

func NewBoraStore(pool *pgxpool.Pool) *BoraStore { return &BoraStore{pool: pool} }

// visibleConversation takes $1 as the user and $2 as whether they are an admin.
const visibleConversation = `c.user_id = $1 AND ($2 OR NOT EXISTS (
	SELECT 1 FROM projects p WHERE p.id = ANY(c.projects) AND NOT EXISTS (
		SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = $1)))`

const (
	conversationColumns = `c.id, c.user_id, c.project_id, c.about, c.projects, c.title, c.created_at, c.updated_at`
	conversationPage    = ` AND ($3::timestamptz IS NULL OR (c.updated_at, c.id) < ($3, $4))
		ORDER BY c.updated_at DESC, c.id DESC LIMIT $5`
)

func (s *BoraStore) Save(ctx context.Context, c core.BoraConversation, turn []core.BoraMessage) error {
	return mapError("save conversation", pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bora_conversations (id, user_id, project_id, about, projects, title) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET projects = EXCLUDED.projects, updated_at = now()`,
			c.ID, c.UserID, c.ProjectID, c.About, c.Projects, c.Title); err != nil {
			return err
		}
		for _, m := range turn {
			if _, err := tx.Exec(ctx, `
				INSERT INTO bora_messages (conversation_id, index, role, content, say, actions, choices, sources, ran, images, at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				c.ID, m.Index, m.Role, m.Content, m.Say, m.Actions, m.Choices, m.Sources, m.Ran, m.Images, m.At); err != nil {
				return err
			}
		}
		return nil
	}))
}

func (s *BoraStore) Get(ctx context.Context, userID uuid.UUID, admin bool, id uuid.UUID) (core.BoraConversation, error) {
	c, err := one(ctx, s.pool, scanConversation, "get conversation",
		`SELECT `+conversationColumns+`, '' FROM bora_conversations c WHERE `+visibleConversation+` AND c.id = $3`, userID, admin, id)
	if err != nil {
		return c, err
	}
	c.Messages, err = many(ctx, s.pool, scanMessage, "list messages", "scan messages", `
		SELECT index, role, content, say, actions, choices, sources, ran, images, at FROM bora_messages
		WHERE conversation_id = $1 ORDER BY index`, id)
	return c, err
}

func (s *BoraStore) List(ctx context.Context, userID uuid.UUID, admin bool, query string, before core.BoraCursor, limit int) ([]core.BoraConversation, error) {
	var at *time.Time
	if !before.At.IsZero() {
		at = &before.At
	}
	if query == "" {
		return many(ctx, s.pool, scanConversation, "list conversations", "scan conversations",
			`SELECT `+conversationColumns+`, '' FROM bora_conversations c WHERE `+visibleConversation+conversationPage,
			userID, admin, at, before.ID, limit)
	}
	// Letters and digits only, so no tsquery syntax gets through.
	words := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) == 0 {
		return nil, nil
	}
	return many(ctx, s.pool, scanConversation, "search conversations", "scan conversations", `
		SELECT `+conversationColumns+`, hit.snippet FROM bora_conversations c
		JOIN LATERAL (
			SELECT substr(m.content, greatest(1, strpos(lower(f_unaccent(m.content)), lower(f_unaccent($7))) - 60), 200) AS snippet
			FROM bora_messages m
			WHERE m.conversation_id = c.id AND m.search @@ to_tsquery('simple', f_unaccent($6))
			ORDER BY m.index DESC LIMIT 1
		) hit ON true
		WHERE `+visibleConversation+conversationPage,
		userID, admin, at, before.ID, limit, strings.Join(words, ":* & ")+":*", words[0])
}

func (s *BoraStore) AddRan(ctx context.Context, id uuid.UUID, index int, line string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE bora_messages SET ran = ran || to_jsonb($3::text)
		WHERE conversation_id = $1 AND index = $2 AND role = 'assistant' AND jsonb_array_length(ran) < 10`, id, index, line)
	if err != nil {
		return mapError("add ran", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.Join(core.ErrInvalidInput, errors.New("index must be an answer holding fewer than 10 ran lines"))
	}
	return nil
}

func (s *BoraStore) Delete(ctx context.Context, userID uuid.UUID, admin bool, id *uuid.UUID) ([]uuid.UUID, error) {
	where, args := `c.user_id = $1`, []any{userID}
	if id != nil {
		where, args = visibleConversation+` AND c.id = $3`, []any{userID, admin, *id}
	}
	var images []uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(i), '{}') FROM bora_conversations c
			JOIN bora_messages m ON m.conversation_id = c.id CROSS JOIN LATERAL unnest(m.images) i WHERE `+where,
			args...).Scan(&images); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM bora_conversations c WHERE `+where, args...)
		if err == nil && id != nil && tag.RowsAffected() == 0 {
			return core.ErrNotFound
		}
		return err
	})
	return images, mapError("delete conversations", err)
}

func scanConversation(row pgx.CollectableRow) (core.BoraConversation, error) {
	var c core.BoraConversation
	err := row.Scan(&c.ID, &c.UserID, &c.ProjectID, &c.About, &c.Projects, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.Snippet)
	return c, err
}

func scanMessage(row pgx.CollectableRow) (core.BoraMessage, error) {
	var m core.BoraMessage
	err := row.Scan(&m.Index, &m.Role, &m.Content, &m.Say, &m.Actions, &m.Choices, &m.Sources, &m.Ran, &m.Images, &m.At)
	return m, err
}
