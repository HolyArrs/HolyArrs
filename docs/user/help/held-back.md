---
title: Held back in this release
description: Which features are off by default, what each one does, and how to turn one on at your own risk.
level: Setup
---

Some features are built but not yet tested end to end with real services, so they are off in this release.

While a feature is held back, its pages and menu items are hidden, its API routes answer "not found" and its jobs do not run. Data you saved for it stays.

## What is held back

| Switch | Feature |
|---|---|
| `ai_apps` | Connect AI apps: **Settings > AI apps** and the `/ai` page |
| `plex_watchlists` | Plex watchlists turn into requests |
| `row_title_stats` | Explore row title statistics in Appearance |
| `trakt_lists` | Trakt key, Trakt lists and Trakt reviews. TMDB lists stay on |
| `alert_channels_more` | Telegram, ntfy, Gotify, Pushover and webhook Alerts. Discord and email stay on |
| `subtitle_sites` | Subtitle sites and subtitle search. Your subtitle language stays on |
| `emby_signin` | Sign in with Emby. Emby library refresh still works |
| `go_back` | The **Go back** panel on the Status page |

## Turn one on

1. Add the switch name to `HOLYARRS_FLAGS` in your compose file, comma-separated: `HOLYARRS_FLAGS: "subtitle_sites,alert_channels_more"`.
2. Restart the container. A page refresh is not enough.

You should see the hidden page or menu item. Remove the name and restart to turn it off again.

For the full steps, see [Updates](../../run/updates.md#held-back-features).

## Related

- [Environment variables](../reference/env-vars.md)
- [Troubleshooting](troubleshooting.md)
