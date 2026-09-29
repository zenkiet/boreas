package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type UserStore struct{ pool *pgxpool.Pool }

func NewUserStore(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

const userColumns = `id, username, email, password_hash, role, disabled_at, created_at, updated_at`

func (s *UserStore) List(ctx context.Context) ([]core.User, error) {
	return many(ctx, s.pool, scanUser, "list users", "scan users",
		`SELECT `+userColumns+` FROM users ORDER BY created_at, username`)
}

func (s *UserStore) Get(ctx context.Context, id uuid.UUID) (core.User, error) {
	return one(ctx, s.pool, scanUser, "get user", `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (core.User, error) {
	return one(ctx, s.pool, scanUser, "get user by username",
		`SELECT `+userColumns+` FROM users WHERE lower(username) = lower($1)`, username)
}

func (s *UserStore) Count(ctx context.Context) (int, error) {
	return one(ctx, s.pool, pgx.RowTo[int], "count users", `SELECT count(*) FROM users`)
}

func (s *UserStore) Create(ctx context.Context, user core.User) (core.User, error) {
	return one(ctx, s.pool, scanUser, "create user", `
		INSERT INTO users (username, email, password_hash, role, disabled_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userColumns,
		user.Username, user.Email, user.PasswordHash, user.Role, user.DisabledAt)
}

func (s *UserStore) Update(ctx context.Context, user core.User) (core.User, error) {
	return one(ctx, s.pool, scanUser, "update user", `
		UPDATE users SET username = $2, email = $3, password_hash = $4, role = $5, disabled_at = $6
		WHERE id = $1
		RETURNING `+userColumns,
		user.ID, user.Username, user.Email, user.PasswordHash, user.Role, user.DisabledAt)
}

func (s *UserStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete user", `DELETE FROM users WHERE id = $1`, id)
}

func scanUser(row pgx.CollectableRow) (core.User, error) {
	var u core.User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role,
		&u.DisabledAt, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}
