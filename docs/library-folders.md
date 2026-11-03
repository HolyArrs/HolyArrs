# Library folders

A Library folder is where HolyArrs keeps finished movies, shows or music. Open
**Settings > Library folders** to add and check them. This screen requires the
**Change settings** permission.

## Add a folder

1. Select whether the folder holds **Movies**, **Shows** or **Music**. Each kind
   uses its own folders.
2. Enter the full path exactly as HolyArrs sees it. In Docker this is a
   container path such as `/data/media/movies`, not the host path.
3. Optionally enter a name. When it is empty, HolyArrs uses the last part of
   the folder path.
4. Select **Add and check**.

The path must already exist, be a folder, be absolute, and not overlap another
Library folder. The running HolyArrs user must be able to write there.

First-run setup adds one folder with the same rules. Add separate folders later
for every enabled kind you use.

## Health

Each row shows the folder kind, path, free space, health reason and last check.
The check verifies:

- the folder exists;
- HolyArrs can create a temporary file there;
- downloads can be hardlinked into it from each active Downloader folder; and
- at least 100 MB is free.

Probe files are removed after the check. **Check again** repeats it.

`Missing` and `Read only` prevent the folder from being used. `No hardlink` is
a warning: imports still work by copying files, which uses more time and disk
space. A low-space note may appear with an otherwise usable folder.

For Docker, put downloads and media below the same `/data` mount so both paths
can share a filesystem. See [Install HolyArrs with Docker](install.md).

## Rename or remove

**Rename** changes only the display name. It does not move the folder or its
files.

**Remove** removes the folder from HolyArrs and leaves every file on disk.
HolyArrs refuses to remove it while Titles still use it; move those Titles
first.


## Folders for Import files by hand

**Library > Import files by hand** reads, copies and moves files only inside
these places:

- the Library folders;
- the folders the Downloaders save into, as HolyArrs sees them: the save path
  set at start and the HolyArrs side of every path map of a Downloader that is
  still set up; and
- the **Extra import folders** on this page.

Any other path is refused, and the answer names the places above. A path with
`..`, a network share path (`\server\share`) and a link that leads out of a
place are refused too.

Add an extra import folder for loose files that sit somewhere else, such as
`/data/imports` or `D:\Imports`: one full path per line, up to 32. Saving them
needs the **Change settings** permission; an API key also needs the admin
scope.
