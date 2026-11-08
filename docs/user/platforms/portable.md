# Run HolyArrs without Docker

You want to run HolyArrs as a plain program, without Docker. Docker stays the main way to run it. Portable archives are not signed, and nobody publishes them for you yet. You build them yourself from the source.

## Steps

### Build the archives

You need Python 3, Git and Docker. The build runs in the project's pinned Go builder image with networking off, so it never downloads anything. Get these ready first:

- A complete Go module cache, filled by a normal project build. It is mounted read-only.
- The frontend dependencies installed from `app/frontend/package-lock.json`. They supply license notices.
- The built web app, committed. The build uses committed source only, so uncommitted edits are left out.

From the checkout, run:

```sh
python deploy/release/package.py --ref HEAD --version 1.0.0 --output /srv/holyarrs-archives/1.0.0 --web-node-modules app/frontend/node_modules --module-cache /home/you/go/pkg/mod
```

Use a new output folder outside the checkout. An existing folder is refused. If any target fails, no output folder is left behind.

### Run it

1. Check the archive against `SHA256SUMS`.
2. Extract it into a new folder. Keep the program and the config in separate folders.
3. Start it with your own config path:

```sh
./holyarrs -config /srv/holyarrs/config -addr :6262
```

On Windows PowerShell:

```powershell
.\holyarrs.exe -config 'C:\HolyArrs\config' -addr ':6262'
```

4. Open http://your-server:6262 and create the owner account right away. See [First run](../setup/first-run.md) for the rest.

## Settings explained

**Build options.**

- `--targets` picks a subset, comma-separated. The defaults are `linux-amd64`, `linux-arm64`, `linux-armv7` and `windows-amd64`.
- `--low-resource` is for a small build machine. Each builder container gets one CPU, 2 GiB of memory and 256 processes, and Go builds one package at a time. Python still packs the archives on the host, outside those limits.
- The commit is added to your version number and built into the program. The app and its backups report it.

**What an archive holds.** The program, license, third-party notices, guides, a source manifest and the package identity. A separate source archive holds the project source and the Go dependencies. Keep it next to the binaries. The checksums prove the files match. They are not signatures. The same commit, version and inputs give the same archives, so you can rebuild to check.

**Config path.** The default is `/config`. Always give your own with `-config`. Keep it on a local disk.

**User.** Run HolyArrs under its own account with write access only to the config and your media. Do not start it as root. As root outside a container, without `PUID` and `PGID`, it stops with "started as root outside a container". With `PUID` and `PGID` set, it hands the config folder to that user and switches to it. On Windows, protect the config folder with an NTFS permission for that account.

**Memory.** Without `GOMEMLIMIT`, HolyArrs aims at 384 MiB, the same as in Docker. Set `GOMEMLIMIT` to about 80 percent of the memory you give it. This is a target, not a hard limit. Use your service manager for hard limits.

**Low-memory mode.** On Linux, a machine with about 2.5 GiB or less turns it on by itself. Then the target drops to a quarter of the machine's memory, between 192 and 384 MiB. Set `HOLYARRS_LOW_MEMORY=1` or `0` to decide yourself. See [Low-memory mode](../help/parts-and-safe-mode.md#low-memory-mode).

**Other needs.** Node is not needed to run it. On Linux, HTTPS needs the system CA certificates. Keep downloads and Library folders on one filesystem if you want hardlinks.

**No service.** The archive does not install a service. Set up restarts and limits in your own service manager.

## Update and roll back

1. Check the new archive before you stop the app.
2. Stop HolyArrs, then copy the whole config folder, master key included. Or use an in-app backup and keep its key separately.
3. Extract the new program into a different folder. Keep the old program and the old config copy.
4. Start the new program on your config. Check sign-in, health and your settings.

To roll back, you need the old program and its matching config from before the update. Never point an older program at databases a newer one has changed. Save the newer config first so you lose nothing. See [Backups](../../run/backups.md).

## Troubleshooting

**"started as root outside a container".** Run it as its own user, or set `PUID` and `PGID`.

**The app exits after a restore.** A restore from Settings > Backups stages the data and exits with code 75. Start it again to apply it. Set your service manager to restart it on its own.

**HTTPS calls fail on Linux.** Install the system CA certificates.

**The build says the output exists.** Pick a new output folder.
