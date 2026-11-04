# Install HolyArrs on a NAS

Every NAS below runs HolyArrs as a Docker container, so first read
[Install HolyArrs with Docker](install.md). This page only says where things
go on each box.

The same three rules hold on every NAS:

- `/config` sits on a local volume of the NAS, never on a network share.
- One shared folder holds downloads and every Library folder, mounted into
  the container once as `/data`, so files can be hardlinked instead of copied.
- The container runs as `1000:1000`. Give that user and group read and write
  on both folders through the NAS permissions screen.

## Synology (Container Manager)

1. In File Station, make `docker/holyarrs/config` and one shared folder for
   media, for example `media` with `downloads` and `library` inside.
2. Build or load the `holyarrs:local` image, then in Container Manager open
   Project, Create, and paste the Compose file from the Docker guide.
3. Use `/volume1/docker/holyarrs/config:/config` and
   `/volume1/media:/data` as the two volumes.
4. Give user id 1000 read and write on both folders (Control Panel, Shared
   Folder, Edit, Permissions, or `chown -R 1000:1000` over SSH).

## QNAP (Container Station)

1. Make `Container/holyarrs/config` and one `media` share.
2. In Container Station, Applications, Create, paste the Compose file.
3. Use `/share/Container/holyarrs/config:/config` and `/share/media:/data`.
4. Set the owner of both folders to 1000:1000.

## TrueNAS SCALE (custom app)

1. Make two datasets: `apps/holyarrs-config` and `media`. Put `downloads`
   and `library` inside `media`, so both are on one dataset.
2. In Apps, add a custom app (or install it from YAML) with the Compose file.
3. Use `/mnt/<pool>/apps/holyarrs-config:/config` and
   `/mnt/<pool>/media:/data`.
4. Give user and group 1000 write access in the dataset ACL.

## Unraid

A template is being prepared; see [NAS packages](nas-packages.md).

- Put `/config` on a pool path such as `/mnt/cache/appdata/holyarrs`.
- Use one share for `/data`, mounted from a disk or pool path. Do not use
  `/mnt/user` paths: hardlinks across them can fail.

## Ports and updates

Map port `18730` only to your home network. Updates follow the Docker guide:
rebuild or pull the new image, then recreate the container. Your data stays
in `/config`.

See [Security basics](install.md#security-basics) before opening HolyArrs to
anyone outside your home.
