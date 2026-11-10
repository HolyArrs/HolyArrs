---
title: Read your old apps
description: How HolyArrs reads an old app or its backup, exactly what it keeps, and how typed keys are stored and deleted.
level: Expert
sidebar: { order: 5 }
---

Read an old app's setup, through the app or from its backup, as step 1 of an import. Reading changes nothing in HolyArrs or in the old app.

A **read** is HolyArrs's copy of one app's setup. It is kept until you import it or remove it.

## Read an app

1. Open **Settings > Import from other apps** and click **Add an app**.
2. Pick the app, then **Through the app** or **From a backup**.
3. Click **Check connection**. You should see the app and version within a few seconds.
4. Start the read. A big library takes a few minutes. **Stop reading** stops it; closing the sheet does not.
5. You should see the read in the list. Add the next app the same way.

**Edit** on a row reads that app again. **Remove** throws the read away.

## Choices

### Through the app

Type the app's address and API key. For qBittorrent, type the user name and password, or its API key on version 5.2 and later.

Old apps usually live on your home network, so turn on the local network switch for the read. Cloud metadata addresses stay blocked, and the app may not redirect HolyArrs to another address.

### From a backup

Type the full path, on the HolyArrs server, of one of these:

- a backup zip from Radarr, Sonarr, Lidarr, Prowlarr or Bazarr,
- the config folder of Seerr, Overseerr, qBittorrent or SABnzbd,
- one database or settings file.

Reads from a backup save no key.

## What is read

| App | What is read |
| --- | --- |
| Radarr, Sonarr, Lidarr | Every movie, show (with seasons) or artist: ids, folder, watch flag, quality profile, tags, date added. Also Library folders, tags, quality profiles, custom formats, release word rules, wait rules, naming, indexers, download clients, lists, notifications, metadata writers, exclusions, language profiles (Sonarr v3), metadata profiles (Lidarr) and Radarr's availability delay. |
| Prowlarr | Indexers, app links, sync profiles, indexer proxies, download clients, notifications. |
| Bazarr | Enabled languages, language profiles, enabled subtitle sites, default profiles. |
| qBittorrent, SABnzbd | Address, port, user name, download folders, categories and (SABnzbd) usenet servers. |
| Seerr, Overseerr | People (name, email, role, permissions, request limits), default limits, and every waiting or approved request: who asked, when, for what. Finished and turned-down requests are only counted. Avatars are not read. |

Reading more than is imported is on purpose: everything read but not imported shows on the **not imported** list with its reason.

## Keys and passwords

A read never holds a key, password, token or cookie, even when a backup has them in plain text. Each is listed under **needs re-entry**, for example `indexer "Nyaa": apiKey`.

Links count as secrets, because tokens hide in them. Notification links, RSS feed addresses and free-text extra parameters are left behind and listed. An indexer address keeps only its host and API path.

### The key you typed

When the owner reads an app with a typed key, HolyArrs keeps that address and key in its encrypted store. Next time, **Edit** reads again with **Use the saved key** on, and the key goes only to the saved address. HolyArrs never shows it.

| The saved key is deleted when |
| --- |
| you remove the read for that app |
| an import finishes with nothing failed (a failed item keeps the keys, so you can import again) |
| you run `holyarrs import-secrets -forget-move-credentials` |

## Who may read

The owner and admins, signed in with their password. The local network sign-in skip does not count. An API key needs the **admin** scope.

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| **Check connection** fails | The address is tested from the HolyArrs server. | Use an address the server reaches. Turn on the local network switch for a home-network app. |
| The key is wrong | Copy error, or qBittorrent wants a user name. | Copy the key again from the old app's settings. |
| A backup is refused | The zip was changed, or the path is outside the container. | Use the app's own backup zip, unchanged, at its path inside the HolyArrs container. |
| The read is too big | Limits per read and per server. | Remove old reads you no longer need. |

## For experts

<details class="ha-more"><summary>Backup handling and limits</summary>

| Rule | Value |
| --- | --- |
| Files copied | Only the files needed, into a private temporary folder, deleted after the read. |
| Size per copied file | 1 GB at most. |
| `config.xml` of an arr app | Never copied; it holds the API key. |
| Database | Opened read only. |
| Refused whole when | an entry points outside the zip, it holds a link, or a file unpacks far bigger than its size. |
| Backups at once | 1. |
| Kept per read | 16 MB. |
| Reads kept | The 50 newest. |

</details>

<details class="ha-more"><summary>API</summary>

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/probe` | Says which app and version answers, or which backup this is. |
| `POST /api/v1/migrate/read` | Reads the whole setup and keeps it. |
| `GET /api/v1/migrate/snapshots` | Lists reads, newest first. |
| `GET /api/v1/migrate/snapshots/{id}` | One read with everything it holds. |
| `DELETE /api/v1/migrate/snapshots/{id}` | Removes a read and the key saved for that app. |
| `GET /api/v1/migrate/saved-keys` | Apps with a saved key and their addresses, never the key. Owner only. |
| `DELETE /api/v1/migrate/saved-keys` | Deletes every saved key, or one with `?kind=`. Owner only. |

The probe and read body names the app: `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr`, `seerr`, `overseerr`, `qbittorrent` or `sabnzbd`. Add `url` with `key` (or `username` and `password`), or `path`. Add `allow_local_network` to reach your home network and `use_saved_key` to use the saved key.

</details>

## Related

- [Import from other apps](index.md)
- [Import keys from a file](import-secrets.md)
