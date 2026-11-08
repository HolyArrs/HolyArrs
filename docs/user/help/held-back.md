# Features held back in the first release

You read about a feature and can't find it in HolyArrs. It may be held back. These features are built, but they have not been tested end to end with real services yet. So they are off in the first release.

While a feature is held back:

- its pages and menu items are hidden,
- its API routes answer "not found",
- its background jobs don't run,
- any data you already saved for it stays in place.

## Turn a held-back feature on

You can turn one on yourself. You do this at your own risk.

1. Add the switch name to the `HOLYARRS_FLAGS` environment variable. Separate several names with commas.
2. Restart HolyArrs.

In a Compose file:

```yaml
environment:
  HOLYARRS_FLAGS: "subtitle_sites,alert_channels_more"
```

To turn it off again, remove the name and restart.

## The switches

| Switch | Feature | What stays hidden while it is off |
|---|---|---|
| `ai_apps` | Connect AI apps | Settings > AI apps, the `/ai` page and the AI connection routes. |
| `plex_watchlists` | Plex watchlists turn into requests | The Plex watchlist settings in Request rules and My account, and the watchlist job. |
| `row_title_stats` | Explore row title statistics | The row title statistics in Appearance. |
| `trakt_lists` | Trakt key, Trakt lists and Trakt reviews | The Trakt key and the Trakt list kind in Outside lists, and Trakt reviews on title pages. TMDB lists stay on. |
| `alert_channels_more` | Telegram, ntfy, Gotify, Pushover and webhook alerts | Those kinds in Settings > Alerts. Discord and email stay on. Channels you already made are kept. |
| `subtitle_sites` | Subtitle sites and subtitle search | Subtitle sites and the subtitle search actions. Your subtitle language choice stays on. |
| `emby_signin` | Sign in with Emby | The Emby sign-in button and its settings. Emby library refresh still works. |
| `go_back` | Go back to the previous version | The "go back" steps on the `/health` page. |

## Troubleshooting

**I added the switch and nothing changed.** Check the spelling. Names are lowercase with underscores. Then make sure you restarted the container, not just reloaded the page.

**I turned a feature off and my settings are gone.** They are not gone. They are only hidden. Turn the switch on again and they come back.
