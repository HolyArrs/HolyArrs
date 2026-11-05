# Backups

Open **Settings > Backups** to choose a schedule, make a backup, download a
copy, or restore one. The screen requires **Change settings**. Restoring is
owner-only. Download, delete and restore also require a password sign-in.

## What a backup holds

Each backup is a zip containing:

- the main database;
- the Events database, when present; and
- `backup.json`, which records the HolyArrs version, database versions, kind
  and creation time.

Media, downloads, artwork and logs are outside these databases and are not in
the zip. The master key is also excluded. Keep the key separately as explained
in [Install: keep the master key](install.md#keep-the-master-key). Without the
matching key, encrypted passwords and keys in a restored database cannot be
read. The easiest copy is the key kit: the owner downloads it with
**Download key kit** on this screen, and the health check warns until one is
saved for the current key.

HolyArrs also makes a backup before an update. Schedule retention removes only
scheduled backups. Manual backups stay until they are deleted; after a newer
version starts successfully, only its newest update backup is kept.

## Choose how often

**Balanced** is the default.

| Choice | Schedule and retention |
| --- | --- |
| Balanced | Every day; keep the newest 7 daily copies and 4 older weekly copies. |
| Light | Once a week; keep copies up to 28 days old. |
| Careful | Every day; keep every scheduled copy up to 30 days old. |

The newest scheduled backup is always kept.

## Saved backups

- **Make a backup now** creates a manual backup.
- **Download** saves the selected zip through the browser.
- The delete button permanently removes that saved zip after confirmation.
- Each row shows its filename, creation time, size and kind.

If the build has no backup manager, the screen says backups are off in that
build and does not offer these controls.

## Restore

1. Confirm that the separate master-key file is the one that belongs with the
   backup. On a new machine, see [Move to a new machine](#move-to-a-new-machine).
2. Select **Restore from a file** and choose a HolyArrs backup zip.
3. Confirm the warning. Changes made after that backup will be lost.
4. Let HolyArrs check the archive. It rejects an oversized or malformed zip,
   unexpected files, damaged databases, a backup from a newer database version,
   or encrypted data that the current master key cannot read.
5. HolyArrs stages the checked databases and applies them on its next start.

Before installing the staged databases, HolyArrs moves the current databases
under `/config/backups/before-restore-<time>`. With the recommended Docker
restart policy, the app restart completes the restore. See
[Install: restart and restore](install.md#restart-and-restore).


## Move to a new machine

A new install makes its own master key on its first start. That key cannot
open the passwords and keys saved in your old backups, so the restore is
refused. Put the old key in place first.

1. Before the first start on the new machine, make the container without
   starting it: `docker compose create`, or your usual `docker run` line with
   `docker create` in place of `docker run -d`. This guide calls the container
   `holyarrs`; use your own name.
2. Copy your old `holyarrs.key` (or your key kit renamed to `holyarrs.key`)
   into it: `docker cp holyarrs.key holyarrs:/config/`. This works for a
   named volume and a host folder alike, and needs no shell. HolyArrs gives
   the files in `/config` to your `PUID` and `PGID` when it starts.
   If you use `HOLYARRS_MASTER_KEY_FILE`, put the key file at that path on the
   host instead, readable by your `PUID` and `PGID`.
3. Restore the backup zip in one of two ways:
   - Start HolyArrs, finish first run, then use **Restore from a file** under
     Settings > Backups.
   - Or restore it before the first start. Put the zip, with its name
     unchanged, in a folder named `backups`, then copy that folder in:
     `docker cp backups holyarrs:/config/`. Run the restore once from the same
     image. With Compose: `docker compose run --rm holyarrs restore <zip name>`.
     With the Docker command line:
     `docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> ghcr.io/coderluii/holyarrs:<tag> restore <zip name>`.
     If you use a named key file, add `-e HOLYARRS_MASTER_KEY_FILE=<its path>`
     to that line, so the restore checks the backup against your key.
     `restore --list` shows the names it can see.
4. Start HolyArrs.

Already started the new install without the old key, and the page says the
master key does not match, or the restore is refused? If you set nothing up
on the new install that you want to keep: stop HolyArrs, copy your old key
over the new one with `docker cp holyarrs.key holyarrs:/config/`, and restore
with the command in step 3. Then start HolyArrs again.
