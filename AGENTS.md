# AGENTS.md

Boreas is a Go HTTP service that manages per-task staging environments: one
Docker container per task, exposed through a reverse proxy, with state in
PostgreSQL. This file covers what an autonomous agent needs to build, test,
and change this repository safely. The README holds the full API and
configuration reference; when routes or DTOs change, the OpenAPI contract
must be regenerated (see below).

## Requirements

- Go 1.27 (`go.mod` is authoritative)
- Docker Engine with the Compose plugin (local database, runtime tests)
- `golangci-lint` v2 (formatting and linting)
- `lefthook` (Git hooks; installed via `make hooks`)

No cgo: the binary builds with `CGO_ENABLED=0`. Only `make test-race` needs
CGO because the race detector requires it.

## Setup

```bash
make hooks   # install the repository Git hooks (lefthook)
make db      # start only PostgreSQL via docker compose
```

Configuration is environment variables only (`BOREAS_*`, documented in the
README Configuration table). A fresh database needs `BOREAS_ADMIN_PASSWORD`
to seed the first administrator:

```bash
BOREAS_ADMIN_PASSWORD=change-me make dev   # go run ./cmd/boreas
```

Or bring up the full stack in one command:

```bash
BOREAS_ADMIN_PASSWORD=change-me docker compose up --build -d
curl http://localhost:8080/api/v1/health
```

Migrations are embedded in the binary and applied at startup; there is no
separate migration step.

## Build, lint, test

```bash
make build            # static pure-Go build of ./cmd/boreas
make fmt              # gofumpt + goimports via golangci-lint fmt
make lint             # golangci-lint run ./...
make test             # go test ./... (unit tests, no external services)
make test-race        # race detector (needs CGO)
make test-integration # Postgres store tests; needs a running database
make test-docker      # Docker runtime tests; needs a running Docker daemon
```

Integration tests are opt-in by environment variable and skip themselves
otherwise: `BOREAS_TEST_DATABASE_URL` gates the Postgres store tests and
`BOREAS_TEST_DOCKER=1` gates the Docker runtime tests. Plain `make test` is
safe to run anywhere.

Before pushing, the pre-push hook runs lint, the unit tests, and
`make openapi-check`; run those three locally first to avoid a rejected push.

## OpenAPI contract

`api/openapi.yaml` is generated from the same route table the server routes
with. Never edit it by hand.

```bash
make openapi        # regenerate api/openapi.yaml after changing routes/DTOs
make openapi-check  # fail if the committed file is stale (runs on pre-push)
```

Any change to routes, request/response DTOs, or their field tags requires
regenerating and committing `api/openapi.yaml` in the same commit.

## Architecture and conventions

Clean-architecture boundaries, enforced by the `internal/` layout:

- `internal/core`: domain types and interfaces; no infrastructure imports
- `internal/service`: use cases; depends on core interfaces only
- `internal/infra`: Docker, Postgres (`pgx`), reverse proxy, Apprise senders
- `internal/transport/http`: routing, handlers, DTOs, SSE, middleware
- `internal/pkg/database`: pool construction and embedded SQL migrations
- `cmd/boreas`: composition root; `cmd/openapi`: spec generator

Follow the dependency direction: transport → service → core, with infra
implementing core interfaces. New persistent state means a new numbered SQL
file in `internal/pkg/database/migrations/`; never rewrite an applied
migration.

Tests live next to the code as `*_test.go` with `TestXxx` functions. Unit
tests use the fakes and stubs already present (`fakes_test.go`,
`stubs_test.go`, `httptest`); follow that pattern rather than mocking
frameworks. Handlers answer `404` (not `403`) for resources the caller
cannot see, so names do not leak; preserve that behavior in new endpoints.

## Commit messages

Commits follow Conventional Commits with a type-specific emoji, enforced by
the commit-msg hook (`scripts/validate-commit-msg.sh`) and used by cocogitto
to generate the changelog:

```
<type>(<scope>)?: <icon> <subject>
```

| Type     | Icon | Type  | Icon |
| -------- | ---- | ----- | ---- |
| feat     | ✨   | perf  | ⚡   |
| fix      | 🐛   | test  | ✅   |
| docs     | 📝   | build | 📦   |
| refactor | ♻️   | ci    | 👷   |
| style    | 💄   | chore | 🔖   |
| revert   | ⏪   |       |      |

Example: `feat(auth): ✨ add session refresh endpoint`

Releases are automated: cocogitto (`cog.toml`) bumps the version, tags, and
pushes; the tag triggers the GoReleaser publish workflow. Do not bump
versions or edit `CHANGELOG.md` manually.
