# Deploy from your pipeline

CI builds and pushes the image; Boreas pulls and runs it for an existing task.
Boreas does not build your code.

## 1. Create a token for the pipeline

[Sign in](getting-started.md#3-sign-in) as the pipeline's account, then create
an API token using that login session's `AUTH` header. Choose RFC 3339 dates:
`valid_from` must precede `valid_to`, the interval cannot exceed 90 days, and
`valid_to` must be in the future. A future `valid_from` delays activation.
Replace the example dates with your intended validity window.

```bash
BOREAS_URL=http://localhost:8080
curl -fsS -X POST "$BOREAS_URL/api/v1/auth/tokens" \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"name":"staging-deployer","valid_from":"2026-11-17T00:00:00Z","valid_to":"2027-02-01T00:00:00Z"}'
```

Replace `BOREAS_URL` with your server address. Store the returned token immediately
as the CI secret `BOREAS_TOKEN`: it is shown once; Boreas stores only its hash.
Use HTTPS outside local development and keep tokens out of logs.

The token inherits its user's roles, memberships and grants. Prefer a dedicated
account with an `operator` [grant](projects-and-people.md#access-to-a-single-task)
on just the task it deploys. API tokens cannot manage tokens or ask Bora.
Use the login session to list or revoke them:

```bash
curl -fsS -H "$AUTH" "$BOREAS_URL/api/v1/auth/tokens"
curl -fsS -X DELETE -H "$AUTH" "$BOREAS_URL/api/v1/auth/tokens/<id>"
```

## 2. Deploy the image you built

After pushing the image, send its immutable reference. Here `DIGEST` includes
`sha256:` followed by 64 lowercase hex digits, as returned by your build tool:

```bash
curl -fsS -X POST "$BOREAS_URL/api/v1/projects/demo/tasks/web/deploy" \
  -H "Authorization: Bearer $BOREAS_TOKEN" -H 'Content-Type: application/json' \
  -d '{"image":"ghcr.io/acme/web@'"$DIGEST"'"}'
```

- Only `repository@sha256:<digest>` is accepted; tags such as `:latest` are not.
- Boreas pulls the image and recreates the container. A running task restarts
  and waits for readiness; a stopped task stays stopped. Allow time for the pull
  and readiness check before timing out the request.
- Retrying an already-applied image is a no-op, with no new notification,
  unless the task still needs container recreation. An attempted update
  records a success or failure notification.
- The task must already exist, and deployment needs `operator` access or higher.
  For private images, attach a [registry credential](configuration.md#registry-credentials)
  to the project.

Keep `curl -f` (included in `-fsS`): an HTTP error must fail the pipeline, not
look like a successful deploy.

## 3. Send the commit too

Include `repository` and `revision` together so Bora can read the deployed code
and compare it with the previous deploy. Use Sourcebot's repository name
(for example, `github.com/acme/web`) and the full commit hash used to build the
image. Changing the image without commit metadata clears the recorded commit.

In GitHub Actions, after a `docker/build-push-action` step with `id: build`
and `push: true`:

```yaml
- name: Deploy to Boreas
  env:
    BOREAS_URL: ${{ vars.BOREAS_URL }}
    BOREAS_TOKEN: ${{ secrets.BOREAS_TOKEN }}
    DIGEST: ${{ steps.build.outputs.digest }}
    REPOSITORY: github.com/${{ github.repository }}
    REVISION: ${{ github.sha }}
  run: |
    curl -fsS -X POST "$BOREAS_URL/api/v1/projects/demo/tasks/web/deploy" \
      -H "Authorization: Bearer $BOREAS_TOKEN" -H 'Content-Type: application/json' \
      -d '{"image":"ghcr.io/acme/web@'"$DIGEST"'","repository":"'"$REPOSITORY"'","revision":"'"$REVISION"'"}'
```

Replace the image repository and `demo/tasks/web` with yours. If you build a
different checkout, send that checkout's commit rather than `github.sha`.

## 4. Report build progress

Optionally use the same token to report stages before deployment and an
outcome when the run ends:

```bash
curl -fsS -X PUT "$BOREAS_URL/api/v1/projects/demo/tasks/web/build" \
  -H "Authorization: Bearer $BOREAS_TOKEN" -H 'Content-Type: application/json' \
  -d '{"state":"running","stage":"Test","progress":40,"url":"https://github.com/acme/web/actions/runs/123"}'

curl -fsS -X PUT "$BOREAS_URL/api/v1/projects/demo/tasks/web/build" \
  -H "Authorization: Bearer $BOREAS_TOKEN" -H 'Content-Type: application/json' \
  -d '{"state":"success"}'
```

- `state` is `running`, `success`, `failure` or `canceled`. Optional fields are
  `stage` (up to 100 characters), `progress` (0–100) and `url` (an HTTP(S) run link).
- Only the latest report is stored. Each `running` report replaces the fields;
  a final report retains omitted stage, progress and URL. Send the actual
  outcome from your CI's finalizer, including failures and cancellations.
- A transition to `failure` notifies; repeating that failure with the same run
  URL does not. Other build states do not notify.
- Serialize runs per task: a late report from an older run overwrites a newer
  one. The public directory shows state, stage and progress, never the run link.

## Access and errors

Unreachable projects or tasks return `404`; insufficient roles return `403`.
Revoked, expired or not-yet-active tokens cannot authenticate. An administrator
changing the account's password or role, or disabling it, revokes its tokens;
changing your own password does not revoke API tokens.
See `/api/v1/docs` for the full request and response schemas.
