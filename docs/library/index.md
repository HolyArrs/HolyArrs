---
title: Library and files
description: Where your finished titles live, what HolyArrs adds to them, and how it fits with your media server.
level: Start
---

Set up the folders, subtitles, audio and media server links that decide how your finished titles are stored and played.

## Pages

| Page | What it covers |
|---|---|
| [Library folders](folders.md) | Add the folders that hold movies, shows and music, and read the folder check |
| [Subtitles](subtitles.md) | Choose languages and providers for subtitles |
| [Subtitle scoring](subtitle-scoring.md) | How HolyArrs rates a subtitle before it keeps it |
| [Audio versions](audio-versions.md) | Keep a second language track and match it to your preference |
| [Protected copies](protected-copies.md) | Keep a file safe from replacement |
| [Codecs and inspection](codecs-inspection.md) | What HolyArrs reads from each file |
| [Media servers](media-servers.md) | Link Jellyfin, Plex or Emby and map paths between containers |
| [Metadata](metadata.md) | Where names, dates and posters come from, and what your install sends |
| [Data packs](data-packs.md) | Copy Pick rules and naming patterns between installs with a JSON pack |
| [TRaSH Guides scores](trash-guides.md) | Use a TRaSH Guides profile's scores as Pick rules points, and sync again later |
| [Trailers](trailers.md) | Turn trailers on or off and fix trailers that do not play |

New install: start with [Library folders](folders.md). The shared-folder layout that keeps imports instant is in [Folders and hardlinks](../start/folders.md).

## Sorting the Library

Open **Sort & filter**, pick a sort under **Sort by**, then **Ascending** or **Descending** under **Order**. A title with no value for the sort stays at the bottom either way.

| Showing | Sorts |
|---|---|
| Movies | Status, Name, Year, Score, Out, Added, Size, Runtime, Quality |
| TV shows | Status, Name, Next episode, Last aired, Progress, Added, Year, Score, Size, Network, Quality |
| Music | Status, Name, Progress, Added, Size, Next release |
| The whole Library | Name, Year, Score, Added, Size |

### Score

**Score** is the members' average out of 10, shown with a ★. Highest comes first.

- A title needs at least 25 votes to have a score. Titles without one stay at the bottom either way, and so does music in the whole Library.
- While you sort by Score, every view shows it: a Score column first in **Rows**, the first fact in **Banners** and **Overview**, the line under each name on a phone, and a small ★ badge on each poster in **Posters**. Titles without a score show nothing.
- The Score column goes away when you pick another sort, unless you turned it on yourself.
- Music has no score, so the Music sort list leaves it out. A show that has only ever taken its details from TVMaze has none either.
- Titles already in your Library get their score over the next few scheduled refreshes, up to 300 movies and 300 shows each run. To fill every title now, open **Settings > Schedules** and select **Run now** on **Refresh movie details** and **Refresh show details**. With **Refresh movie details** turned off, movies fill only with **Run now**.

## Related

- [Downloaders](../downloads/downloaders.md)
- [Health checks](../run/health.md)
