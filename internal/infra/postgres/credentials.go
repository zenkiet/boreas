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
	rows, err := s.pool.Query(ctx, `SELECT `+credentialColumns+` FROM registry_credentials ORDER BY name`)
	if err != nil {
		return nil, mapError("list credentials", err)
	}
	credentials, err := pgx.CollectRows(rows, scanCredential)
	if err != nil {
		return nil, mapError("scan credentials", err)
	}
	return credentials, nil
}

func (s *CredentialStore) Get(ctx context.Context, id uuid.UUID) (core.RegistryCredential, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+credentialColumns+` FROM registry_credentials WHERE id = $1`, id)
	if err != nil {
		return core.RegistryCredential{}, mapError("get credential", err)
	}
	credential, err := pgx.CollectExactlyOneRow(rows, scanCredential)
	if err != nil {
		return core.RegistryCredential{}, mapError("get credential", err)
	}
	return credential, nil
}

func (s *CredentialStore) Create(ctx context.Context, credential core.RegistryCredential) (core.RegistryCredential, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO registry_credentials (name, registry, username, token, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+credentialColumns,
		credential.Name, credential.Registry, credential.Username, credential.Token, credential.CreatedBy)
	if err != nil {
		return core.RegistryCredential{}, mapError("create credential", err)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanCredential)
	if err != nil {
		return core.RegistryCredential{}, mapError("create credential", err)
	}
	return created, nil
}

func (s *CredentialStore) Delete(ctx context.Context, id uuid.UUID) error {
	return deleteRow(ctx, s.pool, "delete credential", `DELETE FROM registry_credentials WHERE id = $1`, id)
}

func scanCredential(row pgx.CollectableRow) (core.RegistryCredential, error) {
	var c core.RegistryCredential
	err := row.Scan(&c.ID, &c.Name, &c.Registry, &c.Username, &c.Token, &c.CreatedBy, &c.CreatedAt)
	return c, err
}
