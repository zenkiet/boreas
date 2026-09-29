package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
)

type CredentialStore struct{ pool *pgxpool.Pool }

func NewCredentialStore(pool *pgxpool.Pool) *CredentialStore { return &CredentialStore{pool: pool} }

const credentialColumns = `id, name, registry, username, token, created_by, created_at`

func (s *CredentialStore) List(ctx context.Context) ([]core.RegistryCredential, error) {
	return many(ctx, s.pool, scanCredential, "list credentials", "scan credentials",
		`SELECT `+credentialColumns+` FROM registry_credentials ORDER BY name`)
}

func (s *CredentialStore) Get(ctx context.Context, id uuid.UUID) (core.RegistryCredential, error) {
	return one(ctx, s.pool, scanCredential, "get credential",
		`SELECT `+credentialColumns+` FROM registry_credentials WHERE id = $1`, id)
}

func (s *CredentialStore) Create(ctx context.Context, credential core.RegistryCredential) (core.RegistryCredential, error) {
	return one(ctx, s.pool, scanCredential, "create credential", `
		INSERT INTO registry_credentials (name, registry, username, token, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+credentialColumns,
		credential.Name, credential.Registry, credential.Username, credential.Token, credential.CreatedBy)
}

func (s *CredentialStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete credential", `DELETE FROM registry_credentials WHERE id = $1`, id)
}

func scanCredential(row pgx.CollectableRow) (core.RegistryCredential, error) {
	var c core.RegistryCredential
	err := row.Scan(&c.ID, &c.Name, &c.Registry, &c.Username, &c.Token, &c.CreatedBy, &c.CreatedAt)
	return c, err
}
