# Backups

You want a copy of your HolyArrs setup that you can put back after a bad
update, a broken disk, or a move to a new machine. Backups do that.

Open **Settings > Backups**. You need the **Change settings** permission to see
this screen. Only the owner can restore. To download, delete or restore, you
also need to have signed in with your password in the last few minutes.

## Make a backup

1. Open **Settings > Backups**.
2. Select **Back up now**.
3. The new backup shows up under **Saved backups**.

To keep a copy off the server, select **Download** next to a backup. Your
browser saves the zip file.

To remove a backup, select the trash button on its row and confirm. This
can't be undone.

## Choose how often

Under **How often**, pick one. **Balanced** is the default.

| Choice | What it does |
| --- | --- |
| Balanced | Backs up every day. Keeps the last 7 days and 4 weekly copies. |
| Light | Backs up once a week. Keeps 4 weeks. Uses the least space. |
| Careful | Backs up every day. Keeps every copy for 30 days. |

The newest scheduled backup is always kept. These rules only clean up
scheduled backups. Backups you make yourself stay until you delete them.

HolyArrs also makes a backup before each update. Once the new version starts
fine, it keeps only the newest of these.

## Save the master key

Backups don't include the master key. The key opens the passwords and API
keys stored in your settings. Without the matching key, a restored backup
can't read them.

1. Under **Master key**, the owner selects **Download key kit**.
2. Keep the file somewhere safe, away from the server.
3. To use it later, rename it to `holyarrs.key` and put it in the config
   folder.

Until a key kit is saved for the current key, the screen and the health check
show a warning. If you are not the owner, ask the owner to download it.

More on the key: [Install: keep the master key](../getting-started/install.md#keep-the-master-key).

## What is in a backup

Each backup is a zip file with:

- the main database,
- the Events database, if there is one,
- `backup.json`, which notes the HolyArrs version, database versions, kind of
  backup, and when it was made.

Media, downloads, artwork, logs and the master key are not in the zip.

## Restore a backup

1. Make sure you have the master key that belongs with this backup. On a new
   machine, read [Move to a new machine](#move-to-a-new-machine) first.
2. Select **Restore from a file** and pick a HolyArrs backup zip.
3. Confirm. Changes made since that backup are lost.
4. HolyArrs checks the file. It refuses a zip that is too big or damaged, has
   unexpected files, comes from a newer database version, or holds data your
   current key can't read.
5. When the check passes, the screen says **Restore ready. It applies on the
   next start.** HolyArrs restarts and puts the backup in place.

Your current databases are not deleted. They move to
`/config/backups/before-restore-<time>`, so you can undo a restore. With the
recommended Docker restart policy, the restart happens on its own. See
[Install: restart and restore](../getting-started/install.md#restart-and-restore).

## Move to a new machine

A new install makes its own master key on first start. That key can't open
your old backups, so the restore is refused. Put the old key in place first.

1. Create the container without starting it. Use `docker compose create`, or
   your usual `docker run` line with `docker create` in place of
   `docker run -d`. The examples call the container `holyarrs`. Use your own
   name.
2. Copy your old key in:
   `docker cp holyarrs.key holyarrs:/config/`
   A key kit works too, once renamed to `holyarrs.key`. This works for a named
   volume or a host folder. HolyArrs hands the files in `/config` to your
   `PUID` and `PGID` when it starts. If you use `HOLYARRS_MASTER_KEY_FILE`,
   put the key at that path on the host instead, readable by your `PUID` and
   `PGID`.
3. Restore the backup. Pick one way:
   - Start HolyArrs, finish first run, then use **Restore from a file** in
     **Settings > Backups**.
   - Or restore before the first start. Put the zip, name unchanged, in a
     folder called `backups`, and copy it in:
     `docker cp backups holyarrs:/config/`
     Then run the restore once with the same image.
     With Compose: `docker compose run --rm holyarrs restore <zip name>`
     With the Docker CLI:
     `docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> ghcr.io/coderluii/holyarrs:<tag> restore <zip name>`
     If you use a key file path, add
     `-e HOLYARRS_MASTER_KEY_FILE=<its path>` so the restore checks against
     your key. `restore --list` shows the backup names it can see.
4. Start HolyArrs.

## Troubleshooting

**The screen says "Backups are off in this build".** This copy of HolyArrs
runs without its backup manager. There are no backup controls to use.

**Download, delete or restore asks for your password.** These need a recent
password sign-in. Sign in again with your password and retry.

**I don't see Restore from a file working for me.** Only the owner can
restore.

**The restore is refused because of the master key.** The key in the config
folder doesn't match the backup. Put the matching `holyarrs.key` back and try
again.

**I started the new install before copying the old key.** If you set up
nothing on the new install that you want to keep: stop HolyArrs, copy your old
key over the new one with `docker cp holyarrs.key holyarrs:/config/`, run the
restore command from step 3, then start HolyArrs.
