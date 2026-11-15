# Projects and people

A project groups a product's staging tasks. Roles control access; assignees
identify who works on each task.
Examples use `AUTH` and `JSON` from [Get started](getting-started.md#3-sign-in).

## Accounts

Accounts have a global role:

| Role | Access |
| --- | --- |
| `admin` | Manage accounts and registry credentials, create projects, and act as owner of every project |
| `user` | Use projects they belong to and tasks they have grants on |

Administrators create accounts; passwords need at least 8 characters and the
default role is `user`:

```bash
curl -X POST http://localhost:8080/api/v1/users -H "$AUTH" -H "$JSON" \
  -d '{"username":"lan","email":"lan@example.com","password":"a-long-passphrase"}'
```

List accounts with `GET /api/v1/users`; PATCH `/api/v1/users/<id>` to change
`email`, `password`, `role` or `disabled`. Administrator password/role updates
and account disabling revoke all sessions and API tokens.
Administrators cannot delete themselves.

## Project roles

Each role includes the permissions above it:

| Role | Adds |
| --- | --- |
| `viewer` | See tasks, logs, live charts and project notifications |
| `operator` | Start, stop, restart and deploy tasks; report builds |
| `member` | Create, edit and delete tasks; list members; assign project members |
| `owner` | Change or delete the project, manage members and task grants; assign outsiders |

All project roles can use [Bora](bora.md), when available, within their
permissions. Task grants alone do not enable Bora.

Owners add or update members (default role: `member`) by `user_id` or exact
username/email (`user`):

```bash
curl -X POST http://localhost:8080/api/v1/projects/demo/members -H "$AUTH" -H "$JSON" \
  -d '{"user":"lan","role":"viewer"}'
```

Remove membership with `DELETE /api/v1/projects/demo/members/<id>`.
The last owner cannot be removed or demoted. Removing membership does not
remove separate task grants.

## Access to a single task

Owners grant `viewer`, `operator` or `member` access without project membership:

```bash
curl -X POST http://localhost:8080/api/v1/projects/demo/tasks/web/grants -H "$AUTH" -H "$JSON" \
  -d '{"user_id":"<id>","role":"operator"}'
```

The higher project/grant role wins; `owner` is project-only. Grant-only users
see their granted tasks, their notifications and the project, but not its
`default_env` values. Other [form defaults](tasks.md#form-defaults-for-new-tasks)
remain visible.

Owners list grants with `GET .../tasks/web/grants` and revoke one with
`DELETE .../tasks/web/grants/<id>`. Grants disappear when the task is deleted.

## Assignees

Assignment gives task-level `member` access, not just a label.
Project membership alone does not assign anyone.

```bash
curl -X PUT http://localhost:8080/api/v1/projects/demo/tasks/web/assignees -H "$AUTH" -H "$JSON" \
  -d '{"user_ids":["<id>"]}'
```

- This replaces the task's grant list (up to 50 IDs); `[]` removes every grant.
  Newly added people get `member`; existing grants keep their roles. Removing
  someone revokes their task grant, not their project membership.
- Project members ranked `member` or higher can add or remove fellow project
  members. Only owners or administrators can change assignments for outsiders;
  task-level `member` access alone cannot assign people.
- Creation accepts `assignees` too; assignments survive startup failure.
  New assignees receive private notifications, except for self-assignment.
- Public assignees are member-grant holders, hidden on password-locked tasks.
  The authenticated `assignees` field includes all grants and roles.

## Who sees what

- **App URLs and the directory are public**, regardless of membership. A
  [task password](tasks.md#lock-a-task-with-a-password) protects the app and
  routes, not management API access or the task's existence.
- Management API calls require authentication and the appropriate role.
  Project and task responses include `my_role`. Inaccessible resources answer
  `404` to avoid leaking names; `403` means visible but insufficient permission.
- [API tokens](deploy-from-ci.md#1-create-a-token-for-the-pipeline) use their
  account's current permissions.

## Good to know

Only administrators create projects or manage
[registry credentials](configuration.md#registry-credentials). Owners manage
existing projects; a project cannot be deleted until its tasks are deleted.
