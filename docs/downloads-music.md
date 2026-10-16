# Downloads for music

This page covers what happens after HolyArrs sends an album Release to your Downloader: how the tracks are put in the Library, what happens when an album is missing tracks or fails, and where to look when something gets stuck. Shows work much the same way and are covered in [Downloads for shows](downloads-shows.md). Only the owner and admins (people who may manage the Library) see any of this.

## One download, a whole album

An album Release holds every track of the album. HolyArrs sends it to your Downloader once, as one download for all the tracks it fills.

When the Downloader is done, HolyArrs matches each audio file to a track of the album's chosen pressing. It reads the file name, not the tags inside the file:

- `01 - Dawn.flac` is track 1;
- `Lab Band - First Light - 03 - Night Train.flac` is track 3;
- `CD2/01 - Night Train.flac`, `2-01 Night Train.flac` and `201 Night Train.flac` are disc 2, track 1;
- `Night Train.flac`, with no number, is the track whose name is in the file name.

When the number in a name points to one track and the title to another, HolyArrs leaves the file alone rather than guess.

Each track then goes into the artist's folder in the Library, in a folder for the album:

```
Lab Band/First Light (2020)/01 - Dawn.flac
Lab Band/First Light (2020)/02 - Glass River.flac
Lab Band/Second Tide (2023)/2-01 - Night Train.flac
```

An album with more than one disc keeps all its tracks in one album folder, each name starting with its disc. Settings for these names come later.

Files are hardlinked when the download and the Library are on the same disk, so they take no extra space and the Downloader can keep sharing them. On another disk they are copied. HolyArrs never moves or deletes the Downloader's files, and never writes over a file it did not put there.

## What is left alone

- **Cover pictures, logs, cue sheets, playlists and text files** stay in the download.
- **Bonus tracks**: a file whose number is not on the chosen pressing is left alone and listed in the import.
- **Tracks this download was not meant to fill** (you already have them, or they are not kept up to date) are left alone.
- **Two files for one track** (a FLAC and an MP3 side by side): the bigger one is used.

## Free space

HolyArrs checks that the whole album fits before placing anything. If one track would not fit, no track is placed, and the album's tracks wait on **Needs a decision** with the reason. Free some space, then choose **Search again**, or let the download be imported again.

## When an album does not hold everything

HolyArrs settles each track on its own:

- **Tracks it holds** go into the Library.
- **Tracks it should have held but does not** look for another copy. The Release is added to Turned down for those tracks only; it still counts for the others.
- **A file that cannot be placed** (for example, a file the Downloader listed that is not on the disk) puts its track on **Needs a decision**.

The row on the Downloads screen says where every track ended up, for example "3 of 4 tracks in the library; Track 04 needs a decision."

## Better copies

When a track already has a copy and a better album comes in, the old file is moved to the holding folder and the new one takes its place. If HolyArrs stops while it is doing this, it picks up where it left off the next time it runs, and a finished album download that was never imported is imported again by itself.

## When an album download fails

If the download fails or stalls for too long (the same settings as for movies and shows), HolyArrs:

1. adds the Release to Turned down for every track of the album,
2. removes the download from your Downloader (if that setting is on),
3. searches the album once more and grabs the next Release that passes your Pick rules.

## The Downloads screen

An album download reads as **Artist – Album (Year)**, with how many tracks it covers ("12 tracks", or "2 discs, 20 tracks"). Open **The 12 tracks it covers** to see where each track is: downloading, in the Library, looking for another copy, or needing a decision.

## Needs a decision

A track that HolyArrs cannot finish on its own shows here by name, for example "Lab Band – First Light Track 04 Last Call". When several tracks of one album download are stuck, they share one card that lists the whole album, with:

- **Search again for these tracks**,
- **Add the album to Turned down**: never pick this Release again for these tracks, and search again,
- **Stop looking for these tracks**: Keep up to date is turned off for each of them; the rest of the artist goes on, and files on disk are not touched.

Each track also has its own buttons. **Add to Turned down** on one track is for that track only.
