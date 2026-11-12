---
title: NAS, Synology and Unraid
description: Run the HolyArrs container on Synology, Unraid, QNAP or TrueNAS with one shared data folder, the same as the install page.
level: Start
sidebar: { order: 8 }
---

Run HolyArrs on your NAS with the right folders, owner and restart setting. Read [Install](../start/install.mdx) first.

## Before you start

- `/config` goes on a local volume or pool. Never a network share.
- Downloads and media go in one shared folder, `data`, with `downloads` and `media` inside, and `movies`, `tv` and `music` inside `media`. Mount it once at `/data`. See [Folders](../start/folders.md).

## Set up your NAS

:::tabs
::tab[Synology]
1. In File Station, make `docker/holyarrs/config`.
2. Make one shared folder `data` with `downloads` and `media` inside, and `movies`, `tv` and `music` inside `media`.
3. In Container Manager, open **Project > Create** and paste the Compose file from [Install](../start/install.mdx).
4. Set the volumes to `/volume1/docker/holyarrs/config:/config` and `/volume1/data:/data`.
5. Set `PUID` and `PGID` to your user, often `1026` and `100`. Check with `id <user>` over SSH.
6. Give that user read and write on `data`: **Control Panel > Shared Folder > Edit > Permissions**.
7. Tick **Enable auto-restart**.
8. Open `http://your-server:6262`. You should see the first-run wizard. Add `/data/media/movies`, `/data/media/tv` and `/data/media/music` as Library folders.

No Container Manager on your model? Try the experimental [Synology package](synology.md).

::tab[Unraid]
There is no Community Applications template yet, so add the container by hand.

1. In the Docker tab, add a container with the HolyArrs image.
2. Map port `6262`.
3. Map `/config` to a pool path, such as `/mnt/<pool>/appdata/holyarrs`. `<pool>` is your pool's name. Not `/mnt/user/appdata`.
4. Map one data folder to `/data`, using a disk or pool path such as `/mnt/<pool>/data:/data`. Inside it, make `downloads` and `media`, with `movies`, `tv` and `music` inside `media`.
5. Set `PUID=99`, `PGID=100` (Unraid's `nobody:users`) and `UMASK=022`.
6. Turn on auto-start for the container, so it restarts on its own.
7. Open `http://your-server:6262`. You should see the first-run wizard. Add `/data/media/movies`, `/data/media/tv` and `/data/media/music` as Library folders.

::tab[QNAP]
1. Make `Container/holyarrs/config` and one `data` share with `downloads` and `media` inside, and `movies`, `tv` and `music` inside `media`.
2. In Container Station, open **Applications > Create** and paste the Compose file.
3. Use `/share/Container/holyarrs/config:/config` and `/share/data:/data`.
4. Set `PUID` and `PGID` to the owner of the `data` share.
5. Set the restart policy to `unless-stopped`.
6. Open `http://your-server:6262`. You should see the first-run wizard. Add `/data/media/movies`, `/data/media/tv` and `/data/media/music` as Library folders.

::tab[TrueNAS SCALE]
1. Make 2 datasets: `apps/holyarrs-config` and `data`. Inside `data`, make plain folders, not datasets: `downloads` and `media`, with `movies`, `tv` and `music` inside `media`.
2. In Apps, add a custom app from YAML with the Compose file.
3. Use `/mnt/<pool>/apps/holyarrs-config:/config` and `/mnt/<pool>/data:/data`.
4. Set `PUID` and `PGID` to the dataset's owner, often `568`. Give that user write access in the dataset ACL.
5. Set the restart policy to `unless-stopped`.
6. Open `http://your-server:6262`. You should see the first-run wizard. Add `/data/media/movies`, `/data/media/tv` and `/data/media/music` as Library folders.
:::

Every NAS: give the `PUID` user read and write on the data folder in the NAS permissions screen; HolyArrs takes care of the config folder itself. Turn on auto-restart for the container (Docker's `unless-stopped` policy).

Give your downloader the same mount at the same path, `/data`, and set its save folder inside `/data/downloads`. Then imports are hardlinks or moves: no extra space, done at once. See [Media folders](../start/install.mdx#media-folders).

## Choices

### Separate mounts

Mounting `/movies`, `/tv`, `/music` and `/downloads` one by one works, but every import becomes a full copy. Each mount is its own filesystem inside the container, and a hardlink can't cross filesystems. Separate shares or datasets under one parent copy too. **Settings > Library folders** warns when HolyArrs has to copy. To switch later without importing again, see [Move to one data folder](../library/move-to-one-folder.md).

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
| Imports are copies, not hardlinks | Downloads and Library are on different mounts or disks | Mount one shared folder once, in HolyArrs and the downloader. Already set up? See [Move to one data folder](../library/move-to-one-folder.md). |

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
