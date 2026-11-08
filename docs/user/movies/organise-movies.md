# Organise movies

This page covers four things: bringing movies you already have into HolyArrs, renaming files, deleting a movie, and the settings for names and disk space. Only the owner and admins see any of this; members never do.

## Import a library you already have

If your movies are already on disk, HolyArrs can take them over as they are. Open **Library**, then **Import a library**.

**Nothing is moved or renamed.** Scanning only reads the folder. Adopting adds each movie with its folder and file exactly where they are. You can rename them later if you want.

1. **Pick a folder.** Choose one of your Library folders. If the folder is not listed, add it first in **Settings**, **Library folders**.
2. **Scan.** HolyArrs looks at each folder inside it, reads the movie name and year from the folder name, and looks the movie up on TMDB. A folder name with a TMDB number in it, such as `Heat (1995) {tmdb-949}`, is always matched to that movie: the TMDB number wins over the name and year in the folder name. A scan reads up to 5000 folders (change it in Settings > Naming and files); a bigger library shows **Scan the rest** to read the next part. If the TMDB lookup fails for one folder, that folder shows as not matched with the reason, and the others are still matched.
3. **Check the groups.**
   - **Matched**: HolyArrs is sure. These are ticked for you.
   - **Unsure**: a likely movie, with a score. Check it. If a list of other movies is shown, pick the right one; or press **Look up** and search by name, name and year, or `tmdb:123`, then **Use this**.
   - **Not matched**: nothing found from the folder name. Press **Look up** to find the movie yourself.
   - **Already in the Library**, **On the Never-add list** and **No video file** are shown so you know why they are left out.
4. **Adopt.** Tick the movies you want and press **Adopt**. The screen shows progress, then which movies were added (with a link to each) and which were not, with the reason.

The biggest video in each folder becomes the movie's file. Samples are left out. HolyArrs then keeps the movie up to date like any other.

Once a day HolyArrs checks the Library on disk. A file that disappeared is marked missing; one that comes back is marked present again. If a whole Library folder is gone (for example a disk that is not mounted), HolyArrs warns you and marks nothing missing. The same happens when every file it knows in a Library folder, or more than half of ten or more, disappears at once, as when one disk of a pool drops out.

## Rename files

HolyArrs names files by the file name pattern (see Settings below). Files you adopt keep their old names until you ask for a rename.

- **One movie**: open the movie and press **Rename files**.
- **Every movie**: open **Library**, then **Rename files**.

Both show the name each file has now and the name it gets. Nothing changes until you press **Rename**. What the preview shows is exactly what happens: if anything changed since you opened it (a new file arrived, the naming settings changed), nothing is renamed and you get a fresh preview to check.

Some files are **skipped**, and the screen says why:

- Files HolyArrs only found on disk (not imported or adopted) are never renamed.
- If another file already has the new name, that file is skipped rather than overwritten.
- If a link (symlink or junction) would take the file or its new name outside the Library folder, it is skipped.
- If the new path would be longer than this system allows, it is skipped. Each file or folder name is kept to 200 bytes, with the extension kept.

If one file cannot be renamed, the others still are, and the result says which one failed and why.

A rename only changes the file's name inside its current folder. Folders keep their names. A rename that only changes capitals (`.MKV` to `.mkv`) is safe on Windows and macOS too. If HolyArrs stops half-way (a restart, a power cut), it finishes the rename when it starts again, so no file is lost or doubled.

## Delete a movie

Open the movie and press **Delete**. Before anything happens, the dialog shows **What will happen**, and it updates as you change the two choices:

- **Also delete its files** (off by default). Off: every file stays on disk. On: files HolyArrs imported or adopted move to the **holding folder** inside their Library folder (`.holyarrs-replaced`), and are removed for good after the holding days (7 by default). Until then you can get them back from there. Files HolyArrs only found are never touched.
- **Add it to the Never-add list** (off by default). Outside lists then never add it again. You can still add it yourself.

The movie leaves the Library and HolyArrs stops looking for it. Its history stays, and you can add it again later. Downloads still running for it stay in the Downloader; the dialog says how many, and you can remove them from **Downloads**.

## Settings: Naming and files

Open **Settings**, then **Naming and files**.

**Names**

- **File name pattern** and **Folder name pattern**: plain text with words in braces, such as `{title} ({year}) {quality}`. Open **Words you can use** to see every word and add one with a click. A word with nothing to fill in is dropped, with its brackets. The **Example** under the patterns shows what they give while you type. A word HolyArrs does not know is shown as an error and cannot be saved.
- The folder pattern is used for movies added from now on. Folders you already have keep their names.
- **A colon in a name becomes**: Windows and many shares cannot hold a colon. Smart (the default) turns "Dune: Part Two" into "Dune - Part Two".
- **Other characters a disk cannot hold**: swap them for safe ones, or drop them.
- **Rename files when they are imported**: on by default. Off keeps the name the download came with.

**Where files go**

- **Keep deleted files for (days)**: how long deleted or replaced files stay in the holding folder. 7 by default. 0 keeps them until you empty it yourself.
- **Always leave this much free space (MB)**: an import that would leave less than this free is refused before anything is written, and the reason says how much space was needed. 100 MB is the lowest.
- **Skip the free-space check**: only for disks that report their free space wrongly.

**Holding folder**: HolyArrs empties old holding days once a day. **Empty old holding days now** does it straight away.
