---
title: Library folders
description: Add the folders where finished movies, shows and music live, and read what the folder check tells you.
level: Setup
---

Tell HolyArrs where your finished movies, shows and music live.

You need the **Change settings** permission. First-run setup adds one; add the rest here.

## Before you start

- The folder exists and HolyArrs can write to it.
- In Docker, you know the path inside the container, such as `/movies`. The host path does not work.

## Add a folder

1. Open **Settings > Library folders**.
2. Pick what it holds: **Movies**, **Shows** or **Music**.
3. Enter the full path as HolyArrs sees it.
4. Enter a name, or leave it empty to use the last part of the path.
5. Select **Add and check**. You should see the row with free space and a health state.

:::tip
For instant moves, keep downloads and media under one shared parent folder on one disk, such as `/data/downloads` and `/data/movies`. Separate `/movies`, `/tv`, `/music` and `/downloads` mounts make every import a full copy.
:::

## What the check tells you

The check confirms the folder exists, writes and removes a small test file, tries a hardlink from each active Downloader folder, and looks for at least 100 MB free. **Check again** reruns it.

A hardlink is a second name for the same file on the same disk. It is instant and uses no extra space.

| State | Meaning | Imports |
|---|---|---|
| **Missing** or **Read only** | The folder is gone or not writable | Stopped until you fix it |
| **No hardlink** | Downloads sit on another disk or mount | Work, but copy the file: slower and uses double space |
| Low space note | Under 100 MB free | Work until the disk fills |

## Rename or remove a folder

**Rename** changes the name you see. Nothing moves on disk.

**Remove** takes the folder out of HolyArrs and leaves every file in place. It is refused while titles use the folder. Move those titles first.

## If it does not work

**Symptom:** **No hardlink** on Docker.
**Cause:** Downloads and media are on separate mounts.
**Fix:** Mount one shared parent folder holding both, at the same path in every container.

**Symptom:** Import says the path is not allowed.
**Cause:** The path is outside every allowed place (see Extra import folders below). The message names the allowed places. Paths with `..`, network shares such as `\\server\share`, and links or junctions that lead outside are refused too.

**Symptom:** Import cannot see the download folder.
**Cause:** `HOLYARRS_SAVE_PATH` is not set and no Downloader has a path pair.
**Fix:** Set `HOLYARRS_SAVE_PATH`, or add the download folder, such as `/downloads`, as an extra import folder.

**Symptom:** Cannot remove a folder.
**Fix:** Move its titles to another folder first.

## For experts

<details class="ha-more"><summary>Folder rules</summary>

A Library folder must exist, be a folder, be a full path, be writable, and not sit inside or around another Library folder. Each kind (Movies, Shows, Music) uses its own folders.

</details>

<details class="ha-more"><summary>Extra import folders</summary>

**Library > Import files by hand** reads, copies and moves files only inside:

- your Library folders,
- the folders your Downloaders save into, as HolyArrs sees them: `HOLYARRS_SAVE_PATH` and the HolyArrs side of each Downloader path pair,
- the **Extra import folders** on this page.

Type one full path per line, such as `/imports` or `D:\Imports`. Up to 32. A whole disk such as `/` or `C:\` is refused.

Only the owner, or an admin signed in with a password, can change the list. An API key also needs the admin scope. **Change settings** can view it.

</details>

## Related

- [Downloaders](../downloads/downloaders.md)
- [Media servers](media-servers.md)
- [Protected copies](protected-copies.md)
