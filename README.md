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

- Go 1.27
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
user's existing login sessions and API tokens.

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

Two global roles decide who administers the installation:

| Role    | Grants                                                                       |
| ------- | ---------------------------------------------------------------------------- |
| `admin` | Users, registry credentials, project creation, and ownership of every project |
| `user`  | Only the projects they are a member of, or hold a task grant in               |

Within a project, four roles stack, each adding to the one before it:

| Role       | Adds                                              |
| ---------- | ------------------------------------------------- |
| `viewer`   | Read tasks, logs, and deploy notifications        |
| `operator` | Start, stop, restart, and deploy                  |
| `member`   | Create, update, and delete tasks                  |
| `owner`    | Project settings, membership, and task grants     |

Add an existing user to a project:

```bash
curl -X POST http://localhost:8080/api/v1/projects/demo/members \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"user_id":"<id>","role":"viewer"}'
```

### Task grants

A grant raises a user's role on one task above whatever the project gives them.
It never lowers it: the effective role is the higher of the two. Grant `owner`
is rejected, since it only means something at project scope.

```bash
curl -X POST http://localhost:8080/api/v1/projects/demo/tasks/web/grants \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"user_id":"<id>","role":"operator"}'
```

A grant is enough to reach the project without being a member. Such a user sees
only the tasks they hold grants on, only those tasks' deploy notifications, and
never the project's task form defaults, which may carry secrets.

Grants are deleted with the task they point at, so access cannot outlive its
subject. A project or task the caller cannot reach answers `404`, not `403`, so
that names do not leak. `403` means the caller can see it but ranks too low.

## API

All API routes are under `/api/v1`.

| Method   | Route                                          | Access        | Purpose                                         |
| -------- | ---------------------------------------------- | ------------- | ----------------------------------------------- |
| `GET`    | `/health`                                      | public        | Health check                                    |
| `POST`   | `/auth/login`                                  | public        | Exchange credentials for a token                |
| `POST`   | `/auth/logout`                                 | user          | Revoke the current token                        |
| `GET`    | `/auth/me`                                     | user          | Current user                                    |
| `GET`    | `/auth/tokens`                                 | login session | List your API tokens                            |
| `POST`   | `/auth/tokens`                                 | login session | Create an API token                             |
| `DELETE` | `/auth/tokens/{id}`                            | login session | Revoke one of your API tokens                   |
| `GET`    | `/stats`                                       | user          | Service statistics                              |
| `GET`    | `/users`                                       | admin         | List users                                      |
| `POST`   | `/users`                                       | admin         | Create a user                                   |
| `PATCH`  | `/users/{id}`                                  | admin         | Update a user                                   |
| `DELETE` | `/users/{id}`                                  | admin         | Delete a user                                   |
| `GET`    | `/registry-credentials`                        | admin         | List registry credentials                       |
| `POST`   | `/registry-credentials`                        | admin         | Create a registry credential                    |
| `DELETE` | `/registry-credentials/{id}`                   | admin         | Delete a registry credential                    |
| `GET`    | `/projects`                                    | user          | List reachable projects                         |
| `POST`   | `/projects`                                    | admin         | Create a project; the creator becomes owner     |
| `GET`    | `/projects/{project}`                          | viewer        | Get a project                                   |
| `PATCH`  | `/projects/{project}`                          | owner         | Update a project                                |
| `DELETE` | `/projects/{project}`                          | owner         | Delete a project                                |
| `GET`    | `/projects/{project}/members`                  | owner         | List members                                    |
| `POST`   | `/projects/{project}/members`                  | owner         | Add or promote a member                         |
| `DELETE` | `/projects/{project}/members/{userID}`         | owner         | Remove a member                                 |
| `GET`    | `/projects/{project}/notifications`            | viewer        | List deploy notifications                       |
| `GET`    | `/projects/{project}/metrics/stream`           | viewer        | Stream metrics for every running task over SSE  |
| `GET`    | `/projects/{project}/tasks`                    | viewer        | List tasks                                      |
| `POST`   | `/projects/{project}/tasks`                    | member        | Create a task                                   |
| `GET`    | `/projects/{project}/tasks/{name}`             | viewer        | Get a task                                      |
| `PATCH`  | `/projects/{project}/tasks/{name}`             | member        | Update image, port, labels, env, description, note, or dev status |
| `POST`   | `/projects/{project}/tasks/{name}/deploy`      | operator      | Deploy an image built elsewhere                 |
| `PUT`    | `/projects/{project}/tasks/{name}/state`       | operator      | Start, stop, or restart                         |
| `DELETE` | `/projects/{project}/tasks/{name}`             | member        | Delete a task and its container                 |
| `GET`    | `/projects/{project}/tasks/{name}/logs`        | viewer        | Read task logs                                  |
| `GET`    | `/projects/{project}/tasks/{name}/logs/stream` | viewer        | Stream logs over SSE                            |
| `GET`    | `/projects/{project}/tasks/{name}/metrics/stream` | viewer     | Stream CPU, memory, and network over SSE        |
| `GET`    | `/projects/{project}/tasks/{name}/grants`      | owner         | List task grants                                |
| `POST`   | `/projects/{project}/tasks/{name}/grants`      | owner         | Grant a user access to one task                 |
| `DELETE` | `/projects/{project}/tasks/{name}/grants/{userID}` | owner     | Revoke a task grant                             |

