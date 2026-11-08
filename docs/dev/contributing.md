---
title: Contributing
description: Build HolyArrs from source, run the checks, find where code lives, and cut a release.
level: Expert
sidebar: { order: 1 }
---

Build HolyArrs on your machine, make a change in the right module, and check it before you open a pull request.

## Before you start

- Go (the version in `go.mod`), Node 24 and npm.
- Docker, to build the image. Python 3, only for release scripts.
- No C compiler: builds use `CGO_ENABLED=0`.

## Build and run

1. Build the web app and the program:

   ```sh
   npm --prefix app/frontend ci
   npm --prefix app/frontend run build
   go build -o bin/holyarrs ./app/backend/cmd/holyarrs
   ./bin/holyarrs -config ./config
   ```

2. Open `http://localhost:6262`. You should see the first-run wizard.
3. To work on the web app, keep the program running and start the dev server in a second terminal: `npm --prefix app/frontend run dev`. It reloads on save and sends API calls to the program.

The program embeds `app/frontend/dist`. After a web change, build the web app again, or the program serves the old one. `make ui`, `make build`, `make test` and `make vet` wrap the same steps.

## Check your change

CI runs these:

```sh
go build ./...
go vet ./...
go test ./...
npm --prefix app/frontend run check
npm --prefix app/frontend run test
```

`go test ./...` includes the structure checks that hold the module rules. Then run the end-to-end tests for the areas you touched:

```sh
npx --prefix app/frontend playwright install chromium
npm --prefix app/frontend run e2e -- --project=movies
```

