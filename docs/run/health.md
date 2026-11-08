---
title: Health checks
description: What each HolyArrs health check means, how often it runs, and how to fix a warning.
level: Setup
sidebar: { order: 3 }
---

HolyArrs checks itself on a timer and tells you when something is wrong.

## See the checks

1. Open **Home**. Anything that needs a look shows there.
2. For the full list, open **Settings > Health checks**. Each result shows its state and since when.
3. Select **Check again** to run every check now. You should see the results refresh.

Checks run every 5 minutes, at start, and right after an import is refused for lack of space. The **Status** page at `/health` shows free space per Library folder.

## What each check means

| Check | Warning | Broken |
|---|---|---|
| Config folder | | `/config` is not mapped to a host folder or named volume. Everything is lost when the container is removed |
| Config folder space | Under 1 GB free on its disk | Under 256 MB free. HolyArrs cannot save anything when it is full |
| Library folder | On another disk than downloads, so imports are copied | Gone (disk or share not mounted) or not writable |
| Free space | Under the warning size (1024 MB by default) | An import was refused moments ago for space |
| Source | Cannot be reached, or refused its key | |
| Downloader | Does not answer, or refused the sign-in | |
| Downloads waiting for files | A download waits because the downloader cannot use its files, often a share that mounted late | One has waited about 3 days |
| Backups | No backup yet, the newest is old, or no key kit saved for the current master key | The last scheduled backup failed |
| Trailer reader (trailers on) | 5 different videos failed in a row, or YouTube turned the reader away | |
| Update | A newer HolyArrs is out | |

A paused source or downloader counts as fine.

## Get an Alert

Create an [Alert](../users/alerts.md) with the **Health** moment. You get one message when a check breaks and one when it is fixed. A downloader that stays down sends one message, not one per run.

## Choices

On **Settings > Health checks**. The first two sit under the card's advanced part.

| Setting | Default | Change it when |
|---|---|---|
| **Check every (minutes)** | 5 | You want faster or slower checks: 1 to 1440 |
| **Alert after this many runs in a row** | 2 | Blips still alert (raise) or alerts come late (lower): 1 to 20 |
| **Warn when free space is under (MB)** | 1024 | Your disks are small or large. Per Library folder disk |

The update settings live on the same card; see [Updates](updates.md#choices).

## If it does not work

**Symptom:** a download waits for its files after a share came back.
**Cause:** qBittorrent, Deluge and Transmission keep the error until the files are checked again.
**Fix:** on **Downloads**, select **Check again** on that row, or recheck it in the downloader.

**Symptom:** a Library folder shows broken after a reboot.
**Cause:** the disk or share was not mounted when HolyArrs started.
**Fix:** mount it, then select **Check again**.

**Symptom:** the key kit warning stays.
**Cause:** no key kit was saved for the current master key.
**Fix:** the owner selects **Settings > Backups > Download key kit**. See [Backups](backups.md#save-the-master-key).

**Symptom:** Config folder is broken.
**Cause:** `/config` lives inside the container.
**Fix:** stop the container, copy the folder out with `docker cp`, map `/config`, copy it back in. See [Install](../start/install.mdx).

## For experts

<details class="ha-more"><summary>How alerting holds back blips</summary>

A new result must hold for **Alert after this many runs in a row** before it counts. With the default 2, a downloader that fails one run and recovers on the next stays quiet. Raising it makes real alerts arrive later too.

</details>

## Related

- [Alerts](../users/alerts.md)
- [Backups](backups.md)
- [Schedules](schedules.md)
- [Troubleshooting](../help/troubleshooting.md)
