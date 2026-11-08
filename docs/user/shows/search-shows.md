---
title: Search for shows
description: How HolyArrs finds episodes and season packs, and how to search or pick a release yourself.
level: Setup
---

Episodes arrive by themselves. Here you search on demand, pick a release by hand, and tune season packs. Only the owner and admins see these screens. Movies work the same way, see [Search for movies](../movies/search-movies.md).

Each episode has its own place in each Version, so one episode can be in the Library while the next is still missing.

## Search from the show page

1. Open the show.
2. Press one **Search now**:

   | Where | Searches |
   | --- | --- |
   | Top of the show | Every season that still needs episodes. |
   | A season | A season pack if the whole season is missing, otherwise each missing episode. |
   | An episode | That episode. Says **Find a better copy** when your copy is below Good enough. |

You should see the episodes on **Downloads** within a minute, and on the **Searches** block on Home, for example "Searching for Copper Valley S01E02".

## Choose a release yourself

1. Press **Choose a release** on a season or an episode. HolyArrs searches every Source and picks nothing.
2. Read the list, best first. Each release shows its label (**Would be chosen**, **Passed over**, **Turned away by the rules**, **Turned down**), the episodes it holds, its Source, size, seeders, points, rank and reasons.
3. Press **Choose** and confirm. For a season pack, the box says which episodes it fills.

You should see the episodes on **Downloads**.

The box warns you when the release fails the rules (the button says **Choose anyway**), when the episode has not aired, or when you already have a good enough copy.

**Turn down** on an episode's list turns the release down for that episode only. On a season's list it covers every episode the release holds.

## What HolyArrs does by itself

An episode is looked for only when the show, its season, the episode and the Version all have **Keep up to date** on.

| When | What happens |
| --- | --- |
| Feed check | Reads the TV categories of your Sources. A new release that fits a missing episode, or one wanting a better copy, is judged by the show's Pick rules. |
| Each night | Several missing episodes of one season get one season search. A single missing episode gets its own search. Unaired episodes are never searched. |
| Always | A copy below **Good enough** keeps looking. |

## What a release can hold

| Kind | Example |
| --- | --- |
| One episode | `Copper.Valley.S01E02.1080p.WEB-DL` |
| Several episodes | `S01E02E03` or `S01E02-E04` |
| A whole season (season pack) | `Copper.Valley.S01.1080p.BluRay` |
| Several seasons | `S01-S03` |
| An air date, for talk shows and news | `The.Late.Lamp.Show.2025.03.14` |
| A running number, for anime | `[Group] Starfall Kitchen - 112 [1080p]` |
| A special | `S00E03`, `S04E00` or `OVA 02` |

HolyArrs matches the release to the show by its name or another name the show is known by, then matches episodes by season and episode, air date or running number. Show releases never match movies, and movie releases never match shows. A season pack goes to your Downloader once, as one download.

## Choices

### Season packs

- A pack is taken only once every episode of the season has aired, or airs within a day. Until then it is turned away with a reason such as "not every episode of season 4 has aired".
- By default a pack is taken only when every episode in it would be new or better. If not, the missing episodes are searched one by one.
- When a pack and single episodes are equally good, the pack wins.
- Releases with more than one season are turned away unless you allow them.

### Several Versions

A show kept in HD and 4K is searched once per Version, each with its own Pick rules. An HD pack fills only HD episodes; the 4K Version needs its own pack.

**Search now** on an episode looks for every Version. **Choose a release** on an episode lists releases for the main Version. A season's list names the Version next to each release.

### Held and Turned down

**Library > Held** and **Library > Turned down** name the show and episode of each release, like "Copper Valley S01E02". **Take off the list** allows a turned-down release again. **Stop holding** sends the episode back to looking.

## If it does not work

**An episode is never searched.**
Cause: **Keep up to date** is off somewhere, or the episode has not aired.
Fix: turn it on for the show, the season, the episode and the Version.

**A season pack is turned away.**
Cause: not every episode has aired, or some episodes already have a copy as good.
Fix: read the reasons in **Choose a release** on the season. To let packs fill only the gaps, change **A season pack is taken when**.

**A release is turned away as early or fake.**
Cause: posted before the last episode it holds aired, or far too small for its length.
Fix: see [Fake and early releases](../movies/search-movies.md#fake-and-early-releases).

## Settings

<details class="ha-more"><summary>Settings reference: Settings > Search</summary>

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| Search for missing episodes too | The nightly search and **Search all missing** cover aired episodes, a season at a time when several are missing. | On | Your Sources limit searches. |
| Episode length when not known (minutes) | Used for season pack size checks. | 20 | Your shows run much longer and packs pass as too small. |
| A season pack is taken when | **Every episode in it would be new or better**, or **At least one episode in it would be new or better**. With the second, the pack fills only the episodes it improves. | Every episode | Packs keep getting turned away. |
| Season pack or single episodes | **Best copy first** (a pack wins a tie), or a season pack whenever one passes the Pick rules. | Best copy first | You prefer whole seasons. |
| Take Releases with several seasons | Lets a release like seasons 1 to 3 fill every missing episode it holds. | Off | You add old, finished shows. |

Feed check interval and nightly search count are shared with movies, see [Search for movies settings](../movies/search-movies.md#settings).

</details>

## Related

- [Show downloads](downloads-shows.md)
- [Organise shows](organise-shows.md)
- [Search for movies](../movies/search-movies.md)
