# Get started

Install Boreas, create a test environment and open its link. This guide uses a
public nginx image so you can try the workflow before connecting your own app.

## What you need

- Git, Docker Engine and the Docker Compose plugin. Shared folders require Engine 26+.
- Free host ports 8080 and 5432 (or change the database's published port).
- `curl` and `jq` for the API examples.

Use a machine reserved for trusted staging apps. Boreas controls Docker through
its socket. Containers share a network and web origin, not a security boundary
suitable for untrusted code.

## 1. Start Boreas

Replace `change-me` with your own password, at least 8 characters:

```bash
git clone https://github.com/zenkiet/boreas.git
cd boreas
BOREAS_ADMIN_PASSWORD=change-me docker compose up --build -d
```

Compose builds the server and starts:

| Container | Purpose |
| --------- | ------- |
| `boreas` | Server and public directory on port 8080 |
| `boreas-db` | PostgreSQL 18; database files in `./data` |
| `boreas-noti` | Apprise; forwards alerts after you configure destinations |

Check startup:

```bash
curl -fsS http://localhost:8080/api/v1/health
```

Look for `"status":"healthy"`. If it fails, run `docker compose logs boreas`.

The initial account is `admin`, email `admin@example.com`, with your password.
Set `BOREAS_ADMIN_USERNAME` and `BOREAS_ADMIN_EMAIL` to choose other values.
These settings seed an empty database only; they do not update existing users.
Always supply a password: Compose otherwise uses a publicly known fallback.

## 2. Open Boreas in a browser

- <http://localhost:8080>: the public task directory, initially empty.
- <http://localhost:8080/api/v1/docs>: interactive API reference.

The separate team app shown in the README is not bundled with the server.
You can complete this guide through the API without it.

## 3. Sign in

Use the password you chose in step 1:

```bash
TOKEN=$(curl -fsS -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}' | jq -er .token)
AUTH="Authorization: Bearer $TOKEN"
JSON='Content-Type: application/json'
```

Keep using this shell: the other guides reuse `AUTH` and `JSON`. Sign-in tokens
last 30 days and renew while used. Treat them like passwords.

## 4. Create a project

A project groups related environments. Only an administrator can create one;
the creator becomes its owner.

```bash
curl -fsS -X POST http://localhost:8080/api/v1/projects -H "$AUTH" -H "$JSON" \
  -d '{"slug":"demo","name":"Demo"}'
```

`slug` is the permanent URL name. Use lowercase letters, digits and hyphens,
starting with a letter or digit, up to 63 characters. Some names, such as `api`,
are reserved; the API reports invalid values. `name` is the editable display name.

## 5. Create a task

A task runs one Docker image in one container. For real work, name it after the
ticket or change it serves, such as `wo-718`.

```bash
curl -fsS -X POST http://localhost:8080/api/v1/projects/demo/tasks -H "$AUTH" -H "$JSON" \
  -d '{"name":"web","image":"nginx:alpine","port":80}'
```

- `name` is permanent and unique within the project, ignoring case. It accepts letters, digits, `.`, `_` and `-`, starting with a letter or digit, up to 63 characters.
- `image` is an image Docker can pull. Private images need [registry credentials](configuration.md#registry-credentials).
- `port` is the app's port **inside the container**, not a host port; the default is 80.

Boreas pulls the image, starts the container and waits up to 20 seconds for its
port to answer. Check the returned `status` and `error` fields: a successful
HTTP request alone does not mean the app started successfully.

## 6. Open the task's link

Open <http://localhost:8080/demo/web/>. You should see nginx's welcome page.

This is the task's **App URL**: `/{project}/{task}/`. The same link is used after
updates. For your own app, follow the [path setup guide](tasks.md#make-your-app-work-under-its-app-url).

The directory and unlocked App URLs are public to anyone who can reach Boreas.
Project roles protect management operations, not these pages. Use a
[task password](tasks.md#lock-a-task-with-a-password) or restrict network access
if the preview should not be public.

## Next steps

- [Add people and permissions](projects-and-people.md).
- [Connect your build pipeline](deploy-from-ci.md).
- [Manage tasks](tasks.md), [notifications](notifications.md) and the optional [Bora assistant](bora.md).
- Before sharing the server, set strong database credentials and configure [HTTPS](configuration.md#behind-a-reverse-proxy-with-https).

## Use the published image

Instead of building from source, replace the `boreas` service's `build:` block
in `docker-compose.yaml` with:

```yaml
image: ghcr.io/zenkiet/boreas:latest
```

Then run `docker compose pull boreas` and `docker compose up -d`. For controlled
upgrades, replace `latest` with a version tag from the
[releases page](https://github.com/zenkiet/boreas/releases). `latest` and minor
line tags move; version tags identify a release.

## Upgrade

Back up the database and task data first. For a source-based installation:

```bash
git pull
docker compose up --build -d
```

For a published image, update its tag, pull it and run `docker compose up -d`.
Migrations run automatically at startup. Task containers keep running while
Boreas restarts, but their links are unavailable until it returns.

## Run the binary without Compose

[Release archives](https://github.com/zenkiet/boreas/releases) include binaries
for Linux, macOS and Windows on amd64 and arm64. You need PostgreSQL 16+, Docker
access, and network access to task container addresses on `boreas-net`.

A Linux host can reach those addresses. Docker Desktop keeps them inside its
VM, so on macOS or Windows run Boreas in a container, as Compose does.

```bash
BOREAS_DATABASE_URL='postgres://postgres:postgres@localhost:5432/boreas?sslmode=disable' \
BOREAS_ADMIN_USERNAME=admin BOREAS_ADMIN_EMAIL=admin@example.com BOREAS_ADMIN_PASSWORD=change-me \
  ./boreas
```

To start only Compose's database, use `docker compose up -d boreas-db`.
See [Configuration](configuration.md) for all settings and storage details.
