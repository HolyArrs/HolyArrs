# The HolyArrs API

You want a script or another app to talk to HolyArrs. Everything the web app does goes through a JSON API under `/api/v1`, and you can use the same API.

## Steps

1. Make a key in **Settings > API > API keys**. Give it only the scopes it needs. You see the key once, so copy it.
2. Send it in the `X-Api-Key` header with every call. Never put it in the address.
3. Call `http://your-server:6262/api/v1`. With a URL base of `/holyarrs`, call `http://your-server:6262/holyarrs/api/v1`.

```sh
export HOLYARRS_KEY=...your key...
curl -H "X-Api-Key: $HOLYARRS_KEY" "http://your-server:6262/api/v1/health"
```

Answers are JSON. Errors are `{"error": "plain words"}` with an HTTP status. Secrets never come back in full. You get them masked.

## Scopes

| Scope | Allows |
|---|---|
| health_read | only the health feed, for monitoring tools |
| read | looking: the read routes, including the health feed |
| write | changing things, except admin routes and the controls |
| admin | people, invites, ask limits, API keys, passwords and two-factor, sign-in settings, restore |
| control | part switches, stages, linked apps, restart and shut down |
| all | read, write and admin together, without the controls |

More detail is in [API keys and scopes](../user/help/parts-and-safe-mode.md#api-keys-and-scopes).

## Finding routes

**Settings > API** shows the base address with a copy button, the scopes, example calls, and every route with its scope and permission. The full OpenAPI file is at `/api/v1/openapi.json` when you are signed in.

## Adding a route

Routes live in each module's `routes.go`. Each one is registered with its method, its path and the permission it needs, so a route can't exist without a permission check.

`openapi.json` comes from the routes. A test fails when they drift, so update it in the same change. Once released, API paths and field names stay. Add fields, don't rename them.

## Older apps

Seerr, Overseerr, Bazarr and similar apps don't use this API. They talk to HolyArrs as if it were Sonarr and Radarr, with keys of their own. See [Connect older apps](../user/arrapi/README.md).

## The metadata server

HolyArrs gets names, pictures and ids from the HolyArrs metadata server. This part is for people who run their own server, or who want to know what the app sends.

### Registration

On its first lookup, an install calls `POST /v1/installations` with no body. The server answers `201` with `{"schema_version":1,"token":"hai_..."}`. The app stores the token encrypted and sends it as `Authorization: Bearer <token>` on every lookup after that. It registers once. After a `401` it doesn't register again, so a revoked install stays revoked. Registration has an hourly limit per address group. Over it, the answer is `429 registration_limit` with `Retry-After`.

### Lookups

All lookups are GET.

| Lookup | Route |
|---|---|
| Search | `/v1/search?provider=tmdb&kind=movie&q=Example&page=1` |
| Movie details | `/v1/movies/tmdb/{id}` |
| Show details | `/v1/shows/{tmdb or tvmaze}/{id}` |
| Season details | `/v1/shows/tmdb/{id}/seasons/{n}` (season 0 is specials) |
| Music details | `/v1/music/musicbrainz/{artist, release-group, release or recording}/{id}` |

Unknown or repeated parameters are refused. Ids from one provider are never used for another.

### Answers

A good answer has `schema_version`, `source`, `source_id`, `fetched_at`, `expires_at`, `completeness`, `stale` and `data`. Treat HTML in summaries as plain text.

| Status | Means |
|---|---|
| 400 | bad query, don't retry |
| 401 | bad or revoked token, don't retry |
| 404 | no such record |
| 429 | too many calls, wait for `Retry-After` |
| 503 | busy, wait and retry once |

### Running your own

Point an install at it with `HOLYARRS_METADATA_URL`. The server code isn't in this repo.

## Troubleshooting

- **401 on every call.** The key is wrong or deleted. Make a new one.
- **403.** The key's scope doesn't cover that route. Check the route's scope on **Settings > API**.
- **404 on every call.** Check the URL base. It goes before `/api/v1`.
