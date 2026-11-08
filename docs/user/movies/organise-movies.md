# Organise your movies

You already have movies on disk, or you want tidy file names, or you want a movie gone. This page covers all three, plus the naming settings. Only the owner and admins see these screens.

## Bring in movies you already have

HolyArrs can take over a folder of movies as it is. Nothing is moved or renamed.

1. Open the **Library**, then **Import a library**.
2. Pick one of your Library folders. If it is not listed, add it first on **Settings**, **Library folders**.
3. Press **Scan**. HolyArrs reads the name and year from each folder and looks the movie up on TMDB. A folder name with a TMDB number, such as `Heat (1995) {tmdb-949}`, always matches that movie.
4. Check the groups:
   - **Matched**: HolyArrs is sure. These are ticked for you.
   - **Unsure**: a likely movie. Check it, or press **Look up** to search by name, name and year, or `tmdb:123`, then **Use this**.
   - **Not matched**: press **Look up** to find the movie yourself.
   - Movies already in the Library, on the Never-add list, or with no video file are shown so you know why they are left out.
5. Tick the movies you want and press **Adopt**.

The biggest video in each folder becomes the movie's file. Samples are left out. One scan reads up to 5000 folders. A bigger library shows **Scan the rest**.

Once a day HolyArrs checks the Library on disk. A file that disappeared is marked missing, and one that comes back is marked present. If a whole Library folder is gone, or most files in it vanish at once, HolyArrs warns you and marks nothing missing. This covers a disk that did not mount.

## Rename files

1. Open a movie and press **Rename files**. To do every movie, open the **Library**, then **Rename files**.
2. Check the preview. It shows each file's name now and the name it gets.
3. Press **Rename**.

If anything changed since you opened the preview, nothing is renamed and you get a fresh preview.

Some files are skipped, and the screen says why. Files HolyArrs only found on disk are never renamed. A file is never overwritten when another file already has the new name. A file is also skipped if a link would take it outside the Library folder, or if the new path is too long.

A rename changes the file name only. Folders keep their names.

## Delete a movie

1. Open the movie and press **Delete**.
2. Read **What will happen**. It updates as you change the choices:
   - **Also delete its files** (off by default). Off, every file stays on disk. On, files HolyArrs imported or adopted move to the holding folder, `.holyarrs-replaced` inside their Library folder. They are removed for good after 7 days. Files HolyArrs only found are never touched.
   - **Add it to the Never-add list** (off by default). Outside lists then never add it again. You can still add it yourself.
3. Confirm.

The movie leaves the Library and HolyArrs stops looking for it. Its history stays. Downloads still running for it stay in the Downloader. You can remove them on [Downloads](downloads-movies.md).

## Settings

Open **Settings**, then **Naming**.

**Names**

- **File name pattern** and **Folder name pattern**: text with words in braces, such as `{title} ({year}) {quality}`. The example under the patterns shows the result as you type. A word HolyArrs does not know cannot be saved.
- The folder pattern is used for movies added from now on.
- **A colon in a name becomes**: many disks cannot hold a colon. The default turns "Dune: Part Two" into "Dune - Part Two".
- **Rename files when they are imported**: on by default. Off keeps the name the download came with.

**Where files go**

| Setting | What it does | Default |
| --- | --- | --- |
| Keep deleted files for (days) | How long deleted or replaced files stay in the holding folder. 0 keeps them until you empty it. | 7 |
| Always leave this much free space (MB) | An import that would leave less free space is refused before anything is written. 100 is the lowest. | 100 |
| Skip the free-space check | Only for disks that report their free space wrong. | Off |

**Empty the holding folder now** clears old files right away instead of waiting for the daily cleanup.

## Troubleshooting

- **A folder shows as Not matched.** Add the year or a TMDB number to the folder name, or use **Look up**.
- **Many movies suddenly show as missing.** Check that the disk or share is mounted.
- **A rename skipped a file.** Read the reason on the preview. Most often another file already has that name.
- **I deleted a movie by mistake.** Add it again. If you deleted its files, they are in `.holyarrs-replaced` until the holding days run out.
