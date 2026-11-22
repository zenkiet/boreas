# Notifications

Boreas tells the team when a build fails, a task is ready for testing or a plan
needs input. Updates appear in the app feed, team chat through Apprise, or on
devices through Firebase Cloud Messaging (FCM).

The feed works without external services; chat and push need administrator
setup. API examples use [AUTH and JSON](getting-started.md#3-sign-in).

## What sends a notification

| Event (`type`) | Severity (`status`) | Meaning |
| -------------- | ------------------- | ------- |
| `deployed` | `success` | Deployment succeeded |
| `deploy_failed` | `failure` | Deployment failed |
| `task_created` | `info` | Task created |
| `task_assigned` | `info` | Task assigned to you |
| `status_changed` | `info` | Development status changed |
| `build_failed` | `failure` | Build failed; includes the reported stage |
| `container_down` | `failure` | Container exited unexpectedly |
| `spec_asking` | `info` | Bora needs answers from the team |
| `spec_confirming` | `info` | Bora has decisions to confirm |
| `spec_ready` | `success` | Ticket plan and tests are ready |
| `spec_failed` | `failure` | Planning stopped: manual stop, model failure or server restart |
| `support_request` | `info` | A visitor started a chat in the project's [Libredesk inbox](#support-chats-from-libredesk) |

Messages name the project and task or ticket. Deploy times use server `TZ`.
Feed integrations should use `type` and `project` (slug), not display text.

Deploying the same image again sends nothing unless the container still needs replacement.
Build failures notify once per run. Unexpected container exits mark the task
`error` and notify once; deliberate Docker stops or kills do not.

## Who receives what

- Task updates reach administrators, project members and team channels.
  [Task grants](projects-and-people.md#access-to-a-single-task) cover only that task.
- Assignments reach only the assignee, not administrators or team channels.
  Self-assignment sends nothing.
- Plan updates (`spec_*`) are private to the starter and ticket-task assignees
  with the `member` role who can view the whole plan. They never go to team
  channels. Stop messages identify the step and, for manual stops, the person.
- Support requests name no task, so they reach administrators, project members
  and team channels, but not task grantees.

## The feed

Read the feed newest first, globally or by project:

```bash
curl -H "$AUTH" 'http://localhost:8080/api/v1/notifications?limit=20'
curl -H "$AUTH" 'http://localhost:8080/api/v1/projects/demo/notifications'
```

Pages default to 50 entries (maximum `limit`: 200). Pass the last ID as `before`
to continue. Entries include `actor` when known and `spec_id` for plan updates.

Seen state is personal. Your signed-in actions are marked seen for you without
pushing to your devices. Mark seen or unseen:

```bash
curl -X POST -H "$AUTH" -H "$JSON" -d '{"ids":["<id>"]}' \
  http://localhost:8080/api/v1/notifications/seen
curl -X DELETE -H "$AUTH" http://localhost:8080/api/v1/projects/demo/notifications/<id>/seen
```

Marking an entry seen twice has no extra effect; inaccessible IDs are skipped.

## Team channels with Apprise

[Apprise](https://github.com/caronc/apprise-api) forwards to Slack, ntfy and other
services. Compose runs `boreas-noti` without a published port.

1. Create `apprise/boreas.yml` beside `docker-compose.yaml`:

   ```yaml
   urls:
     - slack://TokenA/TokenB/TokenC/#staging
     - ntfy://ntfy.example.com/boreas
   ```

2. Run `docker compose restart boreas-noti` after each change.

Without this file, nothing is forwarded. Keep its credentials private;
`apprise/` is Git-ignored.

`BOREAS_NOTIFY_URL` defaults in Compose to `http://boreas-noti:8000/notify`;
Boreas appends `/boreas`. Delivery failures are logged without failing deploys
or preventing feed recording.

## Push to browsers and phones

Administrators enable FCM; users then subscribe through the app:

1. Create a Firebase service account key. Keep it private.
2. Save it as `apprise/fcm-keyfile.json`, or set Compose's `FCM_KEYFILE_PATH`.
3. Set `BOREAS_FCM_PROJECT` to the Firebase project ID and recreate the Boreas
   service (`docker compose up -d boreas`). Compose mounts the key read-only
   and sets `BOREAS_FCM_KEYFILE`. Outside Compose, set that variable yourself.

An unreadable or invalid key stops startup. Each subscription chooses `events`:

- `important` (default): failures, support requests and all updates addressed
  directly to its owner, including assignments and plan updates.
- `all`: every notification its owner can see.

Neither setting pushes the actor's own actions. Disabling an account stops
pushes; deleting it removes subscriptions. Re-registering a token transfers it
to the signed-in user and updates its preference. Users remove only their own
tokens. Invalidated tokens are pruned; other delivery failures are logged.
Push bodies are truncated to 1,000 characters; the feed keeps the full text.

Custom clients register Firebase SDK tokens, or remove them:

```bash
curl -X POST -H "$AUTH" -H "$JSON" \
  -d '{"token":"<FCM registration token>","events":"important"}' \
  http://localhost:8080/api/v1/push/subscriptions
curl -X DELETE -H "$AUTH" "http://localhost:8080/api/v1/push/subscriptions/<FCM registration token>"
```

## Support chats from Libredesk

[Libredesk](https://libredesk.io) gives each project a chat widget in the
corner of its directory page and task pages. Every new chat becomes a
`support_request` for that project.

1. In Libredesk, create one **Live chat** inbox per project and name it exactly
   like the project's slug, such as `shop`. Add your Boreas address, such as
   `https://boreas.example.com`, to the inbox's trusted domains.
2. List the inboxes in `BOREAS_LIBREDESK_INBOXES` as `shop=<inbox UUID>`, set
   `BOREAS_LIBREDESK_URL`, and recreate the Boreas service. See
   [Configuration](configuration.md#support-chat-libredesk).
3. Under **Admin → Integrations → Webhooks**, add a webhook for the
   `conversation.created` event at
   `https://boreas.example.com/api/v1/hooks/libredesk`, with a secret. Set the
   same secret in `BOREAS_LIBREDESK_WEBHOOK_SECRET`.

A chat in an inbox that matches no project slug is logged and sends nothing;
Libredesk still notifies its agents by email. If a proxy in front of Libredesk
adds `X-Frame-Options` to every response, exempt `/widget`, or the chat window
cannot open inside your pages.
