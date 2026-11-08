# Searching for music

This page covers how HolyArrs finds Releases for the albums of your artists: by itself, when you press a button, and when you choose a release yourself. Movies and shows work the same way and are covered in [Searching for movies](search-movies.md) and [Searching for shows](search-shows.md). Only the owner and admins (people who may manage the Library) see any of this; members never do.

## Albums and tracks

Each track of an album has its own place in each Version, but HolyArrs looks for music **one album at a time**: one Release is chosen for every track it holds and sent to your Downloader once, as one download.

HolyArrs reads album names like these:

- `Velvet Cartography - Low Tide Atlas (2019) [FLAC]`
- `Velvet Cartography - Low Tide Atlas (2019) [MP3 320]`, `[MP3 V0]`, `[AAC 256]`, `[FLAC 24-96]`
- `Velvet_Cartography-Low_Tide_Atlas-WEB-FLAC-2019-GROUP` (the scene style)
- `Velvet Cartography - Harbour Songs (2021) CD2 [FLAC]` (one disc: it fills only that disc's tracks)

It matches the artist by name (or another name the artist is known by) and the album by its name among the artist's albums. When two albums share a name, the year tells them apart. A year that differs from the album's own (a remaster from a later year) is fine; the Release just says so.

## Music Pick rules

Music has its own Pick rules, from worst to best:

| Step | What it means |
|---|---|
| Lossy, low | under 192 kbps |
| Lossy, middle | 192 to 256 kbps, MP3 V1 or V2, or a lossy format with no bit rate given |
| Lossy, best | MP3 320 or V0, AAC 256 and up, Opus 160 and up |
| Lossless | FLAC, ALAC, WAV, APE or WavPack at CD quality |
| Lossless hi-res | 24-bit, or a sample rate above 48 kHz |

Three sets come with HolyArrs:

- **Lossless first** (what new artists get): takes the best lossy copy until a lossless one turns up; good enough at lossless.
- **Lossy 320 or V0**: small files; good enough at the best lossy step.
- **Any music**: takes anything, best first.

A CD rip, a web purchase and a rip log or cue sheet earn points; a vinyl rip loses some (it still passes). A whole album in one file with a cue sheet, a Release that says it is a transcode, samples, programs and password archives are always turned away. Movie Pick rules cannot judge music, so an artist always uses music Pick rules.

## How HolyArrs looks by itself

- **Feed checks.** The same feed check that serves movies and shows also reads the music categories of your Sources while an album is missing a track. A new album Release that fits is judged and chosen or held.
- **The nightly search** covers albums too (Settings > Search, **Search for missing albums too**), one search per album. Albums that are not out yet are never searched.
- **Find a better copy.** An album below **Good enough** keeps looking. A new Release replaces it only when it is better for **every** track that already has a copy, so a better album never swaps some tracks for worse ones.

An album is only looked for when the artist, the album, the track and the Version all have **Keep up to date** on.

## Searching by hand

- On the **artist page**, **Search now** looks for every album with a missing track, and the search button on each album row looks for that album.
- On the **album page**, **Search now** (or **Find a better copy**) lets HolyArrs pick, and **Choose a release** lists every Release found with its verdict and the reasons in plain words. Choose one and it fills every track it holds; Turn down one and it is never picked again for this album.
- Home shows each search as "Artist – Album", and so do the Held and Turned down lists.

A search you start for an album that is not out yet still runs; every verdict then says the date check was skipped.

## Early and fake Releases

A Release posted before the album came out, or far too small for it, is turned away like for movies; see [Early and fake Releases](search-movies.md#early-and-fake-releases).

## Settings > Search, Music

- **Search for missing albums too**: on by default.
- **Take whole discographies**: off by default, so a Release holding every album of an artist is turned away; on, it may fill the album searched for.
- **Tracks an album Release may lack**: when a Source says how many files a Release holds, a Release with fewer files than the album's tracks is turned away. 0 (the default) allows none missing; -1 never checks.

## What happens after a choice

The chosen Release goes to your Downloader as one download. Placing the finished files into your music folder comes in a later version: until then a finished album download stays in your Downloader and nothing is moved or lost.
