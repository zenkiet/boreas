# Boreas

Boreas is a per-task staging environment manager. It starts one Docker
container per task, exposes it through a stable subpath, and provides an HTTP
API for lifecycle, environment, and log access. Tasks belong to projects, and
access is controlled per project.

## Architecture

```text
Browser/API client -> Boreas (:8080) -> reverse proxy -> task container
                           |                    |
                    PostgreSQL state       boreas-net
                           |
                    Docker Engine socket
```

The Go code follows clean-architecture boundaries:

- `internal/core`: domain types and interfaces
- `internal/service`: application use cases
- `internal/infra`: Docker, persistence, and proxy implementations
- `internal/transport`: HTTP API, SSE, middleware, and routing

State lives in PostgreSQL and is accessed through `pgx`. The schema is embedded
in the binary and applied at startup, so no separate migration step is needed.
Boreas has no cgo dependencies and builds with `CGO_ENABLED=0`.

## Requirements

- Go 1.26
- Docker Engine
- PostgreSQL 16 or newer

## Quick start

`BOREAS_ADMIN_PASSWORD` seeds the first administrator. It is required only
while the user table is empty and is ignored afterwards.

```bash
BOREAS_ADMIN_PASSWORD=change-me docker compose up --build -d
curl http://localhost:8080/api/v1/health
```

Compose mounts the Docker socket, keeps PostgreSQL data in a named volume, and
connects Boreas to the explicitly named `boreas-net` network. Dynamically
created task containers join the same network.

To run the binary directly against the Compose database:

```bash
make db
BOREAS_ADMIN_PASSWORD=change-me make dev
```

## Authenticate

Every `/api/v1` route except `/health` and `/auth/login` requires a bearer
token. Tokens are returned once at login and expire after 30 days.

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}' | jq -r .token)
```

Changing a password, changing a role, or disabling an account revokes that
user's existing tokens.

## Create and access a task

```bash
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"demo","name":"Demo"}'

curl -X POST http://localhost:8080/api/v1/projects/demo/tasks \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"web","image":"nginx:alpine","port":80}'
```

The task is available at:

```text
http://localhost:8080/demo/web/
```

Proxied task traffic is public; only the API requires a token.

Project slugs and task names must match:

```regex
^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$
```

In other words, 1–63 characters, starting with an ASCII letter or digit and
containing only letters, digits, `.`, `_`, or `-`. Because project slugs
occupy the first path segment, `api`, `health`, and `static` are reserved.
Task names are unique per project, so two projects may each have a `web`.

## Access control

| Role | Scope | Grants |
|---|---|---|
| `admin` | global | Everything, including implicit ownership of every project |
| `user` | global | Access only to projects they belong to |
| `owner` | project | Project settings and membership, plus member rights |
| `member` | project | Task read and lifecycle operations |

## API

All API routes are under `/api/v1`.

| Method | Route | Access | Purpose |
|---|---|---|---|
| `GET` | `/health` | public | Health check |
| `POST` | `/auth/login` | public | Exchange credentials for a token |
| `POST` | `/auth/logout` | user | Revoke the current token |
| `GET` | `/auth/me` | user | Current user |
| `GET` | `/stats` | user | Service statistics |
| `GET` | `/users` | admin | List users |
| `POST` | `/users` | admin | Create a user |
| `PATCH` | `/users/{id}` | admin | Update a user |
| `DELETE` | `/users/{id}` | admin | Delete a user |
| `GET` | `/registry-credentials` | admin | List registry credentials |
| `POST` | `/registry-credentials` | admin | Create a registry credential |
| `DELETE` | `/registry-credentials/{id}` | admin | Delete a registry credential |
| `GET` | `/projects` | user | List reachable projects |
| `POST` | `/projects` | user | Create a project; the creator becomes owner |
| `GET` | `/projects/{project}` | member | Get a project |
| `PATCH` | `/projects/{project}` | owner | Update a project |
| `DELETE` | `/projects/{project}` | owner | Delete a project |
| `GET` | `/projects/{project}/members` | owner | List members |
| `POST` | `/projects/{project}/members` | owner | Add or promote a member |
| `DELETE` | `/projects/{project}/members/{userID}` | owner | Remove a member |
| `GET` | `/projects/{project}/tasks` | member | List tasks |
| `POST` | `/projects/{project}/tasks` | member | Create a task |
| `GET` | `/projects/{project}/tasks/{name}` | member | Get a task |
| `PATCH` | `/projects/{project}/tasks/{name}` | member | Update image, port, labels, env, or description |
| `PUT` | `/projects/{project}/tasks/{name}/state` | member | Start, stop, or restart |
| `DELETE` | `/projects/{project}/tasks/{name}` | member | Delete a task and its container |
| `GET` | `/projects/{project}/tasks/{name}/env` | member | Get task environment |
| `PUT` | `/projects/{project}/tasks/{name}/env` | member | Update task environment |
| `GET` | `/projects/{project}/tasks/{name}/logs` | member | Read task logs |
| `GET` | `/projects/{project}/tasks/{name}/logs/stream` | member | Stream logs over SSE |

Only the fields present in a `PATCH` body are changed. Editing just the
description leaves a running container alone; changing `image`, `port`,
`labels`, or `env` needs a new container, which `auto_restart` applies
immediately (the default) or defers to the next start or restart. A task is
renamed by recreating it, because its name is part of its public URL.

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"image":"nginx:1.27-alpine"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web
```

