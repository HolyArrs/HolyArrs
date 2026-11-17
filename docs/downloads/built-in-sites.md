---
title: Built-in sites
description: "Add Knaben, TorrentsCSV, SubsPlease or IPTorrents as a Source, with no Jackett or Prowlarr in between."
level: Setup
sidebar: { order: 4 }
---

HolyArrs can read 4 torrent sites itself. You do not need Jackett or Prowlarr for them.

Each one becomes an ordinary Source. Priority, use and limits work as on any other Source. See [Sources](sources.md).

## The sites

| Site | For | Needs | Newest list |
|---|---|---|---|
| Knaben | Movies, shows, music | Nothing | Yes |
| TorrentsCSV | Movies, shows, music | Nothing | No, search only |
| SubsPlease | Shows (anime) | Nothing | Yes |
| IPTorrents | Movies, shows, music | Your own cookie or RSS key | Yes |

All 4 hand out torrents, so you need a torrent Downloader. See [Downloaders](downloaders.md).

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

## IPTorrents: cookie or RSS key

HolyArrs never signs in for you. Give it one of these from your own account:

| Option | What to paste | Where to find it |
|---|---|---|
| Cookie | `uid=...; pass=...` | Your browser's cookies for the site, after you sign in |
| RSS key | `u=...; tp=...` | The RSS link the site gives you |

You can paste both in one line. HolyArrs keeps it encrypted.

With the cookie, HolyArrs reads the site's own search pages. With only the RSS key, it uses the site's RSS search.

## If it does not work

| You see | Try |
|---|---|
| IPTorrents says it needs your cookie or RSS key | Paste `uid` and `pass`, or `u` and `tp`, into **Cookie or RSS key** |
| TorrentsCSV never grabs new episodes on its own | Expected. It has no newest list. Searches still work |
| The site answers with something odd | The site may be down or have moved. Try **Test** later, or fix the **Address** |
| Nothing is found for music on SubsPlease | Expected. SubsPlease only has shows |

<details class="ha-more">
<summary>For experts</summary>

Each site starts with polite limits: one call at a time with a gap, and a cap per hour. Knaben, TorrentsCSV and SubsPlease wait 2 seconds between calls and allow 100 searches an hour. IPTorrents waits 3 seconds and allows 60 an hour. Change them under the Source's limits like any other Source.

Searches use the title plus `S01E02` or `S01` for shows, the year for movies, and artist and album for music.

</details>

## Related

- [Sources](sources.md)
- [Downloaders](downloaders.md)
