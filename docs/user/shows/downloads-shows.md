# Downloads for shows

You want episodes to land in the right folder with the right name, and you want to know what to do when a download goes wrong. This page covers what happens after HolyArrs sends a Release to your Downloader. Movies work the same way, see [Downloads for movies](../movies/downloads-movies.md). Only the owner and admins see these screens.

## What happens when a download finishes

1. HolyArrs reads the name of each file and works out its episode:
   - `Copper.Valley.S01E02.1080p.WEB.mkv` is season 1, episode 2.
   - `Copper.Valley.S01E02E03.1080p.WEB.mkv` holds episodes 2 and 3.
   - `The.Late.Lamp.Show.2025.03.14.mkv` is the episode that aired that day.
   - `[Group] Starfall Kitchen - 112.mkv` is running number 112.
2. A file with no show name, like `S01E02.mkv`, takes the show from its folder or the Release name. A file that names another show is refused.
3. HolyArrs checks that the disk keeps the free space set in **Settings > Naming and files**. If it would not, nothing is placed and the episodes wait on **Needs a decision**.
4. Each file goes into the show's season folder under the name your patterns give it.

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
```

Files are hardlinked when the download and the Library are on the same disk, so they take no extra space and your Downloader can keep seeding. On another disk they are copied. HolyArrs never moves or deletes your Downloader's files, and never writes over a file it did not put there.

A download always fills one Version. For a show kept in HD and 4K, each pack goes to its own Version's folder, for example `Copper Valley (2025)` and `Copper Valley (2025) [4K]`.

## When a season pack does not hold everything

HolyArrs settles each episode on its own:

- Episodes the pack holds go into the Library.
- Episodes it should have held but does not go back to looking. The Release is turned down for those episodes only.
- Files for episodes this download was not meant to fill are left alone and listed.
- Extra files, like samples or text files, are left alone and listed.
- A file worse than its Release name said, or worse than the copy you have, is refused for that episode.

The row on the Downloads screen sums it up, for example "2 of 3 episodes in the library; S01E03 looks for another copy."

## Better copies

When a better copy arrives, it takes the old one's place. The old file goes to the holding folder `.holyarrs-replaced` inside its Library folder. It is removed after the holding days set in **Settings > Naming and files**. If the new file cannot be placed, the old one comes back.

A file that holds two episodes stays until both have a better copy.

## Use the Downloads screen

Each row names the show and the episodes it covers, like "S01E02 The Mill" or "S01E01-E03, 3 episodes". Open **The episodes it covers** to see where each episode is: downloading, importing, in the Library, looking for another copy, or needing a decision.

**Mark failed** and **Remove** act on the whole download. For a pack, that is every episode in it.

## Handle Needs a decision

An episode lands here when HolyArrs cannot finish on its own. It ran out of Releases, or a file it did not place is already where the episode should go.

For one episode you can:

- **Search again**
- **Choose a release by hand** on the show page
- **Add to Turned down**, for this episode only
- **Stop looking**, which turns off Keep up to date for this episode only

When several episodes of one pack are stuck, they share one card. **Search again for these episodes**, **Add the pack to Turned down** and **Stop looking for these episodes** settle them all at once. Each episode keeps its own buttons too.

## Settings

**Settings > Naming and files** has a **Show names** card:

| Setting | Default |
| --- | --- |
| Show folder pattern | `{title} ({year})` |
| Season folder | `Season {season}` |
| Specials folder | `Specials` |
| Episode file name | `{title} - {episode} - {episode_title} {quality}` |
| Shows ordered by air day | `{title} - {air_date} - {episode_title} {quality}` |
| Shows ordered by running number | `{title} - {episode} - {absolute} - {episode_title} {quality}` |

`{episode}` gives `S01E02`, or `S01E02-E03` for a file with several episodes. `{episode_title}` joins several names with `+`. The examples under each pattern change as you type. The show folder pattern applies to shows you add from now on.

The same page sets the free space an import must leave (100 MB at least) and the holding days. Holding days set to 0 keep replaced files for good.

## Troubleshooting

**A download makes no progress.** After the time set in **Settings > Downloaders** it is marked **Stalled**. The second time, HolyArrs acts on it like a failed download.

**A download failed.** The Release is turned down for every episode it was filling and removed from your Downloader, unless you turned that off. A failed season pack makes HolyArrs search the whole season again. A failed single episode is searched on its own.

**HolyArrs stopped in the middle of an import.** After a restart or power cut, it finishes the import the next time it runs. Files already placed are recognised and nothing is placed twice.

**Episodes wait on Needs a decision because of disk space.** Free some space on the Library disk, then press **Search again**.
