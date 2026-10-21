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

`BOREAS_ADMIN_USERNAME`, `BOREAS_ADMIN_EMAIL` and `BOREAS_ADMIN_PASSWORD` seed
the first administrator; Compose defaults the first two. They are required only
while the user table is empty and are ignored afterwards.

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
BOREAS_ADMIN_USERNAME=admin BOREAS_ADMIN_EMAIL=admin@localhost BOREAS_ADMIN_PASSWORD=change-me make dev
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
| `operator` | Start, stop, restart, deploy, and report builds   |
| `member`   | Create, update, and delete tasks                  |
| `owner`    | Project settings, membership, and task grants     |

Add an existing user to a project, or change a member's role; the last owner
can be neither removed nor demoted:

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

Every project and task response carries `my_role`, the caller's effective role,
so a client can hide what it cannot do. `GET /projects` lists each project with
the tasks the caller sees, each with its own `my_role` and `last_deploy`.

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
| `GET`    | `/events/stream`                               | login session | Signal changes over SSE, so clients refetch     |
| `GET`    | `/users`                                       | admin         | List users                                      |
| `POST`   | `/users`                                       | admin         | Create a user                                   |
| `PATCH`  | `/users/{id}`                                  | admin         | Update a user                                   |
| `DELETE` | `/users/{id}`                                  | admin         | Delete a user                                   |
| `GET`    | `/registry-credentials`                        | admin         | List registry credentials                       |
| `POST`   | `/registry-credentials`                        | admin         | Create a registry credential                    |
| `DELETE` | `/registry-credentials/{id}`                   | admin         | Delete a registry credential                    |
| `GET`    | `/projects`                                    | user          | List reachable projects with their tasks        |
| `POST`   | `/projects`                                    | admin         | Create a project; the creator becomes owner     |
| `GET`    | `/projects/{project}`                          | viewer        | Get a project                                   |
| `PATCH`  | `/projects/{project}`                          | owner         | Update a project                                |
| `DELETE` | `/projects/{project}`                          | owner         | Delete a project                                |
| `GET`    | `/projects/{project}/members`                  | owner         | List members                                    |
| `POST`   | `/projects/{project}/members`                  | owner         | Add or promote a member                         |
| `DELETE` | `/projects/{project}/members/{userID}`         | owner         | Remove a member                                 |
| `GET`    | `/notifications`                               | user          | List notifications across projects              |
| `POST`   | `/notifications/seen`                          | user          | Mark notifications seen                         |
| `GET`    | `/projects/{project}/notifications`            | viewer        | List a project's notifications                  |
| `GET`    | `/projects/{project}/metrics/stream`           | viewer        | Stream metrics for every running task over SSE  |
| `GET`    | `/projects/{project}/folders`                  | viewer        | List the folders task volumes can mount         |
| `GET`    | `/code/repositories`                           | admin         | Search the repositories Sourcebot indexes       |
| `POST`   | `/bora/conversations`                          | login session | Ask Bora in a new conversation, streamed        |
| `POST`   | `/bora/conversations/{id}/messages`            | login session | Ask a follow-up question, streamed              |
| `GET`    | `/bora/conversations/{id}/stream`              | login session | Follow a running answer from its start          |
| `POST`   | `/bora/conversations/{id}/stop`                | login session | Stop a running answer; nothing is saved         |
| `GET`    | `/bora/conversations`                          | login session | List your conversations                         |
| `POST`   | `/bora/conversations/search`                   | login session | Search your conversations                       |
| `GET`    | `/bora/conversations/{id}`                     | login session | Read one of your conversations                  |
| `POST`   | `/bora/conversations/{id}/messages/{index}/ran` | login session | Record what ran from an answer                 |
| `DELETE` | `/bora/conversations/{id}`                     | login session | Delete one of your conversations                |
| `DELETE` | `/bora/conversations`                          | login session | Delete all your conversations                   |
| `GET`    | `/projects/{project}/tasks`                    | viewer        | List tasks                                      |
| `POST`   | `/projects/{project}/tasks`                    | member        | Create a task                                   |
| `GET`    | `/projects/{project}/tasks/{name}`             | viewer        | Get a task                                      |
| `PATCH`  | `/projects/{project}/tasks/{name}`             | member        | Update image, port, labels, env, volumes, description, note, or dev status |
| `POST`   | `/projects/{project}/tasks/{name}/deploy`      | operator      | Deploy an image built elsewhere                 |
| `PUT`    | `/projects/{project}/tasks/{name}/state`       | operator      | Start, stop, or restart                         |
| `PUT`    | `/projects/{project}/tasks/{name}/build`       | operator      | Report CI build progress                        |
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
renamed by recreating it, because its name is part of its public URL. After a
reboot, Docker starts every task that was running again; a task stopped from
Boreas stays stopped.

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

