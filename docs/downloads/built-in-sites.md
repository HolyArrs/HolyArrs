---
title: Built-in sites
description: "Add Knaben, The Pirate Bay, 1337x, YTS, Nyaa and other torrent sites as a Source, with no Jackett or Prowlarr in between."
level: Setup
sidebar: { order: 4 }
---

HolyArrs can read 14 torrent sites itself. You do not need Jackett or Prowlarr for them.

Each one becomes an ordinary Source. Priority, use and limits work as on any other Source. See [Sources](sources.md).

## The sites

| Site | For | Needs | Newest list |
|---|---|---|---|
| Knaben | Movies, shows, music | Nothing | Yes |
| TorrentsCSV | Movies, shows, music | Nothing | No, search only |
| SubsPlease | Shows (anime) | Nothing | Yes |
| IPTorrents | Movies, shows, music | Your own cookie or RSS key | Yes |
| The Pirate Bay | Movies, shows, music | Nothing | Yes |
| YTS | Movies | Nothing | Yes |
| Nyaa | Shows and movies (anime) | Nothing | Yes |
| Tokyo Toshokan | Shows and movies (anime) | Nothing | Yes |
| Bangumi Moe | Shows and movies (anime) | Nothing | Yes |
| 1337x | Movies, shows, music | Usually FlareSolverr | No, search only |
| Torrent9 | Movies, shows, music (French) | Sometimes FlareSolverr | No, search only |
| TorrentDownload | Movies, shows, music | Nothing | No, search only |
| LimeTorrents | Movies, shows, music | Nothing | No, search only |
| BigFANGroup | Movies, shows, music (Russian) | Nothing | Yes |

All of them hand out torrents, so you need a torrent Downloader. See [Downloaders](downloaders.md).

## Add a built-in site

1. Open **Settings > Sources**.
2. Press **Add a Source**.
3. Set **Kind of feed** to **Built-in site**.
4. Pick the **Site**.

   You should see the name, **Address** and limits filled in for you.

5. For IPTorrents only, fill in **Cookie or RSS key**. See below.
6. Press **Save**. HolyArrs tests it right away.

   You should see **Working** under it.

Leave the **Address** as it is. Change it only if the site moved.

After saving, you cannot change the site of a Source. To switch, add a new one.

## How each site behaves

**Knaben** searches by title and sorts by seeders. Its newest list shows the latest uploads.

**TorrentsCSV** only searches. It has no newest list, so turning on **Newest list** for it finds nothing.

**SubsPlease** searches by the show's title alone, because it numbers episodes its own way. Its newest list shows the latest episodes.

**IPTorrents** is a private site. It searches with your own account and sorts by seeders.

**The Pirate Bay** is read through its own API (apibay.org). A movie is searched by its IMDb id first, then by title and year.

**YTS** only has movies. It is searched by IMDb id, or by title without the year. Release names are made from what the site says about each copy, such as `The Descent (2005) 1080p BRRip 5.1 x264 -YTS`.

**Nyaa**, **Tokyo Toshokan** and **Bangumi Moe** are anime sites. Episodes in absolute order are also searched by title and running number. Tokyo Toshokan gives no seeder counts.

**1337x** sits behind Cloudflare's bot check most of the time. Set a **FlareSolverr address** on it (see below). Its results link to the site's page for each torrent; HolyArrs reads the magnet from that page when it grabs the Release.

**Torrent9** is a French site. A season is searched as `saison 1`. Like 1337x, the magnet is read from the torrent's page when grabbed.

**TorrentDownload** and **LimeTorrents** only search. LimeTorrents is read through its RSS search; a season alone is searched by the show's name.

**BigFANGroup** is a Russian site. It is searched by name only, and its Russian season and episode words become `S01E02` style names.

## IPTorrents: cookie or RSS key

HolyArrs never signs in for you. Give it one of these from your own account:

