# Development

A contributor checklist for Boreas. [AGENTS.md](../AGENTS.md) is the full
repository policy and takes precedence.

## Run locally

Install Go at the version in [`go.mod`](../go.mod), Node.js 26, pnpm 12,
Docker Engine with Compose, and [lefthook](https://github.com/evilmartians/lefthook).

```bash
make hooks
make db
make frontend   # build the directory page embedded by the server
BOREAS_ADMIN_USERNAME=admin BOREAS_ADMIN_EMAIL=admin@localhost \
  BOREAS_ADMIN_PASSWORD=change-me make dev
```

`make dev` runs `go run ./cmd/boreas`; the default database settings match
Compose. Migrations run at startup. Configuration is `BOREAS_*` environment
variables; document new settings and their defaults in
[Configuration](configuration.md).

Tasks use container IPs on `boreas-net`. Docker Desktop does not expose those
IPs to macOS or Windows hosts, so run Boreas inside Compose for end-to-end
use there:

```bash
BOREAS_ADMIN_PASSWORD=change-me docker compose up --build -d
```

## Keep the boundaries

- `cmd/boreas`: composition root; `cmd/openapi`: API spec generator.
- `internal/core`: domain types and interfaces, no infrastructure imports.
- `internal/service`: use cases depending on core interfaces.
- `internal/infra`: implementations for Docker, PostgreSQL and other services.
- `internal/transport/http`: routes, handlers, DTOs, middleware and streams.
- `internal/pkg/database`: connection pool and embedded migrations.
- `internal/config`: environment settings; `frontend`: SvelteKit directory
  source, built into `internal/web/public` and embedded by `internal/web`.

Dependencies flow **transport → service → core**; infra implements core
interfaces. Keep tests beside the code as `*_test.go`, using existing fakes,
stubs and `httptest`, not a mocking framework. Return `404`, not `403`, for
resources the caller cannot see.

## Check your change

```bash
make fmt
make lint
make test
make openapi-check
```

The pre-push hook runs lint, unit tests and the OpenAPI check. `make fmt` and
`make lint` run the golangci-lint version pinned in the Makefile with your Go;
no separate linter install is needed.

| Command | When to use it |
| --- | --- |
| `make build` | Build the frontend, then the pure-Go binary (`CGO_ENABLED=0`) |
| `make test` | Unit tests; no external services required by default |
| `make test-race` | Race checks; requires cgo |
| `make test-integration` | PostgreSQL store tests; requires a running database |
| `make test-docker` | Runtime tests; requires a running Docker daemon |

Integration tests write data: set `BOREAS_TEST_DATABASE_URL` to a scratch
database. The Make target defaults to the local Compose database. Plain
`make test` skips PostgreSQL and Docker integration tests unless
`BOREAS_TEST_DATABASE_URL` or `BOREAS_TEST_DOCKER=1` enables them.

Before changing Bora's screening prompt or model, rerun `TestClarifyEval`
with `BOREAS_TEST_AI_BASE_URL` and `BOREAS_TEST_AI_MODEL` set, plus
`BOREAS_TEST_AI_API_KEY` if needed. Those variables also enable the spec
question evaluation; both call a real model.

## Update contracts and migrations

- After changing routes, request/response DTOs or field tags, run
  `make openapi` and include `api/openapi.yaml` in the same commit. Never edit
  it by hand: the server's route table generates it, and clients generate
  wire types from it.
- Add a new numbered SQL file in `internal/pkg/database/migrations/` for
  persistent changes. Never rewrite an applied migration.

## Commit and release

Use Conventional Commits with the required type-specific emoji, checked by
commit-msg; see the [format and icon table](../AGENTS.md#commit-messages).
For example: `fix(auth): 🐛 reject expired tokens`.

Cocogitto handles version bumps, `CHANGELOG.md`, tags and pushes. Version tags
trigger the Publish workflow and GoReleaser for release archives and container
images. Do not bump versions or edit `CHANGELOG.md` manually.
