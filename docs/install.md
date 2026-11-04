# Install HolyArrs with Docker

HolyArrs v1 is Docker-only. The image is not published yet, so the commands
on this page build `holyarrs:local` from the current checkout. There is no
public registry tag or signed release to pull yet.

Development [portable candidates](portable.md) and an unpublished
[NAS template](nas-packages.md) can be prepared separately. Their build and
native acceptance receipts do not establish NAS platform support.

## Plan the two host locations

Keep the config folder on a local disk. HolyArrs uses SQLite in WAL mode and
refuses to start when `/config` is on a network share.

Give HolyArrs and every Downloader one shared data tree, mounted once at
`/data` in every container. A practical host layout is:

```text
/srv/holyarrs/config
/srv/media-stack/downloads
/srv/media-stack/library/movies
/srv/media-stack/library/shows
/srv/media-stack/library/music
```

The first path becomes `/config`. The whole `/srv/media-stack` tree becomes
`/data`, so the paths inside HolyArrs are:

```text
/data/downloads
/data/library/movies
/data/library/shows
/data/library/music
```

Do not mount the download and Library subfolders separately. Docker may then
present them as different mounts even when they are on the same host disk.
HolyArrs tries a hardlink first and falls back to a copy, so a split mount
uses space twice while a torrent seeds.

One `/data` mount is not enough when its subfolders are separate filesystems,
datasets or NAS shares. Hardlinks require the source and destination to be on
the same filesystem and the filesystem must support hardlinks. Settings >
Library folders reports a warning when HolyArrs has to copy instead.

## The image

The image is an empty base with only HolyArrs in it: the program, CA
certificates, the licences, `/config` and a writable `/tmp`. It runs as
`1000:1000` and has no shell and no other programs, so `docker exec` into it
to look around does not work. Read its log instead: `docker logs holyarrs`, or
`logs/holyarrs.log` in the config folder (see [Logs](logs.md)). HolyArrs' own
commands still work through `docker exec`, because they need no shell:
`docker exec -i holyarrs /holyarrs reset-owner`, `docker exec holyarrs /holyarrs
restore --pre-update` and `docker exec holyarrs /holyarrs import-secrets ...`. HolyArrs carries its own
time zone database (set `TZ` as usual), healthcheck, unpacker and
[trailer reader](trailers.md). Keep `/config` on a volume.

### Raspberry Pi and other ARM boards

