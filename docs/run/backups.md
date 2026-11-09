---
title: Backups
description: Back up HolyArrs, save the master key, and restore after a bad update or on a new machine.
level: Start
sidebar: { order: 2 }
---

Keep a copy of your setup to put back after a bad update, a broken disk or a move to a new machine.

## Before you start

- **Settings > Backups** needs the **Change settings** permission. Only the owner restores.
- Download, delete and restore need a password sign-in from the last few minutes.

## Make a backup

1. Open **Settings > Backups**.
2. Select **Back up now**. You should see the new backup under **Saved backups**.
3. Select **Download** next to it to keep a copy off the server.

## Save the master key

Backups never hold the master key, which opens the passwords and API keys in your settings. Without the matching key, a restored backup cannot read them.

1. Under **Master key**, the owner selects **Download key kit**.
2. Keep the file away from the server and apart from your backups. You should see the key kit warning leave **Settings > Backups** and the health checks.

To use it later, rename it to `holyarrs.key` and put it in the config folder. Not the owner? Ask the owner to download it.

## Restore a backup

:::danger
Changes made since the backup are lost. Have the matching master key ready first.
:::

1. Select **Restore from a file** and pick a HolyArrs backup zip.
2. Confirm.
3. HolyArrs checks the file. You should see **Restore ready. It applies on the next start.**
4. HolyArrs restarts and puts the backup in place. With the recommended Docker restart policy this happens on its own.

Your current databases move to `/config/backups/before-restore-<time>`, so you can undo a restore.

## Choices

### How often

| Choice | Backs up | Keeps |
| --- | --- | --- |
| **Balanced** (default) | Every day | Last 7 days and 4 weekly copies |
| **Light** | Once a week | 4 weeks. Least space |
| **Careful** | Every day | Every copy for 30 days |

The newest scheduled backup is always kept. These rules clean up scheduled backups only. Backups you make stay until you delete them (trash button on the row; this cannot be undone). HolyArrs also backs up before an update that changes stored data and keeps the newest 3; see [Updates](updates.md#the-backup-before-an-update).

## Move to a new machine

A new install makes its own master key on first start. That key cannot open your old backups, so the restore is refused. Put the old key in place first. The examples call the container `holyarrs`; use your own name.

1. Before the first start, create the container without starting it: `docker compose create`, or your `docker run` line with `docker create` in place of `docker run -d`.
2. Copy your old key in: `docker cp holyarrs.key holyarrs:/config/`. A key kit renamed to `holyarrs.key` works too. This works for a named volume or a host folder; HolyArrs hands the files in `/config` to your `PUID` and `PGID` when it starts. If you use `HOLYARRS_MASTER_KEY_FILE`, put the key at that path on the host instead, readable by your `PUID` and `PGID`.
3. Restore the backup. Pick one way:
   - Start HolyArrs, finish first run, then use **Restore from a file** in **Settings > Backups**.
   - Or restore before the first start. Put the zip, name unchanged, in a folder called `backups` and copy it in with `docker cp backups holyarrs:/config/`. Then run the restore once with the same image:
     - Compose: `docker compose run --rm holyarrs restore <zip name>`
     - Docker CLI: `docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> holyarrs/holyarrs:<tag> restore <zip name>`

     With a key file path, add `-e HOLYARRS_MASTER_KEY_FILE=<its path>` so the restore checks against your key. `restore --list` shows the backup names it can see.
4. Start HolyArrs. You should see your library and settings back.

Started the new install before copying the key? If nothing on it is worth keeping, stop HolyArrs, copy your old key over the new one with the same `docker cp`, restore, then start.

## If it does not work

**Symptom:** "Backups are off in this build".
**Cause:** this copy runs without its backup manager.
**Fix:** none in the app; there are no backup controls.

**Symptom:** Download, delete or restore asks for your password.
**Cause:** these need a recent password sign-in.
**Fix:** sign in again with your password and retry.

**Symptom:** the restore is refused because of the master key.
**Cause:** the key in the config folder does not match the backup.
**Fix:** put the matching `holyarrs.key` back and try again.

The restore also refuses a zip that is too big or damaged, holds unexpected files, comes from a newer database version, or holds data your key cannot read.

## For experts

<details class="ha-more"><summary>What is in a backup</summary>

Each backup is a zip with the main database, the Events database if there is one, and `backup.json` (HolyArrs version, database versions, backup kind, time made). Media, downloads, artwork, logs and the master key are not in it.

</details>

<details class="ha-more"><summary>Restore before the first start</summary>

`docker cp` works for a named volume or a host folder; HolyArrs hands the files in `/config` to your `PUID` and `PGID` when it starts. With `HOLYARRS_MASTER_KEY_FILE`, put the key at that host path instead, readable by `PUID` and `PGID`.

1. Put the zip, name unchanged, in a folder called `backups` and copy it in: `docker cp backups holyarrs:/config/`.
2. Run the restore once with the same image:
   - Compose: `docker compose run --rm holyarrs restore <zip name>`
   - Docker CLI: `docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> holyarrs/holyarrs:<tag> restore <zip name>`
3. With a key file path, add `-e HOLYARRS_MASTER_KEY_FILE=<its path>` so the restore checks against your key.
4. Start HolyArrs.

`restore --list` shows the backup names it can see.

</details>

## Related

- [Updates](updates.md)
- [Install](../start/install.mdx)
- [Health checks](health.md)
