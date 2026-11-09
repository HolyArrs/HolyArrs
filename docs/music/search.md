---
title: Search for music
description: How HolyArrs finds albums for your artists, how to search by hand, and how it judges a music Release.
level: Setup
sidebar: { order: 1 }
---

HolyArrs finds each album of your artists as one Release and sends it to your Downloader as one download.

Only the owner and admins see music search. Movies and shows work the same way, see [Search for movies](../movies/search.md) and [Search for shows](../shows/search.md).

## Before you start

- A Downloader and a Source are connected, see [Connect a downloader and a source](../start/connect.md).
- The artist, album, track and Version all have **Keep up to date** on. An album with any of these off is never searched.

## Search an album by hand

1. Open the artist page.
2. Press **Search now** to search every album with a missing track, or press the search button on one album row.
   You should see the search on Home, named after the artist and album.
3. To pick the Release yourself, open the album page and press **Choose a release**.
   You should see every Release found, with its verdict and the reasons in plain words.
4. Choose one. It fills every track it holds.

Turn a Release down and HolyArrs never picks it again for this album.

## What HolyArrs searches by itself

| When | What happens |
|---|---|
| Feed check | While an album misses a track, HolyArrs reads the music categories of your Sources. A new Release that fits is chosen or held. |
| Nightly search | One search per missing album. Turn it off with **Search for missing albums too**. |
| Better copies | An album below **Good enough** keeps looking. A new Release replaces it only if it is better for every track you already have. |

Albums that are not out yet are never searched by themselves. A hand search still runs, and each verdict says the date check was skipped.

## Choices

### Pick rules for music

Music has its own Pick rules. Movie Pick rules cannot judge music. New artists get **Lossless first**.

| Set | Takes | Good enough |
|---|---|---|
| **Lossless first** | the best lossy copy until a lossless one turns up | Lossless |
| **Lossy 320 or V0** | lossy middle and lossy best, for small files | Lossy best |
| **Any music** | anything, best first | Lossless |

### Settings

Open **Settings > Search** and find the music part.

| Setting | What it does | Default | Change it when |
|---|---|---|---|
| **Search for missing albums too** | The nightly search covers albums. | On | You want only feed checks for music. |
| **Take whole discographies** | A Release with every album of an artist may fill the album you searched for. | Off | Your Sources mostly post full discographies. |
| **Tracks an album Release may lack** | Turns away a Release whose file count falls short of the album's track count by more than this. `-1` turns the check off. Only works where a Source gives file counts. | `0` | Your Sources give wrong file counts. |

## If it does not work

**Symptom:** an album is never searched.
**Cause:** **Keep up to date** is off somewhere, or the album is not out yet.
**Fix:** turn it on for the artist, album, track and Version.

**Symptom:** a Release is turned away and you do not know why.
**Fix:** open the album, press **Choose a release** and read the reasons beside it.

**Symptom:** a Release posted before the album came out is turned away.
**Cause:** early and far too small Releases are turned away on purpose, see [Early and fake Releases](../movies/search.md#fake-and-early-releases).

**Symptom:** a discography Release is turned away.
**Fix:** turn on **Take whole discographies**.

## For experts

<details class="ha-more"><summary>Quality steps</summary>

| Step | Meaning |
|---|---|
| Lossy, low | under 192 kbps |
| Lossy, middle | 192 to 256 kbps, MP3 V1 or V2, or lossy with no bit rate given |
| Lossy, best | MP3 320 or V0, AAC 256 and up, Opus 160 and up |
| Lossless | FLAC, ALAC, WAV, APE or WavPack at CD quality |
| Lossless hi-res | 24-bit, or a sample rate above 48 kHz |

A CD rip, a web purchase, a rip log and a cue sheet earn points. A vinyl rip loses some points but still passes.

Always turned away: a whole album in one file with a cue sheet, a lossy transcode, samples, programs and password-protected archives.

</details>

<details class="ha-more"><summary>Release names HolyArrs reads</summary>

- `Velvet Cartography - Low Tide Atlas (2019) [FLAC]`
- `Velvet Cartography - Low Tide Atlas (2019) [MP3 320]`, `[MP3 V0]`, `[AAC 256]`, `[FLAC 24-96]`
- `Velvet_Cartography-Low_Tide_Atlas-WEB-FLAC-2019-GROUP`
- `Velvet Cartography - Harbour Songs (2021) CD2 [FLAC]` fills only the tracks of disc 2.

The artist is matched by name, or by another name the artist is known by. The album is matched by name among that artist's albums. When two albums share a name, the year decides. A different year, such as a later remaster, still matches, and the verdict says so.

</details>

## Related

- [Downloads for music](downloads.md)
- [Organise music](organise.md)
- [Search your library](../downloads/library-search.md)
