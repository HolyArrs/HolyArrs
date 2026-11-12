---
title: Synology package (experimental)
description: Install HolyArrs on a Synology NAS from a package file, without Container Manager. Experimental and not yet tested by the project on a real NAS.
level: Setup
sidebar: { order: 11 }
---

Install HolyArrs on a Synology NAS from a package file, with no Docker and no Container Manager.

:::caution
This package is experimental. Its build and scripts are tested on Linux, but the project has no Synology NAS to install it on. Container Manager stays the proven way: see [NAS, Synology and Unraid](platforms.md). Make a [backup](backups.md) before you rely on it.
:::

## Before you start

- DSM 7.0 or newer. DSM 6 is not supported.
- One shared folder for downloads and media, such as `data`, with `downloads` and `media` inside. See [Folders](../start/folders.md).
- The package file for your NAS, from the version's page on [GitHub releases](https://github.com/HolyArrs/HolyArrs/releases):

| File | For |
|---|---|
| `holyarrs-<version>-x86_64.spk` | Intel and AMD models |
| `holyarrs-<version>-aarch64.spk` | 64-bit ARM models (Realtek RTD1296 and RTD1619, Marvell Armada 37xx) |
| `holyarrs-<version>-armv7.spk` | 32-bit ARM models (Alpine, Armada 370, 375, 38x and XP, Comcerto, Monaco) |

Not sure which? Look up your model's **Package Arch** in Synology's CPU list. `holyarrs-<version>-synology-SHA256SUMS` holds the checksum of each file.

## Install

1. Open **Package Center** and select **Manual Install**.
2. Pick the `.spk` file. DSM warns that the package is not signed by Synology. Every package from outside Synology gets this warning. Select **Agree**.
3. Type the name of your media shared folder, such as `data`. Type the name only, not a path. DSM gives HolyArrs read and write on that folder.
4. Finish the steps. You should see HolyArrs as **Running** in Package Center.
5. Select **Open**, or go to `http://your-server:6262`. You should see the first-run wizard. Make the owner account right away; the first visitor becomes the owner.
6. Add your Library folders by their full path, such as `/volume1/data/media/movies`, `/volume1/data/media/tv` and `/volume1/data/media/music`.

## Update

HolyArrs tells you when a new version is out and never installs it. The package does not update by itself either.

1. Download the new `.spk` for your NAS.
2. Open **Package Center > Manual Install** and pick the file.
3. The folder step shows the shared folder you named before. Leave it as it is.
4. You should see the new version in Package Center. Your settings and Library stay.

To go back to an older version, read [Updates](updates.md#go-back-to-the-older-version) first. Never install an older package over data a newer one has changed.

## Choices

### Where things live

| What | Where |
|---|---|
| Settings and databases (`/config` in Docker) | `/var/packages/holyarrs/var/config` |
| Start-up log | `/var/packages/holyarrs/var/holyarrs.log` |
| Port | `6262` |

HolyArrs runs as the package's own user, `holyarrs`, never as root. There is no `PUID` or `PGID` to set.

### More than one shared folder

The install step grants one shared folder. For another one, open **Control Panel > Shared Folder > Edit > Permissions**, choose **System internal user** in the list at the top and give `holyarrs` read and write.

### Restarts

Restart in the app, safe mode and a backup restore all stop HolyArrs. The package starts it again by itself within a few seconds.

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| Package Center says the package does not fit this NAS | The file is for another CPU type | Download the file that matches your model's Package Arch. |
| The package runs but the page does not open | Another app uses port `6262`, or the firewall blocks it | Read the start-up log. Stop the other app, or allow HolyArrs in **Control Panel > Security > Firewall**. |
| Imports fail with a permission error | The folder is not the one named at install | Give `holyarrs` read and write on it, as in **More than one shared folder**. |
| The install or the update fails | The package is experimental | Run HolyArrs in [Container Manager](platforms.md) and [tell us](https://github.com/HolyArrs/HolyArrs/issues) your model and DSM version. |

## Related

- [NAS, Synology and Unraid](platforms.md)
- [Updates](updates.md)
- [Backups](backups.md)
