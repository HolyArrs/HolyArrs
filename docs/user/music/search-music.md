# Search for music

You want the albums of your artists found and downloaded. This page shows how HolyArrs searches for them, how to search by hand, and how to choose a Release yourself.

Only the owner and admins see this. Members never do. Movies and shows work the same way, see [Search for movies](../movies/search-movies.md) and [Search for shows](../shows/search-shows.md).

## How it works

HolyArrs looks for music one album at a time. It picks one Release for all the tracks it holds and sends it to your Downloader as one download.

An album is only searched when the artist, the album, the track and the Version all have **Keep up to date** on. Albums that are not out yet are never searched by themselves.

HolyArrs searches by itself in three ways:

- **Feed checks.** While an album is missing a track, the feed check also reads the music categories of your Sources. A new Release that fits is judged, then chosen or held.
- **The nightly search.** It covers albums too, one search per album.
- **Better copies.** An album below **Good enough** keeps looking. A new Release replaces it only when it is better for every track that already has a copy. A better album never swaps some tracks for worse ones.

## Search by hand

1. Open the artist page.
2. Press **Search now** to search every album with a missing track. Or press the search button on one album row.
3. To choose for yourself, open the album page and press **Choose a release**.
4. You see every Release found, with its verdict and the reasons in plain words.
5. Choose one and it fills every track it holds. Turn one down and it is never picked again for this album.

Home shows each search as "Artist – Album". The Held and Turned down lists use the same name.

If you search an album that is not out yet, the search still runs. Every verdict then says the date check was skipped.

## Album names HolyArrs reads

- `Velvet Cartography - Low Tide Atlas (2019) [FLAC]`
- `Velvet Cartography - Low Tide Atlas (2019) [MP3 320]`, `[MP3 V0]`, `[AAC 256]`, `[FLAC 24-96]`
- `Velvet_Cartography-Low_Tide_Atlas-WEB-FLAC-2019-GROUP`
- `Velvet Cartography - Harbour Songs (2021) CD2 [FLAC]`. This fills only the tracks of disc 2.

The artist is matched by name, or by another name the artist is known by. The album is matched by name among the artist's albums. When two albums share a name, the year tells them apart. A different year, such as a later remaster, is fine. The Release just says so.

## Music Pick rules

Music has its own Pick rules. The steps, from worst to best:

| Step | What it means |
|---|---|
| Lossy, low | under 192 kbps |
| Lossy, middle | 192 to 256 kbps, MP3 V1 or V2, or a lossy file with no bit rate given |
| Lossy, best | MP3 320 or V0, AAC 256 and up, Opus 160 and up |
| Lossless | FLAC, ALAC, WAV, APE or WavPack at CD quality |
| Lossless hi-res | 24-bit, or a sample rate above 48 kHz |

HolyArrs comes with three sets:

- **Lossless first.** New artists get this one. It takes the best lossy copy until a lossless one turns up. Good enough is lossless.
- **Lossy 320 or V0.** Small files. It takes lossy middle and lossy best. Good enough is lossy best.
- **Any music.** It takes anything, best first. Good enough is lossless.

A CD rip, a web purchase and a rip log or cue sheet earn points. A vinyl rip loses some points, but it still passes.

These are always turned away: a whole album in one file with a cue sheet, a Release that is a lossy transcode, samples, programs and password protected archives.

Movie Pick rules cannot judge music, so an artist always uses music Pick rules.

## Settings

Open **Settings**, then **Search**, and find the music part.

- **Search for missing albums too.** On by default. The nightly search covers albums.
- **Take whole discographies.** Off by default, so a Release with every album of an artist is turned away. Turn it on and such a Release may fill the album you searched for.
- **Tracks an album Release may lack.** Some Sources say how many files a Release holds. A Release with fewer files than the album has tracks is turned away. 0, the default, allows none missing. -1 turns the check off.

## Troubleshooting

**An album is never searched.** Check that **Keep up to date** is on for the artist, the album, the track and the Version. Check that the album is already out.

**A Release is turned away and you do not know why.** Open the album, press **Choose a release** and read the reasons next to it.

**A Release posted before the album came out is turned away.** That is on purpose. Early or far too small Releases are turned away like for movies, see [Early and fake Releases](../movies/search-movies.md#fake-and-early-releases).

**A discography Release is turned away.** Turn on **Take whole discographies**.

After a Release is chosen, see [Downloads for music](downloads-music.md) for what happens next.
