---
title: Outside addresses
description: Every outside address HolyArrs contacts on its own, what for, and how to turn each off or point it elsewhere.
level: Expert
sidebar: { order: 10 }
---

This page lists every outside address HolyArrs contacts on its own. Use it to set up a firewall that blocks outgoing traffic by default.

Your own services are not listed: Sources, Downloaders, media servers, alert apps, subtitle sites, single sign-on providers and old apps you import from. HolyArrs contacts those only at the address you set.

Every address below uses HTTPS on port 443. HolyArrs also needs DNS.

HolyArrs connects to these directly. It ignores `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY`. See [Proxy variables](../reference/env-vars.md#proxy-variables).

## The metadata server

| Address | `api.holyarrs.com` |
|---|---|
| What for | Title, show and music lookups, charts, trailer lists, ratings, Wikipedia details, anime episode maps, extra artwork, and TMDB lookups unless you set your own TMDB key |
| When | On first start (to get an install token), then whenever a page or job looks something up |
| Turn off | Pick **Your own keys** or **Off** under **Where details come from** in **Settings > Metadata** (or set `HOLYARRS_METADATA_MODE`). Nothing is sent to it then. See [By choice](#by-choice) |
| Point elsewhere | `HOLYARRS_METADATA_URL`. Restart after changing |

`holyarrs-api.coderluii.dev` is the server's old address. HolyArrs still loads pictures saved under it, but only while `HOLYARRS_METADATA_URL` is unset. It answers until version 2.

What the install sends is listed in [Metadata](../library/metadata.md#for-experts). To leave your install out of the install count, turn off **Count this install** in **Settings > This app**.

## Other addresses

| Address | What for | When | Turn off or change |
|---|---|---|---|
| `image.tmdb.org` | Posters and backdrops | When a picture is first saved, and to keep saved ones fresh | Always on. If it cannot be reached, the picture comes through the metadata server |
| `api.themoviedb.org` | Movie, show, people and Explore lookups, and TMDB lists | Only with `HOLYARRS_TMDB_KEY` set | Unset the key to use the metadata server instead |
| `www.youtube.com` | Reads a trailer's video address. Downloads nothing, never signs in | Trailer lookup rounds, every 5 min | Turn off **Play trailers by themselves** in **Settings > Appearance** |
| `*.googlevideo.com` | Passes a trailer stream through to your browser, without storing it | Only when the browser cannot play the stream itself, or needs it for sound | Same switch as above |
| `api.github.com` | Update check: the newest HolyArrs release | Once a day by default | Turn off **Check for a newer HolyArrs**, or set your own **Release feed**, in **Settings > Health checks**. See [Updates](updates.md) |
| `api.github.com`, `raw.githubusercontent.com` | TRaSH Guides profiles | Only when you pick or sync a TRaSH Guides profile | Do not use [TRaSH Guides scores](../library/trash-guides.md) |
| `plex.tv`, `app.plex.tv` | Sign in with Plex | Only when someone signs in with Plex | Do not use Plex sign-in |
| `discover.provider.plex.tv` | Plex watchlists | Only when [Plex watchlists](../requests/settings.md#plex-watchlists) are on | Turn Plex watchlists off |
| `plex.tv`, `community.plex.tv` | Plex friends' watchlists, read with the owner's Plex link | Only when Plex watchlists and **Include Plex friends** are both on | Turn **Include Plex friends** off |
| `api.trakt.tv` | Trakt lists and Trakt reviews | Only with the `trakt_lists` switch on and a Trakt key saved | Leave the switch off. See [Held-back features](updates.md#held-back-features) |

With the HolyArrs server, music lookups (MusicBrainz, Deezer, Wikipedia) and TVmaze go through the metadata server; your install does not contact them itself. TheTVDB is contacted only with your own TheTVDB key and PIN; otherwise TVDB ids come from TMDB and TVmaze.

When someone opens a show's season in a language other than English, the season lookup names that language (for example `language=es-ES`), so the metadata server, or TMDB with your own key, sees which app languages your people use. Title pages and Explore cards send no language.

## By choice

**Settings > Metadata** shows the exact list for the choice in force, under **Addresses for your firewall**.

| Where details come from | Addresses lookups reach |
|---|---|
| HolyArrs server | `api.holyarrs.com`, `image.tmdb.org`, `www.youtube.com`, plus `api4.thetvdb.com` and `artworks.thetvdb.com` with a TheTVDB key |
| Your own keys | Never the metadata server. `api.themoviedb.org` and `image.tmdb.org` (with a TMDB token), `api.tvmaze.com`, `static.tvmaze.com`, `musicbrainz.org`, `api.deezer.com`, `en.wikipedia.org`, `www.youtube.com`; `api.mdblist.com` and `webservice.fanart.tv`/`assets.fanart.tv` with those keys; TheTVDB with its key |
| Off | None. Pictures and details already saved keep showing |

Music charts, new music rows, streaming Top 10s, Wikipedia facts and anime maps come only from the metadata server, so they are off with your own keys.

## Your own TMDB key and the metadata server

Set `HOLYARRS_TMDB_KEY` to your own TMDB read access token. The log then says `metadata: TMDB directly with the key from the environment`.

| Goes straight to TMDB | Still goes through the metadata server |
|---|---|
| Movie and show details, search, people and Explore lists | TVmaze show details and air times |
| TMDB lists you follow | All music lookups, charts and artist pictures |
| | Trailer lists, ratings, Wikipedia details, anime maps and extra artwork |
| | TMDB pictures when `image.tmdb.org` cannot be reached |

So with a TMDB key set, the install still registers with the metadata server and still needs it. There is no fallback between the two: with a key set, TMDB lookups never go to the server, and without one they never go to TMDB.

`HOLYARRS_METADATA_URL` moves everything in the right-hand column to another server. The other server must answer the same way as `api.holyarrs.com`. A server on your local network also needs `HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL=true`.

`HOLYARRS_METADATA_SERVICE_TOKEN_FILE` reads the install token from a file instead of the vault.

## Pictures in your browser

Your browser, not the HolyArrs server, may load pictures and video from `image.tmdb.org`, `i.ytimg.com`, `upload.wikimedia.org` and `static.tvmaze.com`, play trailers from `*.googlevideo.com`, and show the privacy-enhanced player from `www.youtube-nocookie.com`. A firewall in front of the server does not affect these.

## Related

- [Metadata](../library/metadata.md)
- [Environment variables](../reference/env-vars.md)
- [Updates](updates.md)
- [Trailers](../library/trailers.md)