Only the fields present in a `PATCH` body are changed. Editing just the
description or dev status leaves a running container alone; changing `image`,
`port`, `labels`, or `env` needs a new container, which `auto_restart` applies
immediately (the default) or defers to the next start or restart. A task is
renamed by recreating it, because its name is part of its public URL.

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"image":"nginx:1.27-alpine"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web
```

Task responses include their current `env`. Update or clear it through the
same PATCH endpoint:

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"env":{"APP_MODE":"staging"},"auto_restart":false}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web
```

The former dedicated `GET` and `PUT` `/tasks/{name}/env` endpoints have been
removed. Clients should read `task.env` and use `PATCH /tasks/{name}` instead.

## Development status

`status` reports the container; `dev_status` reports the code. A task can run
while it is not yet worth testing, so the two are tracked separately.

| `dev_status`  | Meaning                                | Suggested colour |
| ------------- | -------------------------------------- | ---------------- |
| `in_progress` | Normal development, the default        | Yellow           |
| `blocked`     | Broken or stuck, do not hand it to QA  | Red              |
| `ready`       | Dev is done, QA can test it            | Green            |

Every task is created `in_progress`. Any member of the task, that is a project
member or owner, a user holding a `member` grant on it, or an administrator,
moves it between the three values. Deploys never change it, so it says what a
person last said it says.

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"dev_status":"ready"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web
```

The API returns the value only; mapping the three values to colours is the
client's business.

## Task notes

A task carries two free-text fields. `description` is the one-line summary that
list views and notification bodies show, so keep it short. `note` is an
open-ended scratchpad for whatever the task needs — context, checklists,
credentials to rotate, links:

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"note":"## Context\n- seed the staging DB\n\n`make db`"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web
```

Both are stored and returned verbatim, so `note` holds Markdown without any
server-side interpretation: Boreas never renders it. A client that renders it
as HTML must sanitise the result, because any member of a task can write it.
Neither field changes the container and neither raises a notification.

## Task form defaults

A project carries `default_image`, `default_port`, and `default_env` so a UI can
prefill its task creation form. They are stored hints only: task creation uses
exactly the `image`, `port`, and `env` it is sent, and editing them never
changes an existing task or restarts a container.

```bash
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"default_image":"nginx:alpine","default_port":8080,"default_env":{"APP_ENV":"dev"}}' \
  http://localhost:8080/api/v1/projects/demo
```

Send `""` for `default_image` or `{}` for `default_env` to clear a default;
omit a field to leave it unchanged. New projects start at `""`, `80`, and `{}`.

## Deploy from a build pipeline

Create a dedicated API token with a login session. The validity window may be
scheduled in the future but cannot exceed 90 days:

```bash
curl -X POST -H "$AUTH" -H "$JSON" \
  -d '{
    "name":"staging-deployer",
    "valid_from":"2026-08-18T00:00:00Z",
    "valid_to":"2026-11-16T00:00:00Z"
  }' \
  http://localhost:8080/api/v1/auth/tokens
```

The plaintext token is returned only by this response. Store it immediately in
the CI secret manager; Boreas keeps only its SHA-256 hash and cannot show it
again. API tokens inherit their user's role, project memberships, and task
grants, but cannot create, list, or revoke API tokens. For CI, a task grant of
`operator` on the one task the pipeline deploys is the narrowest useful setup.

A pipeline that has just pushed an image then tells Boreas to run it:

```bash
curl -X POST -H "$AUTH" -H "$JSON" \
  -d '{"image":"ghcr.io/acme/web@sha256:'"$DIGEST"'"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web/deploy
```

The image must be immutable, of the form `repository@sha256:<64 hex digits>`, so
the exact artifact that was built is the one that runs; a mutable tag is
rejected because it can change between the pull and the container recreation.
Most build tools report the digest they pushed, for example the `digest` output
of `docker/build-push-action`.

Boreas pulls the image and recreates the container, restarting a running task
and leaving a stopped one stopped. Deploying the image a task already runs
changes nothing, so a pipeline may retry its callback safely.

