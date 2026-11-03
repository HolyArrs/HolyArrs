# Moving from the old apps: reading their setup

HolyArrs can read the setup of the apps it replaces, so the move can bring
it over. This page covers the first half: reading. The second half, the
plan, the dry run, the move and its undo, is on
[Move from other apps](move-from-other-apps.md).
Nothing in HolyArrs changes when you read an app. What was read is kept as a
**snapshot** until you move it or remove it.

## What can be read

| App | What is read |
| --- | --- |
| Radarr, Sonarr, Lidarr | Every movie, show (with its seasons) or artist: ids, folder, whether it is watched, its quality profile, tags and the date it was added. Library folders, tags, quality profiles, custom formats, release word rules, delay profiles, naming, indexers, download clients, lists, notifications, metadata writers, exclusions, language profiles (Sonarr v3), metadata profiles (Lidarr) and Radarr's availability delay. |
| Prowlarr | Its indexers, its links to the other apps, sync profiles, indexer proxies, download clients and notifications. |
| Bazarr | Enabled languages, language profiles, enabled subtitle sites and the default profiles. |
| qBittorrent, SABnzbd | Address, port, user name, download folders, categories and (SABnzbd) usenet servers. |
| Seerr, Overseerr | People (name, email, role, permissions and ask limits), the default limits, and every request still waiting or approved: who asked, when, for what (TMDB/TVDB ids, seasons). Finished and turned-down requests are counted, not read. Avatars are not read. |

## Two ways to read

- **Through the app**: its address and its API key (qBittorrent: your user
  name and password, or its API key on 5.2 and later). The key is used for
  that one read and then forgotten: it is never saved, logged or shown.
  The old apps usually live on your home network, so turn on **local
  network** for the read; cloud metadata addresses stay blocked, and the app
  may not send HolyArrs to another address.
- **From a backup**: the full path, on the HolyArrs server, of a backup zip
  (Radarr, Sonarr, Lidarr, Prowlarr, Bazarr), the app's config folder
  (Seerr, Overseerr, qBittorrent, SABnzbd) or one database or settings file.
  HolyArrs copies only the files it needs into a private temporary folder
  (never an arr app's `config.xml`, which holds its API key), at most 1 GB
  each, opens the database read only and deletes the copy afterwards. A
  backup with an entry that leaves the zip, a link (also a `db` or `config`
  sub-folder that is a link), or a file that unpacks far bigger than it is,
  is refused whole. One backup is read at a time.

## Keys and passwords stay behind

A snapshot never holds a key, password, token or cookie, even when the
backup has them in plain text. Each one is listed under **needs re-entry**
(for example `indexer "Nyaa": apiKey`) so you can type it again during the
move. Links whose secret is part of the address (like `discord://...`) are
listed there too. In indexer and notification settings every link counts as
a secret, because tokens also hide in the path (a webhook address, an ntfy
topic, an RSS feed with a passkey): notification links, RSS feed addresses
and free-text extra parameters are left behind and listed, and an indexer's
base address keeps only its host and API path (no user, no query, nothing
after a part that looks like a key).

## Who may read

The owner and admins, signed in with their password (not through the
local-network sign-in skip). API keys need the **admin** scope.

## API

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/probe` | Says which app and version answers, or which backup this is. |
| `POST /api/v1/migrate/read` | Reads the whole setup and keeps it as a snapshot. |
| `GET /api/v1/migrate/snapshots` | Lists kept snapshots (newest first). |
| `GET /api/v1/migrate/snapshots/{id}` | One snapshot with everything it holds. |
| `DELETE /api/v1/migrate/snapshots/{id}` | Removes a snapshot. |

The body of probe and read names the app (`radarr`, `sonarr`, `lidarr`,
`prowlarr`, `bazarr`, `seerr`, `overseerr`, `qbittorrent`, `sabnzbd`) and
either `url` with `key` (or `username` and `password`) or `path`, plus
`allow_local_network`. The 50 newest snapshots are kept; one snapshot may be
up to 16 MB.