The same image runs on 64-bit ARM (`linux/arm64`, a Raspberry Pi 4 or 5 with a
64-bit OS) and on 32-bit ARM (`linux/arm/v7`); Docker picks the right one. The
program is pure Go, so nothing extra is installed on the board. On a board or a
container with about 2.5 GiB of memory or less, HolyArrs turns on its
[low-memory mode](parts-and-safe-mode.md#low-memory-mode) by itself; its first
log line says so. Keep `/config` on the board's own disk or SSD, not on a
network share, and prefer an SSD over an SD card for it.

## Set permissions

The image runs as numeric user and group `1000:1000` by default. There are no
`PUID` or `PGID` environment variables. Create the folders so that this user
can write the config, download and Library paths:

```sh
sudo install -d -m 0750 -o 1000 -g 1000 /srv/holyarrs/config
sudo install -d -m 0775 -o 1000 -g 1000 /srv/media-stack/downloads
sudo install -d -m 0775 -o 1000 -g 1000 /srv/media-stack/library/movies
sudo install -d -m 0775 -o 1000 -g 1000 /srv/media-stack/library/shows
sudo install -d -m 0775 -o 1000 -g 1000 /srv/media-stack/library/music
```

If the Downloader uses another numeric user, give both containers a shared
group with write access, then set `user:` in the Compose file to the numeric
user and group HolyArrs should use. On a NAS, set the share ACLs to match
those numeric ids. Read access alone is not enough: HolyArrs creates folders,
hardlinks or copies files, and keeps short-lived probe files while checking a
Library folder.

## Build the local image

From the HolyArrs checkout directory:

```sh
docker build --tag holyarrs:local .
```

This tag is local to your Docker host. Rebuild it after updating the checkout.

## Make the Compose file

Save this as `compose.yaml` beside a directory where you keep your Docker
configuration. Replace the two host paths when your layout differs.

```yaml
services:
  holyarrs:
    image: holyarrs:local
    container_name: holyarrs
    user: "1000:1000"
    ports:
      - "18730:18730"
    environment:
      HOLYARRS_SAVE_PATH: /data/downloads
    volumes:
      - /srv/holyarrs/config:/config
      - /srv/media-stack:/data
    restart: unless-stopped
```

`HOLYARRS_SAVE_PATH` is the path where HolyArrs sees completed downloads. Set
the Downloader's save path to `/data/downloads` too. When the Downloader is in
another Compose project, give it the same host bind and the same container
path:

```yaml
volumes:
  - /srv/media-stack:/data
```

If an existing Downloader already sees this tree under another path, add a
path rule under Settings > Downloaders. Keeping `/data` identical in both
containers is simpler and makes hardlink problems easier to spot.

**Library > Import files by hand** reads only inside the Library folders, the
Downloader folders and the extra import folders. Without `HOLYARRS_SAVE_PATH`
(and without a path rule) the download folder is not one of them; add it under
Settings > Library folders > Extra import folders if you leave the variable
unset.

Start HolyArrs:

```sh
docker compose up -d
docker compose ps
```

The container becomes healthy after `GET /healthz` answers successfully.

## Finish the first run

Open port `18730` at the Docker host's network address. There are no default
credentials.

On the first start, HolyArrs writes a one-time code to
`/srv/holyarrs/config/setup-code.txt` and prints the same code in the container
log. Enter it on the setup page, then choose the owner name and password. The
code file is mode `0600` and is deleted after the owner is made.

The first-run screens then ask for:

1. The parts you use: Movies, Shows, Music, Subtitles and Requests.
2. One Library folder. Use a path inside the shared mount, such as
   `/data/library/movies`, `/data/library/shows` or `/data/library/music`.

Add the other Library folders later under Settings > Library folders. Add
Sources and Downloaders only after their addresses, keys and paths are ready.

## Keep the master key

HolyArrs encrypts stored passwords, keys and tokens. By default, the first
start creates `/config/holyarrs.key` with mode `0600`.

Backup zip files never contain this master key. Keep a protected copy apart
from the backup zips. A restore with a different key is refused because the
stored secrets cannot be opened.

You may supply the key as a separate read-only file instead. The named file
must already exist and must hold exactly 32 raw bytes, 64 hexadecimal digits,
or base64 for 32 bytes. For example:

```yaml
services:
  holyarrs:
    environment:
      HOLYARRS_MASTER_KEY_FILE: /keys/holyarrs.key
    volumes:
      - /srv/holyarrs/keys:/keys:ro
```

The container user must be able to read the file. To move an existing install
to an external key file, stop HolyArrs, copy the existing `holyarrs.key` to the
new protected location, set the environment variable, and start HolyArrs.
Keep the original until the app starts and the stored linked-app secrets work.

## Restart and restore

Use a restart policy. A restore is checked and staged, then HolyArrs exits
with status 75 so Docker starts it again and applies the staged databases.
The `unless-stopped` policy in the example handles that restart.

Make and restore backup zips under Settings > Backups. A backup contains the
main and History databases plus version information. It does not contain the
master key, media, downloads, artwork or logs. Before installing a restore,
HolyArrs keeps the replaced databases under
`/config/backups/before-restore-<time>/`.

After rebuilding `holyarrs:local`, recreate the container without changing
the mounts:

```sh
docker build --tag holyarrs:local .
docker compose up -d --force-recreate
```

Do not remove `/config` during an update. It holds the databases, settings,
master key, backups and logs.

## Host names and reverse proxies

IP addresses and `localhost` work without an allow-list entry. Add every DNS
name people use as a comma-separated value when starting the container:

```yaml
environment:
  HOLYARRS_ALLOWED_HOSTS: holyarrs.home.example
```

When a reverse proxy adds `X-Forwarded-For`, add only the proxy address or
network to `HOLYARRS_TRUSTED_PROXIES`. This matters when you enable the
optional local-network sign-in skip: an untrusted request cannot claim a
different source address.

To serve HolyArrs below a path, set `HOLYARRS_URL_BASE=/holyarrs`. The proxy
must pass that path unchanged, keep the `Host` header and leave the live
stream unbuffered. See [Running under a path](url-base.md) for nginx-proxy-
manager, Traefik and Caddy examples. Use HTTPS before exposing sign-in outside
your private network.

## NAS checks

- Keep `/config` on local storage even when `/data` is on a NAS.
- Put downloads and every Library folder that should hardlink in one dataset
  or filesystem, exported and mounted once.
- Give the HolyArrs numeric user write permission through the NAS ACL.
- Check Settings > Library folders after adding each folder. A hardlink
  warning means imports work by copying and use extra space.
- Do not point two independent HolyArrs containers at the same config folder.

## Security basics

- Do not forward port `18730` on your router. Reach HolyArrs from outside
  through a VPN such as Tailscale, or through a reverse proxy with HTTPS
  (see [Host names and reverse proxies](#host-names-and-reverse-proxies)).
- Treat `setup-code.txt` like a password until the owner is made; it is
  deleted afterwards.
- Back up the master key away from the config folder
  ([Keep the master key](#keep-the-master-key)).
- Keep the image up to date ([updates](updates.md)).
- Report security problems privately as described in
  [SECURITY.md](../SECURITY.md).

On a Synology, QNAP, TrueNAS or Unraid box, see [Install on a NAS](nas.md).

## If startup fails

| Message or symptom | Check |
| --- | --- |
| Config folder is on a network share | Move `/config` to a local disk. Keep only `/data` on the share. |
| Permission denied | Check the numeric `user:` and write permissions on both mounts. |
| Unknown host name | Add the exact DNS name to `HOLYARRS_ALLOWED_HOSTS`. |
| Imports are copied | Put downloads and the Library folder on one filesystem and one `/data` mount. |
| Master key does not match | Restore the key that belongs to those databases. Do not make a new key for an old backup. |
| Restore stages but the app stays down | Start the container and add a restart policy. |

The rest of the screen-by-screen guides are listed in the
[documentation index](index.md).
