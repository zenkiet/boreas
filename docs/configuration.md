# Configuration

This reference is for the server administrator. Boreas reads settings from
environment variables at startup; recreate or restart the server after changes.
For a first installation, follow [Get started](getting-started.md).

The core service needs Docker, PostgreSQL and an initial administrator.
Notifications, AI, code search and file storage are optional.

## Setting variables

With Compose, put values in `.env` beside `docker-compose.yaml`, or export them
in your shell, then run `docker compose up -d`. Protect `.env`: it contains secrets.
Only variables listed under the service's `environment:` reach the container.
Add unlisted settings there, including `BOREAS_AI_PLAN_MODEL`,
`BOREAS_DATABASE_URL` or `BOREAS_LOG_LEVEL`, if needed.

Defaults below are the server's defaults; Compose overrides some of them.
Changing `BOREAS_PORT` also requires updating Compose's port mapping.

### Server

| Variable           | Default | What it does                                                          |
| ------------------ | ------- | --------------------------------------------------------------------- |
| `BOREAS_PORT`      | `8080`  | The port Boreas listens on, from 1 to 65535                            |
| `BOREAS_LOG_LEVEL` | `info`  | How much Boreas logs: `debug`, `info`, `warn` or `error`. Logs are JSON lines on standard output |

### Database

| Variable              | Default     | What it does                                                    |
| --------------------- | ----------- | --------------------------------------------------------------- |
| `BOREAS_DATABASE_URL` | none        | A full PostgreSQL connection string; when set, the `BOREAS_DB_*` settings are ignored |
| `BOREAS_DB_HOST`      | `localhost` | The database server; Compose sets `boreas-db`                   |
| `BOREAS_DB_PORT`      | `5432`      | Its port                                                        |
| `BOREAS_DB_USER`      | `postgres`  | The database user                                               |
| `BOREAS_DB_PASSWORD`  | `postgres`  | Its password                                                    |
| `BOREAS_DB_NAME`      | `boreas`    | The database name                                               |
| `BOREAS_DB_SSLMODE`   | `disable`   | PostgreSQL's `sslmode`, such as `require`                       |

Boreas needs PostgreSQL 16 or newer and sets up its tables when it starts. The
database user must be allowed to create the `unaccent` extension, which the
database's owner can do.

### First administrator

| Variable                | Default | What it does                                    |
| ----------------------- | ------- | ----------------------------------------------- |
| `BOREAS_ADMIN_USERNAME` | none    | The first administrator's username              |
| `BOREAS_ADMIN_EMAIL`    | none    | Their email                                     |
| `BOREAS_ADMIN_PASSWORD` | none    | Their password, at least 8 characters           |

Required only when the database has no users; ignored afterward. Compose
supplies `admin`, `admin@example.com` and a publicly known fallback password.
Always set your own password before the first start.

### Notifications

| Variable             | Default | What it does                                                                 |
| -------------------- | ------- | ---------------------------------------------------------------------------- |
| `BOREAS_NOTIFY_URL`  | none    | The Apprise address, ending in `/notify`; Boreas posts to `<address>/boreas`. Compose sets `http://boreas-noti:8000/notify` |
| `BOREAS_FCM_PROJECT` | none    | The Firebase project ID for push notifications                               |
| `BOREAS_FCM_KEYFILE` | none    | The path to the Firebase service account key file, read once at startup. Compose sets it when `BOREAS_FCM_PROJECT` is set |

See [Notifications](notifications.md) for both channels.

### Bora

| Variable                   | Default           | What it does                                                         |
| -------------------------- | ----------------- | -------------------------------------------------------------------- |
| `BOREAS_AI_BASE_URL`       | none              | The OpenAI-compatible endpoint Bora answers with; it must support streaming and tool calling |
| `BOREAS_AI_MODEL`          | none              | The model at that endpoint                                           |
| `BOREAS_AI_API_KEY`        | none              | The key for that endpoint; local servers such as Ollama need none    |
| `BOREAS_AI_PLAN_MODEL`     | `BOREAS_AI_MODEL` | The model that plans tickets, with five minutes per request          |
| `BOREAS_SOURCEBOT_URL`     | none              | The Sourcebot server that indexes the code Bora reads                |
| `BOREAS_SOURCEBOT_API_KEY` | none              | Its API key                                                          |

See [Bora](bora.md) for what each part turns on.

### File storage (S3)

