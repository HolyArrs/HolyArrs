---
title: Environment variables
description: Every environment variable HolyArrs reads for a normal install, with its default and an example.
level: Expert
sidebar: { order: 0 }
---

Every variable the HolyArrs container reads. Most installs set only `PUID`, `PGID` and `TZ`.

## Common

| Variable | What it does | Default | Example |
| --- | --- | --- | --- |
| `PUID`, `PGID` | User and group HolyArrs runs as. Must own `/config` and your media folders. | `1000`, `1000` | `PUID=1000` |
| `TZ` | Time zone for schedules and logs. | `Etc/UTC` | `TZ=Europe/Amsterdam` |
| `UMASK` | Permission bits new files leave out. Affects new files only. | `022` | `UMASK=002` |
| `HOLYARRS_SAVE_PATH` | The downloader's save folder, as the downloader sees it. Sent with every download. | none | `/data/downloads` |
| `HOLYARRS_URL_BASE` | Serve HolyArrs under a path behind a reverse proxy. `/` ignores a path saved in Settings. | none | `/holyarrs` |
| `HOLYARRS_ALLOWED_HOSTS` | Extra host names people use to reach HolyArrs, comma separated. | IPs and `localhost` | `holyarrs.example.com` |
| `HOLYARRS_TRUSTED_PROXIES` | Your reverse proxy's address or network, comma separated. Needed for real client addresses. | none | `172.18.0.0/16` |
| `HOLYARRS_MASTER_KEY_FILE` | Read the vault's master key from its own file. The `PUID` user must be able to read it. | `/config/holyarrs.key` | `/keys/holyarrs.key` |

## Values from files

Put `FILE__` in front of any variable to read its value from a file, such as a Docker secret.

```yaml
environment:
  - FILE__HOLYARRS_ALLOWED_HOSTS=/run/secrets/holyarrs_hosts
```

HolyArrs drops one newline at the end of the file. If the file is missing, HolyArrs stops and the log names the variable and file, never the value.

## Recovery and troubleshooting

| Variable | What it does | Default | Example |
| --- | --- | --- | --- |
| `HOLYARRS_SAFE_MODE` | Start only the core: sign-in, Library, settings, backups and health. Remove it after the fix. | off | `1` |
| `HOLYARRS_LOW_MEMORY` | `1` forces low-memory mode on, `0` forces it off. Unset, it turns on at about 2.5 GiB of RAM or less. | by machine size | `1` |
| `HOLYARRS_FOLLOW_LINKS` | `1` lets the Library scan and imports use video and music files that are symlinks (libraries built from links into a mounted drive). A linked download is placed in the Library as a link. | off | `1` |
| `GOMEMLIMIT` | Override the memory target HolyArrs sets for itself. | set by HolyArrs | `1GiB` |
| `HOLYARRS_SKIP_SPACE_CHECK` | Skip the free-space check before a database update. | off | `yes` |
| `HOLYARRS_CONFIG_IN_MEMORY` | Allow `/config` on a memory-backed folder. For test boxes; data is lost on restart. | off | `yes` |
| `HOLYARRS_ALLOW_DESKTOP_FOLDER` | Start with `/config` in a Docker Desktop shared folder on a Mac, after you check file locks work there. HolyArrs warns on every start. | off | `1` |
| `HOLYARRS_READONLY_DOWNLOADERS` | Downloaders HolyArrs must never change, by id or address, comma separated. | none | `3,http://qbit:8080` |
| `HOLYARRS_HEALTH_URL` | Address the container health check calls. | `http://127.0.0.1:6262` | `http://127.0.0.1:6262/holyarrs` |

On/off variables accept `1`, `true`, `yes` or `on`.

## Single sign-on

Set these before start. If `HOLYARRS_OIDC_ENABLED` is present at all, even as `false`, the environment owns single sign-on and Settings only shows it. The on/off ones here take only `true` or `false`; anything else stops the start.

