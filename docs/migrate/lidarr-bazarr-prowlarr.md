---
title: Lidarr, Bazarr and Prowlarr
description: What comes over from Lidarr, Bazarr, Prowlarr, qBittorrent and SABnzbd when you import into HolyArrs.
level: Setup
sidebar: { order: 3 }
---

Import your music, subtitle languages, Sources and Downloaders. The steps match [Import from other apps](index.md); this page lists what each app brings.

## Import them

1. Open **Settings > Import from other apps** and click **Add an app** for each app.
2. Type the address and API key, or the backup path, and click **Check connection**. For qBittorrent, type the user name and password, or its API key on version 5.2 and later.
3. Read each app, then go through **What to import** and **Import**. You should see each app's groups in **What to import**.

## Lidarr

| From Lidarr | In HolyArrs |
| --- | --- |
| Library folders | Library folders for music. |
| Artists | Titles, matched by MusicBrainz id, folders and files in place. A watched artist gets **Keep up to date**. Nothing is searched for. |
| Quality profiles | The closest **Pick rules** preset (Lossless first, Lossy 320 or V0, Any music). |
| Exclusions | The **Never-add** list. |
| Untagged default wait rule | Waits on the matched Pick rules, up to 7 days. |
| Indexers, download clients, remote path mappings, naming, notifications | Same as for [Sonarr and Radarr](sonarr-radarr.md#what-comes-over). |

Stays behind: metadata profiles, import lists for music, custom formats, release word rules and tags.

## Bazarr

| From Bazarr | In HolyArrs |
| --- | --- |
| Default language profile | Default subtitle languages. |
| Podnapisi and Gestdown | **Subtitle sites**, one of each kind. A kind already here is left alone. |

Stays behind: other subtitle sites and their accounts. HolyArrs reaches Addic7ed through Gestdown with no account, so add Gestdown under **Settings > Subtitles**. For a site with an account, type it there or use [Import keys from a file](import-secrets.md).

## Prowlarr

| From Prowlarr | In HolyArrs |
| --- | --- |
| Torznab, Newznab and RSS indexers | **Sources**, added paused and blocked from your home network. |
| Download clients | **Downloaders**, added paused. |

Stays behind: app links, sync profiles, indexer proxies and notification kinds HolyArrs does not have.

Sources brought in from Prowlarr still search through Prowlarr, so keep it running. If it stops, those Sources say Prowlarr isn't answering.

:::tip
To keep Prowlarr running and let it push Sources into HolyArrs instead, see [Old-app API](../reference/old-app-api.md).
:::

## qBittorrent and SABnzbd

| From | In HolyArrs |
| --- | --- |
| qBittorrent, SABnzbd | A **Downloader**, added paused, using the `holyarrs` category so it never picks up the old apps' downloads. |

Stays behind: download categories and SABnzbd's usenet servers.

## Before you turn things on

Type each key again and resume each item. See [After the import](index.md#after-the-import).

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| qBittorrent read fails | Wrong user name or password, or the key on a version before 5.2. | Use the user name and password from qBittorrent's Web UI settings. |
| A Downloader at home does not answer | **May reach the local network** is off. | Turn it on for that Downloader. |
| A subtitle site is off | It needs an account, which an import cannot bring. | Type the account under **Settings > Subtitles**, then turn it on. |

## Related

- [Import from other apps](index.md)
- [Read your old apps](what-is-read.md)
- [Request apps](requests-apps.md)
