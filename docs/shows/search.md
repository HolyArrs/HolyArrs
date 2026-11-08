---
title: Search for shows
description: How HolyArrs finds episodes and season packs, and how to search or pick a release yourself.
level: Setup
---

HolyArrs finds each aired episode by itself. Here you search on demand, pick a release by hand and tune season packs. Only the owner and admins see these screens.

A **release** is one copy offered by a Source, such as `Copper.Valley.S01E02.1080p.WEB-DL`. Each episode has its own place in each Version, so one episode can be in the Library while the next is still missing.

## Search from the show page

1. Open the show.
2. Press **Search now** at the place that fits:

   | Where | Searches |
   | --- | --- |
   | Top of the show | Every season that still needs episodes. |
   | A season | A season pack if the whole season is missing, otherwise each missing episode. |
   | An episode | That episode. Says **Find a better copy** when your copy is below Good enough. |

You should see the episodes on **Downloads** within a minute, and in the **Searches** block on Home, for example "Searching for Copper Valley S01E02".

## Choose a release yourself

1. Press **Choose a release** on a season or an episode. HolyArrs searches every Source and picks nothing.
2. Read the list, best first. Each release has a label:

   | Label | Means |
   | --- | --- |
   | Would be chosen | The one HolyArrs would pick. |
   | Passed over | Passed the Pick rules, lost to a better one. |
   | Turned away by the rules | Failed the Pick rules, or matched no episode. |
   | Turned down | Someone said never to pick it. |

3. Press **Choose** on the one you want, then confirm. For a season pack, the box lists the episodes it fills.

You should see the episodes on **Downloads**. Each release lists the episodes it holds, its Source, size, seeders, points, rank and the reasons that decided it.

The confirm box warns you when the release failed the rules (the button then says **Choose anyway**), when the episode has not aired, or when you already have a good enough copy.

:::tip
**Turn down** on an episode's list turns the release down for that episode only. On a season's list it covers every episode the release holds.
:::

## What HolyArrs does by itself

An episode is looked for only when **Keep up to date** is on for the show, its season, the episode and the Version.

| When | What happens |
| --- | --- |
| Feed check | Reads the TV categories of your Sources. A new release that fits a missing episode, or is a better copy, is judged by the show's Pick rules. |
| Each night | Several missing episodes of one season get one season search. A single missing episode gets its own search. Unaired episodes are never searched. |
| Always | A copy below **Good enough** keeps looking. |

## Choices

### Season packs

- A pack is taken only once every episode of the season has aired, or airs within a day. Until then it is turned away with a reason such as "not every episode of season 4 has aired".
- By default a pack is taken only when every episode in it would be new or better. Otherwise the missing episodes are searched one by one.
- When a pack and single episodes are equally good, the pack wins.
- A release with more than one season is turned away unless you allow it.

Change these on **Settings > Search**, see the settings reference below.

### Several Versions

A show kept in HD and 4K is searched once per Version, each with its own Pick rules. An HD pack fills only HD episodes. The 4K Version needs its own pack.

**Search now** on an episode looks for every Version. **Choose a release** on an episode lists releases for the main Version. A season's list names the Version next to each release.

### Held and Turned down

Open **Library**, then **Held** or **Turned down**. Each row names the show and episode, like "Copper Valley S01E02".

- **Stop holding** sends the episode back to looking.
- **Take off the list** allows a turned-down release again.

## If it does not work

**An episode is never searched.**
Cause: **Keep up to date** is off somewhere, or the episode has not aired.
Fix: turn it on for the show, the season, the episode and the Version.

**A season pack is turned away.**
Cause: not every episode has aired, or some episodes already have a copy as good.
Fix: read the reasons in **Choose a release** on the season. To let packs fill only the gaps, change **A season pack is taken when**.

**A release is turned away as early or fake.**
Cause: posted before the last episode it holds aired, or far too small for its length.
Fix: see [Fake and early releases](../movies/search.md#fake-and-early-releases).

For downloads that fail or stall, see [Show downloads](downloads.md).

## For experts

<details class="ha-more"><summary>Settings reference: Settings > Search</summary>

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| Search for missing episodes too | The nightly search and **Search all missing** cover aired episodes, a season at a time when several are missing. | On | Your Sources limit searches. |
| Episode length when not known (minutes) | Used for season pack size checks, 1 to 240. | 20 | Your shows run much longer and packs pass as too small. |
| A season pack is taken when | **Every episode in it would be new or better**, or **At least one episode in it would be new or better**. With the second, the pack fills only the episodes it improves. | Every episode | Packs keep getting turned away. |
| Season pack or single episodes | **Best copy first** (a pack wins a tie), or a season pack whenever one passes the Pick rules. | Best copy first | You prefer whole seasons. |
| Take Releases with several seasons | Lets a release like seasons 1 to 3 fill every missing episode it holds. | Off | You add old, finished shows. |

Feed check interval and the nightly search count are shared with movies, see [Search for movies settings](../movies/search.md#settings).

</details>

<details class="ha-more"><summary>What a release can hold</summary>

| Kind | Example |
| --- | --- |
| One episode | `Copper.Valley.S01E02.1080p.WEB-DL` |
| Several episodes | `S01E02E03` or `S01E02-E04` |
| A whole season (season pack) | `Copper.Valley.S01.1080p.BluRay` |
| Several seasons | `S01-S03` |
| An air date, for talk shows and news | `The.Late.Lamp.Show.2025.03.14` |
| A running number, for anime | `[Group] Starfall Kitchen - 112 [1080p]` |
| A special | `S00E03`, `S04E00` or `OVA 02` |

HolyArrs matches the release to the show by its name or another name the show is known by. It then matches episodes by season and episode, air date or running number. Show releases never match movies, and movie releases never match shows. A season pack goes to your Downloader once, as one download.

</details>

## Related

- [Show downloads](downloads.md)
- [Organise shows](organise.md)
- [Search for movies](../movies/search.md)
