---
title: Old-app API clients
description: Connect Seerr, Overseerr, Bazarr, Prowlarr, Unpackerr and Homepage to HolyArrs step by step.
level: Expert
sidebar: { order: 2 }
---

Set up each app that talks to HolyArrs as Sonarr or Radarr. First turn the feature on and make both keys: see [Old-app API](../reference/old-app-api/#turn-it-on).

Every app uses the Sonarr-side key on `/sonarr` and the Radarr-side key on `/radarr`. Under a URL base, put it in front: `/holyarrs/sonarr`.

## Seerr or Overseerr

1. In Seerr, open **Settings > Services**.
2. Click **Add Sonarr Server** and fill in the fields below.
3. Click **Test**. Seerr lists your pick rules as its quality profiles and your show Library folders as its root folders. Pick the ones new requests use.
4. Save.
5. Click **Add Radarr Server** and repeat with the Radarr-side key and `/radarr`.

| Field | Sonarr server | Radarr server |
|---|---|---|
| Hostname or IP | Your HolyArrs host, or `holyarrs` on the same Docker network | same |
| Port | `6262`, or yours | same |
| Use SSL | Only over HTTPS | same |
| API key | Sonarr-side key | Radarr-side key |
| URL base | `/sonarr` | `/radarr` |

:::danger
Add each side once. Don't also add `/radarr` as Seerr's 4K server. HolyArrs keeps every copy of a movie on one title, so removing the "4K" one in Seerr removes the whole movie.
:::

If it does not work:

| Symptom | Fix |
|---|---|
| **Test** fails with 404 | The feature is off, or the URL base is wrong. Check the switch and copy the URL base from the card. |
| **Test** fails with 401 or 403 | The key is wrong or replaced, or its owner lost permission to manage the Library. Make a new key. |
| Changing the root folder in Seerr does nothing | Expected. Move titles in HolyArrs. |

<details class="ha-more"><summary>What Seerr does in HolyArrs</summary>

| Seerr action | In HolyArrs |
|---|---|
| Approved show request | Adds the show and keeps only the asked seasons up to date. A later season ask switches that season on. Anime is added with absolute numbering. |
| Approved movie request | Adds the movie with the release gate matching Seerr's minimum availability. |
| Search on request | Checks the title's folder first, then searches. Files you have are not grabbed again. "Search now" starts a hand search. |
| Progress bars | Read HolyArrs downloads, one line per episode. |
| Available | A movie with a file. A season once every episode has a file; a season still airing stays partly available. |
| Remove | Takes the title out of the Library. Files follow **Let these apps delete files**. An exclusion puts it on the Never-add list. |
| Per-user tags | Made and kept, not attached to titles. |
| Open in Sonarr/Radarr | Opens the title in HolyArrs. |

</details>

## Bazarr

1. In Bazarr, open **Settings > Sonarr**.
2. Enter your HolyArrs host and port, `/sonarr` as the URL base, and the Sonarr-side key.
3. Click **Test**, then save.
4. Repeat in **Settings > Radarr** with `/radarr` and the Radarr-side key.
5. Turn Bazarr's live feed off. HolyArrs has none; its scheduled sync finds new files.

The built-in subtitle search is held back in this release, so use Bazarr for subtitles.

<details class="ha-more"><summary>What syncs and what doesn't</summary>

| Works | Detail |
|---|---|
| Library sync | Every show and movie with the main Version's file: path, size, release name, languages, quality, video codec. Items without a file are skipped. |
| Pictures | Posters and wide pictures from HolyArrs. |
| Rescans | After a subtitle change, HolyArrs looks at that title's folder again. |
| Folders and tags | Read for Bazarr's health checks. |

| Doesn't work | Instead |
|---|---|
| Live updates | Scheduled full sync and subtitle search. |
| Folder browser in path mapping | Type paths by hand. |
| Grab history | Subtitle sites that need the indexer page link can't use it. |
| Webhooks into Bazarr | Scheduled sync. |

</details>

## Prowlarr

Prowlarr fills your HolyArrs Sources from its indexers. Each indexer becomes one Newznab or Torznab Source pointing at Prowlarr.

1. In Prowlarr, open **Settings > Apps**.
2. Add a **Sonarr** app for shows, a **Radarr** app for movies, or one of them.
3. Fill in the fields below.
4. Click **Test**, then **Save**. Prowlarr writes its indexers into HolyArrs.
5. Open **Sources** in HolyArrs. You should see the new Sources, paused. Resume the ones you want.

| Field | Sonarr app | Radarr app |
|---|---|---|
| Prowlarr server | Prowlarr's address as HolyArrs reaches it, such as `http://prowlarr:9696` | same |
| Sonarr / Radarr server | `http://your-server:6262/sonarr` | `http://your-server:6262/radarr` |
| API key | Sonarr-side key | Radarr-side key |

The key's maker must be allowed to change settings and manage the Library, or Prowlarr gets 403.

<details class="ha-more"><summary>How synced Sources behave</summary>

| Topic | Behaviour |
|---|---|
| Paused again | When Prowlarr moves a Source to another address, or turns local network access back on after you turned it off. |
| Local network | Prowlarr Sources may reach the local network. Other Sources need their own **May reach the local network** tick. |
| Used | Address and feed path, key (stored encrypted, shown masked), categories, least seeders, priority, and the feed, automatic search and hand search switches. |
| Stored only | Seed ratio, seed time, tags and other values, kept so the next sync sees no change. Seeding follows your downloader's seed goal. |
| Your edits | Prowlarr puts its own name, address, categories and priority back at its next full sync. |
| Removing | Removing an indexer in Prowlarr removes its Source. Prowlarr never touches Sources you added by hand. |
| Both sides | One indexer sent to both makes 2 Sources. A taken name gets "(shows)" or "(movies)" added. |
| Testing | Each Test and save asks Prowlarr's feed for its capabilities. If that fails, Prowlarr's force save still makes the Source, paused. |

</details>

## Unpackerr

You don't need it. HolyArrs unpacks RAR sets itself during import, and imports only the files the download listed, so files Unpackerr unpacks beside them are never imported.

If you run it anyway, point its Sonarr and Radarr entries at `/sonarr` and `/radarr` with the keys. It reads the download list: a finished download waiting for import shows as completed with its folder; after import it leaves the list. Unpackerr changes nothing in HolyArrs.

## Homepage

1. Add the Sonarr widget with `http://your-server:6262/sonarr` and the Sonarr-side key.
2. You should see the show count, the download list, aired episodes with no file, and the calendar. Lists show 10 per page, newest first.

The Radarr widget doesn't work yet: its missing list and calendar answer 404.

## Related

- [Old-app API](../reference/old-app-api/)
- [Old-app API limits](../reference/old-app-api-limits/)
- [Requests](../requests/)
