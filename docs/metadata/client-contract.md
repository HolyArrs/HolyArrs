# Optional metadata client contract

This describes the standalone build-7 service, not an installed HolyArrs integration. The default app continues to use its direct providers. The service uses its own JSON API; changing a MusicBrainz base URL to this server will not work.

## Requests

All metadata routes accept GET only and require `Authorization: Bearer <installation credential>`. Obtain a credential with the open registration route below, or a separate random one through the private operator. Store it through the application's existing secret-file/vault mechanism, never in a URL, browser-visible setting response, log or export. Revocation is checked even for cached answers. The server exposes no administration routes.

## Registration

When the operator sets `open_registration`, `POST /v1/installations` (no body, no credential) returns 201 `{"schema_version":1,"token":"hai_..."}`: a new random installation credential, stored by hash only. It passes the same client-address and admission gates as lookups, then a total budget (`register_global_per_hour`, default 600) and a per-address-group budget (`register_per_hour`, default 3; IPv6 grouped by /64); over either gives 429 `registration_limit` with Retry-After. Self-registered credentials are kept apart from operator-issued ones, reach only the lookup routes (search and details; never `/metrics` or `/v1/stats`, which need the stats token), expire after `registered_idle_days` without use (default 90, renewed on use), are capped at `registered_max` active (default 100000, least recently used evicted) and share one request limit per source address group. A client registers once, keeps the credential, and never re-registers after a 401 (that would undo a revocation). Without `open_registration` the route answers 405 like any other non-GET request.

| Operation | Route | Supported providers |
|---|---|---|
| Search | `/v1/search?provider=tmdb&kind=movie&q=Example&page=1` | tmdb/movie, tmdb/show, tvmaze/show, musicbrainz/artist, release-group, release, recording |
| Movie details | `/v1/movies/tmdb/550` | tmdb |
| Show details | `/v1/shows/tmdb/1396` | tmdb or tvmaze |
| Season details | `/v1/shows/tmdb/1396/seasons/1` | tmdb only; season 0 represents specials |
| Music details | `/v1/music/musicbrainz/artist/<uuid>` | artist, release-group, release, recording |

Search defaults to page 1, limit 20. MusicBrainz accepts limit 1–100 and pages 1–100. TMDB retains its native page size; its limit must remain 20. TVmaze search accepts only page 1. Language is TMDB-only. Region is movie-search-only. Detail routes reject page, limit and region. Duplicate/unknown parameters are rejected. Provider identities are separate; the client must not substitute one provider's numeric ID for another's.

## Replies and safe interpretation

Successful replies contain schema_version 1, source, source_id, fetched_at, expires_at, completeness, stale and data. Search data contains items and pagination; detail data contains source, source_id, kind, title, summary and source_data. The source_data object preserves allowed upstream fields; it is not a complete normalized HolyArrs movie/show/music model. Dates, optional values, HTML summaries and ratings need the app's existing validation and display rules. Missing values stay unknown. Never render source HTML as trusted markup.

The X-Metadata-Cache header describes hit, miss, stale or bypass. Responses use Cache-Control: no-store; do not add shared edge caching around installation authorization. Completeness means the requested adapter operation parsed successfully. It does not assert a whole artist catalog, all episodes, alternate numbering or every desired optional field exists.

Errors contain `error.code` and `error.retry_after_seconds`. Invalid queries return 400; bad/revoked credentials return 401; missing upstream records may return 404; installation quota returns 429; unavailable providers, storage or queue capacity return 503. Honor Retry-After with bounded jitter. Do not blindly retry 400 or 401, and do not multiply provider retries with an unbounded client retry loop. Keep the caller's deadline bounded around the server's 15-second lookup deadline.

## Smallest useful integration

Add an explicitly optional service URL and separately stored installation credential. Use a distinct adapter for this API, with an explicit allowlist of the routes above. Keep direct-provider behavior the default. A music-base-URL override is not this adapter. If a requested operation is unsupported, retain the current direct-provider path according to the chosen app policy; do not label the service's partial response as a complete catalog.

For an isolated integration test, the deployment owner can start the existing private synthetic service, issue a disposable credential and provide a temporary SSH-forwarded endpoint. No public hostname or Oracle inbound port is necessary. Coordinate port allocation with the app owner before creating the forward. Share only the private credential-file reference with that owner. Stop the private API/fixture and revoke the disposable credential after the test.

Acceptance requires the actual app to make authenticated search and detail requests through this adapter, save provider identity correctly, respect page/freshness/error behavior, reject revoked access, and preserve the direct-provider default. Test movie, show, season and the supported music entities independently; verify no provider credential is sent to the metadata service and no installation credential is forwarded upstream. Include invalid URL/SSRF and untrusted response handling, unavailable service, 429/503 retries, ownership and library-state preservation. Synthetic standalone searches do not close this acceptance.

## Public enablement gates

Before running the opt-in live check on Windows, separately verify the credential file and every ancestor with a read-only ACL check. Only the intended operator, SYSTEM and Administrators may modify or replace that path. Go's file mode check does not enforce Windows ACL privacy. The live check's exclusive report creation protects existing files but does not replace this operator prerequisite.

The current service remains private and stopped. A public endpoint requires account-level Oracle entitlement/network checks, provider hosted-use/attribution/retention compliance, enrollment policy, independent recoverability and dedicated Tunnel/security validation. A historical October 15 run completed nine paced Windows adapter calls and recorded sanitized field-name/count evidence under the earlier check. It predates the current field assertions and does not prove that they pass. Neither that run nor this contract proves public-provider permission, Oracle/Cloudflare latency, full catalog coverage or 1,000-installation capacity.

Official public address (live since 2026-10-19): `https://holyarrs-api.coderluii.dev`.

## Artwork and outside ratings (optional sources)

Both are off unless the server's `.env` holds their key. Only the names go here; never put the values in a file that is tracked.

- `HOLYARRS_FANART_KEY` turns on `GET /v1/art/{movie|tv|music}/{id}`. Movies and shows take a TMDB id (the server maps shows to TVDB), music a MusicBrainz artist id. The answer has `logo`, `clearart`, `banner`, `thumb`, `backgrounds[]` and `thumbs[]` (best first: English, then no language, then most liked). It is cached for 7 days.
- `HOLYARRS_MDBLIST_KEY` turns on `GET /v1/ratings/{movie|tv}/{tmdb_id}`. The answer has `scores[]` of `{source, value, max, votes}`, where source is one of `imdb`, `rt_critics`, `rt_audience`, `metacritic`, `letterboxd` or `trakt`. It is cached for 48 hours. The server budget allows 25 calls an hour (600 a day, under the free tier's 1000). When the budget is spent, the server serves cached or stale answers, or a 503.

When a key is missing, the route answers `404 source_disabled` and `/v1/stats` shows `extras.<fanart|mdblist>.state = "disabled_no_key"`. Deploy: add the key to the server's `.env` and recreate the container.
