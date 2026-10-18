# Apple direction: what the web app needs from the backend

For the backend owner. The web app already calls these and hides what is missing, so nothing breaks before they exist. No Go code changed in this milestone.

## 1. `GET /api/v1/discover`

One endpoint for every Explore row that needs the metadata provider (TMDB). Client: `web/app/src/lib/explore.ts` (`fetchDiscoverRow`, `discoverPath`). A **404 or 501** answer means "not built": the row is hidden silently. Any other error also hides the row (the page catches it).

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
