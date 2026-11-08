# Organise music

You want your music files named your way, your old music brought in, and a way to rename or delete albums. This page covers file names, importing a music library you already have, renaming, deleting, and what History, the Calendar and Alerts show for albums.

Only the owner and admins see this. Members never do. Movies and shows work much the same way, see [Organise movies](../movies/organise-movies.md) and [Organise shows](../shows/organise-shows.md).

## Import a music library you already have

If your music is already on disk, HolyArrs can take it over as it is. Nothing is moved or renamed. Scanning only reads the folder. Adopting adds each artist with its folder and files where they are.

HolyArrs expects one folder per artist, with one folder per album inside:

```
Lab Lanterns/Small Hours (2025)/01 - Tide.flac
Lab Lanterns/2019 - Harbour Songs/CD1/01 - Pier.mp3
Lab Lanterns/Lab Lanterns - Night Ferry (2022)/01 - Fog.mp3
```

1. Open **Library**, then **Import a library**. Pick a music Library folder. They are listed as "Music: ...".
2. **Scan.** Each artist folder is looked up at MusicBrainz by name and gets a confidence score. Each album folder is matched to one of the artist's albums by name and year. "Album (Year)", "Year - Album" and "Artist - Album (Year)" all work. HolyArrs picks the pressing whose track count matches the audio files in the folder, then matches each file to a track.
3. **Check each artist.** Open the summary line under an artist, for example "12 tracks in 2 albums; 1 file left alone". You see each album folder, the album and pressing it fits, and each file with its track. Files outside an album folder, album folders that fit no album, album kinds you leave out (a single, a live album) and files with no clear track are left alone, with the reason. They are never touched.
4. **Check the groups.** Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No audio file. Press **Look up** on an unsure or unmatched artist to search by name, or by `mbid:<MusicBrainz id>`.
5. **Adopt.** Tick the artists you want and press **Adopt**. Each artist is added with its folder, its albums and pressings come from MusicBrainz, and every matched file becomes the file of its track. If a different pressing was picked to fit your files, the album's History says so.

### Checking the Library each day

Once a day HolyArrs checks the Library on disk.

- A track file that disappeared is marked missing. History names the artist, album and track.
- A file that comes back is marked present again.
- A new audio file in an artist's folder is noted as found. It never fills a track by itself, and HolyArrs never renames, moves or deletes it.
- If a whole Library folder is gone, or all its files vanish at once, HolyArrs warns you and marks nothing missing.

## How track files are named

Open **Settings**, then **Naming and files**, then **Music names**. Out of the box an album looks like this:

```
Lab Lanterns/Small Hours (2025)/03 - Harbour Light.flac
Lab Lanterns/Long Night (2025)/2-04 - Second Wind.flac
```

The second line is from an album with two discs. The disc number goes in front of the track number.

The patterns:

- **Artist folder pattern**, default `{artist}`. Used when an artist is added. `{artist_sort}` moves a leading The to the end, so "The Beatles" becomes "Beatles, The". Folders of artists you already have keep their names.
- **Album folder pattern**, default `{album} ({year})`. It must hold `{album}`. `{edition}` adds the pressing's note, such as deluxe. `{album_type}` adds Album or EP.
- **Track file name pattern**, default `{track} - {track_title}`. It must hold `{track}`, the track number with two digits.
- **Track file name on an album with more than one disc**, default `{disc}-{track} - {track_title}`. It must hold `{disc}` and `{track}`.
- **A folder for each disc**, off by default. When on, each disc of a multi-disc album gets its own folder, named by the disc folder pattern (default `Disc {disc}`). The plain track pattern is used inside it.

You can also use `{format}` (FLAC, MP3) and `{year}`. `{track_artist}` is planned for a track's own artist on a various-artists album. For now it is always the album's artist.

Below the patterns, examples show a normal track, a track of a two-disc album and a track of a various-artists album. They change as you type. A pattern that cannot work, such as a track pattern without `{track}` or a movie word like `{quality}`, is refused with the reason.