| Variable | What it does | Default |
| --- | --- | --- |
| `HOLYARRS_OIDC_ENABLED` | Turn on single sign-on. | `false` |
| `HOLYARRS_OIDC_ISSUER` | The provider's exact HTTPS issuer. | none |
| `HOLYARRS_OIDC_CLIENT_ID` | Registered client id. | none |
| `HOLYARRS_OIDC_CLIENT_SECRET_FILE` | File holding the client secret, at most 4096 bytes. Leave out for a public client. | none |
| `HOLYARRS_OIDC_REDIRECT_URL` | Exact callback: your address, URL base if any, then `/api/v1/auth/oidc/callback`. | none |
| `HOLYARRS_OIDC_ALLOW_JOIN` | New people join as members who can only ask. | `false` |
| `HOLYARRS_OIDC_ALLOW_LOCAL_NETWORK` | Allow a provider on your private network. | `false` |
| `HOLYARRS_OIDC_ALLOW_LOOPBACK_HTTP` | Allow plain HTTP on this machine, for testing. | `false` |
| `HOLYARRS_FORWARD_AUTH_ENABLED` | Turn on trusted proxy sign-in. | `false` |
| `HOLYARRS_FORWARD_AUTH_ISSUER` | Stable HTTPS name for the proxy. | none |
| `HOLYARRS_FORWARD_AUTH_TRUSTED_PEERS` | Proxy addresses or ranges allowed to send identity headers, comma separated. | none |
| `HOLYARRS_FORWARD_AUTH_ALLOW_JOIN` | New proxy names join as members who can only ask. | `false` |

A bad setup stops HolyArrs from starting. Steps are in [Sign-in and passwords](../users/sign-in.md).

## For experts

<details class="ha-more"><summary>Metadata</summary>

| Variable | What it does | Default |
| --- | --- | --- |
| `HOLYARRS_METADATA_URL` | The metadata server address. | `https://api.holyarrs.com` |
| `HOLYARRS_TMDB_KEY` | Your own TMDB read access token. Set, TMDB reads go straight to TMDB instead of the metadata server. | none |
| `HOLYARRS_METADATA_SERVICE_TOKEN_FILE` | File holding this install's metadata server credential, read on each call. Unset, the credential lives in the vault. | none |

What still goes through the metadata server with your own TMDB key, and every other outside address, is in [Outside addresses](../run/outside-hosts.md).

</details>

<details class="ha-more"><summary>First-start shortcuts</summary>

Each adds one item at start. Used for demos and tests; the first-run wizard does the same.

| Variable | Adds |
| --- | --- |
| `HOLYARRS_MOVIES` | A movie Library folder. |
| `HOLYARRS_QBITTORRENT_URL` | A qBittorrent downloader. |
| `HOLYARRS_SOURCE_URL` | A Torznab source with no key. |

</details>

<details class="ha-more"><summary>Developer and test variables</summary>

These point HolyArrs at stand-ins for tests. Leave them unset on a real install.

| Variable | What it does |
| --- | --- |
| `HOLYARRS_FLAGS` | Comma-separated switches for features that are off by default in this release. See [Updates](../run/updates.md). |
| `HOLYARRS_DIRECT_SOURCES` | `1` lets this install ask music and TV sources itself instead of the metadata server. |
| `HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL` | Allow a metadata server on the local network. |
| `HOLYARRS_METADATA_COUNT` | Count and log metadata requests. |
| `HOLYARRS_TMDB_TOKEN_FILE`, `HOLYARRS_TMDB_URL` | Test TMDB token file and stand-in address. |
| `HOLYARRS_TVMAZE_URL`, `HOLYARRS_MUSICBRAINZ_URL`, `HOLYARRS_DEEZER_URL`, `HOLYARRS_TRAKT_URL`, `HOLYARRS_PLEX_URL`, `HOLYARRS_PLEX_APP_URL`, `HOLYARRS_PLEX_DISCOVER_URL`, `HOLYARRS_PUSHOVER_URL`, `HOLYARRS_TELEGRAM_URL`, `HOLYARRS_YOUTUBE_URL`, `HOLYARRS_RELEASE_FEED` | Stand-in addresses for outside services. |
| `HOLYARRS_TRAILER_PASS_FIRST`, `HOLYARRS_TRAILER_PASS_EVERY` | Trailer pass timing as durations, such as `2s`. |

</details>

## Related

- [Install](../start/install.mdx)
- [Run under a path](../run/url-base.md)
- [Sign-in and passwords](../users/sign-in.md)
