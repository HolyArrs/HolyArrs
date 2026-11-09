---
title: Organise music
description: Bring in the music you already have, set folder and track names, and rename or delete albums.
level: Setup
sidebar: { order: 3 }
---

Bring your music on disk into HolyArrs, name tracks your way, and rename or delete artists, albums and tracks.

Only the owner and admins see this. Movies and shows work much the same way, see [Organise movies](../movies/organise.md) and [Organise shows](../shows/organise.md).

## Import the music you already have

Importing only reads the folder. Nothing is moved or renamed. HolyArrs expects one folder per artist, with one folder per album inside:

```
Lab Lanterns/Small Hours (2025)/01 - Tide.flac
Lab Lanterns/2019 - Harbour Songs/CD1/01 - Pier.mp3
Lab Lanterns/Lab Lanterns - Night Ferry (2022)/01 - Fog.mp3
```

1. Open **Library > Import a library** and pick a music Library folder (listed as "Music: ...").
2. Press **Scan**.
   You should see each artist with a confidence score and a summary, such as "12 tracks in 2 albums; 1 file left alone".
3. Open an artist's summary to check each album folder, the pressing it fits, and each file's track. Anything left alone shows the reason and is never touched.
4. For an artist under Unsure or Not matched, press **Look up** and search by name or by `mbid:<MusicBrainz id>`.
5. Tick the artists you want and press **Adopt**.
   You should see each artist in your Library with its files filled in.

Album folders named "Album (Year)", "Year - Album" and "Artist - Album (Year)" all match. HolyArrs picks the pressing whose track count matches your files. If that is not the usual pressing, the album's History says so.

## Change how tracks are named

Open **Settings > Naming and files > Music names**. By default an album looks like this:

```
Lab Lanterns/Small Hours (2025)/03 - Harbour Light.flac
Lab Lanterns/Long Night (2025)/2-04 - Second Wind.flac
```

The second line is a two-disc album: the disc number goes in front. Examples under the patterns change as you type. Every pattern and token is in [Music naming reference](naming-reference.md).

If **Rename files when they are imported** is off, the album folder still follows the pattern, but each track keeps the name it came with.

## Rename files

1. Pick what to rename: press **Rename files** on an album or artist page, or open **Library > Rename files** for everything.
2. Read the preview. It lists each file's name now and its new name. Skipped files say why.
3. Press **Rename**.
   You should see the new names on the album page.

The rename does exactly what the preview showed. If a pattern or file changed in between, nothing is renamed and you see a new preview. Only files HolyArrs imported or adopted are renamed.

## Delete an artist, an album or a track

1. Press **Delete** on the artist page, **Delete album** on the album page, or the bin button on a track's line.
2. Read the dialog. The delete does exactly what it shows.
3. Confirm.

| Choice | What happens |
|---|---|
| Default | Files stay on disk. A deleted artist leaves the Library. A deleted album or track stays on its page with **Keep up to date** off. Turn it back on to get it back. |
| **Also delete its files** (whole artist) or **Also delete their files** (an album or track) | Files HolyArrs imported or adopted move to the holding folder inside their Library folder. Files HolyArrs only found are never touched. |
| **Add it to the Never-add list** | Shown when you delete a whole artist. Outside lists can never add it again. You still can. |

:::caution
Files in the holding folder are removed for good after the holding days, 7 by default. Take them back before then.
:::

Empty album and disc folders go with the files. A folder with anything else in it, such as a cover or a booklet, stays.

## Where albums show up

| Place | What you see |
|---|---|
| **History** | The artist page shows the latest rows; **All … rows** (with the count) opens the rest. The album page has its own History: searches, downloads, imports, renames, deletes and pressing changes. |
| **Calendar** | Album release days, with how many tracks you have. Press **Music** for albums only. The phone calendar feed has them as all-day events. |
| **Alerts** | One Alert per album download, tracks listed one per line. For one per track, set **Settings > Alerts > Season packs** to one Alert each: at most 25 single Alerts, plus one listing the rest. |

When files are imported, renamed or deleted, your media servers rescan that album's folder only.

## If it does not work

**Symptom:** the scan finds nothing in a folder you know has music.
**Cause:** a drive or network share is not mounted. The scan says so when HolyArrs already knows files there.
**Fix:** mount it and scan again.

**Symptom:** an album folder is left alone.
**Cause:** its name matched no album of the artist, or it is an album kind you leave out, such as a single or a live album.
**Fix:** rename the folder to "Album (Year)" and scan again, or use **Look up**.

**Symptom:** files are left alone.
**Cause:** their names and tags did not point to one track.
**Fix:** fix the names or tags and scan again.

**Symptom:** a rename shows the preview again.
**Cause:** something changed after the preview.
**Fix:** check the new preview and press **Rename** again.

## For experts

<details class="ha-more"><summary>Daily check of the Library</summary>

Once a day HolyArrs checks the Library on disk:

- A track file that disappeared is marked missing. History names the artist, album and track.
- A file that comes back is marked present again.
- A new audio file in an artist's folder is noted as found. It never fills a track by itself, and is never renamed, moved or deleted.
- If a whole Library folder is gone, or all its files vanish at once, HolyArrs warns you and marks nothing missing.

</details>

<details class="ha-more"><summary>Import groups and lookup</summary>

Artists land in one of 6 groups: Matched, Unsure, Not matched, Already in the Library, On the Never-add list, No audio file. Each artist folder is looked up at MusicBrainz by name. On adopt, albums and pressings come from MusicBrainz and every matched file becomes the file of its track.

</details>

## Related

- [Music naming reference](naming-reference.md)
- [Downloads for music](downloads.md)
- [Protected copies](../library/protected-copies.md)