The colon and character rules for movies apply to music too. A very long album or track name is shortened so each folder or file name stays under 240 bytes. The year, the track number and the extension always stay whole.

If **Rename files when they are imported** is off, the album folder still follows the pattern, but each track file keeps the name it came with.

### Reading a track's own tags

HolyArrs matches a file to its track by name: a track number (`03`, `1-03`, or `103` on a two-disc album), a disc folder (`CD2`, `Disc 2`) and the track name.

When the name is not enough, it reads the file's tags: the title and the track and disc numbers of FLAC and MP3 files. This happens when there is no number and no name, when two tracks share a name, or when the number and the name point to different tracks. The tag title must match the track's name, so a file with wrong tags is left alone. Tags are only read, never changed.

To match by names only, turn off **Read a track's own tags when its name is not enough**.

## Rename files

1. Pick what to rename:
   - One album: open the album and press **Rename files**.
   - One artist: open the artist and press **Rename files**.
   - Everything: open **Library**, then **Rename files**.
2. Read the preview. It lists each file's name now and the name it gets. When the album folder changes too, the whole path is shown. Skipped files say why.
3. Press **Rename**.

The rename does exactly what the preview showed. If something changed in between, such as a pattern or a file, nothing is renamed and you see the preview again. Only files HolyArrs imported or adopted are renamed, always inside the artist's folder. A rename that only changes upper and lower case works on every disk.

## Delete an artist, an album or a track

1. Pick what to delete:
   - A whole artist: open the artist and press **Delete**.
   - An album: open the album and press **Delete album**.
   - One track: press the bin button on the track's line on the album page.
2. Read the dialog. It shows what will happen, and the delete does exactly that.
3. Confirm.

What the choices mean:

- By default every file stays on disk. A deleted artist leaves the Library. A deleted album or track stays on its page, but HolyArrs stops keeping it up to date. Turn **Keep up to date** on again to get it back.
- **Also delete their files** moves the files HolyArrs imported or adopted to the holding folder inside their Library folder. They are removed for good after the holding days, 7 unless you change it. Until then you can take them back. Files HolyArrs only found are never touched.
- **Add it to the Never-add list** shows when you delete a whole artist. Outside lists can then never add it again. You can still add it yourself.

Empty album and disc folders go with the files. A folder with anything else in it, such as a cover or a booklet, stays.

## History

The artist page has a History card with the latest rows. **All rows** opens the full History for that artist. The album page has its own History: searches, downloads and imports of its tracks, renames, deletes and pressing changes for that album only.

## Calendar

The **Calendar** shows the days your artists' albums come out, next to movies and episodes, with how many of each album's tracks you have. Press **Music** to see albums only. An album leads to its own page. The phone calendar feed has the albums too, as all-day events.

## Alerts

An album download sends one Alert, such as "In the Library: Lab Lanterns – Small Hours (2025)", with its tracks listed one per line.

To get one Alert per track instead, open **Settings**, then **Alerts**, and set **Season packs** to one Alert each. The same setting covers albums. A big album then sends at most 25 single Alerts, plus one more that lists the rest.

Deleting an album or tracks counts as a delete for Alerts. When files are imported, renamed or deleted, your media servers are told to look at that album's folder again, not the whole artist.

## Troubleshooting

**The import scan finds nothing in a folder you know has music.** If HolyArrs already knows files there, the scan says a drive or network share may not be mounted. Mount it and scan again.

**An album folder is left alone during import.** Its name did not match an album of the artist, or it is an album kind you leave out. Rename the folder to "Album (Year)" and scan again, or use **Look up** to pick the right artist.

**Files are left alone during import.** Their names and tags did not point to one track. Fix the file names or tags and scan again.

**A pattern is refused.** Read the reason under it. A track pattern needs `{track}`, and the multi-disc pattern needs `{disc}` and `{track}`.

**A rename shows the preview again instead of renaming.** Something changed after the preview. Check the new preview and press **Rename** again.