For live logs, use an SSE-capable client:

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/projects/demo/tasks/web/logs/stream
```

## OpenAPI specification

Every route above is described by an OpenAPI 3.0 document generated from the
same route table the server routes with, so the two cannot drift.

| Location | Purpose |
|---|---|
| `api/openapi.yaml` | Committed contract; generate clients from this |
| `GET /api/v1/openapi.json` | The running server's own specification |
| `GET /api/v1/docs` | Browsable reference (Scalar) |

```bash
make openapi        # Regenerate api/openapi.yaml after changing routes or DTOs
make openapi-check  # Fail if the committed file is stale (suitable for CI)
```

Generate a typed Angular client:

```bash
npx @openapitools/openapi-generator-cli generate \
  -i api/openapi.yaml -g typescript-angular -o ./client
```

The generated client exposes `TaskStatus`, `UserRole`, `ProjectRole`, and
`RegistryKind` as string-literal unions, and UUID fields as `string`.
The documentation endpoints are served only when the server enables them, which
`cmd/boreas` does by default.

## Configuration

Boreas is configured entirely through environment variables. Everything that is
not deployment-specific is a built-in default, so a normal deployment only sets
the database connection and the initial administrator.

| Variable | Default | Purpose |
|---|---|---|
| `BOREAS_PORT` | `8080` | HTTP listen port |
| `BOREAS_DATABASE_URL` | unset | Full connection string; overrides the `BOREAS_DB_*` variables |
| `BOREAS_DB_HOST` | `localhost` | Database host |
| `BOREAS_DB_PORT` | `5432` | Database port |
| `BOREAS_DB_USER` | `postgres` | Database user |
| `BOREAS_DB_PASSWORD` | `postgres` | Database password |
| `BOREAS_DB_NAME` | `boreas` | Database name |
| `BOREAS_DB_SSLMODE` | `disable` | `sslmode` for the connection |
| `BOREAS_ADMIN_USERNAME` | `admin` | Seed administrator username |
| `BOREAS_ADMIN_EMAIL` | `admin@localhost` | Seed administrator email |
| `BOREAS_ADMIN_PASSWORD` | unset | Seed administrator password; required only on an empty database |

Registry credentials are no longer configuration. Store them once through
`/api/v1/registry-credentials` and attach one to a project; Boreas uses it when
pulling that project's images.

## Task application subpaths

Applications behind Boreas must work beneath `/{project}/{task}/` and should
use relative URLs or a matching base path. Boreas passes the prefix to the
container as `BASE_HREF` and rewrites `<base href>` in served HTML. Always
retain the trailing slash, for example `/demo/web/`; requests without it are
redirected.

## Development commands

```bash
make build            # Static pure-Go build (CGO_ENABLED=0)
make db               # Start only PostgreSQL
make test             # Standard Go tests
make test-integration # Postgres store tests; needs a running database
make test-docker      # Docker runtime tests; needs a running Docker daemon
make test-race        # Race detector; CGO is required by the detector itself
make openapi          # Regenerate api/openapi.yaml
make openapi-check    # Verify api/openapi.yaml is current
make docker           # Build boreas:latest
```

## License

MIT
