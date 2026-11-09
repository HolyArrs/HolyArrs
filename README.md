<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/banner-light.gif">
    <img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/banner-dark.gif" alt="HolyArrs: one self-hosted app for shows, movies, music and downloads" width="100%">
  </picture>
</p>

<p align="center"><a href="https://holyarrs.com"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-site-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-site-light.png" height="34" alt="HolyArrs.com" /></picture></a>&nbsp;<a href="https://reddit.com/r/HolyArrs"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-reddit-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-reddit-light.png" height="34" alt="r/HolyArrs" /></picture></a>&nbsp;<a href="https://discord.com/invite/mthsvesKpC"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-discord-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-discord-light.png" height="34" alt="Discord" /></picture></a>&nbsp;<a href="https://holyarrs.com/roadmap/"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-roadmap-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-roadmap-light.png" height="34" alt="Roadmap" /></picture></a>&nbsp;<a href="https://holyarrs.com/whats-new/"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-news-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-news-light.png" height="34" alt="What's new" /></picture></a></p>

<p align="center">
  <a href="https://hub.docker.com/r/holyarrs/holyarrs"><img src="https://img.shields.io/docker/pulls/holyarrs/holyarrs?style=flat-square&logo=docker&logoColor=white&label=pulls&color=3b82f6" alt="Docker pulls"></a>
  <a href="https://hub.docker.com/r/holyarrs/holyarrs"><img src="https://img.shields.io/docker/image-size/holyarrs/holyarrs/latest?style=flat-square&logo=docker&logoColor=white&label=size&color=3b82f6" alt="Image size"></a>
  <a href="https://hub.docker.com/r/holyarrs/holyarrs/tags"><img src="https://img.shields.io/docker/v/holyarrs/holyarrs?sort=semver&style=flat-square&logo=docker&logoColor=white&label=version&color=3b82f6" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licence-AGPL--3.0-3b82f6?style=flat-square" alt="Licence: AGPL-3.0"></a>
</p>

<p align="center"><b>HolyArrs</b> is one self-hosted app for your shows, movies and music.<br>Browse like a streaming app, ask for what you want, and it handles the rest.</p>

<p align="center"><b>About 15 MB download · about 140 MB of RAM idle with 10,000 titles · amd64, arm64, armv7</b><br><sub>The six apps it replaces download 718.2 MB together.</sub></p>

<p align="center"><sub>From the maker of <a href="https://github.com/CoderLuii/HolyClaude">HolyClaude</a>, <a href="https://github.com/CoderLuii/HolyCode">HolyCode</a> and <a href="https://github.com/CoderLuii/ChannelWatch">ChannelWatch</a> · 350k+ Docker pulls</sub></p>

<p align="center"><a href="#install">Install</a> · <a href="#coming-from-sonarr-radarr-and-the-rest">Coming from other apps</a> · <a href="#not-built-yet">Not built yet</a> · <a href="#faq">FAQ</a> · <a href="#help">Help</a></p>

## Why I built it

A normal media server runs six apps: Sonarr, Radarr, Lidarr, Bazarr, Prowlarr and Seerr. That's six logins, six ports, six update cycles and six API keys to copy between them. When a show doesn't land, you check six places to find out why.

HolyArrs is the one app I wanted instead.

<p align="center">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/hero-dark.webp"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/hero-light.webp" width="100%" alt="HolyArrs Explore: hover a title and the trailer plays" /></picture>
</p>

## What it does

Someone asks for a title. HolyArrs searches your Sources, picks the best release with your Pick rules, sends it to your downloader and files it in your library. Your media server hears about it when it lands.

<picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/tiles-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/tiles-light.png" width="100%" alt="Browse like a streaming app. Ask for anything. It finds it. Straight to your downloader. Subtitles kept. Into your library." /></picture>

- **Explore** like a streaming app. Hover a title and the trailer plays.
- **Requests** for a whole show or one season. You approve, or let trusted people skip the queue.
- **Pick rules** decide which release wins: size, quality, language.
- **Subtitles** in the languages you pick, kept with the file. Finding them online comes after 1.0.
- **Calendar** of what's airing for what you follow.
- **Move your setup** from Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent and SABnzbd. Try it first, undo it later.
- **Old-app API**, so Prowlarr can keep pushing Sources to HolyArrs once you turn it on.
- **No API keys needed.** Titles, artwork and charts come from the HolyArrs metadata server.
- **Stored keys encrypted** with a master key that lives in your config folder.
- **Tiny.** An image of about 15 MB with nothing but HolyArrs inside, idling around 140 MB of RAM with 10,000 titles.

<details>
<summary><b>Every page, in motion</b></summary>