| Variable                      | Default | What it does                                                        |
| ----------------------------- | ------- | ------------------------------------------------------------------- |
| `BOREAS_S3_ENDPOINT`          | none    | An S3-compatible endpoint, such as `https://<account>.r2.cloudflarestorage.com`; turns on avatars and images in Bora questions |
| `BOREAS_S3_BUCKET`            | none    | The bucket: images go under `bora/`, avatars under `avatars/`       |
| `BOREAS_S3_REGION`            | `auto`  | The signing region; Cloudflare R2 takes `auto`                       |
| `BOREAS_S3_ACCESS_KEY_ID`     | none    | An access key that can read and write the bucket                    |
| `BOREAS_S3_SECRET_ACCESS_KEY` | none    | Its secret                                                          |

Give only the `bora/` prefix a lifecycle rule that expires old images.
Avatars under `avatars/` must stay.

### Settings that go together

Required combinations (incomplete groups prevent startup):

- `BOREAS_AI_BASE_URL` and `BOREAS_AI_MODEL`; without them, Bora is off.
- `BOREAS_SOURCEBOT_URL` and `BOREAS_SOURCEBOT_API_KEY`; without them, Bora
  cannot read code.
- `BOREAS_FCM_PROJECT` and `BOREAS_FCM_KEYFILE`; without them, there is no push.
- `BOREAS_S3_ENDPOINT` needs `BOREAS_S3_BUCKET` and both keys; without file
  storage, avatars and images are off.

## Compose settings

The Compose file reads a few variables of its own, from the same `.env` file:

| Variable           | Default                      | What it does                                                |
| ------------------ | ---------------------------- | ----------------------------------------------------------- |
| `TZ`               | `Etc/UTC`                    | The time zone of the Boreas container; deploy notifications show times in it |
| `POSTGRES_USER`    | `postgres`                   | The database user, for both PostgreSQL and Boreas           |
| `POSTGRES_PASSWORD`| `postgres`                   | Its password                                                |
| `POSTGRES_DB`      | `boreas`                     | The database name                                           |
| `POSTGRES_PORT`    | `5432`                       | The port on the host where PostgreSQL is published          |
| `DATA_PATH`        | `./data`                     | The host folder for the database files                      |
| `APPRISE_PATH`     | `./apprise`                  | The host folder for Apprise's configuration                 |
| `FCM_KEYFILE_PATH` | `./apprise/fcm-keyfile.json` | The host path of the Firebase key file                      |
| `RESTART_POLICY`   | `unless-stopped`             | The restart policy of the three Compose services            |

- Set the PostgreSQL values before the first start: the database keeps the
  password it was created with.
- Compose publishes PostgreSQL on the host. On a shared machine, choose a strong
  `POSTGRES_PASSWORD`, or remove the `ports:` entry of `boreas-db`; Boreas
  reaches the database over `boreas-net` either way.
- Task containers always use the `unless-stopped` restart policy, whatever
  `RESTART_POLICY` says.

## Registry credentials

Public images need no credentials. For private images, an administrator stores
a credential and attaches it to a project. It is used for that project's pulls.
Supported registries: GitHub Container Registry (`ghcr`) and Docker Hub (`dockerhub`).
Examples use [AUTH and JSON](getting-started.md#3-sign-in).

```bash
curl -X POST http://localhost:8080/api/v1/registry-credentials -H "$AUTH" -H "$JSON" \
  -d '{"name":"ghcr","registry":"ghcr","username":"bot","token":"<access token>"}'

curl -X PATCH http://localhost:8080/api/v1/projects/demo -H "$AUTH" -H "$JSON" \
  -d '{"registry_credential_id":"<id>"}'
```

- Only administrators can list, create or delete credentials, and the token is
  never shown again.
- `"registry_credential_id": null` detaches the credential from the project.

## Behind a reverse proxy with HTTPS

Boreas serves plain HTTP. Put nginx, Caddy or Traefik in front of it for HTTPS;
restrict direct access to Boreas so requests cannot bypass that proxy.

- Give Boreas a host name of its own, such as `boreas.example.com`. Task links
  live at the root of that host, at `/{project}/{task}/`.
- Pass event streams through without buffering. Boreas sends
  `X-Accel-Buffering: no`, which nginx honors.
- Forward `X-Forwarded-For` and `X-Forwarded-Proto`. Boreas passes them on to
  each task, so apps see the visitor's address and that they used HTTPS.
- Leave compression to Boreas: its pages ship precompressed with Brotli and
  gzip, and API replies are gzipped.

## Before exposing the server

- The directory and unlocked App URLs are public. Roles restrict the management API, not preview access.
- Use trusted staging images only. Tasks share `boreas-net` and a web origin; Docker socket access gives Boreas powerful control over the host.
- Back up PostgreSQL, the `boreas-appdata` volume and any external image storage. Container files outside mounted folders are disposable.
- Boreas validates configuration at startup and reports invalid settings in its logs.