| Option | What to paste | Where to find it |
|---|---|---|
| Cookie | `uid=...; pass=...` | Your browser's cookies for the site, after you sign in |
| RSS key | `u=...; tp=...` | The RSS link the site gives you |

You can paste both in one line. HolyArrs keeps it encrypted.

With the cookie, HolyArrs reads the site's own search pages. With only the RSS key, it uses the site's RSS search.

## Sites behind a bot check

Some sites, 1337x most of all, show Cloudflare's bot check instead of results. The Source then shows **Blocked**.

1. Run [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) next to HolyArrs, for example as the `ghcr.io/flaresolverr/flaresolverr` container.
2. Open the Source, then **Proxy**.
3. Put its address in **FlareSolverr address**, such as `http://flaresolverr:8191`.
4. Press **Save**, then **Test**.

   You should see **Working**.

HolyArrs asks FlareSolverr only when the site shows its check. Calls in between go straight to the site with the cookie FlareSolverr got. A Source's proxy is also handed to FlareSolverr, so pages it opens come from the same route.

FlareSolverr opens whatever page it is asked for in a real browser. Run it where it cannot reach the rest of your home network, such as inside your VPN container's network, next to your torrent app.

## Through a VPN

Sites see the address HolyArrs calls them from. To send that through a VPN, use one of these:

- Set an HTTP or SOCKS5 **Proxy** on each Source. A small proxy container inside the VPN container's network works.
- Set the app proxy under **Settings > This app > Proxy**. Every outside call goes through it, Sources included, unless a Source has its own proxy.
- Run HolyArrs itself in the VPN container's network (`network_mode: service:vpn` in Docker Compose). Every call then leaves through the VPN.

## Moving from Prowlarr

When you import from Prowlarr, sites listed above become built-in site Sources, read directly instead of through Prowlarr. A FlareSolverr proxy a site used in Prowlarr is set as its **FlareSolverr address**. Check the address works from HolyArrs, since Prowlarr may have reached it as `127.0.0.1`. Other Prowlarr sites are listed as not moved; link Prowlarr for them.

## If it does not work

| You see | Try |
|---|---|
| IPTorrents says it needs your cookie or RSS key | Paste `uid` and `pass`, or `u` and `tp`, into **Cookie or RSS key** |
| TorrentsCSV never grabs new episodes on its own | Expected. It has no newest list. Searches still work |
| The site answers with something odd | The site may be down or have moved. Try **Test** later, or fix the **Address** |
| Nothing is found for music on SubsPlease | Expected. SubsPlease only has shows |
| 1337x or Torrent9 shows **Blocked** | Set a **FlareSolverr address** on it. See [Sites behind a bot check](#sites-behind-a-bot-check) |
| A site cannot be reached at all | Some internet providers block these sites by name. Use a VPN route, see [Through a VPN](#through-a-vpn) |
| A site moved to a new address | Change the **Address** of the Source to the new one |

<details class="ha-more">
<summary>For experts</summary>

Each site starts with polite limits: one call at a time with a gap, and a cap per hour. Sites with an API or a feed (Knaben, TorrentsCSV, SubsPlease, The Pirate Bay, YTS, Nyaa, Tokyo Toshokan, Bangumi Moe) wait 2 seconds between calls and allow 100 searches an hour. Sites read from their pages (IPTorrents, 1337x, Torrent9, TorrentDownload, LimeTorrents, BigFANGroup) wait 3 seconds and allow 60 an hour. Change them under the Source's limits like any other Source.

Magnets HolyArrs builds from a bare info hash (The Pirate Bay, YTS, TorrentDownload, LimeTorrents) carry a few public trackers, so the torrent app finds peers sooner.

The site readers follow what Prowlarr's definitions for these sites read. No definition file is copied into HolyArrs.

Searches use the title plus `S01E02` or `S01` for shows, the year for movies, and artist and album for music.

</details>

## Related

- [Sources](sources.md)
- [Downloaders](downloaders.md)
