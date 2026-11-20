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

## IMDb, Rotten Tomatoes and Metacritic scores

HolyArrs can show IMDb, Rotten Tomatoes and Metacritic scores for the movies and shows in your Library. They come from OMDb and need your own free OMDb key. Without a key nothing changes.

1. Get a free key at [omdbapi.com](https://www.omdbapi.com/apikey.aspx).
2. Open **Settings > Metadata**.
3. Paste it into **OMDb key** under **Your keys**, select **Test**, then save.

The scores show next to the TMDB score on title pages, and when you hover over the score in the Library list.

The free key allows 1,000 requests a day. HolyArrs asks about 20 titles every 30 minutes and each title at most once a week, so a large Library fills in over a few days. If OMDb refuses the key or the day's requests are used up, HolyArrs waits 12 hours, or until you save a new key. Lookups set to **Off** turn this off too.

## Privacy

Your install sends only what it looks up, plus your language and region. Your files, library, users and keys never leave your server. **Settings > Metadata** lists both.

To leave your install out of the install count, turn off **Count this install** in **Settings > This app**.

Every outside address HolyArrs contacts, and how your own TMDB key changes what goes where, is in [Outside addresses](../run/outside-hosts.md).

## If it does not work

**Symptom:** you set `HOLYARRS_METADATA_URL` and nothing loads.
**Fix:** check the address from the HolyArrs machine, then restart HolyArrs.

**Symptom:** artist pages show no top songs.
**Fix:** check `HOLYARRS_DEEZER_URL` is not `off`.

**Symptom:** lookups fail on a network that only lets traffic out through a proxy, even with `HTTPS_PROXY` set.
**Fix:** HolyArrs ignores proxy variables on purpose. Let it reach the addresses in [Outside addresses](../run/outside-hosts.md). Why is in [Proxy variables](../reference/env-vars.md#proxy-variables).

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
| `HOLYARRS_OMDB_URL` | A stand-in address for OMDb. For tests only | Empty |

</details>

## Related

- [Outside addresses](../run/outside-hosts.md)
- [Data packs](data-packs.md)
- [Trailers](trailers.md)
