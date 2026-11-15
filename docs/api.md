# API

Use the HTTP API to connect a pipeline, script or another app to Boreas. All
API paths start with `/api/v1`. For a first working example, follow
[Get started](getting-started.md); for automated deploys, see
[Deploy from your pipeline](deploy-from-ci.md).

## Full reference

| Location | Use |
| -------- | --- |
| `/api/v1/docs` | Browse routes, permissions and fields; try requests |
| `/api/v1/openapi.json` | Read the running server's OpenAPI document |
| [`api/openapi.yaml`](../api/openapi.yaml) | Generate a client from the checked-in contract |

The server generates its reference from its route definitions. Use it instead
of a second, manually maintained route list. A hosted example is at
<https://boreas.zenkiet.dev/api/v1/docs>.

## Sign in

Protected routes require `Authorization: Bearer <token>`. There are two token types:

| Type | Intended use | Lifetime and restrictions |
| ---- | ------------ | ------------------------- |
| Sign-in token | People using the app | 30 days, renewed while used; supports session-only routes |
| API token | Scripts and CI | Explicit validity window, at most 90 days; cannot manage tokens or use Bora chat |

Both use their account's current permissions. Some routes, including managing
sessions and starting ticket plans, require a sign-in token. Plan exports
accept API tokens.

```bash
curl -fsS -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}'
```

Save the returned `token`; see [the shell setup](getting-started.md#3-sign-in)
for `AUTH` and `JSON` used below.

- `POST /auth/logout` revokes the current token.
- An administrator changing a user's password or role, or disabling the account, revokes its tokens.
- Changing your own password keeps your current session and API tokens, but signs out other sessions.
- `/auth/sessions` lists devices and lets you revoke sessions.
- Device pairing uses `/auth/pairing`, `/auth/pairing/claim` and `/auth/pairing/stream`. A code works once within two minutes and travels in the request body.

## Errors

Errors are JSON, for example `{"error":"not found"}`.

| Status | Meaning |
| ------ | ------- |
| `400` | Invalid request; read the error message |
| `401` | Missing, expired or revoked token |
| `403` | Insufficient role, or the route requires a sign-in token |
| `404` | Resource missing or not visible to you; the response does not reveal which |
| `409` | Conflict with current state, or a required feature is not configured |
| `429` | Bora concurrency limit reached |

Project and task responses include `my_role`, the caller's effective role.
`PATCH` changes only fields supplied, but collection fields such as `env` and
`routes` replace their whole collection. See [task settings](tasks.md#change-a-task).

The API permits cross-origin requests with a bearer header. Clients must
sanitize user-written Markdown or text before rendering it as HTML.

## Live streams

Server-Sent Events (SSE) keep an HTTP response open and deliver updates as they
happen. Examples use `curl -N` to disable output buffering. In a browser, use
`fetch` with a stream reader for protected streams: native `EventSource` cannot
set the bearer header. Reverse proxies must not buffer streams.

### Change signals

- `GET /events/stream` requires a sign-in token.
- `GET /public/events/stream` needs no token and supports `EventSource`.

Both send `{}` on connection and when relevant changes occur. The event is a
signal to refetch data, not a description of the change. Bursts are coalesced;
Bora conversation changes do not use this stream. Public streams are limited
to 256 connections per server; additional connections get `503`.

### Logs

```bash
curl -N -H "$AUTH" 'http://localhost:8080/api/v1/projects/demo/tasks/web/logs/stream?tail=100'
```

Events contain `timestamp`, `stream` (`stdout` or `stderr`) and `message`.
Reconnect with `since=<last timestamp>` to resume. `GET .../logs?tail=100`
returns plain text; add `download=true` for a file. The default tail is 100 lines.

### Resource metrics

```bash
curl -N -H "$AUTH" http://localhost:8080/api/v1/metrics/stream
curl -N -H "$AUTH" http://localhost:8080/api/v1/projects/demo/metrics/stream
curl -N -H "$AUTH" http://localhost:8080/api/v1/projects/demo/tasks/web/metrics/stream
```

These cover accessible running tasks across projects, in one project, or in one
task. Reopen after tasks start: the set of containers is chosen at connection time.

```json
{"project":"demo","task":"web","cpu_percent":12.4,"memory_bytes":52428800,"memory_limit":536870912,"network_rx_bytes":1024,"network_tx_bytes":2048,"observed_at":"2026-01-02T03:04:05Z"}
```

Samples arrive about once per task per second. Streams replay up to one minute
of in-memory history before live data; deduplicate by task and `observed_at`.
Memory excludes page cache, and the first CPU sample may be zero. Metrics are
not persisted. The one-task stream returns `409` if no container exists yet.

### Bora's answers

`POST /bora/conversations` and `POST /bora/conversations/{id}/messages` stream
named events: `conversation`, `step`, `say`, `delta`, `reset`, then `done`,
`error` or `stopped`. `reset` discards provisional text; `done` contains the
saved answer.

Disconnecting does not cancel an answer. Reconnect at
`GET /bora/conversations/{id}/stream` to replay it from the start, or use
`POST /bora/conversations/{id}/stop` to cancel without saving the pending answer.
Ticket plans have separate stream and stop routes under `/bora/specs/{id}`.
See [Bora](bora.md) for access and privacy rules.

## Generate a client

From a repository checkout:

```bash
npx @openapitools/openapi-generator-cli generate \
  -i api/openapi.yaml -g typescript-fetch -o ./client
```

Choose another generator with `-g`, such as `go` or `python`. The generated
contract, not this guide, defines all request and response types.

## For AI agents

`/llms.txt` describes the directory and API. `/.well-known/ard.json` provides an
[ARD manifest](https://agenticresourcediscovery.org/spec/) for the requested host.
