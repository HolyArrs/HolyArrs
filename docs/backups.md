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
   backup.
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