## Volumes

`volumes` mounts folders prepared by an administrator into a task, writable:
certificates, configuration, seed scripts, data the task keeps. It maps a path
in the container to one of the project's folders, and saving fails with `400`
when the folder does not exist.

```bash
curl -H "$AUTH" http://localhost:8080/api/v1/projects/demo/folders
# {"folders":["certs","init"],"total":2}

curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"volumes":{"/etc/certs":"certs","/docker-entrypoint-initdb.d":"init"}}' \
  http://localhost:8080/api/v1/projects/demo/tasks/db
```

The folders live in the `boreas-appdata` volume, one directory per project
(`demo/certs`), which Boreas creates with the project. A task reaches only its own project's folders; to share one
between projects, symlink inside the volume (`demo/certs -> ../_shared/certs`).
Boreas lists them through its own mount at `/opt/appdata`, so on a server back
the volume with a host directory reserved for Boreas (Docker Engine 26 or later):

```yaml
volumes:
  appdata:
    name: boreas-appdata
    driver: local
    driver_opts: { type: none, o: bind, device: /opt/appdata/boreas }
```

Only what a task writes into its volumes survives its container: deploys,
restarts and configuration changes start everything else from the image again.
Every task of the project sees the same folder. Before creating a container,
Boreas opens each mounted folder to every user (mode 0777), so an app that does
not run as root can write too; this needs Boreas's own `/opt/appdata` mount
writable. Files and subfolders inside keep their own permissions.

## Bora

Bora is the app's assistant. Members ask it about the running system (why a
deploy failed, what a log line means, which tasks need attention) and about how
a project's code behaves. It answers from their projects, tasks, builds,
activity, settings, live metrics, task logs (the newest 200 lines, or those
from a time on) and, where a project lists repositories, its code, with the
OpenAI-compatible model in `BOREAS_AI_*`, which must support tool calling and
streaming. Owners also let it read who is a member and who holds a grant. Bora
takes login sessions only, and only for projects the caller is a member of: a
project reached through task grants alone gets 403, and anything the caller
cannot see gets 404.

Every question is asked in a conversation, which Boreas keeps. The first
question starts one about the whole `fleet`, a `project`, a `task`, its latest
`build`, an Activity `alert` (`project` and `id`), or a `log` line (`project`,
`task`, `at`, `line`). Follow-ups go into it, and Boreas gives the model its
last 12 messages:

```bash
curl -N -X POST -H "$AUTH" -H "$JSON" http://localhost:8080/api/v1/bora/conversations -d '{
  "about": {"kind": "task", "project": "demo", "task": "qa-1"},
  "question": "Why did this fail?"
}'
curl -N -X POST -H "$AUTH" -H "$JSON" -d '{"question":"How do I fix it?"}' \
  http://localhost:8080/api/v1/bora/conversations/<id>/messages
```

The answer streams as Server-Sent Events:

- `conversation` names a new conversation;
- a `step` comes before each tool call ("Reading qa-1's logs…");
- `say` carries the line the phone reads aloud, 1 or 2 sentences written as
  they are spoken (numbers, times and names in words), before the answer's
  text, or just before `done` when the model wrote none;
- each `delta` carries text as it is written, and `reset` drops what came so
  far, its `say` included, when it turned out to lead into a tool call;
- `done` carries the saved answer, `error` the failure, and `stopped` ends an
  answer that was stopped.

A dropped connection does not stop the answer, which is saved anyway, and
`GET …/{id}/stream` replays a running answer from its start. Only
`POST …/{id}/stop` cancels one, and then nothing is saved. A new conversation
is saved with its first answer; until then only its stream and `GET …/{id}`
reach it.

