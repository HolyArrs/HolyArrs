# Troubleshooting

Something is wrong and you want to find out why, or collect what a helper will ask for. Start here. The examples use the container name `holyarrs` from the [install guide](../getting-started/install.md). Use your own name if it differs.

## Steps

1. Watch the log.
2. Run the doctor.
3. If you still need help, download a support bundle and share it.

## Watch the log

```sh
docker logs -f holyarrs
```

This follows the log live. Secrets are masked. Press Ctrl+C to stop following. The container keeps running.

At start, HolyArrs checks its folders and writes one summary line, such as `folders: 4 ok, 1 not writable`. Then it writes one line per folder with a problem and a short fix. The check stops after 3 seconds so start-up never hangs. A folder that is too slow to answer is counted as not checked. This is often a disk waking from sleep or a slow network share. Run the doctor to check it without the time limit.

See [Logs](../../run/logs.md) for the log file and its settings.

## Run the doctor

The doctor checks your setup from inside the container, as the same user HolyArrs runs as. What it finds is what HolyArrs runs into.

When the container is running:

```sh
docker exec holyarrs /holyarrs doctor
```

When HolyArrs keeps stopping or restarting, `docker exec` won't work. Run the doctor in a new container from the same image, with the same folders and settings:

```sh
docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> -e TZ=<tz> holyarrs/holyarrs:<tag> doctor
```

With Compose, `docker compose run --rm holyarrs doctor` does the same.

### What the doctor checks

| Section | What it shows |
|---|---|
| App | Version, the user and group it runs as, the umask, and whether it runs in a container. |
| Config folder | Owner, rights, free space, and whether it is on a local disk. The database must be on a local disk. |
| Library and download folders | Whether each folder exists, its owner, whether it can be written, free space, and whether hardlinks work between your download folder and your library folders. |
| Network | Whether the metadata server's name resolves and whether it answers. |
| Clock | The time, the time zone (`TZ`), and a warning when the clock is clearly wrong. |

Each line says `OK`, `WARN` or `FAIL`. A `WARN` or `FAIL` line tells you how to fix it, for example which `chown` to run or which `PUID` and `PGID` to set. The report never shows keys, passwords or tokens.

The doctor only reads, with two exceptions. It writes and removes a small test file to check that a folder can be written. And when it starts as root, it gives the config folder to `PUID:PGID`, the same way a normal start does.

### Doctor options

- `-config <folder>`: check another config folder than `/config`. Point it only at a HolyArrs config folder.
- `-json`: print the report as JSON.

It exits with 0 when nothing failed and 1 when a line says `FAIL`, so you can use it in a script.

## Download a support bundle

A support bundle is one zip file with what a helper usually asks for. Owners and admins can download it.

1. Open the `/health` page.
2. Click **Download support bundle**.

You can also get it from the API at `GET /api/v1/support/bundle`.

| File | What it holds |
|---|---|
| `log.txt` | The newest log, with keys, tokens and passwords removed. |
| `about.json` | Version and setup details. |
| `doctor.txt` | The doctor report. |
| `health.json` | The current health check results. |

Secrets are removed, but the bundle can still show folder paths and host names. Read it before you post it anywhere public.

## Find the version

```sh
docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs
```

The doctor's App section shows the same version.

## Why there is no shell

The image has no shell and no command-line tools. It holds only the HolyArrs program, so it stays small and leaves less for an attacker to use. `docker exec holyarrs sh` fails, and that is expected. The doctor, the log and the support bundle cover the usual reasons to open a shell.

If you really need a closer look, start a throwaway helper container next to HolyArrs:

```sh
docker run --rm -it --pid container:holyarrs --network container:holyarrs --volumes-from holyarrs busybox sh
```

The helper sees the HolyArrs process, uses the same network, and has the same folders at the same paths. `--rm` removes it when you leave.

The helper runs as root. Use it to look, not to change things. A file written as root may be unreadable for HolyArrs later. To test as the HolyArrs user, add `--user <PUID>:<PGID>`.

## Common problems

**The doctor says a folder is not writable.** The folder's owner on the host doesn't match `PUID` and `PGID`. Run the `chown` the doctor suggests, or change `PUID` and `PGID`.

**The doctor says hardlinks don't work.** Your download folder and library folders sit on different disks or different mounts. Mount one shared parent folder into the container instead of two separate ones.

**The doctor says the config folder is on a network share.** Move `/config` to a local disk. HolyArrs won't run its database on a share.

**The metadata server can't be reached.** Check your DNS and firewall. Pages with names and pictures stay empty until it answers.

**A feature is missing.** It may be [held back](held-back.md).

**HolyArrs won't start at all.** Start it in [safe mode](parts-and-safe-mode.md#start-in-safe-mode), fix the problem from the settings, then restart normally.