<table>
<tr>
<td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-home-dark.webp"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-home-light.webp" width="100%" alt="Home" /></picture><br><b>Home</b> · <sub>Everything waiting on you, at a glance.</sub></td>
<td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-explore-dark.webp"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-explore-light.webp" width="100%" alt="Title page" /></picture><br><b>Title page</b> · <sub>Trailer, cast and one button to ask.</sub></td>
</tr>
<tr>
<td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-requests-dark.webp"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-requests-light.webp" width="100%" alt="Requests" /></picture><br><b>Requests</b> · <sub>Approve what friends and family ask for.</sub></td>
<td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-downloads-dark.webp"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/clip-downloads-light.webp" width="100%" alt="Downloads" /></picture><br><b>Downloads</b> · <sub>Every grab, live, in one table.</sub></td>
</tr>
</table>

</details>

## One app instead of six

| App | Download (amd64) | Idle RAM | Logins | Web ports |
|---|---|---|---|---|
| Sonarr | 87.9 MB | | 1 | 1 |
| Radarr | 91.0 MB | | 1 | 1 |
| Lidarr | 121.3 MB | | 1 | 1 |
| Prowlarr | 80.8 MB | | 1 | 1 |
| Bazarr | 156.4 MB | | 1 | 1 |
| Overseerr | 180.8 MB | | 1 | 1 |
| **Six together** | **718.2 MB** | | **6** | **6** |
| **HolyArrs** | **14.9 MB** | **~140 MiB** | **1** | **1 (6262)** |

<sub>About 48 times smaller. Compressed registry download, linuxserver.io images, amd64, measured 2026-11-08. Idle RAM measured with 10,000 titles and 55,000 movies and episodes.</sub>

## Works with

<picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/works-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/works-light.png" width="100%" alt="Downloaders: qBittorrent, Transmission, Deluge, SABnzbd, NZBGet. Media servers: Plex, Jellyfin, Emby." /></picture>

- **Downloaders:** qBittorrent, Transmission, Deluge, SABnzbd, NZBGet
- **Media servers:** Plex, Jellyfin, Emby
- **Sources:** any Torznab or Newznab indexer, or Prowlarr or Jackett
- **Moves your setup from:** Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent, SABnzbd

## Install

Create a `docker-compose.yml` in a new folder on a local disk (not a network share) and paste this:

```yaml
name: HolyArrs
services:
  holyarrs:
    image: holyarrs/holyarrs:latest
    container_name: holyarrs
    ports:
      - 6262:6262
    volumes:
      - /your/local/path:/config
      - /path/to/movies:/movies
      - /path/to/tv:/tv
      - /path/to/music:/music
      - /path/to/downloads:/downloads
    environment:
      PUID: 1000
      PGID: 1000
      TZ: "America/New_York"
    restart: unless-stopped
```

Replace `/your/local/path` with the folder where HolyArrs should keep its config, and set `TZ` to your timezone (for example `America/Chicago` or `Europe/London`).

```sh
docker compose up -d
```

Open `http://your-server:6262` and make the owner account right away. There is no default password. Until you make it, whoever opens HolyArrs first becomes the owner.

Container settings go in compose. Everything else is set in the app.

<details>
<summary>Or with <code>docker run</code></summary>

```sh
docker run -d \
  --name=holyarrs \
  -e PUID=1000 \
  -e PGID=1000 \
  -e TZ=America/New_York \
  -p 6262:6262 \
  -v /your/local/path:/config \
  -v /path/to/movies:/movies \
  -v /path/to/tv:/tv \
  -v /path/to/music:/music \
  -v /path/to/downloads:/downloads \
  --restart unless-stopped \
  holyarrs/holyarrs:latest
```

</details>

<details>
<summary>Environment and platforms</summary>

| Variable | What it does |
|---|---|
| `PUID` / `PGID` | User and group that own your media folders. Run `id youruser` to find them. |
| `TZ` | Your time zone, for the calendar and logs. |
| `UMASK` | Optional. Permission bits new files leave out, for example `002`. |

