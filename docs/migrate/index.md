---
title: Move from other apps
description: Copy your setup from Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent or SABnzbd, then undo it if you want.
level: Start
sidebar: { order: 1 }
---

Bring your old apps' setup into HolyArrs without typing it again.

HolyArrs only reads the old apps. Nothing in them changes, and your media files never move.

## Before you start

- Sign in as the owner or an admin.
- Have each old app's address and API key ready.
- Make sure HolyArrs can see your media folders. See [Folders and hardlinks](../start/folders.md).

## Move your setup

1. Open **Settings > Move from other apps**. You should see five steps in a row at the top.
2. **Read:** click **Add an app** for each old app. You should see each app listed with a switch.
3. **Choose:** turn on the groups you want to bring over.
4. **Try it:** check what will be added. Nothing changes yet.
5. **Move:** click **Move** and confirm. You can leave the page while it runs.
6. **Done:** you should see the results per group and **Undo this move**.

Keys and passwords never come over. After the move, type them again and turn each Source, Downloader and Alert on.

## Pages in this section

| Page | Use it to |
| --- | --- |
| [Sonarr and Radarr](sonarr-radarr.md) | Bring over folders, shows, movies and their rules. Do this first. |
| [Lidarr, Bazarr and Prowlarr](lidarr-bazarr-prowlarr.md) | Bring over music, subtitle languages and Sources. |
| [Seerr and Overseerr](requests-apps.md) | Bring over people and open requests. |
| [Read your old apps](what-is-read.md) | See exactly what HolyArrs reads and keeps. |
| [Import keys from a file](import-secrets.md) | Type many keys at once. |

## Related

- [First run](../start/first-run.md)
- [Glossary](../help/glossary.md)
