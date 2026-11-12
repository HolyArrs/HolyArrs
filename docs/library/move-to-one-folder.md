---
title: Move to one data folder
description: Switch from separate /movies, /tv, /music and /downloads mounts to one /data folder with Change path, without importing your Library again.
level: Setup
---

Move your downloads and your Library into one data folder, so new imports become hardlinks or moves instead of full copies. Your titles, History, requests and settings stay as they are.

New in 1.1.0. It uses **Change path** on each Library folder, so HolyArrs never imports your Library again.

## Before you start

- HolyArrs 1.1.0 or later, and the **Change settings** permission.
- Access to the server's folders and to the compose files of HolyArrs and your downloader.
- One disk, or one share or dataset, with room for everything. A move inside one filesystem is instant. A move between disks is a full copy and takes as long.
- Nothing in progress: let running downloads finish and import, so the **Downloads** page shows nothing downloading or waiting to import.
- A fresh backup: **Settings > Backups > Back up now**.

This guide uses `/srv/movies`, `/srv/tv`, `/srv/music` and `/srv/downloads` as the old folders on the host and `/srv/data` as the new one. Use your own paths.

## Move the folders

1. Stop HolyArrs and your downloader. If both are in one compose file:

   ```sh
   docker compose stop
   ```

   Otherwise run it in each compose folder.

2. On the host, move each folder whole into the data folder:

   ```sh
   mkdir -p /srv/data/media
   mv /srv/movies /srv/data/media/movies
   mv /srv/tv /srv/data/media/tv
   mv /srv/music /srv/data/media/music
   mv /srv/downloads /srv/data/downloads
   ```

   Keep everything inside a Library folder exactly as it is. HolyArrs looks for each file at the same place under the new path.

3. In HolyArrs' compose file, swap the four media mounts for one:

   ```yaml
       volumes:
         - /srv/holyarrs/config:/config
         - /srv/data:/data
   ```

   If you set `HOLYARRS_SAVE_PATH`, change it to the new download folder, such as `/data/downloads`.

4. Give your downloader the same mount, `- /srv/data:/data`, in its own compose file.
5. Start both again, in each compose folder:

   ```sh
   docker compose up -d
   ```

   Your Library folders now show **Folder not found**. That is expected until the next steps. While a folder is gone, a rescan marks no file as missing.

## Point HolyArrs at the new paths

1. Open **Settings > Library folders**.
2. On each folder's row, select **Change path** and type the new path:

   | Old path | New path |
   |---|---|
   | `/movies` | `/data/media/movies` |
   | `/tv` | `/data/media/tv` |
   | `/music` | `/data/media/music` |

   HolyArrs checks that every file it knows is at the new path, with the same size, then saves. You should see the folder as **Healthy**, without **Works, but files are copied (no hardlinks)**.
3. Change the other places that name the old download folder:
   - **Extra import folders** on the same page: change `/downloads` to `/data/downloads`.
   - **Settings > Downloaders**: remove a path pair once both apps see the same path.
   - **Settings > Media servers**: change the **HolyArrs path** of a path pair to the new folder.
4. In your downloader, set the save folder to `/data/downloads`. Torrents that still seed point at the old path. Set their location to the same folder under `/data/downloads`, or remove the ones you no longer seed.
5. In Seerr, Overseerr or any app that adds titles through the [old apps' API](../reference/old-app-api.md), open the HolyArrs server's settings and pick the new folder. A request that names the old path is refused.

In History, the next import says `by hardlink` or `by move`.

## What stays the same

- Titles, Versions, History, requests and settings.
- Files imported as copies before the switch stay on disk twice until the downloader removes its copy. HolyArrs does not turn old copies into hardlinks.
- Your media server and apps such as Bazarr read files themselves. Give them the same mount change, and update their own library paths.

## If it does not work

| You see | Fix |
|---|---|
| **Change path** names a file that is not at the new path | Put that file at the same place under the new path, then try again. Nothing changed. |
| A file has another size at the new path | The move is still running, or another file sits there. Let the move finish, then try again. |
| The path is refused as already used | It sits inside or around another Library folder. Give each folder its own path, such as `/data/media/movies`, not `/data/media`. |
| The folder does not exist | Type the path inside the container, such as `/data/media/movies`, not the host path. Check the mount and recreate the container. |
| Still **Works, but files are copied (no hardlinks)** | Downloads and media are on two filesystems, such as two NAS shares or datasets under `/srv/data`. Use folders inside one. |
| The `mv` takes hours | The old and new folders are on different disks, so `mv` copies. Let it finish before you select **Change path**. |

## For experts

<details class="ha-more"><summary>What Change path checks</summary>

- The new path follows the same rules as a new Library folder: a full path that exists, not a whole disk, not inside or around the config folder, and not inside or around another Library folder.
- Every file HolyArrs lists as present in this folder must be at the new path with the size it recorded. Files already marked missing are not checked.
- Imports and deletes for this folder wait while the check runs. If a file changes in the meantime, nothing is saved and the message asks you to try again.
- After saving, the folder check runs again: write test, hardlink test from each downloader folder, and free space.

</details>

## Related

- [Library folders](folders.md)
- [Folders and hardlinks](../start/folders.md)
- [Install](../start/install.mdx)
- [Downloaders](../downloads/downloaders.md)
- [Media servers](media-servers.md)