**Privacy and search:**

- A conversation is its user's alone: anyone else, an administrator included,
  gets 404, and deleting a user deletes their conversations.
- It records every project its answers read and stays hidden while one of
  them that still exists is out of the user's reach; it comes back with
  access.
- Nothing is pruned: the user deletes one conversation or all of them.
- `POST /bora/conversations/search` finds the conversations with a message
  holding every word of `query` as a prefix, whatever its accents: "loi bui"
  finds "lỗi build". The query travels in the body, so request logs never hold
  it. The migration creates Postgres' `unaccent` extension, which needs the
  database owner.

Bora changes nothing. An answer may carry `actions`, each checked against the
caller's role as the route that runs it will check again, and run only when
the user taps it, through the existing routes, with the user's own session.
For each action it runs, the app posts its own result line to
`…/messages/{index}/ran` (at most 10 for an answer), and Bora takes those lines
as fact.

| Role     | Actions                                                                                                         |
| -------- | --------------------------------------------------------------------------------------------------------------- |
| operator | `task.start`, `task.stop`, `task.restart`                                                                       |
| member   | `task.create`, `task.delete`, `task.update` (description, note, dev status, image, port, volumes), `task.env`   |
| owner    | `member.add`, `member.role`, `member.remove`, `grant.add`, `grant.remove`, `project.update`, `project.delete`   |

Bora turns the names it hears into the ids the routes take. Since only
administrators may list users and registry credentials:

- an owner can name only people already in the project (members, or holders of
  a grant there);
- only an administrator can change a project's registry credential;
- anyone else named gets the same refusal as a name nobody has.

### Code

