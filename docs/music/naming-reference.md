---
title: Music naming reference
description: Every music folder and file pattern, its tokens, multi-disc rules, name limits and tag reading.
level: Expert
sidebar: { order: 4 }
---

Every pattern and token for music folders and track names, in the order **Settings > Naming and files > Music names** shows them.

For the everyday steps, see [Organise music](organise.md).

## Patterns

| Setting | Default | Must hold | Notes |
|---|---|---|---|
| **Artist folder pattern** | `{artist}` | | Used when an artist is added. Existing artist folders keep their names. |
| **Album folder pattern** | `{album} ({year})` | `{album}` | |
| **Track file name pattern** | `{track} - {track_title}` | `{track}` | |
| **Track file name on an album with more than one disc** | `{disc}-{track} - {track_title}` | `{disc}`, `{track}` | |
| **A folder for each disc** | Off | | When on, each disc gets its own folder and the plain track pattern is used inside it. |
| Disc folder pattern | `Disc {disc}` | | Used only with **A folder for each disc**. |

## Tokens

| Token | Gives |
|---|---|
| `{artist}` | Artist name |
| `{artist_sort}` | Artist name with a leading The moved to the end: "Beatles, The" |
| `{album}` | Album name |
| `{year}` | Release year |
| `{edition}` | The pressing's note, such as deluxe |
| `{album_type}` | Album or EP |
| `{disc}` | Disc number |
| `{track}` | Track number, 2 digits |
| `{track_title}` | Track name |
| `{format}` | File format, such as FLAC or MP3 |
| `{track_artist}` | **Building.** Meant for a track's own artist on a various-artists album. Today it gives the album's artist. |

A pattern that cannot work, such as a track pattern without `{track}` or a movie token like `{quality}`, is refused with the reason under it. The examples under the patterns show a normal track, a track of a two-disc album and a track of a various-artists album.

## Name limits

- The colon and character rules for movies apply to music too.
- A long album or track name is shortened so each folder or file name stays under 240 bytes. The year, the track number and the extension always stay whole.
- A rename that only changes upper and lower case works on every disk.
- Renames stay inside the artist's folder.

## Reading a track's own tags

HolyArrs matches a file to a track by name first: a track number (`03`, `1-03`, or `103` on a two-disc album), a disc folder (`CD2`, `Disc 2`) and the track name.

It reads the file's tags (the title, track and disc numbers of FLAC and MP3 files) only when:

- the name has no number and no track name,
- two tracks share a name, or
- the number and the name point to different tracks.

The tag title must match the track's name, so a file with wrong tags is left alone. Tags are only read, never changed.

To match by names only, turn off **Read a track's own tags when its name is not enough**.

## Related

- [Organise music](organise.md)
- [Downloads for music](downloads.md)
