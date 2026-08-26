# Summary

<!-- What does this change do, and why? Link the issue it addresses, if one
exists: "Closes #123". Note anything a reviewer cannot see from the diff:
tradeoffs, rejected alternatives, follow-ups deliberately left out. -->

## What changed

-

## Testing done

<!-- Paste the commands you ran and their outcome. Pre-push runs the first
three; run them yourself first to avoid a rejected push. -->

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] `make openapi-check` passes
- [ ] `make test-integration` (only when Postgres queries or migrations changed; needs a running database)
- [ ] `make test-docker` (only when Docker runtime code changed; needs a running daemon)
- [ ] Exercised manually against a running instance when behavior changed (curl the affected route, or drive the compose stack)

## Contract and schema

<!-- Delete lines that do not apply. -->

- [ ] Routes or DTOs changed → `make openapi` was run and the regenerated `api/openapi.yaml` is in this PR
- [ ] Persistent state changed → a new numbered migration was added under `internal/pkg/database/migrations/` (never rewrite an applied one)
- [ ] New `BOREAS_*` variable → documented in the README configuration table with its default

## Breaking changes

<!-- API responses, route paths, environment variables, or startup behavior
that existing deployments must react to. Write "None" when there are none;
mark the commit with `!` (for example `feat!:`) when there are. -->

None
