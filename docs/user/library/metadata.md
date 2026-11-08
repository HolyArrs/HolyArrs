---
title: Metadata
description: Where names, dates, posters and ids come from, what your install sends, and what still works offline.
level: Start
---

Get names, dates, posters and ids for your titles with no keys to set up.

HolyArrs fetches them from the HolyArrs metadata server. A new install works without typing anything.

## Check that it works

1. Open **Settings > Metadata**.
2. Look at the **Metadata server** card. You should see that the server answers.

On its first lookup, your install registers once and gets its own credential. HolyArrs keeps it encrypted and never shows it.

## When the server is down

Home, Search and Explore say so. Your Library, downloads and everything already saved keep working. Searches and new artwork come back when the server answers. HolyArrs checks at most once a minute.

## What your install sends

| Sent | Why |
|---|---|
| An install token | Identifies the install, not a person |
| What you look up: search words, and TMDB and MusicBrainz ids on the page | To answer the lookup |
| Your display language and region | So posters, trailers and where-to-watch match |
| Standard request headers | To say what answer is wanted and whether a stored copy is current |
| Your public IP address, like any web request | Used for an install count by country |

Never sent: your files, folder paths or file names; your library, downloads or history; user names, passwords or who asked for what; keys and addresses of your Downloaders, Sources, media servers and lists; crash reports. Apart from the country count, HolyArrs has no telemetry.

**Settings > Metadata** shows a short version in the **What we send** card.

### Leave your install out of the count

1. Open **Settings > This app**.
2. On the **This server** card, turn off **Count this install**.

## For experts

<details class="ha-more"><summary>Where each kind of data comes from</summary>

| Data | Source |
|---|---|
| Movie and show searches, details, seasons, Explore rows, people, age ratings, TMDB lists | Metadata server. Show details can also come from TVMaze |
| Posters and backdrops | TMDB's picture host, in TMDB's sizes. The metadata server only when TMDB is unreachable |
| Music details | MusicBrainz. Rate and contact settings under **Settings > Music defaults** |
| Album covers, artist photos, backgrounds, logos | Metadata server, through your install's artwork cache. HolyArrs never asks a picture site itself |
| Artist top songs and explicit marks | Deezer's public API, no key |

</details>

<details class="ha-more"><summary>Env vars and headers</summary>

| Variable | Does | Default |
|---|---|---|
| `HOLYARRS_METADATA_URL` | Points HolyArrs at another metadata server. Restart after changing | `https://holyarrs-api.coderluii.dev` |
| `HOLYARRS_TMDB_KEY` | Your own TMDB read token. Movie and show lookups go to TMDB directly | Empty |
| `HOLYARRS_DEEZER_URL=off` | Turns off artist top songs | On |

Headers sent: `Accept`, `Cache-Control`, `If-None-Match`, and `X-Fetch: ahead` when a page is loaded ahead of time. With **Count this install** off, requests carry `X-Country-Count: off`.

</details>

<details class="ha-more"><summary>Trakt lists (held back)</summary>

Trakt lists stay off until the `trakt_lists` switch is on. See [Held back](../help/held-back.md). Then open **Settings > Outside lists**, find **Trakt key**, and paste the Client ID of a Trakt app. After saving you see a masked value.

Keys from other apps can come in through [Import secrets](../getting-started/import-secrets.md), which stores them in the encrypted vault.

</details>

<details class="ha-more"><summary>If it does not work</summary>

**Symptom:** You set `HOLYARRS_METADATA_URL` and nothing loads.
**Fix:** Check the address from the HolyArrs machine, then restart HolyArrs.

**Symptom:** Artist pages show no top songs.
**Fix:** Check `HOLYARRS_DEEZER_URL` is not `off`.

**Symptom:** You turned off **Count this install** and want to check it.
**Fix:** Reload **Settings > This app**. The switch is saved for the whole install.

</details>

## Related

- [Data packs](data-packs.md)
- [Trailers](../metadata/trailers.md)
