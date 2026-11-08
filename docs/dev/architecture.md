# Architecture

Read this before you change something big. It tells you where things live and the few rules that keep them apart. The full module list is in [Modules](modules.md).

## The short version

HolyArrs is one Go program with the web app built in. It serves the web app and a JSON API on port 6262. It keeps its data in two SQLite files in the config folder. It calls out to Downloaders, Sources (through Jackett or Prowlarr), media servers, alert channels and the HolyArrs metadata server.

Here is one request from start to end. Someone asks for a movie on Explore. The requests module records the ask. Once it is approved, movies asks sources for releases. The pick rules in mediafiles score them. downloads sends the best one to the Downloader. When it finishes, downloads hands the file to mediafiles to import. Then media servers and alerts are told.

## Code map

```
app/backend/cmd/holyarrs/            the program: main.go starts it, wiring.go builds the modules and joins them
app/backend/cmd/holyarrs-companion/  a small separate program that reads health states over the API
app/backend/internal/<module>/       one flat folder per module
app/backend/internal/core/           plumbing every module may use
app/backend/tools/                   dev-only programs (demo data, a fake subtitle site, structure checks)
app/frontend/                        the SvelteKit web app; embed.go bundles its build into the program
app/frontend/src/routes/<page>/      thin pages that put module parts together
app/frontend/src/lib/<module>/       each module's web code
app/frontend/src/lib/ui/             parts used by two or more modules, and nothing else
app/frontend/src/lib/core/           server client, app state, helpers
app/tests/                           end-to-end tests (Playwright)
deploy/                              the Dockerfile and release scripts
docs/                                user guides and these pages
```

`go.mod` sits at the repo root, because `go:embed` can't reach a parent folder.

### Inside a module

Every backend module uses the same file names, so you know where to look:

- `routes.go` registers the module's API routes, each with the permission it needs.
- `service.go`, plus topic files, holds the logic. Other modules call this.
- `store.go`, plus `store_<topic>.go`, holds all SQL for the module's own tables.
- `types.go` holds the module's records.

On the web side, `lib/<module>/` holds `api.ts`, `state.svelte.ts`, its Svelte parts and their tests.

### core

`core/` is plumbing. It holds no product rules and never imports a module.

| Package | Holds |
|---|---|
| `core/database` | opening and migrating both SQLite files, query helpers, settings and events helpers, the shared row fields, history trimming |
| `core/files` | plain file and folder helpers, free disk space |
| `core/releasename` | reading quality, group and episode numbers out of a release name |
| `core/outbound` | the one way to call outside servers, with address checks |
| `core/secrets` | the key store and key rotation |
| `core/logs` | the app log and crash notes |
| `core/jobs` | the job queue, schedules, and the loops that restart after a crash |
| `core/webserver` | router plumbing, URL base, compression, live push |
| `core/startup` | run-as user, low-memory mode, held-back switches, memory limit, parts |

When core needs to touch module data, the module registers a hook with core in `wiring.go`. Key rotation works this way.

## Data

Two SQLite files live in the config folder:

- `holyarrs.db` holds the Library, requests, settings and everything else that matters.
- `holyarrs-events.db` holds History and other event rows. It can grow and be trimmed without touching the main file.

Migrations are numbered SQL files in `core/database/migrations/main` and `core/database/migrations/events`. They run at start.

## Rules

Automatic checks in `app/backend/tools/structurecheck` fail the build when one of these breaks.

- **Layer order.** A module imports only modules in a lower layer, and only their root package. Modules in the same layer never import each other. The order is in [Modules](modules.md#layer-order).
- **Calling up.** When a lower module needs something from a higher one, the higher one passes plain data down, or the lower one defines a small interface that `wiring.go` fills in.
- **SQL stays home.** Only a module's own `store*.go` files hold SQL, and only over its own tables. To read another module's rows, call that module's service.
- **Shared tables belong to core.** `settings`, `schedules`, `secrets`, `jobs` and `events` are reached through core helpers. Each settings group has one owner module. To change another module's group, call its owner.
- **One way out.** Every outside call goes through `core/outbound`.
- **Held-back features** register a switch with `core/startup`. When the switch is off, the route isn't there and the page hides it. See [Setup](setup.md#held-back-features).
- **No grab-bag packages.** No util, common, shared or helpers packages. Core names never shadow the Go standard library.

## Where to find things

| You want to change | Look in |
|---|---|
| how a release is picked | `internal/mediafiles` |
| a Downloader client | `internal/downloads`, contract tests in `internal/downloads/clienttest` |
| an alert channel | `internal/alerts` |
| sign-in, users, API keys | `internal/accounts` |
| the first-run wizard | `internal/setup`, `app/frontend/src/routes/welcome` |
| a page address | `app/frontend/src/routes/` |
| a migration | `internal/core/database/migrations/` |
| the Docker image | `deploy/Dockerfile` |