List token metadata or revoke a token by ID with the login session:

```bash
curl -H "$AUTH" http://localhost:8080/api/v1/auth/tokens
curl -X DELETE -H "$AUTH" http://localhost:8080/api/v1/auth/tokens/<id>
```

For live logs, use an SSE-capable client:

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/projects/demo/tasks/web/logs/stream
```

## Resource metrics

Boreas streams live CPU, memory, and network usage straight from the Docker
socket, roughly one sample per task per second. Nothing is stored, so the stream
only reports what is happening while a client is listening.

```bash
# every running task in the project
curl -N -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/projects/demo/metrics/stream

# one task
curl -N -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/projects/demo/tasks/web/metrics/stream
```

Each event carries one sample:

```json
{"task":"web","cpu_percent":12.4,"memory_bytes":52428800,"memory_limit":536870912,
 "network_rx_bytes":1024,"network_tx_bytes":2048,"observed_at":"2026-01-02T03:04:05Z"}
```

`cpu_percent` is a rate, so the first event for each task reports `0`: Docker
reports cumulative counters and the first sample has nothing to compare against.
`memory_bytes` excludes page cache, matching what `docker stats` shows rather
than the larger raw cgroup figure. Tasks that are not running are omitted, and a
grantee sees only the tasks they were granted.

## Notifications

Deploys and task lifecycle changes — creation, assignment, development status —
each record a notification, and Boreas serves them newest-first for an in-app
feed:

```bash
curl -H "$AUTH" \
  'http://localhost:8080/api/v1/projects/demo/notifications?limit=20'
```

Every notification shares one shape across every channel (the feed, browser
push, and team destinations): title `<emoji> <Event> • <Project>`, body
`<task>: <detail>`. Deploy bodies carry the completion time as a local
12-hour clock with no timezone suffix; other channels render their own
arrival timestamp.

| Event              | `status`  | Example                                                                          |
| ------------------ | --------- | -------------------------------------------------------------------------------- |
| Deploy succeeded   | `success` | `🚀 Deploy Succeeded • Shop` / `web: Task completed at 11:00PM`                  |
| Deploy failed      | `failure` | `❌ Deploy Failed • Shop` / `web: Failed at 11:00PM: pull image: unavailable`    |
| Task created       | `info`    | `📋 Task Created • Shop` / `web: Customer checkout service`                      |
| Task assigned      | `info`    | `👤 Task Assigned • Shop` / `web: assigned to nam (member)`                      |
| Dev status changed | `info`    | `🔄 Status Changed • Shop` / `web: In Progress ➔ Ready`                          |

A retried callback for the image a task already runs records nothing, so it
does not repeat a notification a pipeline has already produced.

Each entry carries a per-user `seen` flag, so a badge can count what the
caller has not read without one member clearing it for everyone. Marking is
idempotent, and an id outside the caller's visibility is a no-op:

```bash
curl -X POST -H "$AUTH" \
  'http://localhost:8080/api/v1/projects/demo/notifications/<id>/seen'

curl -X DELETE -H "$AUTH" \
  'http://localhost:8080/api/v1/projects/demo/notifications/<id>/seen'
