---
title: Portable build
description: Build HolyArrs archives from source and run the program without Docker on Linux or Windows.
level: Setup
sidebar: { order: 9 }
---

Run HolyArrs as a plain program on Linux or Windows, without Docker. Docker stays the main way to run it.

:::caution
The release workflow builds portable archives and attaches them, with the source archive and `SHA256SUMS`, to each GitHub release. They are not signed, so check them against `SHA256SUMS` before you run them.
:::

## Download instead of building

Releases built by the workflow have one archive per target: `linux-amd64`, `linux-arm64`, `linux-armv7` and `windows-amd64`. Download the one for your machine and its `SHA256SUMS`, check it, then continue at step 3 under [Build and run](#build-and-run). The rest of this page explains how to build the same archives yourself.

## Before you start

To build the archives yourself, you also need:

- Python 3, Git and Docker on the build machine.
- A filled Go module cache from a normal project build, and frontend dependencies installed from `app/frontend/package-lock.json`.
- The built web app committed. The build uses committed source only; uncommitted edits are left out.

## Build and run

1. From the checkout, build the archives:

   ```sh
   python deploy/release/package.py --ref HEAD --version 1.0.0 --output /srv/holyarrs-archives/1.0.0 --web-node-modules app/frontend/node_modules --module-cache /home/you/go/pkg/mod
   ```

   Use a new output folder outside the checkout. You should see one archive per target and a `SHA256SUMS` file.
2. Check the archive against `SHA256SUMS`.
3. Extract it into a new folder. Keep the program and the config in separate folders.
4. Start it with your own config path:

   ```sh
   ./holyarrs -config /srv/holyarrs/config -addr :6262
   ```

   On Windows PowerShell:

   ```powershell
   .\holyarrs.exe -config 'C:\HolyArrs\config' -addr ':6262'
   ```

5. Open `http://your-server:6262`. You should see the first-run wizard. Make the owner account right away; the first visitor becomes the owner.

## Choices

### Which user runs it

Run HolyArrs under its own account, with write access only to the config and your media.

- As root outside a container, without `PUID` and `PGID`, it stops with "started as root outside a container".
- With `PUID` and `PGID` set, it hands the config folder to that user and switches to it.
- On Windows, protect the config folder with an NTFS permission for that account.

### Restarts and limits

The archive installs no service. Set up auto-restart and hard memory limits in your own service manager (systemd, NSSM, Task Scheduler). Restart from the app and a backup restore both exit and expect to be started again.

## Update and roll back

1. Check the new archive before you stop the app.
2. Stop HolyArrs and copy the whole config folder, master key included. Or make an in-app backup and keep its key separately.
3. Extract the new program into a different folder. Keep the old program and the old config copy.
4. Start the new program on your config. You should be able to sign in, and `/health` should show running.

:::danger
Never point an older program at databases a newer one has changed. To roll back, save the newer config first, then start the old program on its matching old config copy.
:::

See [Backups](../../run/backups/).

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| "started as root outside a container" | Started as root | Run it as its own user, or set `PUID` and `PGID`. |
| The app exits after a restore | A restore stages the data and exits with code 75 | Start it again to apply it. Set your service manager to restart it. |
| HTTPS calls fail on Linux | No system CA certificates | Install them. |
| The build says the output exists | Existing output folders are refused | Pick a new folder. |

## For experts

<details class="ha-more"><summary>Build options</summary>

| Option | What it does | Default |
|---|---|---|
| `--targets` | Comma-separated subset to build | `linux-amd64,linux-arm64,linux-armv7,windows-amd64` |
| `--low-resource` | Each builder container gets 1 CPU, 2 GiB and 256 processes; Go builds one package at a time. Packing still runs on the host, outside those limits. | off |

The build runs in the project's pinned Go builder image with networking off, so it downloads nothing. The module cache is mounted read-only. If any target fails, no output folder is left. The commit is added to the version number and built into the program; the app and its backups report it.

</details>

<details class="ha-more"><summary>What an archive holds</summary>

The program, licence, third-party notices, guides, a source manifest and the package identity. A separate source archive holds the project source and Go dependencies; keep it next to the binaries.

`SHA256SUMS` proves the files match. It is not a signature. The same commit, version and inputs give the same archives, so you can rebuild to check.

</details>

<details class="ha-more"><summary>Runtime notes</summary>

| Topic | Detail |
|---|---|
| Config path | Default `/config`. Always pass `-config`, on a local disk. |
| Memory target | 384 MiB without `GOMEMLIMIT`. Set it to about 80 percent of the memory you give. A target, not a hard limit. |
| Low-memory mode | On Linux, on by itself at about 2.5 GiB or less; the target drops to a quarter of memory, between 192 and 384 MiB. `HOLYARRS_LOW_MEMORY=1` or `0` decides. |
| Node | Not needed to run. |
| Hardlinks | Keep downloads and Library folders on one filesystem. |

</details>

## Related

- [Install](../../start/install/)
- [Parts and safe mode](../../run/parts-safe-mode/)
- [Updates](../../run/updates/)
