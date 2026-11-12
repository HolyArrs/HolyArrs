---
title: Library folders
description: Add the folders where finished movies, shows and music live, and read what the folder check tells you.
level: Setup
---

Tell HolyArrs where your finished movies, shows and music live.

You need the **Change settings** permission. First-run setup adds one folder; add the rest here.

## Before you start

- The folder exists and HolyArrs can write to it.
- In Docker, you know the path inside the container, such as `/movies`. The host path does not work.

## Add a folder

1. Open **Settings > Library folders**.
2. Select **Add a folder**. The **Add a Library folder** card opens.
3. Under **Holds**, pick **Movies**, **Shows** or **Music**.
4. Enter the full path as HolyArrs sees it under **Folder path**.
5. Enter a **Name**, or leave it empty to use the last part of the path, such as `movies`.
6. Select **Add**. You should see the folder in its group with free space and a health state.

:::tip
For instant moves, keep downloads and media under one shared parent folder on one disk, such as `/data/downloads` and `/data/media/movies`. Separate `/movies`, `/tv`, `/music` and `/downloads` mounts make every import a full copy. The layout is in [Folders and hardlinks](../start/folders.md). To switch an existing setup, see [Move to one data folder](move-to-one-folder.md).
:::

## What the check tells you

The check confirms the folder exists, writes and removes a small test file, tries a hardlink from each active Downloader folder, and looks for at least 100 MB free. The refresh button on a row runs it again.

A hardlink is a second name for the same file on the same disk. It is instant and uses no extra space.

| State shown | Meaning | Imports |
|---|---|---|
| **Instant, no extra space** | Downloads and this folder are on the same drive | Each import is instant and uses no extra space |
| **Folder not found** or **Cannot write here** | The folder is gone or not writable | Stopped until you fix it |
| **Each import makes a full copy (uses extra space)** | Downloads sit on another drive or mount | Imports work, but each one copies the file: slower, and double the space until the Downloader deletes its copy |

Under 100 MB free adds a low space note. Imports work until the disk fills.

## Change a folder's path

New in 1.1.0. Use it when the files already sit somewhere else: a new mount point, a renamed share, or a move to one data folder. HolyArrs does not import anything again.

1. Move the folder on disk yourself, with everything inside it as it is.
2. Make sure HolyArrs sees the new place. In Docker, mount it and recreate the container.
3. On the folder's row in **Settings > Library folders**, select **Change path**.
4. Type the new path as HolyArrs sees it, such as `/data/media/movies`, and save.

HolyArrs checks that every file it knows in this folder is at the new path, with the same size. If one is not, nothing changes and the message names the first file that is missing or different. When all are there, HolyArrs saves the new path and runs the folder check again. It moves and renames nothing on disk.

The step-by-step guide for a whole setup is [Move to one data folder](move-to-one-folder.md).

## Rename or remove a folder

**Rename** changes the name you see. Nothing moves on disk.

The trash button removes the folder from HolyArrs and leaves every file in place. HolyArrs refuses while titles in the Library use the folder. Move those titles first. Titles you deleted from the Library don't count. They stay in History.

HolyArrs also keeps the folder while deleted files in it can still be restored from **Recently deleted**. Remove it once their holding days are over, or delete its `.holyarrs-replaced` folder on disk. A folder that is gone from disk has nothing left to restore, so it can always be removed once no title in the Library uses it.

## If it does not work

**Symptom:** the state says "Each import makes a full copy (uses extra space)".
**Cause:** downloads and media are on separate mounts.
**Fix:** mount one shared parent folder holding both, at the same path in every container.

**Symptom:** import says the path is not allowed.
**Cause:** the path is outside every allowed place (see Extra import folders below). The message names the allowed places. Paths with `..`, network shares such as `\\server\share`, and links or junctions that lead outside are refused too.

**Symptom:** import cannot see the download folder.
**Cause:** `HOLYARRS_SAVE_PATH` is not set and no Downloader has a path pair.
**Fix:** set `HOLYARRS_SAVE_PATH`, or add the download folder, such as `/downloads`, under **Extra import folders**.

**Symptom:** **Change path** says a file is not at the new path.
**Cause:** that file was not moved, was renamed, or was still being copied.
**Fix:** put the named file at the same place under the new path, then try again. A file with another size is still being copied, or is a different file.

**Symptom:** cannot remove a folder.
**Fix:** move its titles to another folder first. If it says deleted files can still be restored, wait until their holding days are over, or delete the folder's `.holyarrs-replaced` folder on disk. If an outside list adds to it, point that list at another folder.

## For experts

<details class="ha-more"><summary>Folder rules</summary>

A Library folder must exist, be a folder, be a full path, be writable, and not sit inside or around another Library folder. Each kind (Movies, Shows, Music) uses its own folders.

</details>

<details class="ha-more"><summary>Extra import folders</summary>

**Library > Import files by hand** reads and places files only inside:

- your Library folders,
- the folders your Downloaders save into, as HolyArrs sees them: `HOLYARRS_SAVE_PATH` and the HolyArrs side of each Downloader path pair,
- the **Extra import folders** on **Settings > Library folders**.

Files are placed like automatic imports: a link when the file and the Library folder are on the same drive (no extra space used), otherwise a real copy. The original stays where it was.

Type one full path per line, such as `/imports` or `D:\Imports`. Up to 32. A whole disk such as `/` or `C:\` is refused.

Only the owner, or an admin signed in with a password, can change the list. An API key also needs the admin scope. Others see **Owner and admins only**.

</details>

## Related

- [Folders and hardlinks](../start/folders.md)
- [Move to one data folder](move-to-one-folder.md)
- [Downloaders](../downloads/downloaders.md)
- [Media servers](media-servers.md)
- [Protected copies](protected-copies.md)
