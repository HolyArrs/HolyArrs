---
title: The HolyArrs API
description: Make a scoped key and call the JSON API from a script, or run your own metadata server.
level: Setup
sidebar: { order: 7 }
---

Call the same JSON API the web app uses, under `/api/v1`, from your own scripts.

## Make your first call

1. Open **Settings > API > API keys** and make a key with only the scopes it needs. Copy it now, because it is shown once.
2. Send it in the `X-Api-Key` header on every call. Never put it in the address.
3. Call the base address `http://your-server:6262/api/v1`. Under a URL base of `/holyarrs`, call `http://your-server:6262/holyarrs/api/v1`.

```sh
export HOLYARRS_KEY=...your key...
curl -H "X-Api-Key: $HOLYARRS_KEY" "http://your-server:6262/api/v1/health"
```

You should get JSON back. Errors are `{"error": "plain words"}` with an HTTP status. Secrets come back masked.

## Choices

### Pick a scope

| Scope | Allows |
|---|---|
| `health_read` | Only the health feed, for monitoring tools |
| `read` | The read routes, health feed included |
| `write` | Changes, except admin routes and controls |
| `admin` | People, invites, ask limits, API keys, passwords and two-factor, sign-in settings, restore |
| `control` | Part switches, stages, linked apps, restart and shut down |
| `all` | `read`, `write` and `admin`. Not `control` |

Give a monitoring tool `health_read` and nothing else.

### Find a route

**Settings > API** shows the base address, the scopes, example calls, and every route with its scope and permission. The OpenAPI file is at `/api/v1/openapi.json` when you are signed in. [API routes](api-routes.md) lists every route.

### Talk to HolyArrs as an older app

Seerr, Overseerr, Bazarr and similar apps do not use this API. They talk to HolyArrs as if it were Sonarr and Radarr, with keys of their own. See [Old-app API](../reference/old-app-api.md).

## If it does not work

**Symptom.** 401 on every call.
**Cause.** The key is wrong or deleted.
**Fix.** Make a new key.

**Symptom.** 403 on one route.
**Cause.** The key's scope does not cover that route.
**Fix.** Check the route's scope on **Settings > API**.

**Symptom.** 404 on every call.
**Cause.** The URL base is missing.
**Fix.** Put it before `/api/v1`. See [URL base](../run/url-base.md).

## For experts

<details class="ha-more"><summary>Add a route</summary>

Routes live in each module's `routes.go`. Each one is registered with its method, path and permission, so every route has a permission check.

`openapi.json` comes from the routes. `TestOpenAPIGenerated` fails when they drift, so update it in the same change. A route-table snapshot also fails when a route is added, removed or moved without a snapshot update.

Once released, paths and field names stay. Add fields, never rename them. After a change, run `go run ./app/backend/tools/apidoc` to rewrite [API routes](api-routes.md).

</details>

<details class="ha-more"><summary>Metadata server calls</summary>

HolyArrs gets names, pictures and ids from the HolyArrs metadata server. Point an install at another server with `HOLYARRS_METADATA_URL`. The server code is not in this repo.

**Registration.** On its first lookup, an install calls `POST /v1/installations` with no body. The answer is `201` with `{"schema_version":1,"token":"hai_..."}`. The app stores the token encrypted and sends `Authorization: Bearer <token>` on every lookup. It registers once. After a `401` it does not register again, so a revoked install stays revoked. Over the hourly limit per address group, the answer is `429 registration_limit` with `Retry-After`.

| Lookup (all GET) | Route |
|---|---|
| Search | `/v1/search?provider=tmdb&kind=movie&q=Example&page=1` |
| Movie | `/v1/movies/tmdb/{id}` |
| Show | `/v1/shows/{tmdb or tvmaze}/{id}` |
| Season | `/v1/shows/tmdb/{id}/seasons/{n}` (season 0 is specials) |
| Music | `/v1/music/musicbrainz/{artist, release-group, release or recording}/{id}` |

Unknown or repeated parameters are refused. Ids from one provider are never used for another.

A good answer has `schema_version`, `source`, `source_id`, `fetched_at`, `expires_at`, `completeness`, `stale` and `data`. Treat HTML in summaries as plain text.

| Status | Means |
|---|---|
| 400 | Bad query, do not retry |
| 401 | Bad or revoked token, do not retry |
| 404 | No such record |
| 429 | Too many calls, wait for `Retry-After` |
| 503 | Busy, wait and retry once |

</details>

## Related

- [API routes](api-routes.md)
- [Old-app API](../reference/old-app-api.md)
- [Environment variables](../reference/env-vars.md)
