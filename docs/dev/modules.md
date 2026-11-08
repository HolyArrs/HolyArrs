# Modules

HolyArrs has 20 modules. Each one has the same name in four places:

- `app/backend/internal/<module>/`
- `app/frontend/src/lib/<module>/`
- `app/tests/<module>/`
- `docs/user/<module>/`

If you want the alerts code, look under alerts.

## The modules

| Module | What it does |
|---|---|
| requests | asking for titles, approvals, request rules, outside lists, reports |
| movies | movie titles, editions, movie sets, movie search and organising |
| shows | shows, seasons, episodes, show search and organising |
| music | artists, albums, tracks, music search and organising |
| sources | Sources, the Jackett and Prowlarr bridge, searching for releases |
| downloads | Downloaders, the queue, seen files, path maps, tags, copy choices |
| subtitles | subtitle languages, subtitle sites, matching and saving subtitles |
| discover | Explore, Popular, Search, People, Calendar and title pages |
| mediafiles | catalog records, pick rules, naming, data packs, media details, importing, unpacking |
| metadata | the client for outside data, music lookups, rating ladders, trailers |
| mediaservers | telling Plex, Jellyfin and Emby to look again |
| accounts | users, sign-in, single sign-on, AI app sign-in, kid limits, viewing choices |
| alerts | alert channels and when they speak |
| migrate | bringing data over from older apps |
| arrapi | the Sonarr and Radarr style API for older apps |
| ai | AI app connections (held back) |
| backups | making, keeping and restoring backups |
| health | health checks, the doctor command, the support bundle |
| updates | the update check and the signed update list |
| setup | the first-run wizard |

## Layer order

Bottom up. A module imports only modules in a lower row, and only their root package.

| Layer | Modules |
|---|---|
| 0 | core |
| 1 | metadata |
| 2 | accounts |
| 3 | mediafiles |
| 4 | sources |
| 5 | downloads |
| 6 | subtitles, mediaservers, alerts |
| 7 | movies, shows, music |
| 8 | requests |
| 9 | discover, arrapi, migrate, ai, backups, health, updates, setup |

Nothing imports the top row. When a lower module needs something from a higher one, the higher one passes plain data in, or the lower one defines a small interface. `app/backend/cmd/holyarrs/wiring.go` builds the higher module and passes it in.

Core has an order too. A core package imports only core packages on its own row or lower:

| Row | Packages |
|---|---|
| top | webserver, startup |
| | logs, jobs |
| | outbound, secrets |
| bottom | database, files, releasename |

## Who owns which records

- mediafiles: Title, Item, Version, Slot, Part, File, LibraryFolder, Quality, PickRules.
- downloads: Download, Downloader, SeenFile, PathMap.
- sources: Source, Release.
- requests: Ask, Report.
- accounts: User.
- core/jobs: Job, Schedule, JobError.
- core/database: Event, and the shared row fields every record carries.

## SQL rules

- SQL lives only in a module's `store*.go` files.
- A store touches only its own module's tables.
- Store functions take a `database.Querier`, so they work on the database or inside a transaction.
- To read another module's rows, call its service with the IDs you need. Keep paging on your own table.
- Pass a long ID list as one JSON array with `json_each(?)`.

## The checks

These tests in `app/backend/tools/structurecheck` hold the rules:

| Test | Checks |
|---|---|
| `TestTopLayout` | the top folders of the repo |
| `TestModuleSet`, `TestCoreSet` | which folders may sit in `internal/` and `internal/core/` |
| `TestModuleEdges` | imports follow the layer order |
| `TestModuleImportsRootOnly` | a module imports another module's root package only |
| `TestCoreImportsNoModule`, `TestCoreRowOrder` | core imports no module and keeps its row order |
| `TestSQLOnlyInStore`, `TestTableOwnership` | SQL sits in store files and only over the owner's tables |
| `TestGoFileNames`, `TestNoBannedPackageNames` | file and package names |
| `TestFileSizeCap`, `TestCorePackageSize` | files and core packages stay small |
| `TestRouteTableUnchanged`, `TestMigrationsUnchanged` | moving code doesn't change routes or migrations |
| `TestNoOldPageAddresses` | old page addresses don't come back |

## Naming

- Go files: lowercase with underscores.
- Svelte parts: PascalCase.
- TypeScript, docs, tests and deploy files: kebab-case.
- Folders in `app/`: lowercase, one word.

You can rename code freely. Anything stored or sent keeps its name: database tables and columns, settings keys, JSON fields, job names, switch names.

## Adding a module

Try to fit your change into the 20 first. Open an issue before you add one. A new module needs its four folders, a place in the layer order in `tools/structurecheck`, and a line in `wiring.go`.
