# MCP key permissions

## What exists today

- Routes register as `"METHOD /path"` on one `http.ServeMux` (`core/webserver/router.go`); `accounts.RoutePerms` (`accounts/scopes.go`) says what each needs: a person permission (`ask`, `approve`, `manage_library`, `settings`...) or `signed_in`, `users` (owner or admin), `owner`, `public`. Missing routes are owner-only.
- People have a role (`owner`, `admin`, `member`) plus a perm list (`core/webserver/perms.go`).
- API keys carry scopes (`read`, `write`, `admin`, `control`, `all`, `health_read`, `compat`, `mcp`). `accounts.RouteScope` maps each route to one scope; `accounts.Gate` refuses a key whose scopes do not cover it. A key also acts within its person's permissions.
- An `mcp` key reaches only `/mcp` and `/api/v1/mcp`; the AI tools in `internal/ai` then call app routes as the key's person.
- The old-app API (`/sonarr/`, `/radarr/`) takes its own keys (`accounts/arr_keys.go`).
- Nothing yet lets the owner narrow what an MCP key may do beyond the person's permissions.

## Groups

`internal/ai/mcpkeys` sorts every route into one group. Each MCP key gets one tick box per group. Rules, first match wins:

| Group | Tick box | Routes |
|---|---|---|
| `see` | See library and activity | every GET/HEAD not below; plus POST reads (explore counts, live interest, mark messages read, naming example) and the `/mcp` routes themselves |
| `add` | Add and request | every other write: titles, asks (incl. approve / turn down), follows, music, artists, shows, hidden, never-add, reports, old-app movie/series/episode writes |
| `control` | Control downloads | writes under downloads, downloaders/{id}/..., holding, needs-decision, releases, manual-import, rename, imports, search/, jobs, subtitles/, library copies search, any `.../search` or `.../rescan`, old-app `command` and `queue` |
| `settings` | Change settings | GET and writes under `/api/v1/settings`; writes to downloaders (add, edit, test), sources, bridges, media servers, library folders, subtitle sites, pick rules, own traits, schedules, alerts, ask limits, outside lists, tags, watch services, `my` and `account`, health check run; old-app config, profiles, indexers, notifications, tags |
| `delete` | Delete | every DELETE, and every write with a path word `remove`, `delete`, `trash`, `clean`, `clear-now` or `abandon` |
| `admin` | Admin | any method under auth (sign-in, keys, SSO, two-factor), users, backups, app (update, restart, shut down), system, data packs, migrate, logs, support, parts, stages, links, old-app API keys, import secrets, URL base, oauth, .well-known, login, invite pages |

A few single routes are pinned in `overrides` (e.g. `GET /api/v1/auth/me` is `see`; adding or editing a downloader is `settings`). A route outside the known roots (`/api/v1`, old-app paths, pages, `/mcp`, sign-in) has no group and `Allows` refuses it.

Today's split (from the test): see 198, settings 116, admin 115, delete 56, control 55, add 50.

## Guard

`TestEveryRouteHasAGroup` walks every non-test `.go` file under `app/backend/internal` and `app/backend/cmd`, reads each `Handle`/`HandleFunc("METHOD /path")` registration and fails if one has no group. It finds every `accounts.RoutePerms` entry plus the old-app inner routes and the public sign-in routes.

## Not done yet

No middleware, no database column, no screen. The tick boxes still sit on top of the person's own permissions and the key's scope.
