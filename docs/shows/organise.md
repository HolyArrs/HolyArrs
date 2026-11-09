---
title: Organise shows
description: Adopt shows you already have, rename episode files, delete safely and follow an episode's history.
level: Setup
---

Bring in shows already on disk, tidy names, delete what you no longer watch and see what happened to an episode. Only the owner and admins see these screens.

## Bring in shows you already have

HolyArrs takes over a show folder as it is. The scan only reads. Nothing is moved or renamed.

1. Open **Library**, then **Import a library**, and pick a Library folder for shows. They are listed as "Shows: ...".
2. Press **Scan**. HolyArrs reads the show name, and the year if there is one, from each folder name and looks it up at your show details source.
3. Open each show's summary line, like "8 episodes in 7 files; 1 file with no episode, left alone". Each video is listed with the episodes its name gives.
4. Check the groups: Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No video file. Press **Look up** on an unsure or unmatched show to search by name or by an id like `tvdb:81189`.
5. Tick the shows you want and press **Adopt**.

You should see every episode with a file in the Library, or looking for a better copy when the file is below Good enough. The other episodes follow your show settings.

:::tip
A folder name with an id, like `Copper Valley (2025) {tvdb-81189}`, always matches that show.
:::

If one lookup fails, that folder shows as not matched with the reason and the rest go on.

These files are **left alone**, with the reason: a file that names another show, a file with no episode in its name, and a smaller copy of an episode another file holds.

## Rename episode files

Episode files follow the patterns in **Settings > Naming and files > Show names**, see [Show downloads](downloads.md#for-experts).

1. For one show, open it and press **Rename files**. For everything, open **Library** and press **Rename files**.
2. Read the preview: each file's name now, the name it gets, and why any file is skipped.
3. Press **Rename**.

You should see the new names on the show page. Rename does exactly what the preview showed. If something changed in between, nothing is renamed and you get a fresh preview.

Only files HolyArrs imported or adopted are renamed. A file holding two episodes gets one name for both. A show in several Versions is renamed per Version, and each file stays in its own Version's folder.

## Delete a show, a season or an episode

:::caution
**Also delete the files** moves files to the holding folder. They are removed for good after the holding days (7 by default).
:::

1. Open the show. Press **Delete show** for the whole show, **Delete Season 1** (or whichever season) on a season, or **Delete episode** on an episode.
2. Read **What will happen** in the dialog.
3. Choose your options and confirm.

You should see a deleted show leave the Library. A deleted season or episode stays on the show page with **Keep up to date** off. Turn it back on whenever you want it.

| Option or case | What happens |
| --- | --- |
| Default | Every file stays on disk. |
| Also delete the files | Files HolyArrs imported or adopted move to the holding folder inside their Library folder. Take them back from there before the holding days run out. Files HolyArrs only found are never touched. |
| A file with several episodes | Goes only when every episode in it is deleted. The dialog says when a file stays. |
| Several Versions | Every Version's copy goes. |
| Add it to the Never-add list | Shown for a whole show. Lists can then never add it again. |
| Empty folders | Empty season and show folders go with the files. A folder with anything else in it stays. |

When files are deleted, your media servers are told to rescan that show's folder.

## Choices

### History

The show page has a **History** card with the latest rows. **All 12 rows** (the number varies) opens the full History for the show. Rows name their episodes, like "S01E02 The Mill", and their Version when the show has more than one. Each episode has a **History** button with its searches, the release sent, the import, renames, deletes and decisions.

### Calendar

**Calendar** shows air dates next to movie dates. An episode with a known air time shows at that time in your time zone, so a late evening episode abroad may land on the next day. Filter with **Everything**, **Movies** or **Shows**. An episode in several Versions is listed once and names the Versions you have. The calendar feed has episodes at their air time, or as all-day events when the time is unknown.

### Alerts

Show Alerts name the show and episodes, like "In the Library: Copper Valley S01E02". A season pack sends one Alert listing every episode. For one Alert per episode, change **Season packs** on **Settings > Alerts**.

## If it does not work

**The scan finds nothing in a folder that has shows.**
Cause: a drive or share is not mounted. HolyArrs says so when it knows files in the folder but sees it empty.
Fix: mount it and scan again.

**An episode shows as missing.**
Cause: the daily disk check found its file gone. History names it. If a whole Library folder is gone, HolyArrs warns you and marks nothing missing.
Fix: put the file back. It is marked present again.

**A new file in a show folder is not used.**
Cause: a video HolyArrs finds on its own is recorded with its episodes but never fills an episode by itself, and is never renamed, moved or deleted.
Fix: import the folder again to adopt it.

**A file is skipped in the rename preview.**
Cause: only imported or adopted files are renamed.
Fix: read the reason on the preview, or adopt the file first.

## For experts

<details class="ha-more"><summary>Naming examples</summary>

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
Night Talk (2019)/Season 2025/Night Talk - 2025-03-14 - Guests 1080p WEB.mkv
Blade Song (2015)/Season 5/Blade Song - S05E12 - 112 - Rain 1080p WEB.mkv
```

The last two show the air day and running number layouts. Patterns and defaults are in [Show downloads](downloads.md#for-experts).

</details>

<details class="ha-more"><summary>Per-episode Alerts limit</summary>

With **Season packs** set to one Alert per episode, a big pack sends at most 25 single Alerts and one more listing the rest. Change the number, 1 to 500, with the `each_limit` API setting.

</details>

## Related

- [Show downloads](downloads.md)
- [Search for shows](search.md)
- [Organise movies](../movies/organise.md)
