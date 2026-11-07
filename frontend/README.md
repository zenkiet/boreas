# Boreas directory

This folder holds the public pages Boreas serves:

- the task directory;
- the status page that answers for an environment that is not running;
- the privacy policy.

It is a SvelteKit 3 single-page app using Svelte 5 runes and Tailwind 4. adapter-static writes it into `../internal/web/public`, which the Go binary embeds.

## Develop

```sh
pnpm install
pnpm dev                                  # /api is proxied to http://localhost:8080
BOREAS_API=https://boreas.example pnpm dev # or to another server
```

| Command      | Runs                                                           |
| ------------ | -------------------------------------------------------------- |
| `pnpm check` | svelte-check, which must stay at 0 errors and 0 warnings       |
| `pnpm lint`  | Prettier and ESLint, including the FSD import boundaries       |
| `pnpm test`  | Node's built-in runner on `src/**/*.spec.ts`                   |
| `pnpm build` | the site into `../internal/web/public`; `make build` embeds it |

## Structure

The code follows Feature-Sliced Design: `app > pages > entities > shared`. A layer imports only from the layers below it, and enters a slice only through the slice's `index.ts`.

- `app/`: tokens (`tokens.css` maps the design system into Tailwind), base styles and the root layout.
- `routes/`: thin SvelteKit routes, each rendering one page.
- `pages/{directory,env-status,error,privacy}`: code that only one page uses stays in that page.
- `entities/{task,project,user}`: models and UI shared by several pages, including the live `directory` store, which refetches on the public SSE stream.
- `shared/{api,lib,theme,ui}`: the API client, helpers, the theme and design-system components.

Cross-slice imports use the package.json `imports` aliases, `#shared/ui`, `#entities/task` and `#pages/directory`, each of which resolves to the slice's `index.ts`. Inside a slice, import by relative path.

## Conventions

- **Specs** run under plain Node. A module with a spec imports values only from its own slice (by relative path, with `.ts`) or from `#shared/api` and `#shared/lib`. Those two barrels never re-export `.svelte` files.
- **Type sizes** come from the ramp, `text-caption2` through `text-large`. **Colours** come from tokens, such as `text-label-2` and `bg-surface`. Avoid raw values.
- **Motion** uses the boreas-client tokens: `ease-out` and `ease-layout` in `tokens.css`, `T`, `move`, `rise`, `flash`, `viewTransition` and `Count` in `shared/ui`. JavaScript motion checks `prefersReducedMotion`; CSS motion falls under the global reduced-motion rule.
- **Files** stay small: a component is about 150 lines at most. Code explains itself without comments. Adding a dependency is a team decision.
- **Password-protected tasks:** the API never sends their note or people, and `toTask` in `shared/api` enforces the same rule again on the client.
