# Read your old apps

You want HolyArrs to see how your old apps are set up, so you can bring that setup over. This page covers the first step, reading. Choosing, testing, moving and undoing are on [Move from other apps](move-from-other-apps.md).

Reading changes nothing in HolyArrs and nothing in the old app. What HolyArrs reads is kept as a **read** until you move it or remove it.

## Read an app

1. Open **Settings > Move from other apps** (at `http://your-server:6262/settings/move`).
2. Click **Add an app** and pick the app.
3. Choose how to read it: through the app, or from a backup (see below).
4. Click **Check connection** to see that the app answers. This takes a few seconds.
5. Start the read. A big library can take a few minutes. **Stop reading** stops it. Closing the sheet does not.
6. The read shows up in the list. Add the next app the same way.

To read an app again, click **Edit** on its row. To throw a read away, click **Remove**.

## Two ways to read

### Through the app

You type the app's address and its API key. For qBittorrent you type its user name and password, or its API key on version 5.2 and later.

Your old apps usually live on your home network. Turn on the local network switch for the read. Cloud metadata addresses stay blocked either way, and the app may not send HolyArrs on to another address.

### From a backup

You type the full path, on the HolyArrs server, of one of these:

- a backup zip from Radarr, Sonarr, Lidarr, Prowlarr or Bazarr,
- the config folder of Seerr, Overseerr, qBittorrent or SABnzbd,
- or one database or settings file.

HolyArrs copies only the files it needs into a private temporary folder, at most 1 GB each. It never copies an arr app's `config.xml`, because that file holds the API key. It opens the database read only and deletes the copy when it is done. A backup is refused whole when an entry points outside the zip, when it holds a link, or when a file unpacks far bigger than its size. One backup is read at a time.

## What is read

| App | What is read |
| --- | --- |
| Radarr, Sonarr, Lidarr | Every movie, show (with seasons) or artist: its ids, folder, watch flag, quality profile, tags and date added. Also Library folders, tags, quality profiles, custom formats, release word rules, wait rules, naming, indexers, download clients, lists, notifications, metadata writers, exclusions, language profiles (Sonarr v3), metadata profiles (Lidarr) and Radarr's availability delay. |
| Prowlarr | Indexers, links to the other apps, sync profiles, indexer proxies, download clients and notifications. |
| Bazarr | Enabled languages, language profiles, enabled subtitle sites and the default profiles. |
| qBittorrent, SABnzbd | Address, port, user name, download folders, categories and (SABnzbd) usenet servers. |
| Seerr, Overseerr | People (name, email, role, permissions, request limits), the default limits, and every request still waiting or approved: who asked, when, and for what. Finished and turned-down requests are only counted. Avatars are not read. |

## Keys and passwords

A read never holds a key, password, token or cookie, even when a backup has them in plain text. Each one is listed under **needs re-entry**, for example `indexer "Nyaa": apiKey`, so you can type it again later.

Links count as secrets too, because tokens hide in them. Notification links, RSS feed addresses and free-text extra parameters are left behind and listed. An indexer address keeps only its host and API path.

### The key you typed

When the owner reads an app with a typed key, HolyArrs keeps that address and key in its encrypted store, so **Edit** can read the app again without typing. The next time, **Use the saved key** is on, and the key is sent only to the saved address. HolyArrs never shows the key.

The saved key is deleted when:

- you remove the read for that app,
- a move finishes with nothing failed (a move with a failed item keeps the keys, so you can move again without typing),
- or you run `holyarrs import-secrets -forget-move-credentials`.

Reads from a backup save nothing.

## Who may read

The owner and admins, signed in with their password. The local network sign-in skip does not count. An API key needs the **admin** scope.

## Troubleshooting

- **Check connection fails.** Check the address and port from the HolyArrs server, not from your own computer. If the app is on your home network, turn on the local network switch.
- **The key is wrong.** Copy it again from the old app's settings. For qBittorrent, check the user name and password.
- **A backup is refused.** Use the backup zip the app made itself, unchanged. Make sure the path is the path inside the HolyArrs container.
- **The read is too big.** HolyArrs keeps up to 16 MB per read and the 50 newest reads. Remove old reads you no longer need.

## API

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/probe` | Says which app and version answers, or which backup this is. |
| `POST /api/v1/migrate/read` | Reads the whole setup and keeps it. |
| `GET /api/v1/migrate/snapshots` | Lists kept reads, newest first. |
| `GET /api/v1/migrate/snapshots/{id}` | One read with everything it holds. |
| `DELETE /api/v1/migrate/snapshots/{id}` | Removes a read and the key saved for that app. |
| `GET /api/v1/migrate/saved-keys` | Lists the apps with a saved key and their addresses, never the key. Owner only. |
| `DELETE /api/v1/migrate/saved-keys` | Deletes every saved key, or one with `?kind=`. Owner only. |

The body of probe and read names the app (`radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr`, `seerr`, `overseerr`, `qbittorrent`, `sabnzbd`). Add either `url` with `key` (or `username` and `password`), or `path`. Add `allow_local_network` to reach your home network, and `use_saved_key` to read with the saved key.
