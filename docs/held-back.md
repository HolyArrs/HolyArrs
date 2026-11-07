# Held back from the first release

These features are built but not yet proven end to end, so they are switched off in the first release (Luis, 2026-11-07). The code and any saved data stay; each comes back as its own roadmap item once it is fully tested.

A held-back feature is off unless its name is listed in `HOLYARRS_FLAGS` (comma separated). Off means its routes are not registered (the server answers not found), its jobs do not run, and the web app hides every place that shows it. The switches live in `internal/feature`; the web app gets the off list with `/auth/me` (`held`) and asks `session.held(name)`.

| Switch | Feature | Hidden | What is missing before it comes back |
|---|---|---|---|
| `ai_apps` | AI apps: the AI connector (MCP), AI keys, AI sign-in for apps, AI app guides | Settings > AI apps, `/api/v1/mcp`, `/mcp`, `/api/v1/settings/ai`, the AI sign-in routes (`/.well-known/oauth-*`, `/oauth/*`, `/api/v1/auth/ai-connect`) | Tested with real AI apps (desktop and cloud), key handling reviewed, sign-in flow security review |
| `plex_watchlists` | Plex watchlists become requests | Request rules > Plex watchlists, My account > Plex watchlist, `/api/v1/my/plex-watchlist*`, the watchlist job | Tested with real Plex accounts and watchlists over days; limits and duplicates checked |
| `row_title_stats` | Explore row titles statistics (owner table) | Appearance > Explore row titles, `/api/v1/explore/title-stats`, the counting | Decide whether owners need it at all; it measures which row-title style gets opens and requests |
| `trakt_lists` | Trakt: key, VIP token, Trakt lists, Trakt reviews | Outside lists > Trakt key and the Trakt list kind, `/api/v1/settings/trakt-*`, Trakt reviews on title pages | Tested with a real Trakt app and lists; TMDB lists stay on |
| `alert_channels_more` | Alert channels not yet proven: Telegram, ntfy, Gotify, Pushover, webhook | Those kinds in Settings > Alerts; the server refuses to add them. Existing channels of those kinds are kept | Each channel sent and received end to end; Discord and email stay on |
| `subtitle_sites` | Subtitle sites and subtitle searching | Subtitles > Subtitle sites, title-page Look for subtitles, Choose, Sync fix, Turn down, the subtitle search routes and the after-import search | A real site searched and a subtitle downloaded and placed; the language choice stays on |
| `emby_signin` | Sign in with Emby | Sign-in > Emby, the Emby button on the sign-in page; Emby sign-in is refused | Tested with a real Emby server; Emby library refresh (media servers) is not affected |
| `go_back` | Go back to the previous version | Status > Updates > Go back to | Proven after many real updates; the command-line restore stays for support |

## How to work on one

1. Add its switch to `HOLYARRS_FLAGS` on the lab or preview server and restart.
2. The end-to-end server (`web/app/e2e/server.mjs`) and the Go package tests switch all of them on, so their tests keep running.
3. When it is proven, remove its switch checks and its row here, and close its roadmap item.
