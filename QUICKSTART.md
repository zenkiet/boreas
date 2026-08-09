# Boreas quick start

## Docker Compose

Docker Compose is the shortest path to a complete deployment. Boreas and
PostgreSQL start together, and the schema is applied automatically on first
boot.

```bash
BOREAS_ADMIN_PASSWORD=change-me docker compose up --build -d
curl http://localhost:8080/api/v1/health
```

`BOREAS_ADMIN_PASSWORD` creates the first administrator. It is required only
while the user table is empty and is ignored on later starts.

The deployment uses:

- `/var/run/docker.sock` so Boreas can manage task containers
- one `boreas-db-data` volume for the PostgreSQL database
- the Docker network whose exact name is `boreas-net`

Boreas waits for the database healthcheck before starting. It runs as root
inside its minimal container so it can access the mounted Docker socket, and
the runtime image includes CA certificates for TLS connections.

## Local build

Requirements: Go 1.26, a running Docker Engine, and PostgreSQL.

```bash
make db                                     # PostgreSQL on localhost:5432
BOREAS_ADMIN_PASSWORD=change-me make dev
```

The binary uses `pgx` and needs no cgo, so it is built with `CGO_ENABLED=0`.

## Log in

Every API route except `/health` and `/auth/login` needs a bearer token.

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}' | jq -r .token)
```

## Create a project and a task

Tasks live inside projects, and the project slug is the first path segment.

```bash
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"demo","name":"Demo"}'

curl -X POST http://localhost:8080/api/v1/projects/demo/tasks \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "name": "web",
    "image": "nginx:alpine",
    "port": 80
  }'
```

Open <http://localhost:8080/demo/web/>. Proxied traffic needs no token.

Slugs and task names must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`, and task
URLs always use the `/{project}/{task}/` form with a trailing slash. The slugs
`api`, `health`, and `static` are reserved. Task names only need to be unique
within their project.

## Common API operations

```bash
AUTH="Authorization: Bearer $TOKEN"
JSON='Content-Type: application/json'
BASE=http://localhost:8080/api/v1/projects/demo/tasks

# List tasks
curl -H "$AUTH" $BASE

# Get one task
curl -H "$AUTH" $BASE/web

# Start or stop a task
curl -X PUT -H "$AUTH" -H "$JSON" -d '{"action":"start"}' $BASE/web/state
curl -X PUT -H "$AUTH" -H "$JSON" -d '{"action":"stop"}'  $BASE/web/state

# Read logs once
curl -H "$AUTH" $BASE/web/logs

# Stream live logs over Server-Sent Events (SSE)
curl -N -H "$AUTH" $BASE/web/logs/stream

# Read or update environment variables
curl -H "$AUTH" $BASE/web/env
curl -X PUT -H "$AUTH" -H "$JSON" \
  -d '{"env":{"APP_MODE":"staging"}}' $BASE/web/env

# Delete the task and its container
curl -X DELETE -H "$AUTH" $BASE/web
```

## API documentation and clients

Open <http://localhost:8080/api/v1/docs> for the browsable reference, or fetch
the specification directly:

```bash
curl http://localhost:8080/api/v1/openapi.json
```

The same document is committed at `api/openapi.yaml` and is what you generate
clients from:

```bash
npx @openapitools/openapi-generator-cli generate \
  -i api/openapi.yaml -g typescript-angular -o ./client
```

Run `make openapi` after changing any route or payload; `make openapi-check`
fails when the committed file no longer matches the code.

## Private images

Register a credential once, then attach it to the projects that need it.
Boreas uses the project's credential when pulling its images.

```bash
curl -X POST http://localhost:8080/api/v1/registry-credentials \
  -H "$AUTH" -H "$JSON" \
  -d '{"name":"ghcr","registry":"ghcr","username":"bot","token":"..."}'

curl -X PATCH http://localhost:8080/api/v1/projects/demo \
  -H "$AUTH" -H "$JSON" -d '{"registry_credential_id":"<id>"}'
```

## Team access

Administrators reach every project. Other users only see projects they belong
to; `member` covers task operations and `owner` adds project settings and
membership.

```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "$AUTH" -H "$JSON" \
  -d '{"username":"bob","email":"bob@example.com","password":"..."}'

curl -X POST http://localhost:8080/api/v1/projects/demo/members \
  -H "$AUTH" -H "$JSON" -d '{"user_id":"<id>","role":"member"}'
```

## Tests

```bash
make test
make test-integration   # needs PostgreSQL; `make db` starts one
make test-race
```

The race detector requires CGO, but the production binary does not.
