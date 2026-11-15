# Work with tasks

Boreas gives each task its own staging environment: one Docker container running
one image, at `/{project}/{task}/` on your server (its **App URL**).
The link is public unless you set a task password; managing the task through
the API needs a token and the right [role](projects-and-people.md).
Examples use `AUTH` and `JSON` from [Get started](getting-started.md#3-sign-in).

## Two statuses

The **container status** (`status`) describes the environment:

| Status | Meaning |
| --- | --- |
| `creating` | Pulling the image and creating the container |
| `starting` | Waiting for the container's port to answer |
| `running` | Container is running and registered at its App URL |
| `stopped` | Container is not running |
| `error` | An operation failed; `error` explains why |
| `unknown` | Boreas could not find or inspect the container at startup |

The **dev status** (`dev_status`) describes the work: `in_progress` (the
initial value), `blocked` (not ready for QA), or `ready` (ready to test).
A task member or owner can change it; changes send a
[notification](notifications.md). Deploys leave it unchanged.

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"dev_status":"ready"}'
```

## Start, stop and restart

An `operator` or higher can send `start`, `stop` or `restart`:

```bash
curl -X PUT http://localhost:8080/api/v1/projects/demo/tasks/web/state -H "$AUTH" -H "$JSON" \
  -d '{"action":"restart"}'
```

Restart pulls the image again and replaces the container. With the default
Docker restart policy, running tasks return after the machine restarts;
tasks stopped through Boreas stay stopped. Restarting Boreas alone does not
restart task containers.

## Change a task

A task member or owner can update it, sending only changed fields:

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"image":"nginx:alpine"}'
```

- Changing `image`, `port`, `env`, `labels` or `volumes` replaces the container;
  a running task starts again, while a stopped task stays stopped.
  With `"auto_restart":false`, a running task stops and replacement waits for
  the next start or restart.
- `description`, `note`, `dev_status`, `form_factor`, `password` and `routes`
  apply without replacing the container.
- Names cannot change: create a new task instead. Delete a task with
  `DELETE /api/v1/projects/demo/tasks/web`; this removes its container and grants.

### Environment variables

`env` replaces all variables: include those you want to keep, or send `{}` to
clear them.

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"env":{"APP_MODE":"staging"}}'
```

Anyone with API access to the task, including viewers, can read these values.
[Bora](bora.md) reads variable names, not stored values. Boreas reserves
`BOREAS_PROJECT`, `BOREAS_TASK`, `BOREAS_PORT` and `BASE_HREF`.
`labels` similarly replaces Docker labels; `managed-by`, `project` and `task`
are reserved.

### Description and note

`description` is a short summary; `note` holds Markdown context, checklists or
links. Boreas stores the note without rendering it. Neither change restarts
the container or sends a notification. **Notes are public unless the task is
password-locked: keep secrets out.**

### Form defaults for new tasks

A project owner can prefill the app's new-task form:

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo -H "$AUTH" -H "$JSON" \
  -d '{"default_image":"nginx:alpine","default_port":80,"default_env":{"APP_ENV":"dev"}}'
```

These defaults do not apply to API task creation or change existing tasks.
Initially they are no image, port 80 and no variables; `""` clears the image
and `{}` clears the variables. Grant-only users do not receive `default_env`
values, which may contain secrets. Unlike these form hints,
`default_form_factor` applies to API creation too; see [Mobile apps](#mobile-apps).

## Shared folders

Tasks can mount administrator-prepared folders for certificates, configuration
or persistent data. Requires Docker Engine 26 or later.

1. Create a folder under the project's directory in `boreas-appdata`.
   With Compose:

   ```bash
   docker exec boreas mkdir -p /opt/appdata/demo/certs
   ```

2. List available folders through the API:

   ```bash
   curl -H "$AUTH" http://localhost:8080/api/v1/projects/demo/folders
   ```

3. Map container paths to folder names (`volumes` replaces the whole map):

   ```bash
   curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
     -d '{"volumes":{"/etc/certs":"certs"}}'
   ```

Folders are writable and shared by every task mounting them. Their contents
survive container replacement; other container files do not. Before creation,
Boreas attempts to make mounted folders writable by everyone (mode `0777`);
files and subfolders keep their permissions. Missing folders cause `400`.
Tasks select folders only from their project; an administrator can share one
across projects with a link inside the volume, such as
`demo/certs -> ../_shared/certs`.

For server storage outside Docker's own directory, create a dedicated host
directory and configure this before the volume's first creation. Docker does
not update an existing volume's options:

```yaml
volumes:
  appdata:
    name: boreas-appdata
    driver: local
    driver_opts: { type: none, o: bind, device: /opt/appdata/boreas }
```

## Routes to other services

Routes forward paths under the App URL to services reachable from Boreas,
even while the task container is stopped:

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"routes":[{"path":"/api","url":"http://10.20.0.15:8080"}]}'
```

Here `/demo/web/api/orders` reaches `http://10.20.0.15:8080/orders`.
`routes` replaces the list (up to 20); the longest matching path wins.
Paths use `/`-separated letters, digits, `-` and `_`, up to 64 characters;
`/` alone is reserved. Targets must be HTTP(S), without credentials, query
or fragment. `"any_cert":true` disables target TLS certificate verification.

