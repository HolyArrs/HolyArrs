# Downloads for shows

This page covers what happens after HolyArrs sends a Release for one or more episodes to your Downloader: how the files are put in the Library, what a season pack does, what happens when a download fails, and where to look when something gets stuck. Movies work the same way and are covered in [Downloads for movies](downloads-movies.md). Only the owner and admins (people who may manage the Library) see any of this.

## One download, many episodes

A Release can hold one episode, a file with several episodes, or a whole season (a season pack). HolyArrs sends it to your Downloader once, as one download for every episode it fills.

When the Downloader is done, HolyArrs reads the name of each file in the download and works out which episode it is:

- `Copper.Valley.S01E02.1080p.WEB.mkv` is season 1, episode 2;
- `Copper.Valley.S01E02E03.1080p.WEB.mkv` holds episodes 2 and 3 in one file;
- `The.Late.Lamp.Show.2026.03.14.mkv` is the episode that aired on that day (for shows ordered by air day);
- `[Group] Starfall Kitchen - 112.mkv` is running number 112 (for shows ordered by running number).

A file whose name has no show name, such as `S01E02.mkv`, takes the show from its folder or from the Release name. A file that names another show is refused.

Each file then goes into the show's folder in the Library, in its season folder, under the name your settings give it:

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
```

Files are hardlinked when the download and the Library are on the same disk, so they take no extra space and the Downloader can keep sharing them. On another disk they are copied. HolyArrs never moves or deletes the Downloader's files, and never writes over a file it did not put there.

A download always fills one Version. When a show is kept in HD and 4K, the HD pack and the 4K pack are two downloads, and each file goes to its own Version's folder (for example `Copper Valley (2025)` and `Copper Valley (2025) [4K]`), so neither copy replaces the other.

## When a pack does not hold everything

A season pack is not always what its name promises. HolyArrs settles each episode on its own:

- **Episodes it holds** go into the Library.
- **Episodes it should have held but does not** look for another copy. The Release is added to Turned down for those episodes only, so it is never picked again for them; it still counts for the others.
- **Files for episodes this download was not meant to fill** (you already have them, or they are not kept up to date) are left alone and listed in the import.
- **Extra files** (samples, text files, files with no episode in the name) are left alone and listed in the import.
- **A file that is worse than its Release name said**, or worse than the copy you already have, is refused for that episode; that episode keeps its copy (or looks again).

The row on the Downloads screen says where every episode ended up, for example "2 of 3 episodes in the library; S01E03 looks for another copy."

## Better copies

When a better copy of an episode arrives, it takes the old one's place. The old file goes to the holding folder in its Library folder (`.holyarrs-replaced`) and is removed after the number of days in **Settings > Naming and files**. If the new file cannot be placed, the old one comes back.

A file that held two episodes, when only one of them gets a better copy, stays where it is for the other episode.

## When something stops halfway

If HolyArrs stops in the middle of an import (a restart, a power cut), it finishes the import the next time it runs. Files already placed are recognised as its own and nothing is placed twice.

Before anything is placed, HolyArrs checks that the disk keeps the free space set in **Settings > Naming and files** (100 MB at least). If it would not, nothing is placed and the episodes wait on **Needs a decision**.

Downloads for shows that finished before this version of HolyArrs could place episodes are picked up at start and every 10 minutes.

## Failed and stalled downloads

The same rules as for movies apply, for one episode or a whole pack:

- A download that makes no progress for the time in **Settings > Downloaders** is **Stalled**; after the second time it is acted on.
- A download that fails, or that you **Mark failed**, has its Release added to Turned down for every episode it was filling, is removed from the Downloader (unless you turned that off), and every episode looks again.
- After a failed season pack, HolyArrs searches the whole season again, so another pack can take its place. A failed single episode is searched on its own.

## The Downloads screen

Each row names the movie or the show. For a show it also says which episodes the download covers: one episode by its name ("S01E02 The Mill"), a run of episodes in short ("S01E01-E03, 3 episodes"). Open **The episodes it covers** to see each episode and where it is: downloading, importing, in the Library, looking for another copy, or needing a decision.

**Mark failed** and **Remove** work on the whole download: for a pack, on every episode it fills.

## Needs a decision

An episode lands here when HolyArrs cannot finish it on its own: it ran out of Releases, or an import hit a conflict (for example a file it did not place is already where the episode should go).

- A single episode has the same choices as a movie: **Search again**, **Choose a release by hand** (on the show page), **Add to Turned down** (for this episode only) and **Stop looking**. Stop looking turns off Keep up to date for that episode only; the rest of the show goes on.
- When several episodes of one season pack are stuck, they share one card. The card lists every episode of the pack and where each one is. **Search again for these episodes**, **Add the pack to Turned down** (for every episode it holds) and **Stop looking for these episodes** settle them all at once. Each episode also keeps its own buttons.

## Naming

**Settings > Naming and files** has a **Show names** card:

- **Show folder pattern**: the folder of a show added from now on, `{title} ({year})` at first.
- **A folder for each season**, with its pattern (`Season {season}`) and the folder for specials (`Specials`).
- **Episode file name pattern**: `{title} - {episode} - {episode_title} {quality}` at first. `{episode}` gives `S01E02`, or `S01E02-E03` for a file with several episodes; `{episode_title}` joins several episode names with `+`.
- One pattern each for shows ordered by air day (`{air_date}`) and by running number (`{absolute}`).

The examples under the patterns change as you type. **Rename files** uses the same patterns for episodes you already have, and its preview is exactly what it does.
