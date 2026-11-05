# Install HolyArrs with Docker

HolyArrs v1 is Docker-only. The image is not published yet:
`ghcr.io/coderluii/holyarrs:beta` in the examples below is a placeholder for
the future registry name. Until the first stable version, new versions land on
`:beta`; `:latest` and `:stable` only move once a version has been tried for a
while. Until the image exists, build `holyarrs:local` from the
current checkout (see [Build the local image](#build-the-local-image)) and use
that tag in its place.

Development [portable candidates](portable.md) and an unpublished
[NAS template](nas-packages.md) can be prepared separately. Their build and
native acceptance receipts do not establish NAS platform support.

## The image

The image is an empty base with only HolyArrs in it: the program, CA
certificates, the licences, `/config` and a writable `/tmp`. It has no shell
and no other programs, so `docker exec` into it to look around does not work.
Read its log instead: `docker logs holyarrs`, or `logs/holyarrs.log` in the
config folder (see [Logs](logs.md)).

The container starts as root only long enough to set itself up: it fixes the
owner of `/config` when needed, then switches to the user and group from
`PUID` and `PGID` (1000 and 1000 when you leave them out) and runs as that
user from then on. Its first log line shows the user, group and umask it ended
up with. See [User / Group identifiers](#user--group-identifiers).

HolyArrs' own commands still work through `docker exec`, because they need no
shell: `docker exec -i holyarrs /holyarrs reset-owner`,
`docker exec holyarrs /holyarrs restore --pre-update` and
`docker exec holyarrs /holyarrs import-secrets ...`. They switch to the same
user first, so running them as root does not leave root-owned files behind.
HolyArrs carries its own time zone database (set `TZ` as usual), healthcheck,
unpacker and [trailer reader](trailers.md).

### ARM machines

Release images are built for 64-bit ARM (`linux/arm64`) and 32-bit ARM
(`linux/arm/v7`) as well as amd64. On a machine or a container with about
2.5 GiB of memory or less, HolyArrs turns on its
[low-memory mode](parts-and-safe-mode.md#low-memory-mode) by itself; its first
log line says so.

## Build the local image

From the HolyArrs checkout directory:

```sh
docker build --tag holyarrs:local .
```

This tag is local to your Docker host. Rebuild it after updating the checkout.

## Usage

Keep the config folder on a local disk. HolyArrs uses SQLite in WAL mode and
refuses to start when `/config` is on a network share.

### Docker Compose (recommended)

```yaml
services:
  holyarrs:
    image: ghcr.io/coderluii/holyarrs:beta
    container_name: holyarrs
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /path/to/holyarrs/config:/config
      - /path/to/movies:/movies #optional
      - /path/to/tv:/tv #optional
      - /path/to/music:/music #optional
      - /path/to/downloads:/downloads #optional
    ports:
      - 6262:6262
    restart: unless-stopped
```

### Docker CLI

```sh
docker run -d \
  --name=holyarrs \
  -e PUID=1000 \
  -e PGID=1000 \
  -e TZ=Etc/UTC \
  -p 6262:6262 \
  -v /path/to/holyarrs/config:/config \
  -v /path/to/movies:/movies `#optional` \
  -v /path/to/tv:/tv `#optional` \
  -v /path/to/music:/music `#optional` \
  -v /path/to/downloads:/downloads `#optional` \
  --restart unless-stopped \
  ghcr.io/coderluii/holyarrs:beta
```

Start it with `docker compose up -d` and check it with `docker compose ps`.
The container becomes healthy after `GET /healthz` answers successfully.

## Parameters

| Parameter | What it does |
| --- | --- |
| `-p 6262:6262` | The web app. |
| `-e PUID=1000` | User id HolyArrs runs as. See [User / Group identifiers](#user--group-identifiers). |
| `-e PGID=1000` | Group id HolyArrs runs as. |
| `-e TZ=Etc/UTC` | Time zone, for example `Europe/Amsterdam`. |
| `-e UMASK=022` | Optional. Which permission bits new files and folders leave out. See [Umask](#umask). |
| `-e HOLYARRS_SAVE_PATH=/downloads` | Optional. The Downloader's save folder as the Downloader sees it; HolyArrs sends it with every download. Leave it out to keep the Downloader's own setting. See [Media folders](#media-folders). |
| `-v /config` | Databases, settings, master key, backups and logs. Local disk only. |
| `-v /movies` | Optional. A movie Library folder. |
| `-v /tv` | Optional. A shows Library folder. |
| `-v /music` | Optional. A music Library folder. |
| `-v /downloads` | Optional. The Downloader's download folder. |
| `--read-only=true` | Optional. Run with a read-only root filesystem. See [Read-only operation](#read-only-operation). |
| `--user=1000:1000` | Optional. Start as this user instead of root. See [Non-root operation](#non-root-operation). |

## Environment variables from files (FILE__)

Any setting can come from a file instead of the Compose file. Put `FILE__` in
front of the variable name and give the path of the file as its value.
HolyArrs reads the file and uses its contents as the value of the variable
without the prefix. One newline at the end of the file is dropped, so a file
written with `echo` works.

```yaml
environment:
  - FILE__HOLYARRS_ALLOWED_HOSTS=/run/secrets/holyarrs_hosts
```

This happens before anything else at start, so it works for `PUID`, `PGID`,
`UMASK` and `TZ` too. When the file is missing or cannot be read, HolyArrs
stops and names the variable in the log. It never prints the value.

The master key has its own file setting, `HOLYARRS_MASTER_KEY_FILE`; see
[Keep the master key](#keep-the-master-key).

## Umask

`UMASK` decides which permission bits new files and folders leave out. Leave
it out to keep the container's own umask, which is `022` under Docker: you can
write, everyone else can read. Set `UMASK=002` when other containers or people
share files with HolyArrs through a group and must be able to write them too.
Write it as an octal number, such as `022` or `002`.

`UMASK` only changes new files. It does not change files that already exist.

## User / Group identifiers

HolyArrs runs as the user and group you give in `PUID` and `PGID`. Use the
same ids that own your media and download folders on the host, so files
HolyArrs makes are yours and not root's. Find them with `id`:

```sh
id youruser
# uid=1000(youruser) gid=1000(youruser) groups=1000(youruser)
```

Use the `uid` as `PUID` and the `gid` as `PGID`. On Unraid that is usually
`99` and `100`; on Synology the first user is often `1026` and group `100`.

At start, HolyArrs checks who owns the top of the config folder. When it is
not `PUID:PGID` yet, for example on the first start or after you change the
ids, HolyArrs walks the config folder once and gives it to that user. When the
top is already right it does nothing, so normal starts stay fast. It never
changes the owner of your library or download folders: give the `PUID` user
write access to those yourself. Read access alone is not enough, because
HolyArrs creates folders, hardlinks or copies files, and keeps short-lived
probe files while checking a Library folder.

On some storage the owner cannot be changed (NFS with root squash, SMB, a
read-only mount, Docker Desktop folders). HolyArrs then logs a warning and
carries on; make sure the config folder is writable by `PUID:PGID` by hand.

Extra groups given with `group_add:` in Compose (or `--group-add`) are kept
when HolyArrs switches user.

HolyArrs needs the `CHOWN`, `DAC_OVERRIDE`, `SETUID` and `SETGID`
capabilities to switch user (`DAC_OVERRIDE` lets it enter config folders
that only their owner can open). Docker gives them by default. If you drop them (for example
`cap_drop: ALL`), HolyArrs stops with a message instead of running as root.
Add those four back, or use [Non-root operation](#non-root-operation).
To really run as root, set both `PUID=0` and `PGID=0`; HolyArrs warns about
it once.

## Read-only operation

HolyArrs can run with a read-only root filesystem. Only `/config` and `/tmp`
need to be writable. Give `/tmp` a tmpfs:

```yaml
services:
  holyarrs:
    read_only: true
    tmpfs:
      - /tmp
```

or `--read-only=true --tmpfs /tmp` with `docker run`. `PUID`, `PGID` and
`UMASK` still work, because switching user does not write anything inside the
image.

## Non-root operation

You can start the container as a fixed user instead of root:

```yaml
services:
  holyarrs:
    user: "1000:1000"
```

or `--user=1000:1000` with `docker run`. The same applies to rootless Docker
and to Kubernetes `runAsUser`. HolyArrs then runs as that user and changes no
owners at all, so the config folder must already be writable by it.
`PUID` and `PGID` are ignored; when they are set, HolyArrs logs one line
saying so. `UMASK` still works.

## Media folders

Each folder you mount separately (`/movies`, `/tv`, `/music`, `/downloads`) is
its own mount inside the container, even when the folders sit on the same
disk on the host. A file cannot be hardlinked from one mount to another, so
moving a finished download into the Library is a full copy, and a torrent
that keeps seeding uses its space twice.

To get hardlinks, mount one shared parent folder instead, with the downloads
and the Library folders inside it, and pick the Library folders inside it in
the app. For example, a host folder `/srv/media` with `downloads`, `movies`,
`tv` and `music` in it:

```yaml
volumes:
  - /path/to/holyarrs/config:/config
  - /srv/media:/data
```

Then use `/data/movies`, `/data/tv` and `/data/music` as Library folders and
`/data/downloads` as the download folder. Give the Downloader the same mount
at the same path, so both containers name every file the same way. Hardlinks
also need the shared folder to be one filesystem: separate datasets or NAS
shares under it still copy. Settings > Library folders warns when HolyArrs has
to copy.

`HOLYARRS_SAVE_PATH` is the Downloader's save folder as the Downloader sees
it, for example `/downloads` or `/data/downloads`. HolyArrs sends it to the
Downloader with every download, so it also changes where the Downloader saves;
leave it unset to keep the Downloader's own setting. HolyArrs reads finished
downloads from the same path, so the path must be the same in both
containers, or add a path rule under Settings > Downloaders.
**Library > Import files by hand** reads only inside the Library folders, the
Downloader folders and the extra import folders. Without `HOLYARRS_SAVE_PATH`
(and without a path rule under Settings > Downloaders) the download folder is
not one of them; add it under Settings > Library folders > Extra import
folders if you leave the variable unset.

## Finish the first run

Open port `6262` at the Docker host's network address. There are no default
credentials.

On the first start, HolyArrs writes a one-time code to `setup-code.txt` in
the config folder and prints the same code in the container log. Enter it on
the setup page, then choose the owner name and password. The code file is
mode `0600` and is deleted after the owner is made.

The first-run screens then ask for:

1. The parts you use: Movies, Shows, Music, Subtitles and Requests.
2. One Library folder, as HolyArrs sees it inside the container, such as
   `/movies`, `/tv` or `/music`.

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
      - HOLYARRS_MASTER_KEY_FILE=/keys/holyarrs.key
    volumes:
      - /path/to/holyarrs/keys:/keys:ro
```

The `PUID` user must be able to read the file. To move an existing install
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
  - HOLYARRS_ALLOWED_HOSTS=holyarrs.home.example
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

- Keep `/config` on local storage even when the media folders are on a NAS.
- Set `PUID` and `PGID` to the NAS user and group that own the media, and give
  them write access through the NAS ACL.
- For hardlinks, put downloads and every Library folder in one dataset or
  filesystem and mount it once (see [Media folders](#media-folders)).
- Check Settings > Library folders after adding each folder. A hardlink
  warning means imports work by copying and use extra space.
- Do not point two independent HolyArrs containers at the same config folder.

## Security basics

- Do not forward port `6262` on your router. Reach HolyArrs from outside
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

Run the doctor first: it checks the config, Library and download folders, the
network and the clock, and says how to fix each problem. If the container is
running, use `docker exec holyarrs /holyarrs doctor`. If it keeps stopping or
restarting, `docker exec` cannot reach it; run the doctor once from the same
image instead, with the same `PUID`, `PGID` and `TZ` and the tag you run:
`docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> -e TZ=<tz> ghcr.io/coderluii/holyarrs:<tag> doctor`
(with Compose: `docker compose run --rm holyarrs doctor`).
[Support info](support.md) covers the doctor, the log and the support bundle.

| Message or symptom | Check |
| --- | --- |
| Config folder is on a network share | Move `/config` to a local disk. Keep only the media folders on the share. |
| Cannot switch user | Add back the `CHOWN`, `DAC_OVERRIDE`, `SETUID` and `SETGID` capabilities, or start the container with `user: "<uid>:<gid>"`. |
| A `FILE__` variable cannot be read | Check the path in that variable and that the file is mounted. |
| Permission denied | Check `PUID`, `PGID` and write permissions on the config, library and download folders. |
| Unknown host name | Add the exact DNS name to `HOLYARRS_ALLOWED_HOSTS`. |
| Imports are copied | Mount downloads and the Library folders as one shared folder on one filesystem (see [Media folders](#media-folders)). |
| Master key does not match | Restore the key that belongs to those databases. Do not make a new key for an old backup. |
| Restore stages but the app stays down | Start the container and add a restart policy. |

The rest of the screen-by-screen guides are listed in the
[documentation index](index.md).
