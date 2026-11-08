# Library folders

You want HolyArrs to know where your finished movies, shows and music live. A Library folder is that place. You add and check them in **Settings > Library folders**. You need the **Change settings** permission to open this screen.

## Add a folder

1. Open **Settings > Library folders**.
2. Pick what the folder holds: **Movies**, **Shows** or **Music**. Each kind uses its own folders.
3. Enter the full path as HolyArrs sees it. In Docker, this is the path inside the container, such as `/movies`. The host path will not work.
4. Enter a name if you want one. Leave it empty and HolyArrs uses the last part of the path.
5. Select **Add and check**.

The path must already exist, must be a folder, must be a full path, and must not sit inside or around another Library folder. HolyArrs must be able to write there.

First-run setup adds one folder with these same rules. Add a folder for each other kind you use later.

## What the check tells you

Each row shows the kind, the path, free space, health and when it was last checked. The check makes sure that:

- the folder exists,
- HolyArrs can write a small test file there,
- downloads can be hardlinked into it from each active Downloader folder,
- at least 100 MB is free.

Test files are removed when the check ends. Select **Check again** to run it again.

- **Missing** or **Read only**: HolyArrs will not use the folder until you fix it.
- **No hardlink**: a warning. Imports still work, but files are copied. That takes longer and uses more disk space.
- A low space note can show on a folder that otherwise works.

In Docker, separate `/movies`, `/tv`, `/music` and `/downloads` mounts mean imports are copies. For hardlinks, mount one shared parent folder holding downloads and media instead, on one filesystem. See [Install HolyArrs with Docker](../getting-started/install.md).

## Rename or remove a folder

**Rename** changes only the name you see. Nothing moves on disk.

**Remove** takes the folder out of HolyArrs and leaves every file where it is. HolyArrs refuses while titles still use the folder. Move those titles first.

## Extra import folders

**Library > Import files by hand** only reads, copies and moves files inside these places:

- your Library folders,
- the folders your Downloaders save into, as HolyArrs sees them. That is the save path set at start (`HOLYARRS_SAVE_PATH`) and the HolyArrs side of each path map on a Downloader that is still set up,
- the **Extra import folders** on this page.

To add one, type one full path per line in **Extra import folders**, such as `/imports` or `D:\Imports`. You can list up to 32. A whole disk, such as `/` or `C:\`, is refused.

Only the owner or an admin signed in with a password can change this list. An API key also needs the admin scope. People with **Change settings** can see the list.

## Troubleshooting

**Import says the path is not allowed.** The path is outside every place listed above. The message names the allowed places. Paths with `..`, network share paths such as `\\server\share`, and links or Windows junctions that lead outside are refused too.

**Import cannot see the download folder.** If `HOLYARRS_SAVE_PATH` is not set and no Downloader has a path map, the download folder is not allowed. Set `HOLYARRS_SAVE_PATH` (see [Install](../getting-started/install.md)) or add the download folder, such as `/downloads`, as an extra import folder.

**No hardlink on Docker.** Downloads and media are on different mounts. Mount one shared parent folder holding both instead.

**Cannot remove a folder.** Titles still use it. Move them to another folder first.
