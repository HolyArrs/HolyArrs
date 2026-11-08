# Organise your shows

You already have shows on disk, or you want to tidy names, delete what you no longer watch, or see what happened to an episode. This page covers all of that. Movies work much the same way, see [Organise movies](../movies/organise-movies.md). Only the owner and admins see these screens.

## Bring in shows you already have

HolyArrs can take over a show folder as it is. Scanning only reads the folder, and adopting adds each show with its files where they are. Nothing is moved or renamed.

1. Open **Library**, then **Import a library**, and pick a Library folder for shows. They are listed as "Shows: ...".
2. **Scan.** HolyArrs reads the show name, and the year when there is one, from each folder name and looks it up at your show details source. A folder name with an id, like `Copper Valley (2025) {tvdb-81189}`, always matches that show. If one lookup fails, that folder shows as not matched with the reason, and the rest go on.
3. **Check each show's files.** Open the line that sums them up, like "8 episodes in 7 files; 1 file with no episode, left alone". Each video is listed with the episodes its name gives. A file that names another show, a file with no episode in its name, and a smaller copy of an episode another file holds are **left alone**, with the reason. They are never touched.
4. **Check the groups**: Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No video file. Press **Look up** on an unsure or unmatched show to search by name or by an id like `tvdb:81189`.
5. **Adopt.** Tick the shows you want and press **Adopt**. Every episode with a file is in the Library, or looks for a better copy when the file is below Good enough. The other episodes follow your show settings.

## Rename episode files

Episode files follow the patterns in **Settings > Naming and files > Show names**:

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
Night Talk (2019)/Season 2025/Night Talk - 2025-03-14 - Guests 1080p WEB.mkv
Blade Song (2015)/Season 5/Blade Song - S05E12 - 112 - Rain 1080p WEB.mkv
```

1. For one show, open it and press **Rename files**. For everything, open **Library** and press **Rename files**.
2. Read the preview. It lists each file's name now, the name it gets, and why any file is skipped.
3. Press **Rename**. It does exactly what the preview showed. If something changed in between, nothing is renamed and you see a fresh preview.

Only files HolyArrs imported or adopted are renamed. A file holding two episodes gets one name for both. A show kept in several Versions is renamed per Version, and each file stays in its own Version's folder.

The patterns and their defaults are listed in [Downloads for shows](downloads-shows.md#settings).

## Delete a show, a season or an episode

1. Open the show. Press **Delete** for the whole show, **Delete season** on a season, or **Delete** on an episode's line.
2. Read the dialog. It shows what will happen before anything changes.
3. Choose your options and confirm.

What the options do:

- By default every file stays on disk. A deleted show leaves the Library. A deleted season or episode stays on the show page, but HolyArrs stops keeping it up to date. Turn Keep up to date back on whenever you want it.
- **Also delete the files** moves the files HolyArrs imported or adopted into the holding folder inside their Library folder. They are removed after the holding days. Until then you can take them back from there. Files HolyArrs only found are never touched.
- A file holding several episodes goes only when every episode in it is deleted. The dialog tells you when a file stays.
- Every Version's copy goes. For a show kept in HD and 4K, both copies are deleted.
- **Add it to the Never-add list** appears when you delete a whole show. Lists can then never add it again.
- Empty season and show folders go with the files. A folder with anything else in it stays.

When files are deleted, your media servers are told to look at that show's folder again.

## See the history of a show or an episode

The show page has a History card with the latest rows. **All rows** opens the full History for the show. Rows name their episodes, like "S01E02 The Mill" or "S01E01-E03, 3 episodes". When the show has more than one Version, each row names its Version too.

Each episode has a **History** button for that episode alone: its searches, the Release sent, the import, renames, deletes and decisions.

## Calendar

The **Calendar** shows when your episodes air, next to movie dates. An episode with a known air time shows at that time in your own time zone, so a late evening episode abroad may land on the next day for you. Use **Everything**, **Movies** or **Shows** to filter. An episode with several Versions is listed once and says which Versions you have.

The calendar feed for your phone has the episodes too, at their air time, or as all-day events when the time is not known.

## Alerts

Alerts about shows name the show and episodes, like "In the Library: Copper Valley S01E02". A season pack sends one Alert listing every episode it brought.

To get one Alert per episode instead, open **Settings > Alerts** and change **Season packs**. A big pack then sends at most 25 single Alerts and one more that lists the rest. You can change that number from 1 to 500 with the `each_limit` API setting.

## Troubleshooting

**The scan finds nothing in a folder that has shows.** If HolyArrs knows files in that folder but sees it empty, it says a drive or network share may not be mounted. Mount it and scan again.

**An episode shows as missing.** Once a day HolyArrs checks the Library on disk. A file that disappeared is marked missing, and History names it. When the file comes back, it is marked present again. If a whole Library folder is gone, HolyArrs warns you and marks nothing missing.

**A new file in a show folder is not used.** A video HolyArrs finds on its own is recorded with the episodes its name gives, but it never fills an episode by itself. HolyArrs never renames, moves or deletes it. Import the folder again to adopt it.

**A file is skipped in the rename preview.** Only files HolyArrs imported or adopted are renamed. The preview gives the reason for each skipped file.