Bora reads a project's code through a self-hosted
[Sourcebot](https://www.sourcebot.dev) (its free plan is enough), set by
`BOREAS_SOURCEBOT_URL` and `BOREAS_SOURCEBOT_API_KEY`. An admin picks the
repositories each project reads:

```bash
curl -H "$AUTH" 'http://localhost:8080/api/v1/code/repositories?query=acme'
curl -X PATCH -H "$AUTH" -H "$JSON" \
  -d '{"repositories":["github.com/acme/web","github.com/acme/api"]}' \
  http://localhost:8080/api/v1/projects/demo
```

- It reads each repository's default branch, and lists the files it read under
  its answer.
- A turn that can read code gets a 96 KiB tool budget instead of 48 KiB.
- Code has many authors, so once Bora has read code in a turn, it proposes
  nothing but start, stop and restart.
- Files that usually hold secrets (`*.config`, `appsettings*.json`, env files,
  keys, certificates, cloud and Terraform credentials) are never read.

### Safety and limits

**Screening.** Every question is screened before it is answered, unless it
answers Bora's own question back. When the question points at several tasks or
at none, or asks for a change without saying to what, Bora asks back with 2 to
4 `choices` that the app shows as replies to tap. It may also ask back after
looking, when what it found fits several tasks or projects.

**What reaches the model.**

- Env values never reach it.
- What other people wrote (logs, errors, notes, descriptions, Activity, code)
  reaches it only as tool results or as data under the question.
- Before that, passwords, keys and tokens in it become `[redacted]`: quoted or
  not, in flags, in `Authorization` headers, connection strings and URLs,
  private keys, and well-known token formats. The question and the
  conversation so far go through the same redaction, and so does everything
  stored from an answer: its text, spoken line, title, choices and reasons.
- A secret in any other shape still goes to the model provider.

**Load.**

- Each user may have three answers running at once and the server twenty,
  answers whose stream dropped included. Past that, a question gets 429 at once
  instead of queueing.
- An answer has no time limit beyond each model request's two minutes.
- A restart drops the answers in flight.
- Set a spending limit at the model provider too: these caps bound load, not
  cost.

With `claude-sonnet-5-5`, EVAL_RESULT. Score a model before relying on it:

```bash
BOREAS_TEST_AI_BASE_URL=https://… BOREAS_TEST_AI_MODEL=claude-sonnet-5-5 BOREAS_TEST_AI_API_KEY=… \
  go test ./internal/infra/llm -run TestClarifyEval -v
```

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

### Report build progress

The same token can report the pipeline's progress, so the build shows in Boreas
before anything is deployed. Any CI can do it with curl: call it as each stage
starts, and once more when the run ends.

```bash
curl -X PUT -H "$AUTH" -H "$JSON" \
  -d '{"state":"running","stage":"Test","progress":40,"url":"https://jenkins.example.com/job/web/42/"}' \
  http://localhost:8080/api/v1/projects/demo/tasks/web/build
```

- `state` is `running`, `success`, `failure` or `canceled`; `stage`, `progress`
  (0-100, worked out by the pipeline) and `url` are optional.
- Only the latest report is kept, shown as `build` on the task and in
  `GET /projects`. `running` starts afresh; any other state keeps the stage,
  progress and url it does not send, so a final hook needs only the outcome.
- A build that turns to `failure` records a `build_failed` notification; other
  states notify nothing, since a deploy already does.
- Run one build per task at a time: a late report from an older run overwrites
  the newer one until its next report.

This script maps each CI's result words and never fails the build:

```sh
#!/bin/sh
# boreas-report.sh STATE [STAGE] [PROGRESS]
# env: BOREAS_URL BOREAS_TOKEN BOREAS_PROJECT BOREAS_TASK [BOREAS_RUN_URL]
case "$1" in
  running) state=running ;;
  success|SUCCESS) state=success ;;
  failure|failed|FAILURE|UNSTABLE) state=failure ;;
  canceled|cancelled|ABORTED|NOT_BUILT) state=canceled ;;
  *) echo "boreas-report: unknown state '$1'" >&2; exit 0 ;;
esac
curl -fsS --max-time 10 -X PUT -H "Authorization: Bearer $BOREAS_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"state\":\"$state\",\"stage\":\"${2:-}\",\"progress\":${3:-0},\"url\":\"${BOREAS_RUN_URL:-}\"}" \
  "$BOREAS_URL/api/v1/projects/$BOREAS_PROJECT/tasks/$BOREAS_TASK/build" >/dev/null \
  || echo "boreas-report: $state not recorded" >&2
```

Jenkins (declarative):

```groovy
environment {
    BOREAS_TOKEN = credentials('boreas-token')
    BOREAS_RUN_URL = "${env.BUILD_URL}"   // with BOREAS_URL, BOREAS_PROJECT and BOREAS_TASK
}
stages {
    stage('Test')   { steps { sh 'ci/boreas-report.sh running Test 0';   sh 'make test' } }
    stage('Image')  { steps { sh 'ci/boreas-report.sh running Image 40'; sh 'make image' } }
    stage('Deploy') { steps { sh 'ci/boreas-report.sh running Deploy 90'; sh 'ci/deploy.sh' } }
}
post { always { sh "ci/boreas-report.sh ${currentBuild.currentResult}" } }
```

GitHub Actions ends the job with a step that always runs. GitLab CI runs each
stage as its own job, so `after_script` reports only a job that did not succeed
and the last job ends its `script` with `ci/boreas-report.sh success`:

```yaml
- if: always()
  run: ci/boreas-report.sh ${{ job.status }}
```

```yaml
after_script: ['[ "$CI_JOB_STATUS" = success ] || ci/boreas-report.sh "$CI_JOB_STATUS"']
```

List token metadata or revoke a token by ID with the login session:

```bash
curl -H "$AUTH" http://localhost:8080/api/v1/auth/tokens
curl -X DELETE -H "$AUTH" http://localhost:8080/api/v1/auth/tokens/<id>
```

For live logs, use an SSE-capable client. A browser reads the stream with
`fetch`, since `EventSource` cannot send the `Authorization` header. To resume
after a disconnect, pass the last timestamp received as `since`:

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/v1/projects/demo/tasks/web/logs/stream?since=2026-01-02T03:04:05.123456789Z'
```

To keep a screen current without polling, hold `GET /api/v1/events/stream`
open with the login session. It sends `{}` on connect, after every successful
authenticated write through the API outside Bora's conversations, during a
deploy as the task's status moves, and when a notification is recorded. It never says what changed, so
refetch what the screen shows, such as `GET /api/v1/projects`. A burst of changes arrives as one event. A proxy
in front of Boreas must pass streams through unbuffered; Boreas sends
`X-Accel-Buffering: no` for nginx.

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
feed, across every project the caller reaches or for one project. To page, pass
the id of the last notification received as `before`:

```bash
curl -H "$AUTH" 'http://localhost:8080/api/v1/notifications?limit=20'
curl -H "$AUTH" 'http://localhost:8080/api/v1/notifications?limit=20&before=<id>'
curl -H "$AUTH" 'http://localhost:8080/api/v1/projects/demo/notifications'
```

Every notification shares one shape across every channel (the feed, browser
push, and team destinations): title `<emoji> <Event> • <Project>`, body
`<task>: <detail>`. Deploy bodies carry the completion time as a local
12-hour clock with no timezone suffix; other channels render their own
arrival timestamp.

| `type`           | `status`  | Example                                                                          |
| ---------------- | --------- | -------------------------------------------------------------------------------- |
| `deployed`       | `success` | `🚀 Deploy Succeeded • Shop` / `web: Task completed at 11:00PM`                  |
| `deploy_failed`  | `failure` | `❌ Deploy Failed • Shop` / `web: Failed at 11:00PM: pull image: unavailable`    |
| `task_created`   | `info`    | `📋 Task Created • Shop` / `web: Customer checkout service`                      |
| `task_assigned`  | `info`    | `👤 Task Assigned • Shop` / `web: assigned to nam (member)`                      |
| `status_changed` | `info`    | `🔄 Status Changed • Shop` / `web: In Progress ➔ Ready`                          |
| `build_failed`   | `failure` | `🔨 Build Failed • Shop` / `web: failed at Test`                                 |

Route on `type` and `project` (the slug), not on the title, which is for people.

A retried callback for the image a task already runs records nothing, so it
does not repeat a notification a pipeline has already produced.

Each entry carries a per-user `seen` flag, so a badge can count what the
caller has not read without one member clearing it for everyone. Marking is
idempotent, and an id outside the caller's visibility is a no-op:

```bash
curl -X POST -H "$AUTH" -H "$JSON" -d '{"ids":["<id>","<id>"]}' \
  'http://localhost:8080/api/v1/notifications/seen'

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

The client obtains the token from the Firebase SDK (web, iOS or Android). A
device is identified by its token, so re-registering one moves the subscription
to whoever is logged in now — a shared machine never keeps notifying the
previous user — and a user can delete only tokens they currently own.

Boreas sends each push straight to FCM HTTP v1 with the service account in
`BOREAS_FCM_KEYFILE`, a path inside the `boreas` container; Compose mounts
`${FCM_KEYFILE_PATH:-./apprise/fcm-keyfile.json}` there read-only. The keyfile
is read once at startup, so a missing or invalid one stops Boreas. A push lives
24 hours, a failure arrives at high priority and anything else at normal, and
the body is cut to 1000 characters; the feed keeps it whole. A token FCM no
longer knows (`UNREGISTERED`, `SENDER_ID_MISMATCH`, or an invalid
`message.token`) is deleted on the spot. Quota, outage and payload errors are
logged and the token is kept.

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
| `BOREAS_ADMIN_USERNAME` | unset             | Seed administrator username; required only on an empty database              |
| `BOREAS_ADMIN_EMAIL`    | unset             | Seed administrator email; required only on an empty database                 |
| `BOREAS_ADMIN_PASSWORD` | unset             | Seed administrator password; required only on an empty database              |
| `BOREAS_NOTIFY_URL`     | unset             | Apprise endpoint (`…/notify`); team notifications go to `…/notify/boreas`     |
| `BOREAS_FCM_PROJECT`    | unset             | Firebase project ID; with the keyfile, enables browser push subscriptions    |
| `BOREAS_FCM_KEYFILE`    | unset             | Service-account JSON path; read once at startup                              |
| `BOREAS_AI_BASE_URL`    | unset             | OpenAI-compatible endpoint Bora answers with, streaming and calling tools   |
| `BOREAS_AI_MODEL`       | unset             | Model name at that endpoint                                                  |
| `BOREAS_AI_API_KEY`     | unset             | Bearer key for that endpoint; local servers such as Ollama need none         |
| `BOREAS_SOURCEBOT_URL`  | unset             | Sourcebot that indexes the code Bora reads                                   |
| `BOREAS_SOURCEBOT_API_KEY` | unset          | Sourcebot API key                                                            |

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
