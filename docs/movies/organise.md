---
title: Organise movies
description: Bring in movies already on disk, rename files and delete movies safely.
level: Setup
---

Adopt movies you already have, tidy file names or remove a movie. Only the owner and admins see these screens.

## Bring in movies you already have

HolyArrs takes over a folder of movies as it is. It moves and renames nothing.

1. Open **Library**, then **Import a library**.
2. Pick a Library folder. If it is not listed, add it on **Settings > Library folders**.
3. Press **Scan**. HolyArrs reads the name and year from each folder and looks the movie up on TMDB.
4. Check the groups:

   | Group | Do |
   | --- | --- |
   | Matched | Nothing. These are ticked for you. |
   | Unsure | Check it, or press **Look up** and search by name, name and year, or `tmdb:123`. Then press **Use this**. |
   | Not matched | Press **Look up** and find the movie. |

   Movies already in the Library, on the Never-add list or without a video file are listed with the reason they are left out.
5. Tick the movies you want and press **Adopt**. The button shows the count, such as **Adopt 3 movies**.

You should see the movies in **Library** with their files present.

A folder name with a TMDB number, such as `Heat (1995) {tmdb-949}`, always matches that movie. The biggest video in each folder becomes the movie's file. Samples are left out.

This tree adopts cleanly:

```
Movies/
  Heat (1995) {tmdb-949}/
    Heat (1995) Bluray-1080p.mkv
  Dune - Part Two (2024)/
    Dune - Part Two (2024) WEBDL-2160p.mkv
```

## Rename files

1. For one movie, open it and press **Rename files**. For every movie, open **Library**, then **Rename files**.
2. Read the preview: each file's name now and the name it gets.
3. Press **Rename**.

You should see the new names on the movie page. If anything changed since the preview opened, HolyArrs renames nothing and shows a fresh preview.

A rename changes file names only. Folders keep their names. The preview says why it skips a file:

- HolyArrs only found the file on disk and did not import or adopt it.
- Another file already has the new name. HolyArrs never overwrites.
- A link would take the file outside the Library folder.
- The new path is too long.

## Delete a movie

:::caution
**Also delete its files** moves files to the holding folder. HolyArrs removes them for good after 7 days by default.
:::

1. Open the movie and press **Delete**.
2. Read **What will happen**. It updates as you change the options:
   - **Also delete its files** (off). Off, every file stays on disk. On, files HolyArrs imported or adopted move to `.holyarrs-replaced` inside their Library folder. HolyArrs never touches files it only found.
   - **Add it to the Never-add list** (off). Outside lists can then never add the movie again. You can still add it yourself.
3. Confirm.

You should see the movie gone from **Library**. Its history stays. Downloads still running for it stay in the Downloader. Remove them on [Movie downloads](downloads.md#remove-a-download).

## Choices

### Missing files

Once a day HolyArrs checks the Library on disk. A file that disappeared is marked missing, and one that comes back is marked present.

If a whole Library folder is gone, or most of its files vanish at once, HolyArrs warns you and marks nothing missing. This covers a disk that did not mount.

### File names

A colon becomes " - " by default, because Windows and many shares cannot store colons. "Dune: Part Two" is filed as "Dune - Part Two". Change this, and the patterns, under **Settings > Naming and files**.

## If it does not work

**A folder shows as Not matched.**
Fix: add the year or a TMDB number to the folder name, or press **Look up**.

**Many movies suddenly show as missing.**
Cause: the disk or share is not mounted.
Fix: mount it. HolyArrs marks the files present at the next daily check.

**A rename skipped a file.**
Fix: read the reason on the preview. Most often another file already has that name.

**I deleted a movie by mistake.**
Fix: add it again. If you deleted its files, take them back from `.holyarrs-replaced` before the holding days run out.

## For experts

<details class="ha-more"><summary>Settings reference: Settings > Naming and files</summary>

**Where files go**

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| Keep deleted files for (days) | How long deleted or replaced files stay in `.holyarrs-replaced`. 0 never removes them. Maximum 3650. | 7 | You want a longer undo window or need the space. |
| Folders read per library scan | 10 to 100000. A bigger Library shows **Scan the rest**. | 5000 | Scans of a huge Library are slow. |
| Always leave this much free space (MB) | HolyArrs refuses an import that would leave less, before it writes anything. 100 to 1048576. | 100 | Your disk needs more headroom. |
| Skip the free-space check | Turns the check off. | Off | Your disk misreports free space. |

**Movie names**

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| File name pattern | Text with words in braces, such as `{title} ({year}) {quality}`. Empty words drop out. The example shows the result as you type. HolyArrs cannot save an unknown word. | `{title} ({year}) {quality}` | Your media server wants another layout. |
| Folder name pattern | Same, for folders. Applies to movies added from now on. | `{title} ({year})` | Same. |
| A colon in a name becomes | Replaces the colon. | Smart | You prefer another style. |
| Other characters a disk cannot hold | Swap them for safe ones, or drop them. | Swap | Same. |
| Rename files when they are imported | Off keeps the name the download came with. | On | You rename files yourself. |

</details>

## Related

- [Editions and collections](editions-collections.md)
- [Movie downloads](downloads.md)
- [Organise shows](../shows/organise.md)
