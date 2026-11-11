---
title: Metadata
description: Where names, dates and posters come from, and what still works offline.
level: Start
---

Names, dates, posters and ids come from the HolyArrs metadata server. There are no keys to set up.

## Check that it works

1. Open **Settings > Metadata**.
2. The **Metadata server** card should show **Connected** and **Answering**.

If it says **Not answering**, select **Check again**.

## When the server is down

Your Library, downloads and everything already saved keep working. Searches and new artwork come back when the server answers.

## Privacy

Your install sends only what it looks up, plus your language and region. Your files, library, users and keys never leave your server. **Settings > Metadata** lists both.

To leave your install out of the install count, turn off **Count this install** in **Settings > This app**.

Every outside address HolyArrs contacts, and how your own TMDB key changes what goes where, is in [Outside addresses](../run/outside-hosts.md).

## If it does not work

**Symptom:** you set `HOLYARRS_METADATA_URL` and nothing loads.
**Fix:** check the address from the HolyArrs machine, then restart HolyArrs.

**Symptom:** artist pages show no top songs.
**Fix:** check `HOLYARRS_DEEZER_URL` is not `off`.

## For experts

<details class="ha-more"><summary>What your install sends</summary>

| Sent | Why |
|---|---|
| An install token | Identifies the install, not a person |
| What you look up: search words, and TMDB and MusicBrainz ids on the page | To answer the lookup |
| Your display language and region | So posters, trailers and where-to-watch match |
| Standard request headers | To say what answer is wanted and whether a stored copy is current |
| Your public IP address, like any web request | Used for an install count by country |

Headers sent: `Accept`, `Cache-Control`, `If-None-Match`, and `X-Fetch: ahead` when a page is loaded ahead of time. With **Count this install** off, requests also carry `X-Country-Count: off` and the server leaves your install out of the count.

Never sent: your files, folder paths or file names; your library, downloads or history; user names, passwords or who asked for what; keys and addresses of your Downloaders, Sources, media servers and lists. Apart from the country count, HolyArrs has no telemetry.

</details>

<details class="ha-more"><summary>Env vars</summary>

| Variable | Does | Default |
|---|---|---|
| `HOLYARRS_METADATA_URL` | Points HolyArrs at another metadata server. Restart after changing | `https://api.holyarrs.com` |
| `HOLYARRS_TMDB_KEY` | Your own TMDB read token. Movie and show lookups go to TMDB directly | Empty |
| `HOLYARRS_DEEZER_URL=off` | Turns off artist top songs | On |

</details>

## Related

- [Outside addresses](../run/outside-hosts.md)
- [Data packs](data-packs.md)
- [Trailers](trailers.md)