**Routes expose the target through a public task link**, unless the task is
password-locked. Choose targets accordingly. To test one, POST
`{"url":"http://10.20.0.15:8080"}` to
`/api/v1/projects/demo/tasks/web/routes/check`. Boreas makes one GET within
3 seconds, without redirects; any HTTP response counts as reachable.
Failures return an `error` reason. Unreachable routes give visitors an error
page and other requests `502` with `route_unreachable`, without revealing the
target address. A detected proxy loop returns `508`.

## Mobile apps

Mark a web-served mobile app, such as an Ionic or Capacitor build, as mobile:

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"form_factor":"mobile"}'
```

Desktop browsers show an iPhone frame with simulated touch, safe areas and
Safari browser identity. This is a preview, not a real iOS browser. Phones,
tablets, automated browsers and externally embedded pages bypass the frame;
desktop touch emulation also bypasses it.

A project owner can PATCH the project with `{"default_form_factor":"mobile"}`
to apply it to future tasks, including API-created ones. Otherwise the default
is `web`. These settings never restart containers or change existing tasks
when only the project default changes.

## Lock a task with a password

```bash
curl -X PATCH http://localhost:8080/api/v1/projects/demo/tasks/web -H "$AUTH" -H "$JSON" \
  -d '{"password":"correct horse battery"}'
```

The lock covers the App URL and its routes, including for project members.
Each browser unlocks for 24 hours. It does **not** grant or restrict management
API access; API roles still apply.

- Passwords need 8–72 bytes (8–72 ASCII characters). Only a hash is stored;
  `has_password` reports whether one exists. `""` removes it. Setting it again,
  even to the same password, invalidates all browser unlocks without restarting
  the container.
- Password attempts share a task-wide limit: a burst of 10, then one per
  30 seconds. Already-unlocked browsers stay in.
- The app never receives the lock cookie. The public directory hides the note
  and assignees, **not the task's existence or basic details**.
- Tasks share one browser origin: scripts in another task can access a task
  the same browser has unlocked. This is not isolation between untrusted apps.
- Requests without a valid unlock cookie get `401`, including webhooks and
  cross-site POST callbacks such as OIDC `form_post`. Cross-origin clients
  cannot assume access. A service worker may still serve cached content after
  the unlock expires.

## Make your app work under its App URL

Boreas strips the task prefix: `/demo/web/assets/app.js` reaches the container
as `/assets/app.js`. Links starting with `/assets` escape the task.

- Use relative URLs, or declare `<base href="/">` in HTML. Boreas rewrites an
  existing `<base>` under the task path; it never adds one. Compressed HTML
  cannot be rewritten, so honor its request for uncompressed HTML.
- Read the prefix from `BASE_HREF` or `X-Forwarded-Prefix`, here `/demo/web/`.
- Boreas rewrites redirects to the app's own paths and scopes response cookies
  to the task path. These paths are not a security boundary between tasks.
- Keep the trailing slash; `/demo/web` redirects to `/demo/web/`.

Container variables are `BOREAS_PROJECT=demo`, `BOREAS_TASK=web`,
`BOREAS_PORT=80` (the configured port) and `BASE_HREF=/demo/web/`.
Requests carry `X-Boreas-Project`, `X-Boreas-Task`, `X-Forwarded-For` and
`X-Forwarded-Proto` headers.

## When a task is not running

The App URL shows a status page: starting pages retry automatically; stopped
or failed tasks point visitors to someone who can start them or check logs.
Unknown task addresses suggest nearby names.

## Good to know

- Boreas waits up to 20 seconds for a new container's port; a timeout leaves
  the task in `error`.
- Names are unique per project regardless of case; `/demo/WEB/` redirects to
  `/demo/web/`.
- Containers publish no host ports. Boreas reaches them over its Docker network;
  this does not isolate them from other containers on that network.
- Logs and live CPU, memory and network charts are available in the app and
  through the API's [live streams](api.md#live-streams).