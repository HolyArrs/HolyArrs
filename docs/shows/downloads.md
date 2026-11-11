---
title: Show downloads
description: Follow show downloads, see how season packs are settled and fix stuck ones.
level: Setup
---

See what is downloading, how finished episodes are placed and what to do when a download goes wrong. Only the owner and admins see these screens.

When HolyArrs chooses a release, it sends it to a **Downloader** and watches it until each episode is in your Library. The tabs, **Mark failed** and **Remove** all work as on [Movie downloads](../movies/downloads.md).

## Watch your downloads

1. Open **Downloads** in the menu.
2. Find the row for your show.

You should see the show and its episodes, like "S01E02 The Mill" or "S01E01-E03, 3 episodes", with progress, speed and Downloader.

Open **The episodes it covers** to see each episode's state: downloading, importing, in the Library, looking for another copy, or needing a decision.

**Mark failed** and **Remove** act on the whole download. For a pack, that is every episode in it.

## What happens when a download finishes

1. HolyArrs reads each file's name and works out its episodes.
2. A file with no show name, like `S01E02.mkv`, takes the show from its folder or the release name. A file that names another show is refused.
3. HolyArrs checks the disk keeps the free space set in **Settings > Naming and files**. If not, nothing is placed and the episodes wait for you.
4. Each file goes into its season folder under the name your patterns give it.

You should see this in your Library:

```
Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv
Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv
Copper Valley (2025)/Specials/Copper Valley - S00E01 - Behind the scenes 1080p WEB.mkv
```

Torrent files are hardlinked when the download and the Library share a disk: they take no extra space and your Downloader keeps seeding. On another disk they are copied. A finished Usenet download is moved instead, which is instant on the same disk; if it can't be moved, it is copied. A Downloader marked read-only keeps its files. HolyArrs never writes over a file it did not put there.

:::tip
A download fills one Version. For a show kept in HD and 4K, each pack goes to its own Version's folder, for example `Copper Valley (2025)` and `Copper Valley (2025) [4K]`.
:::

## Settle episodes in Review

An episode lands in **Review** (shown as Needs a decision in History) when HolyArrs ran out of releases, or a file it did not place already sits where the episode should go.

For one episode, pick one:

| Choice | What happens |
| --- | --- |
| Search again | Searches every Source now. |
| Choose a release by hand | Opens the release list on the show page. |
| Add to Turned down | For this episode only. |
| Stop looking | Turns **Keep up to date** off for this episode only. |

Stuck episodes from one pack share a card with **Search again for these episodes**, **Add the pack to Turned down** and **Stop looking for these episodes**. Each episode keeps its own buttons too.

## Choices

### When a season pack does not hold everything

HolyArrs settles each episode on its own:

- Episodes the pack holds go into the Library.
- Episodes it should have held but does not go back to looking. The release is turned down for those episodes only.
- Files for episodes this download was not meant to fill, and extras like samples or text files, are left alone and listed.
- A file worse than its release name said, or worse than your copy, is refused for that episode.

The row sums it up, for example "2 of 3 episodes in the library; S01E03 looks for another copy."

### Better copies

A better copy takes the old one's place. The old file moves to `.holyarrs-replaced` inside its Library folder and is removed after the holding days. If the new file cannot be placed, the old one comes back. A file holding two episodes stays until both have a better copy.

## If it does not work

**A download makes no progress.**
Cause: stalled past the time set in **Settings > Downloaders**.
Fix: none needed. HolyArrs marks it stalled, then tries the next release after **Try the next Release after (hours)**.

**A download failed.**
Cause: the Downloader or the files.
Fix: none needed. The release is turned down for every episode it was filling and removed from your Downloader, unless you turned that off. A failed pack makes HolyArrs search the whole season again. A failed single episode is searched on its own.

**HolyArrs stopped in the middle of an import.**
Fix: none needed. After a restart or power cut it finishes the import on its next run. Placed files are recognised and nothing is placed twice.

**Episodes wait in Review because of disk space.**
Fix: free space on the Library disk, then press **Search again**.

## For experts

<details class="ha-more"><summary>Settings reference: Settings > Naming and files > Show names</summary>

| Setting | Default | Change it when |
| --- | --- | --- |
| Show folder pattern | `{title} ({year})` | Your media server wants another layout. Applies to shows added from now on. |
| A folder for each season | On | You want every episode in the show folder. |
| Season folder pattern | `Season {season}` | You prefer `Season 01` or similar. |
| Episode file name pattern | `{title} - {episode} - {episode_title} {quality}` | Your media server wants another name. |
| Folder for specials (season 0) | `Specials` | Your media server expects `Season 00`. |
| For shows ordered by air day | `{title} - {air_date} - {episode_title} {quality}` | Talk shows and news need another layout. |
| For shows ordered by running number | `{title} - {episode} - {absolute} - {episode_title} {quality}` | Anime needs another layout. |
| A file holding several episodes | First and last: `S01E02-E04` | Your media server reads another style. |
| Air day | Dashes: `2026-03-14` | You prefer dots or spaces. |
| Digits in a running number | 3: `007` | Short numbers need more leading zeros. |

`{episode}` gives `S01E02`, or `S01E02-E03` for a file with several episodes. `{episode_title}` joins several names with `+`. The examples under each pattern change as you type.

The same page sets the free space an import must leave (default and minimum 100 MB) and the holding days (default 7). 0 holding days never removes replaced files.

</details>

<details class="ha-more"><summary>How file names become episodes</summary>

| File | Is |
| --- | --- |
| `Copper.Valley.S01E02.1080p.WEB.mkv` | Season 1, episode 2 |
| `Copper.Valley.S01E02E03.1080p.WEB.mkv` | Episodes 2 and 3 |
| `The.Late.Lamp.Show.2025.03.14.mkv` | The episode that aired that day |
| `[Group] Starfall Kitchen - 112.mkv` | Running number 112 |

</details>

## Related

- [Search for shows](search.md)
- [Organise shows](organise.md)
- [Movie downloads](../movies/downloads.md)
