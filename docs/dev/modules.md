---
title: Modules
description: What each of the 20 modules owns, the layer order between them and the structure checks that enforce it.
level: Expert
sidebar: { order: 4 }
---

Pick the module your change belongs in, and know which modules it may call.

## Before you start

- Read [Architecture](architecture.md) for the folder map and the rules.
- Each module has the same name in `app/backend/internal/<module>/`, `app/frontend/src/lib/<module>/` and `app/tests/<module>/`.

## Find the right module

| Module | Does |
|---|---|
| `requests` | Asking, approvals, request rules, outside lists, reports |
| `movies` | Movie titles, editions, sets, search and organising |
| `shows` | Shows, seasons, episodes, search and organising |
| `music` | Artists, albums, tracks, search and organising |
| `sources` | Sources, the Jackett and Prowlarr bridge, Release search |
| `downloads` | Downloaders, the queue, seen files, path maps, tags, copy choices |
| `subtitles` | Subtitle languages, sites, matching and saving |
| `discover` | Explore, Popular, Search, People, Calendar, title pages |
| `mediafiles` | Catalog records, pick rules, naming, data packs, media details, importing, unpacking |
| `metadata` | The outside data client, music lookups, rating ladders, trailers |
| `mediaservers` | Telling Plex, Jellyfin and Emby to look again |
| `accounts` | Users, sign-in, single sign-on, AI app sign-in, kid limits, viewing choices |
| `alerts` | Alert channels and when they speak |
| `migrate` | Bringing data over from older apps |
| `arrapi` | The Sonarr and Radarr style API |
| `ai` | AI app connections (held back) |
| `backups` | Making, keeping and restoring backups |
| `health` | Health checks, the doctor, the support bundle |
| `updates` | The update check and the signed update list |
| `setup` | The first-run wizard |

## Layer order

A module imports only a lower layer. Nothing imports layer 9.

| Layer | Modules |
|---|---|
| 0 | core |
| 1 | `metadata` |
| 2 | `accounts` |
| 3 | `mediafiles` |
| 4 | `sources` |
| 5 | `downloads` |
| 6 | `subtitles`, `mediaservers`, `alerts` |
| 7 | `movies`, `shows`, `music` |
| 8 | `requests` |
| 9 | `discover`, `arrapi`, `migrate`, `ai`, `backups`, `health`, `updates`, `setup` |

If your module needs something from a higher layer, do not import it. Have the higher module pass plain data down, or define a small interface in the lower one that `wiring.go` fills in.

## Add a route or a module

### New route

Register it in the module's `routes.go` with its permission. `openapi.json` comes from the routes, and a test fails on drift. Released paths and field names stay. Add fields, never rename them. Then rewrite [API routes](api-routes.md) with `go run ./app/backend/tools/apidoc`.

### New module

Fit your change into the 20 first, and open an issue before you add one. A new module needs its 3 code folders, a place in the layer order in `tools/structurecheck`, and a line in `wiring.go`.

## If it does not work

**Symptom.** `TestModuleEdges` fails.
**Cause.** A module imports one on the same or a higher layer.
**Fix.** Pass plain data down from the higher module, or add a small interface wired in `wiring.go`.

**Symptom.** `TestSQLOnlyInStore` or `TestTableOwnership` fails.
**Cause.** SQL sits outside a `store*.go` file, or touches another module's table.
**Fix.** Move the query into the owner's store and call the owner's service by ID.

## For experts

<details class="ha-more"><summary>More rules</summary>

- A module imports only the root package of another module.
- Store functions take a `database.Querier`. Read another module's rows through its service by ID, and pass long ID lists as one JSON array with `json_each(?)`.
- `settings`, `schedules`, `secrets`, `jobs` and `events` belong to core. Each settings group has one owner module. To change another module's group, call its owner.
- Every outside call goes through `core/outbound`.
- Core packages sit in rows, top to bottom: `webserver` and `startup`, `logs` and `jobs`, `outbound` and `secrets`, then `database`, `files` and `releasename`. A core package imports only its own row or lower.
- Names: Go files are lowercase with underscores, Svelte parts are PascalCase, and TypeScript, docs, tests and deploy files are kebab-case. Folders in `app/` are lowercase, one word.
- Rename code freely, but anything stored or sent keeps its name: tables and columns, settings keys, JSON fields, job names and switch names.
- Comments carry no roadmap numbers, dates, names or em dashes.

</details>

<details class="ha-more"><summary>Record owners</summary>

| Owner | Records |
|---|---|
| `mediafiles` | Title, Item, Version, Slot, Part, File, LibraryFolder, Quality, PickRules |
| `downloads` | Download, Downloader, SeenFile, PathMap |
| `sources` | Source, Release |
| `requests` | Ask, Report |
| `accounts` | User |
| `core/jobs` | Job, Schedule, JobError |
| `core/database` | Event and the shared row fields |

</details>

<details class="ha-more"><summary id="the-checks">The checks</summary>

The checks live in `app/backend/tools/structurecheck`. Most need `-tags structure`.

| Check | Holds |
|---|---|
| `TestTopLayout` | Top repo folders |
| `TestModuleSet`, `TestCoreSet`, `TestPackageMap` | Folders allowed in `internal/` and `internal/core/` |
| `TestModuleEdges` | Layer order |
| `TestModuleImportsRootOnly` | Root-package imports only |
| `TestCoreImportsNoModule`, `TestCoreRowOrder`, `TestCoreNamesNoModuleTable` | Core imports, row order and names |
| `TestSQLOnlyInStore`, `TestTableOwnership`, `TestSQLAllowlistShrinks`, `TestSeedSQLAllowlist` | SQL placement and table owners |
| `TestGoFileNames`, `TestNoBannedPackageNames` | File and package names |
| `TestFileSizeCap`, `TestCorePackageSize` | File and core package size |
| `TestCommentHygiene` | Comment rules above |
| `TestRouteTableUnchanged`, `TestMigrationsUnchanged`, `TestHeaderAndJSONLiteralsUnchanged` | A move changes no route, migration, HTTP header name or JSON key |
| `TestNoOldPageAddresses`, `TestNoOldPagesInTestsOrDocs` | Old page addresses stay gone from code, tests and docs |
| `TestNoDependencyBotConfig` | No dependency bot file in `.github` |

</details>

## Related

- [Architecture](architecture.md)
- [Testing](testing.md)
- [API routes](api-routes.md)