More (reverse proxy, URL base, save path): [Environment variables](https://docs.HolyArrs.com/start/install/#environment-variables).

| Platform | Status |
|---|---|
| amd64 | Supported |
| arm64 | Supported (Apple silicon, newer Pis, ARM VPS) |
| armv7 | Supported |

</details>

<details>
<summary>On a NAS?</summary>

[Unraid, Synology and other NAS boxes](https://docs.HolyArrs.com/run/platforms/)

</details>

<details>
<summary>Keep your master key safe</summary>

`holyarrs.key` in your config folder unlocks your stored passwords and keys. Backups don't include it, so copy it somewhere safe. See [Backups](https://docs.HolyArrs.com/run/backups/).

</details>

### Tags

Images live on Docker Hub as [`holyarrs/holyarrs`](https://hub.docker.com/r/holyarrs/holyarrs). Every tag has amd64, arm64 and armv7.

| Tag | Use it for |
|---|---|
| `beta` | New versions land here first. |
| `latest` | Moves once a version has been out for a while. |
| `stable` | The slowest, safest channel. |
| `1.0.0` | One exact version. It never moves. |


Full guide: [Install HolyArrs with Docker](https://docs.HolyArrs.com/start/install/).

## Coming from Sonarr, Radarr and the rest

Open **Settings > Move from other apps**, add each old app's address and API key (or a backup zip), pick what comes over, run **Try it**, then **Move**. You can undo a move later.

| From | What comes over |
|---|---|
| Sonarr, Radarr, Lidarr | Library folders, titles, quality profiles as Pick rules, exclusions, naming, indexers and download clients |
| Prowlarr | Indexers as Sources |
| Bazarr | Default subtitle languages |
| Seerr, Overseerr | People and their waiting or approved requests |
| qBittorrent, SABnzbd | Downloaders |

Keys and passwords stay behind; you type them in again or import them from a file. Custom formats, tags and history don't come over. Media files never move.

HolyArrs only reads your old apps and changes nothing in them, so you can run both side by side. Prowlarr can keep pushing Sources to HolyArrs through the old-app API once you turn it on. Step by step: [Move from other apps](https://docs.HolyArrs.com/migrate/).

## Not built yet

- **Downloading by itself.** HolyArrs connects to qBittorrent, SABnzbd and the others. It doesn't replace them.
- **AI or MCP features.** Not in 1.0.
- **Phone and TV apps.** Not built. The web app works on a phone.
- **Music in Explore** is still being built.

What's next is on the [roadmap](https://holyarrs.com/roadmap/). Vote there, or post ideas on [r/HolyArrs](https://reddit.com/r/HolyArrs).

## FAQ

**Do I keep qBittorrent or SABnzbd?**  
Yes, for now. HolyArrs connects to the downloader you already run. A built-in downloader is planned for a later release.

**Can Sonarr and Radarr keep running while I try it?**  
Yes. HolyArrs reads them once and never changes them.

**Is it free?**  
Yes. It's free and open source.

## Security

- The owner account is made at first visit. There is no default password.
- Stored passwords and keys are encrypted with `holyarrs.key`.
- The image has no shell inside it.
- The log masks secrets.
- Moved Sources and Downloaders start paused and blocked from your home network.
- Report security problems privately: [SECURITY](.github/SECURITY.md).

## Help

- Follow the log: `docker logs -f holyarrs`
- Check your setup: `docker exec holyarrs /holyarrs doctor`
- Locked out of the owner account: `docker exec -i holyarrs /holyarrs reset-owner`

<details>
<summary>Permission denied on media folders</summary>

`PUID` and `PGID` don't match the owner of those folders. Run `id youruser` on the host, put those numbers in compose and recreate the container.

</details>

<details>
<summary>Errors with the config folder on a network share</summary>

The database needs a local disk. Move `/config` to a local path. Media folders on a share are fine.

</details>

<details>
<summary>Port 6262 is already in use</summary>

Change the left side only, for example `6263:6262`, and open `http://your-server:6263`.

</details>

<details>
<summary>Locked out</summary>

Run `docker exec -i holyarrs /holyarrs reset-owner` and follow the prompt.

</details>

<details>
<summary>HolyArrs can't reach qBittorrent or SABnzbd</summary>

Inside a container, `localhost` means the container itself. Use the downloader's container name (on the same Docker network) or the host's IP address.

</details>

**Where to ask**

- Need help → [Discord](https://discord.com/invite/mthsvesKpC)
- Idea or vote → [r/HolyArrs](https://reddit.com/r/HolyArrs) and the [roadmap](https://holyarrs.com/roadmap/)
- Bug → [Issues](https://github.com/HolyArrs/HolyArrs/issues) (bugs and roadmap only)
- Security → [SECURITY](.github/SECURITY.md)
- Anything else → support@HolyArrs.com

Every guide lives at [docs.HolyArrs.com](https://docs.HolyArrs.com).

## Contributing

Read [CONTRIBUTING](.github/CONTRIBUTING.md), then [Architecture](docs/dev/architecture.md) and [Setup](docs/dev/setup.md). Open an issue before a big change.

On your first pull request a bot asks you to accept a short licence agreement ([CLA.md](.github/CLA.md)). One click, once.

## Licence

HolyArrs is free and open source under the [GNU AGPL-3.0](LICENSE).

Want to build it into a closed product? Email support@HolyArrs.com for a commercial licence.

## Keep it going

HolyArrs is free, but it isn't free to run. Every install talks to a metadata server I pay for every month. Sponsors cover that, and buy more hours for the things you vote for.

<p align="center"><a href="https://github.com/sponsors/CoderLuii"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-sponsor-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-sponsor-light.png" height="34" alt="Sponsor HolyArrs" /></picture></a>&nbsp;<a href="https://holyarrs.com/roadmap/"><picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-vote-dark.png"><img src="https://raw.githubusercontent.com/HolyArrs/.github/main/profile/btn-vote-light.png" height="34" alt="Vote on the roadmap" /></picture></a></p>

---

<p align="center">Built by <a href="https://github.com/CoderLuii">CoderLuii</a>.<br><sub>If HolyArrs saves you from running six apps, a star helps other people find it.</sub></p>
