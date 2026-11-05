# Install HolyArrs on a NAS

Every NAS below runs HolyArrs as a Docker container, so first read
[Install HolyArrs with Docker](install.md). This page only says where things
go on each box.

The same three rules hold on every NAS:

- `/config` sits on a local volume of the NAS, never on a network share.
- Set `PUID` and `PGID` to the NAS user and group that own your media (find
  them with `id <user>` over SSH). HolyArrs gives the config folder to that
  user itself; give the same user read and write on the media folders
  through the NAS permissions screen.
- For hardlinks instead of copies, keep downloads and every Library folder in
  one shared folder and mount it once (see
  [Media folders](install.md#media-folders)). Separate `/movies`, `/tv`,
  `/music` and `/downloads` mounts work too, but imports are then copied.

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

- Put `/config` on a pool path such as `/mnt/cache/appdata/holyarrs`.
- The template offers optional `/movies`, `/tv`, `/music` and `/downloads`
  paths. For hardlinks, map one share holding all of them instead, from a
  disk or pool path. Do not use `/mnt/user` paths for that: hardlinks across
  them can fail.

## Ports and updates

Map port `6262` only to your home network. Updates follow the Docker guide:
rebuild or pull the new image, then recreate the container. Your data stays
in `/config`.

See [Security basics](install.md#security-basics) before opening HolyArrs to
anyone outside your home.
