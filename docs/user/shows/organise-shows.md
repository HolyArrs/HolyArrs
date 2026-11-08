# Organise shows

This page covers bringing shows you already have into HolyArrs, renaming episode files, deleting a show, a season or an episode, and what the History, the Calendar and Alerts say about episodes. Movies work much the same way and are covered in [Organise movies](organise-movies.md). Only the owner and admins see any of this; members never do.

## Import a show library you already have

If your shows are already on disk, HolyArrs can take them over as they are. Open **Library**, then **Import a library**, and pick a Library folder for shows (they are listed as "Shows: ...").

**Nothing is moved or renamed.** Scanning only reads the folder. Adopting adds each show with its folder and files exactly where they are.

1. **Scan.** HolyArrs looks at each folder inside the Library folder, reads the show name (and year, when there is one) from the folder name, and looks the show up at your show details source (Settings > Shows). A folder name with an id in it, such as `Copper Valley (2025) {tvdb-81189}`, is always matched to that show. If the lookup fails for one folder, that folder shows as not matched with the reason, and the others go on.
2. **Check each show's files.** Under each show, open the line that sums up its files, for example "8 episodes in 7 files; 1 file with no episode, left alone". Every video is listed with the episodes its name gives:
   - `Copper.Valley.S01E02.1080p.WEB.mkv` is S01E02;
   - `Copper Valley - S01E02-E03 - The Mill.mkv` holds S01E02 and S01E03 in one file;
   - a show ordered by air day or by running number is read by its date or number, as for downloads.

   A file that names another show, a file with no episode in its name (an interview, a trailer) and a smaller copy of an episode another file already holds are listed as **left alone** with the reason. They are never touched.
3. **Check the groups** (Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No video file) just as for movies. Press **Look up** on an unsure or unmatched show to search by name or by an id such as `tvdb:81189`.
4. **Adopt.** Tick the shows you want and press **Adopt**. Each show is added with its folder as it is. Every episode with a file is in the Library (or looks for a better copy, when the file is below the Pick rules' good enough), and the other episodes follow your show settings: which seasons to keep up to date and the episode order.

If the Library folder is empty while HolyArrs knows files in it, the scan says a drive or network share may not be mounted, instead of finding nothing.

Once a day HolyArrs checks the Library on disk. An episode file that disappeared is marked missing (History names the show and episode); one that comes back is marked present again. A new video in a show's folder is recorded as found, with the episodes its name gives, but it never fills an episode on its own and HolyArrs never renames, moves or deletes it. If a whole Library folder is gone, HolyArrs warns you and marks nothing missing.

## Rename files

Episode files are named by the show patterns in **Settings**, **Naming and files**, **Show names**:

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
Night Talk/Season 2026/Night Talk - 2026-03-14 - Guests 1080p WEB.mkv
Blade Song/Season 5/Blade Song - S05E12 - 112 - Rain 1080p WEB.mkv
```

- **One show**: open the show and press **Rename files**.
- **Every movie and show**: open **Library**, then **Rename files**.

The preview lists each file's name now and the name it gets, and why any file is skipped. Nothing changes until you press **Rename**, and the rename does exactly what the preview showed; if something changed in between, nothing is renamed and the preview is shown again. Only files HolyArrs imported or adopted are renamed. A file holding two episodes gets one name for both.

## Delete a show, a season or an episode

- **A whole show**: open the show and press **Delete**.
- **A season**: open the season on the show page and press **Delete season**.
- **One episode**: press **Delete** on the episode's line.

The dialog shows what will happen before anything changes, and the delete does exactly that.

- By default **every file stays on disk**. A deleted show leaves the Library; a deleted season or episode stays on the show page, but HolyArrs stops keeping it up to date. Turn Keep up to date on again whenever you want it back.
- **Also delete the files** moves the files HolyArrs imported or adopted into the holding folder inside their Library folder. They are removed for good after the holding days (7 unless you change it); until then you can take them back from there. Files HolyArrs only found are never touched.
- **A file holding several episodes** goes only when every episode it holds is deleted. Delete S01E02 alone and a file holding S01E02 and S01E03 stays for S01E03; the dialog says so.
- **Every Version's copy goes**: for a show kept in HD and 4K, deleting an episode with its files takes both copies.
- **Add it to the Never-add list** is there when you delete a whole show: lists can then never add it again.
- Empty season folders and an empty show folder go with the files; a folder with anything else in it stays.

Better copies work the same way: the file they replace goes to the holding folder, and a file holding several episodes is replaced only when every one of them has a better copy.

A show kept in several Versions is renamed per Version: each file stays inside its own Version's show folder.

## History of a show or an episode

The show page has a History card with the latest rows; **All rows** opens the full History for the show. Every row about a show names its episodes, such as "S01E02 The Mill" or "S01E01-E03, 3 episodes" for a season pack. When the show has more than one Version, each row also names its Version, such as 4K.

Each episode has a **History** button that opens the History of that episode alone: its searches, the Release sent to download (also when it came in a season pack), the import, renames, deletes and decisions. In **Downloads**, **History** you can also pick any movie or show.

## Calendar

The **Calendar** shows when the episodes of your shows air, next to the movie dates. An episode with a known air time is shown at that time in your own time zone, so an episode that airs late in the evening in another country may land on the next day for you. An episode with only an air day is shown on that day. Use **Everything**, **Movies** or **Shows** to see one kind. For a show with several Versions, an episode is listed once and says which Versions are in the Library.

The phone calendar feed has the episodes too: each one at its air time for its running time, or as an all-day event when the time is not known. Your phone shows them in your time zone.

## Alerts

Alerts about a show name the show and its episodes, for example "In the Library: Copper Valley S01E02". A season pack sends one Alert that lists every episode it brought, one per line. If you would rather get one Alert per episode, change it in **Settings**, **Alerts**, **Season packs**. A big pack then sends at most 25 single Alerts and one more Alert that lists the rest; the API setting `each_limit` changes that number.

Deleting a season or episodes is a delete moment for Alerts. When files were deleted too, your media servers are told to look at the show's folder again (only that folder).
