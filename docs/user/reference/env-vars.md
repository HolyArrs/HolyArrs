---
title: Environment variables
description: Every environment variable HolyArrs reads, with its default and an example.
level: Expert
sidebar: { order: 1 }
---

Every variable the HolyArrs container reads. Most installs set only `PUID`, `PGID` and `TZ`.

## Common

| Variable | What it does | Default | Example |
| --- | --- | --- | --- |
| `PUID`, `PGID` | User and group HolyArrs runs as. Must own `/config` and your media folders. | `1000`, `1000` | `PUID=1000` |
| `TZ` | Time zone for schedules and logs. | `Etc/UTC` | `TZ=Europe/Amsterdam` |
| `UMASK` | Permission bits new files leave out. Affects new files only. | `022` | `UMASK=002` |
| `HOLYARRS_SAVE_PATH` | The Downloader's save folder, as the Downloader sees it. Sent with every download. | none | `/data/downloads` |
| `HOLYARRS_URL_BASE` | Serve HolyArrs under a path behind a reverse proxy. | none | `/holyarrs` |
| `HOLYARRS_ALLOWED_HOSTS` | Extra host names people use to reach HolyArrs, comma separated. | none | `holyarrs.example.com` |
| `HOLYARRS_TRUSTED_PROXIES` | Your reverse proxy's address or network. Needed for single sign-on and real client addresses. | none | `172.18.0.0/16` |
| `HOLYARRS_MASTER_KEY_FILE` | Read the vault's master key from its own file. The `PUID` user must be able to read it. | none | `/keys/holyarrs.key` |

## Values from files

Add `FILE__` in front of any variable to read its value from a file, for example Docker secrets:

```yaml
environment:
  - FILE__HOLYARRS_ALLOWED_HOSTS=/run/secrets/holyarrs_hosts
```

One newline at the end of the file is dropped. If the file is missing, HolyArrs stops and names the variable and file, never the value.

## Recovery and troubleshooting

| Variable | What it does | Default | Example |
| --- | --- | --- | --- |
| `HOLYARRS_SAFE_MODE` | Start only the core: sign-in, Library, settings, backups and health. | off | `1` |
| `HOLYARRS_LOW_MEMORY` | Force low-memory mode on (`1`) or off (`0`). Unset, it turns on at 2.5 GiB of RAM or less. | by machine size | `1` |
| `GOMEMLIMIT` | Override the memory target HolyArrs sets for itself. | set by HolyArrs | `1GiB` |
| `HOLYARRS_SKIP_SPACE_CHECK` | Skip the free-space check before a database update. | off | `yes` |
| `HOLYARRS_CONFIG_IN_MEMORY` | Allow `/config` on a memory-backed folder. For test boxes; data is lost on restart. | off | `yes` |
| `HOLYARRS_ALLOW_DESKTOP_FOLDER` | Start with `/config` in a Docker Desktop shared folder (Mac) once you have checked file locks work there. HolyArrs warns on every start. Never lifts the refusal for network shares. | off | `1` |
| `HOLYARRS_READONLY_DOWNLOADERS` | Downloaders HolyArrs must never change, by id or address, comma separated. | none | `3,http://qbit:8080` |
| `HOLYARRS_HEALTH_URL` | Address the container health check calls. | `http://127.0.0.1:6262` | `http://127.0.0.1:6262/holyarrs` |

On/off variables accept `1`, `true`, `yes` or `on`.

## For experts

<details class="ha-more"><summary>Metadata</summary>

| Variable | What it does | Default |
| --- | --- | --- |
| `HOLYARRS_METADATA_URL` | The metadata server address. | `https://holyarrs-api.coderluii.dev` |
| `HOLYARRS_TMDB_KEY` | Your own TMDB read access token. Set, TMDB reads go straight to TMDB instead of the metadata server. Not shown in the app. | none |
| `HOLYARRS_METADATA_SERVICE_TOKEN_FILE` | File holding this install's metadata server credential, read on each call. Unset, the credential lives in the vault. | none |

</details>

<details class="ha-more"><summary>First-start shortcuts</summary>

Each adds one item at start. Used for demos and tests; the wizard does the same.

| Variable | Adds |
| --- | --- |
| `HOLYARRS_MOVIES` | A movie Library folder. |
| `HOLYARRS_QBITTORRENT_URL` | A qBittorrent Downloader. |
| `HOLYARRS_SOURCE_URL` | A Torznab Source with no key. |

</details>

<details class="ha-more"><summary>Developer and test variables</summary>

These point HolyArrs at stand-ins for tests. Leave them unset on a real install.

| Variable | What it does |
| --- | --- |
| `HOLYARRS_FLAGS` | Comma-separated feature flags for unfinished features. |
| `HOLYARRS_DIRECT_SOURCES` | `1` lets this install ask music and TV sources itself instead of the metadata server. |
| `HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL` | Allow a metadata server on the local network. |
| `HOLYARRS_METADATA_COUNT` | Count and log metadata requests. |
| `HOLYARRS_TMDB_TOKEN_FILE`, `HOLYARRS_TMDB_URL` | Lab TMDB token file and stand-in address. |
| `HOLYARRS_TVDB_URL`, `HOLYARRS_TVMAZE_URL`, `HOLYARRS_MUSICBRAINZ_URL`, `HOLYARRS_DEEZER_URL`, `HOLYARRS_TRAKT_URL`, `HOLYARRS_PLEX_URL`, `HOLYARRS_PLEX_APP_URL`, `HOLYARRS_PLEX_DISCOVER_URL`, `HOLYARRS_PUSHOVER_URL`, `HOLYARRS_TELEGRAM_URL`, `HOLYARRS_YOUTUBE_URL`, `HOLYARRS_RELEASE_FEED` | Stand-in addresses for outside services. |
| `HOLYARRS_TRAILER_PASS_FIRST`, `HOLYARRS_TRAILER_PASS_EVERY` | Trailer pass timing as durations, for example `2s`. |

</details>

## Related

- [Install](../getting-started/install.md)
- [Run under a path](../getting-started/url-base.md)
- [Import keys from a file](../../migrate/import-secrets.md)