```

`DELETE` clears the caller's own mark, making the entry unread again.

### Team destinations (Slack, ntfy, …)

With `BOREAS_NOTIFY_URL` set, Boreas also posts each notification to the keyed
endpoint it derives from it — `<BOREAS_NOTIFY_URL>/boreas`, so
`http://boreas-noti:8000/notify/boreas` under Compose — and an
[Apprise](https://github.com/caronc/apprise-api) instance fans that out to
every service listed in its configuration. Compose runs one as `boreas-noti`,
reachable only from `boreas-net`, so no notification port is exposed. While
`apprise/boreas.yml` does not exist Apprise answers 204 and nothing is sent;
list your destinations there to enable it:

```yaml
urls:
  - slack://TokenA/TokenB/TokenC/#staging
  - ntfy://ntfy.example.com/boreas
```

That file holds credentials and is gitignored. Adding a platform means editing
it and restarting `boreas-noti`; Boreas itself never changes. Delivery is
fire-and-forget: an unreachable Apprise instance is logged and never delays or
fails a deploy, and the feed above still records the notification.

### Browser push (FCM)

Setting both `BOREAS_FCM_PROJECT` and `BOREAS_FCM_KEYFILE` lets users subscribe
their own browsers instead of a destination being hand-written into
`apprise/boreas.yml`. A device receives exactly the deploys its owner can list
from the notification feed, so push carries no more than the API already shows:

```bash
curl -X POST -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"token":"<FCM registration token>"}' \
  http://localhost:8080/api/v1/push/subscriptions

curl -X DELETE -H "$AUTH" \
  "http://localhost:8080/api/v1/push/subscriptions/<FCM registration token>"
```

The client obtains the token from the Firebase Web SDK. A browser is identified
by its token, so re-registering one moves the subscription to whoever is logged
in now — a shared machine never keeps notifying the previous user — and a user
can delete only tokens they currently own.

Two deployment details matter. `BOREAS_FCM_KEYFILE` is the service-account JSON
path **inside the `boreas-noti` container** (mount it under `./apprise`), because
Apprise, not Boreas, talks to Google. And `BOREAS_NOTIFY_URL` must then point at
`http://boreas-noti:8000/notify` without a configuration key: only Apprise's
stateless endpoint accepts per-request destinations, so `/notify/boreas` would
ignore the subscribed devices and deliver to `boreas.yml` alone. Boreas refuses
to start on that combination rather than drop every subscriber silently.
Legacy FCM was shut down in June 2024, so a service-account keyfile is the only
supported credential.

Browser push and team destinations run side by side, on separate endpoints
derived from the same variable: FCM devices ride the stateless
`BOREAS_NOTIFY_URL` request by request, while `<BOREAS_NOTIFY_URL>/boreas`
delivers the same notification to everything in `apprise/boreas.yml`. One
Apprise request cannot do both — the keyed endpoint ignores per-request
destinations — which is why the two requests stay separate. Without FCM
credentials the stateless endpoint is never called.

Delivery follows the same rules as `GET /api/v1/projects/{slug}/notifications`:
administrators and project members receive every task in a project, a task
grantee receives only the tasks they hold, and everyone else receives nothing.
Disabling a user stops their devices along with their login, and deleting the
account removes its subscriptions.

## OpenAPI specification

Every route above is described by an OpenAPI 3.0 document generated from the
same route table the server routes with, so the two cannot drift.

| Location                   | Purpose                                        |
| -------------------------- | ---------------------------------------------- |
| `api/openapi.yaml`         | Committed contract; generate clients from this |
| `GET /api/v1/openapi.json` | The running server's own specification         |
| `GET /api/v1/docs`         | Browsable reference (Scalar)                   |

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
The documentation endpoints are always available.

## Configuration

Boreas is configured entirely through environment variables. Everything that is
not deployment-specific is a built-in default, so a normal deployment only sets
the database connection and the initial administrator.

| Variable                | Default           | Purpose                                                                      |
| ----------------------- | ----------------- | ---------------------------------------------------------------------------- |
| `BOREAS_PORT`           | `8080`            | HTTP listen port                                                             |
| `BOREAS_LOG_LEVEL`      | `info`            | Minimum log level: `debug`, `info`, `warn`, or `error`; logs are JSON lines  |
| `BOREAS_DATABASE_URL`   | unset             | Full connection string; overrides the `BOREAS_DB_*` variables                |
| `BOREAS_DB_HOST`        | `localhost`       | Database host                                                                |
| `BOREAS_DB_PORT`        | `5432`            | Database port                                                                |
| `BOREAS_DB_USER`        | `postgres`        | Database user                                                                |
| `BOREAS_DB_PASSWORD`    | `postgres`        | Database password                                                            |
| `BOREAS_DB_NAME`        | `boreas`          | Database name                                                                |
| `BOREAS_DB_SSLMODE`     | `disable`         | `sslmode` for the connection                                                 |
| `BOREAS_ADMIN_USERNAME` | `admin`           | Seed administrator username                                                  |
| `BOREAS_ADMIN_EMAIL`    | `admin@localhost` | Seed administrator email                                                     |
| `BOREAS_ADMIN_PASSWORD` | unset             | Seed administrator password; required only on an empty database              |
| `BOREAS_NOTIFY_URL`     | unset             | Apprise stateless endpoint (`…/notify`); Boreas derives `…/notify/boreas` itself |
| `BOREAS_FCM_PROJECT`    | unset             | Firebase project ID; with the keyfile, enables browser push subscriptions    |
| `BOREAS_FCM_KEYFILE`    | unset             | Service-account JSON path *inside the Apprise container*                     |

Registry credentials are no longer configuration. Store them once through
`/api/v1/registry-credentials` and attach one to a project; Boreas uses it when
pulling that project's images.

```bash
curl -X POST http://localhost:8080/api/v1/registry-credentials \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"ghcr","registry":"ghcr","username":"bot","token":"..."}'

curl -X PATCH http://localhost:8080/api/v1/projects/demo \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"registry_credential_id":"<id>"}'
```

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
make hooks            # Install the repository Git hooks
make fmt              # Format Go source with the repository lint configuration
make lint             # Run the full Go linter set
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