Read **Modules and rules** under For experts so your change lands in the right place. Questions go to [Discord](https://discord.com/invite/mthsvesKpC); issues are for bugs and the roadmap.

## If it does not work

| Symptom | Fix |
|---|---|
| A structure check fails | The message names the file and rule. Move the code, or pass plain data in from the higher module. |
| The web app looks old | Build the web app again before the program. |
| End-to-end tests can't start | Run the `playwright install` line above. |

## For experts

<details class="ha-more"><summary>Testing in depth</summary>

- End-to-end tests use Playwright in `app/tests/`, with stand-in servers for downloaders, sources and the metadata server, so no test reaches a real outside service.
- Each run starts its own HolyArrs on a fresh config. The `setup` project makes the owner first; the other projects follow. Run the full suite before a release.
- Long tests (crash recovery in every loop, restart timing, memory under load) carry the `overnight` build tag: `go test -tags overnight ./app/backend/cmd/holyarrs`. Run them before a release, with the machine free for 1 h or more.
- Stand-in variables such as `HOLYARRS_TMDB_URL`, `HOLYARRS_MUSICBRAINZ_URL`, `HOLYARRS_PLEX_URL` and `HOLYARRS_QBITTORRENT_URL` are set by the tests; a real install never needs them. Lab tests read `HOLYARRS_LAB_*` and skip when unset.

| Test kind | Where |
|---|---|
| Go | Beside the file it tests |
| Web unit | Beside its file, as `name.test.ts` |
| End-to-end | `app/tests/<module>/` |

Never call a real outside service; add a stand-in. A test that passes only on your machine is broken.

</details>

<details class="ha-more"><summary>Fake data and held-back features</summary>

- `app/backend/tools/demo` works on a stopped config folder: it fills the Library with a demo collection, removes it, and counts what is stored.
- `app/backend/tools/fakesubs` runs a fake subtitle site for subtitle tests. Neither ships in the image.

Held-back features stay off unless named in `HOLYARRS_FLAGS`:

```sh
HOLYARRS_FLAGS=subtitle_sites,ai_apps ./bin/holyarrs -config ./config
```

| Switch | Feature |
|---|---|
| `ai_apps` | AI app connections and keys |
| `plex_watchlists` | Plex watchlists become requests |
| `row_title_stats` | Explore row statistics for the owner |
| `trakt_lists` | Trakt lists |
| `alert_channels_more` | Alert channels beyond Discord and email |
| `subtitle_sites` | Subtitle sites and subtitle searching |
| `emby_signin` | Sign in with Emby |
| `go_back` | Steps to go back to the previous version |

To add one, register it with `startup.RegisterFeature` and check `startup.FeatureOn` before registering its routes. The web app hides switched-off features.

</details>

<details class="ha-more"><summary>Architecture</summary>

HolyArrs is one Go program with the web app built in, serving the web app and a JSON API on port `6262`. It calls out to downloaders, sources (through Jackett or Prowlarr), media servers, alert channels and the HolyArrs metadata server.

One request end to end: someone asks for a movie on Explore; requests records it; once approved, movies asks sources for releases; pick rules in mediafiles score them; downloads sends the best to the downloader; when it finishes, mediafiles imports the file; media servers and alerts are told.

```
app/backend/cmd/holyarrs/            main.go starts it; wiring.go builds and joins the modules
app/backend/cmd/holyarrs-companion/  reads health states over the API
app/backend/internal/<module>/       one flat folder per module
app/backend/internal/core/           plumbing every module may use
app/backend/tools/                   dev-only programs
app/frontend/                        SvelteKit web app; embed.go bundles its build
app/frontend/src/routes/<page>/      thin pages that combine module parts
app/frontend/src/lib/<module>/       each module's web code
app/frontend/src/lib/ui/             parts used by 2 or more modules
app/frontend/src/lib/core/           server client, app state, helpers
app/tests/                           end-to-end tests
deploy/                              Dockerfile and release scripts
docs/                                user guides and these pages
```

`go.mod` sits at the repo root because `go:embed` can't reach a parent folder.

| Backend file | Holds |
|---|---|
| `routes.go` | API routes, each with its permission |
| `service.go` and topic files | Logic other modules call |
| `store.go`, `store_<topic>.go` | All SQL for the module's own tables |
| `types.go` | The module's records |

On the web side, `lib/<module>/` holds `api.ts`, `state.svelte.ts`, its Svelte parts and tests.

**Data.** 2 SQLite files in the config folder: `holyarrs.db` (Library, requests, settings and everything else) and `holyarrs-events.db` (History and event rows, trimmed without touching the main file). Numbered SQL migrations in `core/database/migrations/main` and `.../events` run at start.

| Core package | Holds |
|---|---|
| `core/database` | Opening and migrating both files, query helpers, settings and events helpers, shared row fields, history trimming |
| `core/files` | File and folder helpers, free disk space |
| `core/releasename` | Quality, group and episode numbers from a release name |
| `core/outbound` | The one way to call outside servers, with address checks |
| `core/secrets` | Key store and key rotation |
| `core/logs` | App log and crash notes |
| `core/jobs` | Job queue, schedules, loops that restart after a crash |
| `core/webserver` | Router plumbing, URL base, compression, live push |
| `core/startup` | Run-as user, low-memory mode, held-back switches, memory limit, parts |

Core holds no product rules and imports no module. When core must touch module data, the module registers a hook in `wiring.go` (key rotation works this way).

</details>

<details class="ha-more"><summary>Modules and rules</summary>

20 modules, each with the same name in `app/backend/internal/<module>/`, `app/frontend/src/lib/<module>/`, `app/tests/<module>/` and `docs/user/<module>/`.

| Module | Does |
|---|---|
| requests | Asking, approvals, request rules, outside lists, reports |
| movies | Movie titles, editions, sets, search and organising |
| shows | Shows, seasons, episodes, search and organising |
| music | Artists, albums, tracks, search and organising |
| sources | Sources, the Jackett and Prowlarr bridge, release search |
| downloads | Downloaders, the queue, seen files, path maps, tags, copy choices |
| subtitles | Subtitle languages, sites, matching and saving |
| discover | Explore, Popular, Search, People, Calendar, title pages |
| mediafiles | Catalog records, pick rules, naming, data packs, media details, importing, unpacking |
| metadata | Outside data client, music lookups, rating ladders, trailers |
| mediaservers | Telling Plex, Jellyfin and Emby to look again |
| accounts | Users, sign-in, single sign-on, AI app sign-in, kid limits, viewing choices |
| alerts | Alert channels and when they speak |
| migrate | Bringing data over from older apps |
| arrapi | The Sonarr and Radarr style API |
| ai | AI app connections (held back) |
| backups | Making, keeping and restoring backups |
| health | Health checks, the doctor, the support bundle |
| updates | Update check and the signed update list |
| setup | First-run wizard |

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

Core rows, top to bottom: webserver, startup / logs, jobs / outbound, secrets / database, files, releasename. A core package imports only its own row or lower.

**Rules** (checked in `app/backend/tools/structurecheck`; a break fails the build):

- A module imports only lower layers, and only their root package. Same-layer modules never import each other. Nothing imports layer 9.
- Calling up: the higher module passes plain data down, or the lower one defines a small interface that `wiring.go` fills.
- SQL lives only in a module's `store*.go` files, over its own tables. Store functions take a `database.Querier`. Read another module's rows through its service by ID; pass long ID lists as one JSON array with `json_each(?)`.
- `settings`, `schedules`, `secrets`, `jobs` and `events` belong to core. Each settings group has one owner module.
- Every outside call goes through `core/outbound`.
- No util, common, shared or helpers packages. Core names never shadow the Go standard library.

| Owner | Records |
|---|---|
| mediafiles | Title, Item, Version, Slot, Part, File, LibraryFolder, Quality, PickRules |
| downloads | Download, Downloader, SeenFile, PathMap |
| sources | Source, Release |
| requests | Ask, Report |
| accounts | User |
| core/jobs | Job, Schedule, JobError |
| core/database | Event and the shared row fields |

| Check | Holds |
|---|---|
| `TestTopLayout` | Top repo folders |
| `TestModuleSet`, `TestCoreSet` | Folders allowed in `internal/` and `internal/core/` |
| `TestModuleEdges` | Layer order |
| `TestModuleImportsRootOnly` | Root-package imports only |
| `TestCoreImportsNoModule`, `TestCoreRowOrder` | Core imports and row order |
| `TestSQLOnlyInStore`, `TestTableOwnership` | SQL placement and table owners |
| `TestGoFileNames`, `TestNoBannedPackageNames` | File and package names |
| `TestFileSizeCap`, `TestCorePackageSize` | File and core package size |
| `TestRouteTableUnchanged`, `TestMigrationsUnchanged` | Moves don't change routes or migrations |
| `TestNoOldPageAddresses` | Old page addresses stay gone |

**Naming.** Go files lowercase with underscores; Svelte parts PascalCase; TypeScript, docs, tests and deploy files kebab-case; folders in `app/` lowercase, one word. Rename code freely, but anything stored or sent keeps its name: tables and columns, settings keys, JSON fields, job names, switch names.

**New module.** Fit your change into the 20 first, and open an issue before adding one. It needs its 4 folders, a place in the layer order in `structurecheck`, and a line in `wiring.go`.

**New route.** Register it in the module's `routes.go` with its permission. `openapi.json` comes from the routes and a test fails on drift. Released paths and field names stay: add fields, don't rename. See [API routes](../reference/api-routes/).

</details>

<details class="ha-more"><summary>Build the image</summary>

```sh
docker build --file deploy/Dockerfile --tag holyarrs:dev .
docker run -p 6262:6262 -v ./config:/config holyarrs:dev
```

The image starts `FROM scratch` and holds the program, CA certificates and licences.

</details>

<details class="ha-more"><summary>Releasing (maintainers)</summary>

Tags are plain numbers (`1.0.0`, `1.1.0`, `1.1.1`), built from a tag on the main branch. The workflow builds one image for `linux/amd64`, `linux/arm64` and `linux/arm/v7` and pushes `holyarrs/holyarrs` to Docker Hub. A new version lands on `:beta` and its version tag; `:latest` and `:stable` move only after it has run well for a while.

Before you tag:

1. Every check above passes, including a full end-to-end run and the `overnight` tests.
2. `CHANGELOG.md` has the new version.
3. `python3 deploy/release/notices.py --check` passes.
4. `docker build --file deploy/Dockerfile .` builds and starts on a fresh config.

**Release notes.** Every release text comes from one file, `releases/notes-<version>.toml`; never hand-edit what it builds.

| Field | Content |
|---|---|
| `version` | Plain `X.Y.Z` |
| `theme` | One line |
| `images` | List of `{registry, name}`; Docker Hub `holyarrs/holyarrs` only |
| `docs_url` | Optional; empty means no Docs link |
| `[[highlights]]` | Exactly 3, each with `line`, `issues`, `shot`. Their issues appear in no group line. |
| `[groups]` | Lines per place. New by default; others start `Changed: `, `Fixed: ` or `Docs: `. Each ends with its issue, such as `(#46)`. Places with under 3 lines go under Also new. |
| Optional | `[before_you_update]` (`breaking`, `manual_steps`, `backup`, `rollback`), `security`, `known_issues`, `credits`, `[stats]`. Empty parts are hidden. |

GitHub logins only when known, in the notes file or `releases/credits-map.json`. Body order: theme, Highlights, places (Requests, Library, Downloads, Indexers, Accounts, Health, Setup, Install), Also new, Security, Known issues, Before you update, Image, Thanks, stats line.

```sh
python deploy/release/notes.py 1.0.0 --check
python deploy/release/notes.py 1.0.0
```

The second writes `changelog.md`, `github.md`, `update-notes.json`, `reddit.txt`, `discord-dm.txt`, `discord-announce.md` and `roadmap-card.json` to `releases/out/1.0.0/`. `--check` exits 1 on any error (banned words, em dashes, dates or launch talk, a repeated highlight issue, a Discord text over 1,000 characters) and warns on long lines and near-repeats.

**Update list.** The app reads a signed update list. Each release adds its entry and signs it. The signing key never goes in the repo; HolyArrs refuses a list it can't verify.

**After.** Check the image's version label matches the tag (`docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs`) and watch new issues for the first day.

**Taking one back.** Move `:beta` to the previous version and mark the bad one in the update list so installs don't offer it. Keep the tag.

**Unraid test template.** `deploy/release/nas-template.py` writes an Unraid user template for one exact image. It pulls, installs and publishes nothing.

```sh
python deploy/release/nas-template.py --image 'holyarrs/holyarrs@sha256:<digest>' --config '/mnt/<pool>/appdata/holyarrs-test' --movies /mnt/user/holyarrs-test-data/movies --downloads /mnt/user/holyarrs-test-data/downloads --port 6262 --output holyarrs-test.xml
```

- Use the registry manifest digest, not a Docker image ID.
- `--config` must be a pool path; the tool refuses `/mnt/user` and `/mnt/user0`.
- `--movies`, `--tv`, `--music` and `--downloads` are optional; separate paths are separate mounts, so imports copy.
- The template sets `PUID=99`, `PGID=100` and `UMASK=022`.
- Put the file in `/boot/config/plugins/dockerMan/templates-user/` and pick it under **Docker > Add Container > Template**.

</details>

## Related

- [API routes](../reference/api-routes/)
- [Portable build](../run/portable/)
