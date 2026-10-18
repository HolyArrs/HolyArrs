# Apple direction: what the web app needs from the backend

For the backend owner. The web app already calls these and hides what is missing, so nothing breaks before they exist. No Go code changed in this milestone.

## 1. `GET /api/v1/discover`

One endpoint for every Explore row that needs the metadata provider (TMDB). Client: `web/app/src/lib/explore.ts` (`fetchProviderRow`, `discoverPath`). A **404 or 501** answer means "not built": the row is hidden silently. Any other error also hides the row (the page catches it).

### Query

| Param | Values | Notes |
|---|---|---|
| `row` | `trending` \| `popular` \| `upcoming` \| `similar` \| `genre` | required |
| `id` | Library title id | required for `similar` |
| `g` | genre name, as sent in `genres` | required for `genre` |
| `kind` | `movie` \| `show` | optional; both when absent |

Rows asked for today: `trending`, `popular`, `upcoming`, and `similar&id=<newest movie or show>`. `genre` is wired but not yet requested.

Unknown `row`, missing `id`/`g`: `400 {"error": "..."}`.

### Answer `200`

```json
{
  "row": "trending",
  "title": "Trending this week",
  "items": [
    {
      "tmdb_id": "693134",
      "kind": "movie",
      "name": "Dune: Part Two",
      "year": 2024,
      "poster": "https://image.tmdb.org/t/p/w342/...jpg",
      "backdrop": "https://image.tmdb.org/t/p/w1280/...jpg",
      "overview": "…",
      "genres": ["Science Fiction", "Adventure"],
      "in_library": true,
      "title_id": 43,
      "ask": { "id": 12, "state": "asked", "state_label": "Waiting for approval", "mine": false, "people": 2 }
    }
  ]
}
```

- `title` optional; the app has its own fallback words.
- `in_library` + `title_id`: matched by TMDB id against the Library (and by TVDB/TMDB show id for shows).
- `ask`: the open Wishlist ask for this item, or `null`. Same shape as the `ask` on `PersonMovie` today.
- At most 20 items. Never-add items are left out.
- The card's + uses `tmdb:<tmdb_id>` with the existing `POST /asks` (members) or the Search add flow (`manage_library`).

### Permissions

- Signed in; any role. No new permission.
- Filtered by Parts: no `show` items when the TV part is off, no `movie` items when movies are off. `kind=show` with shows off: `200` with empty `items`.
- Members see `ask.mine` for themselves only; `ask.people` is a count, never names.
- Kid preset (below): items above the person's rating limit are left out.

### Caching and limits

- Server caches each `(row, id, g, kind)` answer: `trending` 6 h, `popular`/`upcoming` 12 h, `similar`/`genre` 24 h. `in_library` and `ask` are joined fresh on every request (cheap, local), not cached.
- `Cache-Control: private, max-age=300`.
- Shares the existing TMDB rate limiter. When TMDB is not set up: `501`. When TMDB is rate limited and nothing is cached: `503` with `Retry-After`.
- Per person: 30 requests a minute; above that `429` with `Retry-After`.

## 2. Genres on Library titles

`GET /titles` and `GET /titles/{id}` do not send genres. Explore has "<Genre> in your library" and uses genre overlap in "Because you have <title>"; both read an optional `genres: string[]` on `TitleView` (`genresOf` in `lib/explore.ts`) and stay hidden until it is sent. Ask: add `genres` (provider genre names, English) to the title view.

Today "Because you have" and "From <person>" only use people from `GET /titles/{id}/people` for the 6 newest movies (one TMDB call each). A `people` list of names (top 10 cast + directors) on the title view would remove those calls.

## 3. Kid preset: age-rating limit

Role presets are display-only (spec §7): Admin, Manager, Member, Kid. Kid = `ask` plus a rating limit, which needs:

- Per person: `max_rating` (`G`, `PG`, `PG-13`, `R`, `NC-17`; for shows `TV-Y`…`TV-MA`), with a country (`US` default).
- Title view: `rating` (certification for that country) so cards and title pages can show it.
- Enforcement on the server, not the app: titles above the limit are left out of `/titles`, `/calendar`, `/discover`, search and `POST /asks` (`403` with a plain message).
- Until this exists the invite screen shows Kid as "coming soon".

## 4. Newest titles on the Library summary

Home must not read the whole Library (`GET /titles`); the e2e suite checks that it only reads `GET /library-summary`, which is bounded. "Recently added" (and the posters on "Coming soon") need the newest titles, so Home reads an optional `recent: TitleView[]` on `GET /library-summary` and hides the shelf while it is absent. Ask: add `recent`, the 20 newest titles by date added (same view as `GET /titles`, parts and the Kid limit applied). Members may read it (the route is already open to every signed-in person).

## 5. Settings status, and what Settings could not place

Settings home shows a status line for each group: "All set", "2 need attention" or "Not set up". It also shows a "Needs attention" strip with Fix links. Today the page builds all of this from routes that already exist, with one call to each:
- `GET /health/checks`
- `GET /health/states`
- `GET /library-folders`
- `GET /sources`
- `GET /downloaders`
- `GET /backups`
- `GET /app/update`
- `GET /settings/tmdb-token`, only when the session has a recent password

The rules are in `web/app/src/lib/settings-status.ts`, which has unit tests.

What is asked for:

- **One status route.** `GET /api/v1/settings/status` with the settings permission. It answers `{ items: [{ group, page, text, severity }], unset: { group: [page] } }`, using the same rules as `settings-status.ts`. That turns eight calls into one, and the rules then live in one place. It is small, so a 30-second cache is fine.
- **TMDB key present.** `GET /settings/tmdb-token` needs a recent password, so without one the home cannot tell whether a key is missing. Please add `tmdb_key_set: boolean` to that status route, or to `GET /health`, for anyone with the settings permission. Never send the key itself.
- **Status for the other groups.**
  - Media servers and Alerts keep no last test result, so Linked apps can only show a failure through `/health/states`.
  - Sign-in and security, and Wishlist and people, have no health signal at all, so their cards show no status line.
  - Ask: store the last test result for media servers and Alerts (`last_test_at`, `last_test_ok`, `last_test_text`), the same way Sources and Downloaders already do.
- **Editing Pick rules.** The app can only list them (`GET /pick-rules`). The new page *Getting copies › Pick rules* lists them and says where each is chosen. Editing needs `POST`, `PUT` and `DELETE /api/v1/pick-rules`, with the rule body the server already keeps.
- **Reset in Danger zone.** HolyArrs has no in-app reset; the owner reset is the `holyarrs reset-owner` command. If one is wanted, it needs a route that asks a second time, like `POST /app/restart` does. Until then Danger zone holds Restart, Restart in safe mode and Shut down.
- **Appearance: density and reduced motion.**
  - The app has no density setting.
  - Reduced motion follows the operating system only.
  - If either is wanted per person, keep it with the person's other choices (`GET` and `PUT /api/v1/my/preferences`, `{ density: 'comfortable'|'compact', motion: 'system'|'reduce' }`). The same store could also keep which Advanced disclosures a person left open. Today that lives in the browser's own storage, per person.
