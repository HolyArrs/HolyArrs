---
title: Proxmox container (no Docker)
description: Make a Proxmox container with one script and install HolyArrs in it from the Linux download of a release. Community script, tested outside Proxmox.
level: Setup
sidebar: { order: 12 }
---

Run HolyArrs in its own Proxmox container (LXC) without Docker, from the plain Linux download that every release carries.

:::caution
This script is new and the project has no Proxmox machine to run it on. The install, update and go-back steps were run on Debian 13 inside a Docker container, with a stand-in for systemd. The Proxmox commands were tested with stand-in commands only and have never run on Proxmox. It is not in the Proxmox community scripts collection yet. Read the two files before you run them: [`holyarrs-lxc.sh`](https://github.com/HolyArrs/HolyArrs/blob/main/deploy/proxmox/holyarrs-lxc.sh) and [`holyarrs-install.sh`](https://github.com/HolyArrs/HolyArrs/blob/main/deploy/proxmox/holyarrs-install.sh).
:::

Already run Docker in a container on Proxmox? That works too and stays the main way. See [Install](../start/install.mdx).

## Before you start

- Proxmox VE 8 or newer, and the **Shell** of your Proxmox host (the host, not a container).
- Internet on the host and on the network the container joins. The container downloads HolyArrs from GitHub.
- One host folder for downloads and media, if you want your Library on the host. Put `downloads` and `media` inside it, so files move instead of copying. See [Folders](../start/folders.md).

## Install

1. Copy `deploy/proxmox/holyarrs-lxc.sh` and `holyarrs-install.sh` to the host, into one folder. Or download both from GitHub with `curl`.
2. In the host shell, run it. Everything is a default you can leave out:

   ```sh
   bash holyarrs-lxc.sh
   ```

   With your own settings, for example a fixed address and a shared folder:

   ```sh
   CTID=210 IP=192.168.1.50/24 GATEWAY=192.168.1.1 MOUNT=/srv/data bash holyarrs-lxc.sh
   ```
3. Wait for it to finish. You should see `Done. HolyArrs is at http://<address>:6262`.
4. Open that address. You should see the first-run wizard. Make the owner account right away; the first visitor becomes the owner.
5. If you set `MOUNT`, add your Library folders by their full path, such as `/data/media/movies`. See the folder step below.

## Choices

### Settings you can change

Set them in front of the command, as in the example above.

| Setting | Default | Meaning |
|---|---|---|
| `CTID` | next free number | Container number |
| `CT_NAME` | `holyarrs` | Host name |
| `CORES`, `RAM` | `2`, `1024` | Cores and memory in MiB (at least 512) |
| `DISK` | `8` | Disk in GB, for settings and data only |
| `STORAGE`, `TEMPLATE_STORAGE` | first that fits | Where the disk and the Debian template go |
| `BRIDGE`, `VLAN` | `vmbr0`, none | Network bridge and tag |
| `IP`, `GATEWAY` | `dhcp` | A fixed address such as `192.168.1.50/24` needs `GATEWAY` |
| `MOUNT` | none | A host folder shared into the container as `/data` |
| `VERSION` | newest | A release such as `1.1.3`. Downloads exist from 1.1.3 |
| `PRIVILEGED` | off | `1` makes a privileged container |

### The shared folder and who owns it

The container is unprivileged, so the HolyArrs user inside it is a different number on the host: 100000 plus its number inside. The script prints the exact command at the end when you set `MOUNT`. To work it out yourself, find the numbers, on the host:

```sh
pct exec <CTID> -- id holyarrs
```

It prints something like `uid=999(holyarrs) gid=999(holyarrs)`. Add 100000 to both numbers and give the folder to that user and the group, so change both numbers:

```sh
chown -R 100999:100999 /srv/data
```

You should then be able to add `/data/media/movies` as a Library folder in HolyArrs. Keep downloads and media under the same `MOUNT`.

### What it sets up

- The program in `/opt/holyarrs`, owned by root. Your database and settings in `/var/lib/holyarrs`, owned by the `holyarrs` user.
- A systemd service `holyarrs` that starts at boot and restarts itself. A restart from the app and a backup restore both exit and expect to be started again.
- The container starts at host boot. HolyArrs listens on port `6262`.

### Another port

Every update writes the service file `/etc/systemd/system/holyarrs.service` again, so changes made in that file are lost. Put your own settings in a drop-in instead. Inside the container, run:

```sh
systemctl edit holyarrs
```

Add these two lines, save, then run `systemctl restart holyarrs`:

```ini
[Service]
Environment=HOLYARRS_PORT=8080
```

The update reads the port from there, waits on it and prints it. Use `HOLYARRS_PORT`, not a full listen address, so the update can still reach HolyArrs inside the container. See [Environment variables](../reference/env-vars.md).

## Update and roll back

1. In the host shell, run:

   ```sh
   bash holyarrs-lxc.sh update <CTID>
   ```

   Or inside the container: `update-holyarrs update`. Add a version at the end to pick one.
2. It checks the download against `SHA256SUMS` and that there is room on the disk, then stops HolyArrs and saves your settings and data to `/var/backups/holyarrs`. The saved copy leaves out the picture cache, the logs and HolyArrs's own `backups` folder, and only the newest two copies are kept. If the copy fails, HolyArrs is started again. The old program stays as `/opt/holyarrs/holyarrs.previous`, with its version in `/opt/holyarrs/version.previous`. A version older than the installed one is refused.
3. It waits up to five minutes for HolyArrs to be ready, which includes the time it needs to update its data. You should see `HolyArrs <version> is running.` If HolyArrs could not start or keeps stopping, it says so within seconds.

When that version is already installed, it prints `HolyArrs <version> is already installed, so nothing was changed.` and changes nothing. It does not start or restart HolyArrs either; if HolyArrs is stopped, start it with `systemctl start holyarrs`.

The update needs to know the installed version, which it keeps in `/opt/holyarrs/version`. If that file is missing, the update stops with `could not read which version is installed here` and changes nothing. Write the version back, for example `echo 1.1.4 > /opt/holyarrs/version`, and run the update again.

:::danger
Never start an older program on data a newer one has changed. To go back, run this inside the container (replace the name of the saved copy with your own; it is in `/var/backups/holyarrs`):

```sh
systemctl stop holyarrs
mv /var/lib/holyarrs /var/lib/holyarrs.newer
tar -xzf /var/backups/holyarrs/config-before-<version>-<time>.tar.gz -C /var/lib
mv /opt/holyarrs/holyarrs.previous /opt/holyarrs/holyarrs
mv /opt/holyarrs/version.previous /opt/holyarrs/version
systemctl start holyarrs
```

The newer data stays in `/var/lib/holyarrs.newer` until you delete it. If the app shows its go-back page, the one-time code is in `journalctl -u holyarrs`, not in a Docker log. Read [Updates](updates.md#go-back-to-the-older-version) first.
:::

HolyArrs tells you when a new version is out and never installs it by itself. Make a [backup](backups.md) and keep the master key before you update.

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| "this runs on a Proxmox VE host" | Run inside a container or on another machine | Run it in the Proxmox host shell |
| "the network bridge vmbr0 does not exist" | Your bridge has another name | Set `BRIDGE=<name>`, as in **Network** on the host |
| "could not reach github.com" | No internet from that network | Fix the gateway, DNS or VLAN. The container is left in place; remove it with `pct stop <CTID>; pct destroy <CTID>` |
| "Proxmox could not create the container from ..." | The Proxmox version does not accept that template | It tries Debian 12 next, and shows Proxmox's own message. Debian 13 is used only on Proxmox VE 9 and newer |
| "could not download ..." | That version has no Linux download | Use 1.1.3 or newer |
| "does not match its checksum" | A broken download | Run it again. Nothing was installed |
| Port `6262` does not answer | The service did not start | In the container run `journalctl -u holyarrs -n 50` |
| "could not read which version is installed here" | `/opt/holyarrs/version` is missing or damaged | Write the installed version into it, then update again |

## Related

- [Portable build](portable.md)
- [Updates](updates.md)
- [Reverse proxy and URL base](url-base.md)
