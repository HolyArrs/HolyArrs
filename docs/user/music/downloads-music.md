# Downloads for music

You want a downloaded album to land in your Library with the right names. This page shows how HolyArrs places the tracks, what it does when an album is missing tracks or fails, and what to do when something gets stuck.

Only the owner and admins see this. Shows work much the same way, see [Downloads for shows](../shows/downloads-shows.md).

## How an album comes in

1. HolyArrs sends the album Release to your Downloader as one download.
2. When the Downloader is done, HolyArrs matches each audio file to a track of the album's chosen pressing.
3. It checks that the whole album fits on the disk.
4. It places each track in the artist's folder, in a folder for the album.

Matching uses the file name first:

- `01 - Dawn.flac` is track 1.
- `Lab Band - First Light - 03 - Night Train.flac` is track 3.
- `CD2/01 - Night Train.flac`, `2-01 Night Train.flac` and `201 Night Train.flac` are disc 2, track 1.
- `Night Train.flac`, with no number, is the track with that name.

When the name is not enough, HolyArrs can read the file's own tags. See [Organise music](organise-music.md#reading-a-tracks-own-tags). When the number points to one track and the title to another, the file is left alone. HolyArrs never guesses.

The result looks like this:

```
Lab Band/First Light (2020)/01 - Dawn.flac
Lab Band/First Light (2020)/02 - Glass River.flac
Lab Band/Second Tide (2023)/2-01 - Night Train.flac
```

An album with more than one disc keeps all tracks in one album folder. Each name starts with its disc. You can change the names, see [Organise music](organise-music.md#how-track-files-are-named).

When the download and the Library are on the same disk, files are hardlinked. They take no extra space and your Downloader can keep sharing them. On another disk they are copied. HolyArrs never moves or deletes the Downloader's files, and never writes over a file it did not put there.

## What is left alone

- Cover pictures, logs, cue sheets, playlists and text files stay in the download.
- A bonus track whose number is not on the chosen pressing is left alone and listed in the import.
- Tracks you already have, or that are not kept up to date, are left alone.
- When two files fit one track, such as a FLAC and an MP3, the bigger one is used.

## When an album does not hold everything

Each track is settled on its own:

- Tracks the album holds go into the Library.
- Tracks it should have held but does not get searched again. The Release is turned down for those tracks only. It still counts for the others.
- A file that cannot be placed, such as a file the Downloader listed that is not on the disk, puts its track on **Needs a decision**.

The row on the Downloads screen says where every track ended up, for example "3 of 4 tracks in the library; Track 04 needs a decision."

## Better copies

When a better album comes in for tracks you already have, the old file moves to the holding folder and the new one takes its place. If HolyArrs stops in the middle, it picks up where it left off the next time it runs. A finished album download that was never imported is imported again by itself.

## When an album download fails

If the download fails or stalls too long, HolyArrs:

1. turns the Release down for every track of the album,
2. removes the download from your Downloader, if that setting is on,
3. searches the album once more and grabs the next Release that passes your Pick rules.

The stall limits are the same settings used for movies and shows.

## The Downloads screen

An album download reads as **Artist – Album (Year)**, with how many tracks it covers, such as "12 tracks" or "2 discs, 20 tracks". Open the list of tracks to see where each one is: downloading, in the Library, looking for another copy, or needing a decision.

## Needs a decision

A track HolyArrs cannot finish by itself shows here by name, for example "Lab Band – First Light Track 04 Last Call". When several tracks of one album download are stuck, they share one card with these buttons:

- **Search again** searches again for these tracks.
- **Add the album to Turned down** never picks this Release again for these tracks, then searches again.
- **Stop looking for these tracks** turns Keep up to date off for each of them. The rest of the artist goes on. Files on disk are not touched.

Each track also has its own buttons. **Add to Turned down** on one track is for that track only.

## Troubleshooting

**No track was placed and the album waits on Needs a decision.** The disk was too full. HolyArrs checks the whole album first, and if one track does not fit, none is placed. Free some space, then press **Search again**, or let the download import again.

**A track stays missing after the album came in.** The file name and tags did not point to one track, or the number is not on the chosen pressing. Check the import list on the Downloads screen for the reason.

**Files were copied instead of hardlinked.** The download folder and the Library are on different disks. Put them on the same disk to save space.
