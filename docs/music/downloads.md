---
title: Downloads for music
description: How a downloaded album lands in your Library, what happens to missing tracks, and how to fix stuck tracks.
level: Setup
sidebar: { order: 2 }
---

A finished album download lands in your Library as named tracks, one folder per album.

Only the owner and admins see this. Shows work much the same way, see [Downloads for shows](../shows/downloads.md).

## How an album comes in

1. HolyArrs sends the album Release to your Downloader as one download.
2. When it finishes, HolyArrs matches each audio file to a track of the album's chosen pressing.
3. It checks that the whole album fits on the disk. If one track does not fit, none is placed.
4. It places each track in the artist's folder, in a folder for the album.
   You should see the album on the Downloads screen by artist, album and year, with its track count, such as "12 tracks" or "2 discs, 20 tracks".

```
Lab Band/First Light (2020)/01 - Dawn.flac
Lab Band/First Light (2020)/02 - Glass River.flac
Lab Band/Second Tide (2023)/2-01 - Night Train.flac
```

A multi-disc album keeps every track in one album folder, each name starting with its disc. To change names, see [Organise music](organise.md#change-how-tracks-are-named).

:::tip
Put downloads and the Library on the same disk. HolyArrs then hardlinks each file, a second name for the same data, so it takes no extra space and your Downloader keeps sharing it. On another disk files are copied. See [Folders](../start/folders.md).
:::

A finished Usenet download is moved instead, which is instant on the same disk; if it can't be moved, it is copied. A Downloader marked read-only keeps its files. HolyArrs never writes over a file it did not put there.

## When an album misses tracks

Each track is handled on its own:

- Tracks the album holds go into the Library.
- Tracks it should hold but does not are searched again. The Release is turned down for those tracks only.
- A file that cannot be placed, such as one the Downloader listed that is not on disk, sends its track to **Needs a decision**.

The Downloads row says where every track went, for example "3 of 4 tracks in the library; Track 04 needs a decision." Open the track list to see each one: downloading, in the Library, looking for another copy, or needing a decision.

## When an album download fails

If the download fails or stalls too long, HolyArrs:

1. turns the Release down for every track of the album,
2. removes the download from your Downloader, if that setting is on,
3. searches once more and grabs the next Release that passes your Pick rules.

The stall limits are the same settings movies and shows use.

## Needs a decision

A stuck track shows here by name. Stuck tracks from one album download share one card:

| Button | What it does |
|---|---|
| **Search again** | Searches again for these tracks. |
| **Add the album to Turned down** | Never picks this Release again for these tracks, then searches again. |
| **Stop looking for these tracks** | Turns **Keep up to date** off for each. The rest of the artist carries on. Files on disk are not touched. |

Each track also has its own buttons. **Add to Turned down** on a track covers that track only.

## If it does not work

**Symptom:** no track was placed and the album waits on **Needs a decision**.
**Cause:** the disk was too full for the whole album.
**Fix:** free space, then press **Search again**, or let the download import again.

**Symptom:** a track stays missing after the album came in.
**Cause:** the file name and tags did not point to one track, or the number is not on the chosen pressing.
**Fix:** read the reason in the import list on the Downloads screen.

**Symptom:** files were copied instead of hardlinked.
**Cause:** downloads and the Library are on different disks.
**Fix:** put them on the same disk, see [Folders](../start/folders.md).

## For experts

<details class="ha-more"><summary>How files are matched to tracks</summary>

The file name comes first:

| File | Track |
|---|---|
| `01 - Dawn.flac` | track 1 |
| `Lab Band - First Light - 03 - Night Train.flac` | track 3 |
| `CD2/01 - Night Train.flac`, `2-01 Night Train.flac`, `201 Night Train.flac` | disc 2, track 1 |
| `Night Train.flac` | the track with that name |

When the name is not enough, HolyArrs reads the file's tags, see [Reading a track's own tags](naming-reference.md#reading-a-tracks-own-tags). When the number points to one track and the title to another, the file is left alone. HolyArrs never guesses.

</details>

<details class="ha-more"><summary>What is left alone</summary>

- Covers, logs, cue sheets, playlists and text files stay in the download.
- A bonus track whose number is not on the chosen pressing is left alone and listed in the import.
- Tracks you already have, or that are not kept up to date.
- When two files fit one track, such as a FLAC and an MP3, the bigger one is used.

</details>

<details class="ha-more"><summary>Better copies and restarts</summary>

When a better album comes in for tracks you have, the old file moves to the holding folder and the new one takes its place. If HolyArrs stops halfway, it picks up where it left off on the next run. A finished album download that was never imported is imported again by itself.

</details>

## Related

- [Search for music](search.md)
- [Organise music](organise.md)
- [Downloaders](../downloads/downloaders.md)
