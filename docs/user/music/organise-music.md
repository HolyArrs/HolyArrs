# Organise music

This page covers how track files and folders are named, bringing music you already have into HolyArrs, renaming track files, deleting an artist, an album or a track, and what the History, the Calendar and Alerts say about albums. Movies and shows work much the same way and are covered in [Organise movies](organise-movies.md) and [Organise shows](organise-shows.md). Only the owner and admins see any of this; members never do.

## How track files are named

Track files are named by the music patterns in **Settings**, **Naming and files**, **Music names**. Out of the box an album looks like this:

```
Lab Lanterns/Small Hours (2025)/03 - Harbour Light.flac
Lab Lanterns/Long Night (2025)/2-04 - Second Wind.flac
```

The second line is a track of an album with two discs: the disc number goes in front of the track number. The patterns are:

- **Artist folder pattern** (`{artist}`): used when an artist is added. `{artist_sort}` moves a leading The to the end, so "The Beatles" becomes "Beatles, The". Folders of artists you already have keep their names.
- **Album folder pattern** (`{album} ({year})`): it must hold `{album}`. `{edition}` adds the Pressing's note, such as deluxe; `{album_type}` adds Album or EP.
- **Track file name pattern** (`{track} - {track_title}`): it must hold `{track}`, the track number with two digits.
- **Track file name on an album of more than one disc** (`{disc}-{track} - {track_title}`): it must hold `{disc}` and `{track}`.
- **A folder for each disc** (off): on an album of more than one disc, each disc gets its own folder named by the **disc folder pattern** (`Disc {disc}`), and the plain track pattern is used inside it.

Other words: `{track_artist}` (the track's own artist on a various-artists album; for now this is always the album's artist), `{format}` (FLAC, MP3) and `{year}`. Below the patterns, the examples show what a track, a track of a two-disc album and a track of a various-artists album would be called, and they change as you type. A pattern that cannot work (a track pattern without `{track}`, a movie word such as `{quality}`) is refused with the reason.

The colon and character rules above the movie patterns apply to music too. A very long album or track name is shortened so the folder or file name stays under 240 bytes; the year, the track number and the extension always stay whole.

Album downloads are placed with these patterns. If you turned off **Rename files when they are imported**, the album folder still follows the pattern but each track file keeps the name it came with.

### Reading a track's own tags

HolyArrs matches a track file to its track by its name: a track number (`03`, `1-03`, `103` on a two-disc album), a disc folder (`CD2`, `Disc 2`) and the track's name. When the name is not enough (no number and no track name, a name two tracks share, or a number and a name that point at different tracks), it reads the file's own tags: the title and the track and disc numbers of FLAC and MP3 files. A tag title must be the track's name, so a file with wrong tags is left alone rather than placed on the wrong track. Tags are only read, never changed. Turn **Read a track's own tags when its name is not enough** off to match by names alone.

## Import a music library you already have

If your music is already on disk, HolyArrs can take it over as it is. Open **Library**, then **Import a library**, and pick a Library folder for music (they are listed as "Music: ...").

**Nothing is moved or renamed.** Scanning only reads the folder. Adopting adds each artist with its folder and files exactly where they are.

HolyArrs expects one folder per artist, with one folder per album inside it:

```
Lab Lanterns/Small Hours (2025)/01 - Tide.flac
Lab Lanterns/2019 - Harbour Songs/CD1/01 - Pier.mp3
Lab Lanterns/Lab Lanterns - Night Ferry (2022)/01 - Fog.mp3
```

1. **Scan.** Each folder is looked up at MusicBrainz by its name and given a confidence score. For the artist that fits best, each album folder is matched to one of the artist's albums by its name and year ("Album (Year)", "Year - Album" and "Artist - Album (Year)" all work). For each album, HolyArrs picks the Pressing whose number of tracks is the number of audio files in the folder (by your Pressing rule when several fit), and matches each file to one of its tracks the same way album downloads are matched.
2. **Check each artist.** Under each artist, open the line that sums it up, for example "12 tracks in 2 albums; 1 file left alone". Every album folder is listed with the album and Pressing it fits and how many of its files hold a track, and every file with its track. Files outside an album folder, album folders that fit no album, albums your album kinds leave out (a single, a live album) and files whose track cannot be told are **left alone** with the reason. They are never touched.
3. **Check the groups** (Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No audio file). Press **Look up** on an unsure or unmatched artist to search by name or by `mbid:<MusicBrainz id>`.
4. **Adopt.** Tick the artists you want and press **Adopt**. Each artist is added with its folder as it is, its albums and Pressings are read from MusicBrainz, and every matched file becomes the file of its track. Choosing a different Pressing to fit your files shows on the album's History.

If the Library folder is empty while HolyArrs knows files in it, the scan says a drive or network share may not be mounted, instead of finding nothing.

Once a day HolyArrs checks the Library on disk. A track file that disappeared is marked missing (History names the artist, album and track); one that comes back is marked present again. A new audio file in an artist's folder is recorded as found, but it never fills a track on its own and HolyArrs never renames, moves or deletes it. If a whole Library folder is gone, or every file of it disappears at once, HolyArrs warns you and marks nothing missing.

## Rename files

- **One album**: open the album and press **Rename files**. Only that album's files are listed.
- **One artist**: open the artist and press **Rename files**.
- **Everything**: open **Library**, then **Rename files**.

The preview lists each file's name now and the name it gets; when the album folder changes too, the whole path is shown. Files that are skipped say why. Nothing changes until you press **Rename**, and the rename does exactly what the preview showed; if something changed in between (a pattern, a file), nothing is renamed and the preview is shown again. Only files HolyArrs imported or adopted are renamed, always inside the artist's folder, and a rename that only changes upper and lower case works on every disk.

## Delete an artist, an album or a track

- **A whole artist**: open the artist and press **Delete**.
- **An album**: open the album and press **Delete album**.
- **One track**: press the bin button on the track's line on the album page.

The dialog shows what will happen before anything changes, and the delete does exactly that.

- By default **every file stays on disk**. A deleted artist leaves the Library; a deleted album or track stays on its page, but HolyArrs stops keeping it up to date. Turn Keep up to date on again whenever you want it back.
- **Also delete their files** moves the files HolyArrs imported or adopted into the holding folder inside their Library folder. They are removed for good after the holding days (7 unless you change it); until then you can take them back from there. Files HolyArrs only found are never touched.
- **Add it to the Never-add list** is there when you delete a whole artist: outside lists can then never add it again. You can still add it yourself.
- Empty album and disc folders go with the files; a folder with anything else in it (a cover, a booklet) stays.

## History of an artist or an album

The artist page has a History card with the latest rows; **All rows** opens the full History for the artist. The album page has its own History: the searches, downloads and imports of its tracks, renames, deletes, and Pressing changes of that album only.

## Calendar

The **Calendar** shows the days your artists' albums come out, next to movie dates and episodes, with how many of an album's tracks are in the Library. Use **Music** to see albums only; an album leads to its own page. The phone calendar feed has the albums too, as all-day events.

## Alerts

An album download is one Alert: "In the Library: Lab Lanterns – Small Hours (2025)", with its tracks listed one per line. If you would rather get one Alert per track, change **Settings**, **Alerts**, **Season packs** to one Alert each: the same setting covers album downloads. A big album then sends at most 25 single Alerts and one more that lists the rest.

Deleting an album or tracks is a delete moment for Alerts. When files are imported, renamed or deleted, your media servers are told to look at that album's folder again, not the whole artist.
