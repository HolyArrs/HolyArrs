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

## Trakt

Trakt needs a Trakt app of your own. It is free.

1. On trakt.tv, open your API apps ([trakt.tv/oauth/applications](https://trakt.tv/oauth/applications)) and make a new app. Any name works. For the redirect address use `urn:ietf:wg:oauth:2.0:oob`. Trakt asks for one but sign-in does not use it. A new app has only a **Client ID**; that is all HolyArrs needs.
2. In HolyArrs, open **Settings > Outside lists** and paste the Client ID under **Trakt key**, then save.

With the key alone, HolyArrs reads public Trakt lists and watchlists, and each Library title's Trakt rating.

### Sign in with Trakt

Signing in lets HolyArrs read your private lists and watchlist, the way Sonarr and Radarr do, and adds what you watched on Trakt to the watch history. Only the owner can sign in.

1. Under **Trakt sign-in**, select **Sign in with Trakt**. HolyArrs shows a code.
2. On any device, open [trakt.tv/activate](https://trakt.tv/activate), type the code and approve HolyArrs.
3. Keep the HolyArrs page open. It sees the approval by itself and shows the Trakt username you signed in with.

The code lasts about 10 minutes. If it runs out or you turn it down, select **Sign in with Trakt** again for a new one.

HolyArrs renews the sign-in by itself. If Trakt ends it (you removed the app on trakt.tv, for example), HolyArrs says so on the page and on each Trakt list it could not read; sign in again. Until then only public lists are read. **Sign out of Trakt** ends it here and at Trakt.

Only the lists you saved last are read with your sign-in, including the ones an import you ran brought over. A list someone else adds, or changes after you, is read without it, so it sees only public lists; open it and save it yourself to use your sign-in.

An older Trakt app that has a client secret still works: paste the secret in **Client secret**. New apps have none; leave it empty.

### Watch history from Trakt

With **Read watch history** on (**Settings > Media servers**), HolyArrs also reads what you watched on Trakt, every 30 minutes, and matches it to your Library. It shows in your Watched marks, in **Watched by anyone** and in the watch history summary as **Trakt**. No media server is needed. Signing out or turning watch history off forgets it.

### Trakt ratings

With the Trakt key saved, HolyArrs reads each Library movie's and show's Trakt rating by its IMDb id: up to 50 titles every 30 minutes, each at most once a week. The rating shows as a percent next to the other scores on title pages and on hover over the score in the Library list. If Trakt refuses the key, HolyArrs waits 12 hours, or until you save a new key.

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
