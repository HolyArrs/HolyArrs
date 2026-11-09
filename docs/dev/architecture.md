---
title: Architecture
description: See how one request moves through HolyArrs, where each part of the code lives and which rules keep modules apart.
level: Expert
sidebar: { order: 3 }
---

Find where code lives and which rules apply before you change something big.

## The short version

HolyArrs is one Go program with the web app built in. It serves the web app and a JSON API on port `6262`, and keeps its data in 2 SQLite files in the config folder. It calls out to Downloaders, Sources (through Jackett or Prowlarr), media servers, alert channels and the HolyArrs metadata server.

Follow one request. Someone asks for a movie on Explore.

1. `requests` records the ask.
2. After approval, `movies` asks `sources` for Releases.
3. The pick rules in `mediafiles` score them.
4. `downloads` sends the best one to the Downloader.
5. When it finishes, `mediafiles` imports the file.
6. `mediaservers` and `alerts` are told.

## Code map

```
app/backend/cmd/holyarrs/            main.go starts the program; wiring.go builds the modules and joins them
app/backend/internal/<module>/       one flat folder per module
app/backend/internal/core/           plumbing every module may use
app/backend/tools/                   dev-only programs
app/frontend/                        the SvelteKit web app; embed.go bundles its build
app/frontend/src/routes/<page>/      thin pages that combine module parts
app/frontend/src/lib/<module>/       each module's web code
app/frontend/src/lib/ui/             parts used by 2 or more modules, and nothing else
app/frontend/src/lib/core/           server client, app state, helpers
app/tests/                           end-to-end tests (Playwright)
deploy/                              the Dockerfile and release scripts
docs/                                guides and these pages
```

`go.mod` sits at the repo root because `go:embed` cannot reach a parent folder.

### Where to find things

| You want to change | Look in |
|---|---|
| How a Release is picked | `internal/mediafiles` |
| A Downloader client | `internal/downloads`, contract tests in `internal/downloads/conformance_test.go` |
| An alert channel | `internal/alerts` |
| Sign-in, users, API keys | `internal/accounts` |
| The first-run wizard | `internal/setup`, `app/frontend/src/routes/welcome` |
| Importing a finished file (unpack, tags, matching) | `internal/importer` |
| Library folders, adopting and renaming | `internal/library` |
| How rows and shelves are picked and ordered | `internal/suggest` |
| Which parts of the app are on | `internal/core/parts` |
| A page address | `app/frontend/src/routes/` |
| A migration | `internal/core/database/migrations/` |
| The Docker image | `deploy/Dockerfile` |

## The rules

Checks in `app/backend/tools/structurecheck` fail the build when one breaks. Run them with the line in [Testing](testing.md).

- **Layer order.** A module imports only modules on a lower layer, and only their root package. Modules on the same layer never import each other.
- **Calling up.** When a lower module needs something from a higher one, the higher one passes plain data down, or the lower one defines a small interface that `wiring.go` fills in.
- **SQL stays home.** Only a module's own `store*.go` files hold SQL, and only over its own tables. To read another module's rows, call its service.
- **Shared tables belong to core.** Reach `settings`, `schedules`, `secrets`, `jobs` and `events` through core helpers.
- **One way out.** Every outside call goes through `core/outbound`.
- **Held-back features** register a switch with `core/startup`. While it is off, the route is not there and the page hides the feature.
- **No grab-bag packages.** No `util`, `common`, `shared` or `helpers`.

The layer order is in [Modules](modules.md#layer-order).

## For experts

<details class="ha-more"><summary>Inside a module</summary>

Every backend module uses the same file names.

| File | Holds |
|---|---|
| `routes.go` | API routes, each with the permission it needs |
| `service.go` and topic files | Logic other modules call |
| `store.go`, `store_<topic>.go` | All SQL for the module's own tables |
| `types.go` | The module's records |

On the web side, `lib/<module>/` holds `index.ts` (what other modules may use), its Svelte parts and their tests, plus `api.ts` and `state.svelte.ts` only when it has its own calls or state.

</details>

<details class="ha-more"><summary>Core packages</summary>

Core is plumbing. It holds no product rules and imports no module.

| Package | Holds |
|---|---|
| `core/database` | Opening and migrating both SQLite files, query helpers, settings and events helpers, shared row fields, history trimming |
| `core/files` | File and folder helpers, free disk space |
| `core/releasename` | Quality, group and episode numbers from a release name |
| `core/outbound` | The one way to call outside servers, with address checks |
| `core/secrets` | The key store and key rotation |
| `core/logs` | The app log and crash notes |
| `core/jobs` | The job queue, schedules and the loops that restart after a crash |
| `core/webserver` | Router plumbing, URL base, compression, live push |
| `core/startup` | Run-as user, low-memory mode, held-back switches, memory limit, parts |
| `core/parts` | The part switches every module asks before it serves or runs |
| `core/model` | Shared types such as actors and subjects |

When core must touch module data, the module registers a hook with core in `wiring.go`. Key rotation works this way.

</details>

<details class="ha-more"><summary>Data</summary>

| File | Holds |
|---|---|
| `holyarrs.db` | The Library, requests, settings and everything else that matters |
| `holyarrs-events.db` | History and other event rows. It grows and is trimmed without touching the main file |

Migrations are numbered SQL files in `core/database/migrations/main` and `core/database/migrations/events`. They run at start.

</details>

## Related

- [Modules](modules.md)
- [Contributing](contributing.md)
- [Testing](testing.md)
