# Install HolyArrs on a NAS

Every NAS below runs HolyArrs as a Docker container, so first read
[Install HolyArrs with Docker](install.md). This page only says where things
go on each box.

The same four rules hold on every NAS:

- `/config` sits on a local volume of the NAS, never on a network share.
- Set `PUID` and `PGID` to the NAS user and group that own your media (find
  them with `id <user>` over SSH). HolyArrs gives the config folder to that
  user itself; give the same user read and write on the media folders
  through the NAS permissions screen.
- For hardlinks instead of copies, keep downloads and every Library folder in
  one shared folder and mount it once (see
  [Media folders](install.md#media-folders)). Separate `/movies`, `/tv`,
  `/music` and `/downloads` mounts work too, but imports are then copied.
- Turn on auto-restart for the container (Docker's `unless-stopped` restart
  policy). Restart in the app, safe mode and a restore stop HolyArrs and
  count on the NAS to start it again; without auto-restart it just looks
  stopped. In Synology Container Manager this is "Enable auto-restart"; the
  Unraid template sets it for you.

## Synology (Container Manager)

1. In File Station, make `docker/holyarrs/config` and one shared folder for
   media, for example `media` with `downloads`, `movies`, `tv` and `music`
   inside.
2. Build or load the `holyarrs:local` image, then in Container Manager open
   Project, Create, and paste the Compose file from the Docker guide.
3. Use `/volume1/docker/holyarrs/config:/config` and
   `/volume1/media:/data` as the volumes, and pick `/data/movies` and the
   others as Library folders in the app.
4. Set `PUID` and `PGID` to your user (often `1026` and `100`) and give that
   user read and write on the media folder (Control Panel, Shared Folder,
   Edit, Permissions).

## QNAP (Container Station)

1. Make `Container/holyarrs/config` and one `media` share.
2. In Container Station, Applications, Create, paste the Compose file.
3. Use `/share/Container/holyarrs/config:/config` and `/share/media:/data`.
4. Set `PUID` and `PGID` to the user and group that own the `media` share.

## TrueNAS SCALE (custom app)

1. Make two datasets: `apps/holyarrs-config` and `media`. Put `downloads`,
   `movies`, `tv` and `music` inside `media`, so all are on one dataset.
2. In Apps, add a custom app (or install it from YAML) with the Compose file.
3. Use `/mnt/<pool>/apps/holyarrs-config:/config` and
   `/mnt/<pool>/media:/data`.
4. Set `PUID` and `PGID` to the dataset's owner (TrueNAS apps often use
   `568`) and give that user write access in the dataset ACL.

## Unraid

A template is being prepared; see [NAS packages](nas-packages.md). It sets
`PUID=99` and `PGID=100` (Unraid's `nobody:users`) and `UMASK=022`, shown
under the advanced view.

- Put `/config` on a pool path such as `/mnt/<pool>/appdata/holyarrs`
  (`<pool>` is your pool's name), or set the appdata share to Exclusive
  access (first turn on Permit exclusive shares under Settings > Global
  Share Settings, then Shares > appdata > Primary storage on one pool,
  Secondary storage none; Unraid then shows it as an Exclusive share). HolyArrs refuses
  `/mnt/user/appdata`, which is a FUSE path, and a pool path that does not
  exist (it lands in memory).
- The template offers optional `/movies`, `/tv`, `/music` and `/downloads`
  paths. For hardlinks, map one share holding all of them instead, from a
  disk or pool path. Do not use `/mnt/user` paths for that: hardlinks across
  them can fail.

## Disks that sleep

Many NAS boxes spin their disks down when nothing reads them. These are the
things HolyArrs does on its own that reach the media disks, with their usual
times (all can be changed on the Schedules page):

- **Rescan the Library**, once a day: lists every Library folder to find files
  added, changed or removed by hand. This is the one that wakes disks most.
- **Check Library folders**, every 6 hours: reads each folder's details and
  free space only. The write and hardlink test runs at start-up, on Run now
  and when the download folders change.
- **Look for missing subtitles** (every 6 hours) and **better subtitles**
  (every 12 hours): list the folder beside each video. A video is read again
  only when its size or time changed.
- Imports, adds and deletes touch the disk they work on, when they happen.

To let disks sleep longer:

- Put `/config` and the download folder on an SSD or a cache pool, not on the
  media disks. On Unraid, use a pool path for `/config` and the downloads
  share.
- On Unraid, the Cache Directories plugin keeps folder listings in memory, so
  the rescan and folder checks can be answered without waking a disk.
- Turn the rescan off on the Schedules page, or set it to 7 days. Files you add
  by hand are then found only when you press Run now on Rescan the Library in
  Settings > Schedules.
- A torrent that is still seeding reads from its file. When that file is a
  hardlink into the Library, it sits on the media disk, so that disk stays
  awake until seeding stops.

## Ports and updates

Map port `6262` only to your home network. Updates follow the Docker guide:
rebuild or pull the new image, then recreate the container. Your data stays
in `/config`.

See [Security basics](install.md#security-basics) before opening HolyArrs to
anyone outside your home.
