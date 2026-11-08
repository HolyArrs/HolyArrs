# Run HolyArrs on a NAS

You want HolyArrs on your NAS. Every NAS runs it as a Docker container, so read [Install HolyArrs with Docker](../getting-started/install.md) first. This page covers what changes on a NAS.

Pages for single boxes: [Unraid](unraid.md), [Synology](synology.md). QNAP and TrueNAS are below.

## Steps for every NAS

1. Make a config folder on a local volume of the NAS. Never use a network share for `/config`.
2. Make one shared folder for media, with `downloads`, `movies`, `tv` and `music` inside it.
3. Find the user and group that own your media. Over SSH, run `id <user>`.
4. Paste the Compose file from the Docker guide into your NAS's container app.
5. Set `PUID` and `PGID` to that user and group.
6. Give that user read and write on the media folder in the NAS permissions screen. HolyArrs takes care of the config folder itself.
7. Turn on auto-restart for the container (Docker's `unless-stopped` policy).
8. Open http://your-server:6262 and create the owner account.

## QNAP (Container Station)

1. Make `Container/holyarrs/config` and one `media` share.
2. In Container Station, open Applications, then Create, and paste the Compose file.
3. Use `/share/Container/holyarrs/config:/config` and `/share/media:/data`.
4. Set `PUID` and `PGID` to the owner of the `media` share.

## TrueNAS SCALE (custom app)

1. Make two datasets: `apps/holyarrs-config` and `media`. Put `downloads`, `movies`, `tv` and `music` inside `media`.
2. In Apps, add a custom app from YAML with the Compose file.
3. Use `/mnt/<pool>/apps/holyarrs-config:/config` and `/mnt/<pool>/media:/data`.
4. Set `PUID` and `PGID` to the dataset's owner. TrueNAS apps often use `568`. Give that user write access in the dataset ACL.

## Settings explained

**Config on local disk.** HolyArrs keeps its databases in `/config`. A network share or a FUSE path can corrupt them, so HolyArrs refuses to start on one and tells you why.

**One media mount for hardlinks.** When downloads and Library folders sit in one folder mounted once as `/data`, imports are hardlinks. They take no extra space and finish at once. Separate `/movies`, `/tv`, `/music` and `/downloads` mounts also work, but imports are then copies. See [Media folders](../getting-started/install.md#media-folders).

**Auto-restart.** Restart in the app, safe mode and a backup restore all stop HolyArrs and expect Docker to start it again. Without auto-restart it just stays stopped.

**Port.** Map port `6262` to your home network only. Read [Security basics](../getting-started/install.md#security-basics) before you open it to anyone outside.

**Updates.** Pull the new image and recreate the container. Your data stays in `/config`.

## Let your disks sleep

Many NAS boxes spin disks down when nothing reads them. These HolyArrs jobs reach the media disks. You can change how often each runs on the Schedules page.

- **Rescan the Library** lists every Library folder to find files you added, changed or removed by hand. This one wakes disks the most.
- **Check Library folders** reads each folder's details and free space. The write and hardlink test runs only at start-up, on Run now and when folders change.
- **Look for missing subtitles** and **Look for better subtitles** read the folder next to each video.
- Imports, adds and deletes touch the disk they work on, when they happen.

To let disks sleep longer:

- Put `/config` and the download folder on an SSD or cache pool.
- If `/config` shares a disk with downloads, keep at least 1 GB free. HolyArrs shows a health warning under 1 GB.
- On Unraid, the Cache Directories plugin keeps folder listings in memory, so scans can skip the disk.
- Turn the rescan off on the Schedules page, or set it to every 7 days. Files you add by hand are then found only when you press Run now on Rescan the Library.
- A torrent that is still seeding reads its file. If that file is a hardlink in your Library, the media disk stays awake until seeding stops.

## Troubleshooting

**HolyArrs refuses to start and names a FUSE path or network share.** Move `/config` to a local disk or pool path.

**HolyArrs stopped after a restart or restore.** Turn on auto-restart for the container and start it.

**Imports fail with a permission error.** The `PUID` user has no write access to the media folder. Fix it in the NAS permissions screen.

**Imports are copies, not hardlinks.** Downloads and the Library are on different mounts or disks. Use one shared folder mounted once.
