# Changelog

## Unreleased

### Storage

- Replaced SQLite with PostgreSQL, accessed through `pgx`. The
  `modernc.org/sqlite` dependency and the `internal/infra/sqlite` package are
  gone.
- The schema is embedded in the binary and applied at startup under a
  PostgreSQL advisory lock, so concurrent instances cannot race and no separate
  migration step is required.
- `updated_at` is now maintained by a database trigger rather than by
  application code.

### Multi-tenancy and access control

- Added users, API tokens, projects, project membership, and per-project
  registry credentials.
- Tasks now belong to a project. Task names only need to be unique within their
  project, so two projects can each run a task called `web`.
- Added bearer-token authentication. Every `/api/v1` route except `/health` and
  `/auth/login` requires a token; tokens expire after 30 days and are revoked
  when a password or role changes or the account is disabled.
- Added `admin` and `user` global roles, plus `owner` and `member` project
  roles. Administrators implicitly own every project, and a project cannot be
  left without an owner.
- Registry credentials moved out of configuration and into the database, where
  they are managed through `/api/v1/registry-credentials` and attached to
  projects. Credential tokens are never serialized in API responses.

### API and proxy

- Task routes are now project-scoped under
  `/api/v1/projects/{project}/tasks/{name}`.
- Proxied task URLs changed from `/{id}/` to `/{project}/{task}/`. Proxied
  traffic remains public; only the API requires a token.
- Reserved the `api`, `health`, and `static` project slugs, which would
  otherwise collide with the first path segment.
- Task identifiers are now UUIDs; `name` carries the previous identifier's
  meaning and keeps the `^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$` format.

### Removed

- Removed per-task CPU and memory limits from the API, the domain model, the
  database, and the Docker runtime.
- Removed the background reaper. Containers are now cleaned up when a task is
  deleted.
- Removed HTTP Basic authentication in favor of bearer tokens.

### Tasks

- Creating or recreating a task now reclaims a container name still held by an
  earlier container of that same task, instead of failing with a Docker name
  conflict. This is what a task hits when Boreas loses its record of a
  container that Docker still has. Containers Boreas does not manage, and
  containers belonging to a different task, keep their name and the request
  still fails.

- Added `PATCH /api/v1/projects/{project}/tasks/{name}`, which updates
  `description`, `image`, `port`, `labels`, and `env`. Only the fields sent are
  changed; a description-only edit leaves a running container untouched, while
  the rest are applied by `auto_restart` (default true) or by the next start.
  Changing the image pulls it before recreating the container, and a successful
  update clears an error left by a previous failure.
- Fixed `Access-Control-Allow-Methods`, which omitted `PATCH` and so blocked
  browsers from reaching the existing user and project update endpoints.

### API documentation

- Added an OpenAPI 3.0 specification covering all 29 endpoints, committed as
  `api/openapi.yaml` and generated from the same route table the server routes
  with, so the router and the contract cannot disagree.
- Added `GET /api/v1/openapi.json` and a browsable Scalar reference at
  `GET /api/v1/docs`, which remembers the bearer token between reloads.
- Added `make openapi` and `make openapi-check`; a test fails when the
  committed specification is stale.
- Request and response payloads are now named Go types rather than anonymous
  structs and maps, which is what makes them describable and keeps generated
  clients accurate.

### Configuration

- Removed YAML configuration, including `config.example.yaml` and the
  `--config` flag. Boreas is configured entirely through environment
  variables.
- Everything that is not deployment-specific is now a built-in default, so a
  deployment normally sets only `BOREAS_DB_*` and the seed administrator.
- Added `BOREAS_ADMIN_USERNAME`, `BOREAS_ADMIN_EMAIL`, and
  `BOREAS_ADMIN_PASSWORD`, which create the first administrator while the user
  table is empty and are ignored afterwards.

### Build and packaging

- Compose now runs PostgreSQL alongside Boreas and waits for its healthcheck
  before starting the application.
- Production Go builds continue to use `CGO_ENABLED=0`; `pgx` needs no C
  compiler.
- Added `make db` and `make test-integration`; the PostgreSQL store tests are
  skipped unless `BOREAS_TEST_DATABASE_URL` is set.

## v1.0.0 - 2025-08-04

### Added

- SQLite-backed persistent task metadata.
- Container state recovery after a Boreas restart.
- `--db-path` for overriding the configured database location.
- Angular dashboard serving from `web-dist/browser`.
