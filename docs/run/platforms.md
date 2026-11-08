---
title: NAS, Synology and Unraid
description: Run the HolyArrs container on Synology, Unraid, QNAP or TrueNAS with separate media mounts, the same as the install page.
level: Start
sidebar: { order: 8 }
---

Run HolyArrs on your NAS with the right folders, owner and restart setting. Read [Install](../start/install.mdx) first.

## Before you start

- `/config` goes on a local volume or pool. Never a network share.
- Media goes in one shared folder with `downloads`, `movies`, `tv` and `music` inside. Mount each one, or the parent once for hardlinks. See [Folders](../start/folders.md).

## Set up your NAS

:::tabs
::tab[Synology]
1. In File Station, make `docker/holyarrs/config`.
2. Make one shared folder `media` with `downloads`, `movies`, `tv` and `music` inside.
3. In Container Manager, open **Project > Create** and paste the Compose file from [Install](../start/install.mdx).
4. Set the volumes to `/volume1/docker/holyarrs/config:/config`, then `/volume1/media/movies:/movies`, `/volume1/media/tv:/tv`, `/volume1/media/music:/music` and `/volume1/media/downloads:/downloads`.
5. Set `PUID` and `PGID` to your user, often `1026` and `100`. Check with `id <user>` over SSH.
6. Give that user read and write on `media`: **Control Panel > Shared Folder > Edit > Permissions**.
7. Tick **Enable auto-restart**.
8. Open `http://your-server:6262`. You should see the first-run wizard. Add `/movies`, `/tv` and `/music` as Library folders.

::tab[Unraid]
There is no Community Applications template yet, so add the container by hand.

1. In the Docker tab, add a container with the HolyArrs image.
2. Map port `6262`.
3. Map `/config` to a pool path, such as `/mnt/<pool>/appdata/holyarrs`. `<pool>` is your pool's name. Not `/mnt/user/appdata`.
4. Map your media folders to `/movies`, `/tv`, `/music` and `/downloads`, using disk or pool paths, for example `/mnt/<pool>/media/movies:/movies`, `/mnt/<pool>/media/tv:/tv`, `/mnt/<pool>/media/music:/music` and `/mnt/<pool>/media/downloads:/downloads`.
5. Set `PUID=99`, `PGID=100` (Unraid's `nobody:users`) and `UMASK=022`.
6. Turn on auto-start for the container, so it restarts on its own.
7. Open `http://your-server:6262`. You should see the first-run wizard.

::tab[QNAP]
1. Make `Container/holyarrs/config` and one `media` share with `downloads`, `movies`, `tv` and `music` inside.
2. In Container Station, open **Applications > Create** and paste the Compose file.
3. Use `/share/Container/holyarrs/config:/config`, then `/share/media/movies:/movies`, `/share/media/tv:/tv`, `/share/media/music:/music` and `/share/media/downloads:/downloads`.
4. Set `PUID` and `PGID` to the owner of the `media` share.
5. Set the restart policy to `unless-stopped`.
6. Open `http://your-server:6262`. You should see the first-run wizard.

::tab[TrueNAS SCALE]
1. Make 2 datasets: `apps/holyarrs-config` and `media`, with `downloads`, `movies`, `tv` and `music` folders inside `media`.
2. In Apps, add a custom app from YAML with the Compose file.
3. Use `/mnt/<pool>/apps/holyarrs-config:/config`, then `/mnt/<pool>/media/movies:/movies`, `/mnt/<pool>/media/tv:/tv`, `/mnt/<pool>/media/music:/music` and `/mnt/<pool>/media/downloads:/downloads`.
4. Set `PUID` and `PGID` to the dataset's owner, often `568`. Give that user write access in the dataset ACL.
5. Set the restart policy to `unless-stopped`.
6. Open `http://your-server:6262`. You should see the first-run wizard.
:::

Every NAS: give the `PUID` user read and write on the media folder in the NAS permissions screen; HolyArrs takes care of the config folder itself. Turn on auto-restart for the container (Docker's `unless-stopped` policy).

For hardlinks, which take no extra space and finish at once, mount the one media folder in place of the four, and give your downloader the same mount at the same path. See [Media folders](../start/install.mdx#media-folders).

## Choices

### Separate mounts

Mounting `/movies`, `/tv`, `/music` and `/downloads` one by one works, but every import becomes a full copy. Each mount is its own filesystem inside the container, and a hardlink can't cross filesystems. Separate shares or datasets under one parent copy too. **Settings > Library folders** warns when HolyArrs has to copy.

### Let your disks sleep

These jobs wake the media disks. Change how often they run on the **Schedules** page.

| Job | What it reads |
|---|---|
| **Rescan the Library** | Every Library folder. Wakes disks the most. |
| **Check Library folders** | Each folder's details and free space. The write and hardlink test runs only at start, on **Run now** and when folders change. |
| **Look for missing subtitles**, **Look for better subtitles** | The folder next to each video. |

- Put `/config` and the download folder on an SSD or cache pool.
- Keep 1 GB free on the disk that holds `/config`. Below that you get a health warning.
- On Unraid, the Cache Directories plugin keeps folder listings in memory, so scans skip the disk.
- Imports, adds and deletes touch the disk they work on, when they happen.
- Set the rescan to every 7 days or off. Files you add by hand are then found only when you press **Run now** on **Rescan the Library**.
- A seeding torrent reads its file. If it is hardlinked into your Library, that disk stays awake until seeding stops.

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| Won't start, names a FUSE path, network share or Unraid user share | `/config` is not on local disk (on Unraid, it points at `/mnt/user`) | Move `/config` to a local disk or pool path, such as `/mnt/<pool>/appdata/holyarrs`. |
| Stays stopped after a restart or restore | Auto-restart is off | Turn it on and start the container. |
| Imports fail with a permission error | The `PUID` user can't write to media | Give it read and write in the NAS permissions screen (on Synology: **Control Panel > Shared Folder > Edit > Permissions**). |
| Imports are copies, not hardlinks | Downloads and Library are on different mounts or disks | Mount one shared folder once, in HolyArrs and the downloader. |

More fixes: [Troubleshooting](../help/troubleshooting.md).

## For experts

<details class="ha-more"><summary>Why HolyArrs refuses some config paths</summary>

HolyArrs keeps its databases in `/config`. Network shares and FUSE paths can corrupt them, so HolyArrs refuses to start on one and says why.

On Unraid, `/mnt/user` is a FUSE path. Use a pool path, or make appdata Exclusive: turn on **Permit exclusive shares** in **Settings > Global Share Settings**, then set **Shares > appdata** to primary storage on one pool and secondary storage to none.

A `/config` path on a pool that does not exist lands in memory and is lost on reboot. HolyArrs refuses that too. Avoid `/mnt/user` for media as well; hardlinks across it can fail.

</details>

<details class="ha-more"><summary>Why auto-restart matters</summary>

Restart in the app, safe mode and a backup restore all stop HolyArrs and expect Docker to start it again. Without auto-restart it stays stopped. See [Parts and safe mode](parts-safe-mode.md).

</details>

<details class="ha-more"><summary>Port and updates</summary>

- Map port `6262` to your home network only. Read [Security basics](../start/install.mdx#security-basics) before opening it to anyone outside.
- To update, pull the new image and recreate the container. Your data stays in `/config`. See [Updates](updates.md).

</details>

## Related

- [Folders](../start/folders.md)
- [Library folders](../library/folders.md)
- [Schedules](schedules.md)
