---
title: Search for movies
description: How HolyArrs finds a release for a movie, and how to search or pick one yourself.
level: Setup
---

HolyArrs finds each movie you want by itself. Here you search on demand, pick a release by hand, and read why one won. Only the owner and admins see these screens.

A **release** is one copy of a movie offered by a Source, such as `Heat.1995.1080p.BluRay`. Each movie's **Pick rules** decide which releases are good enough and which one wins.

## Search for one movie

1. Open the movie.
2. If it has more than one Version, for example HD and 4K, pick the Version.
3. Press **Search now**.

You should see the movie on **Downloads** within a minute. If nothing passes the Pick rules, the movie keeps looking.

## Choose a release yourself

1. Open the movie and press **Choose a release**. HolyArrs searches every Source and picks nothing.
2. Read the list, best first. Each release has a label:

   | Label | Means |
   | --- | --- |
   | Would be chosen | The one HolyArrs would pick. |
   | Passed over | Passed the Pick rules, lost to a better one. |
   | Turned away by the rules | Failed the Pick rules, or matched no movie. |
   | Turned down | Someone said never to pick it for this movie. |

3. Press **Choose** on the one you want, then confirm.

You should see the movie on **Downloads**. Each release lists its Source, size, seeders, points and the reasons that decided it.

:::tip
A search you start from the movie page skips the wait set in the Pick rules. The reasons say so.
:::

## What HolyArrs does by itself

| When | What happens |
| --- | --- |
| Every 15 min | Reads the newest releases from each Source with its feed on. A release that fits a movie you want is judged by its Pick rules, then chosen or held. Releases for other movies are dropped. |
| Each night | Searches up to 20 missing titles, longest unsearched first. Skips movies not out yet, movies already downloading, and movies with **Keep up to date** off. |
| Always | A copy below **Good enough** keeps looking. A better release replaces it. |

To search every missing title now, open **Library**, filter to **Missing** and press **Search all missing**. HolyArrs says how many titles it will search and asks first.

## Choices

### Confirm warnings

The confirm box warns you when the release failed the Pick rules or was turned down. The button then says **Choose anyway**. It also warns when the movie is not out yet, or when the Version already has a good enough copy that this one would replace.

### Turn down a release

Press **Turn down** on a release so HolyArrs never picks it for this movie. You can add a reason.

### Held and Turned down lists

Open **Library**, then **Held** or **Turned down**.

- **Held** lists picked releases waiting out the wait in their Pick rules. A better release found meanwhile takes the place. **Stop holding** sends the movie back to looking.
- **Turned down** lists releases never picked again for that movie, with the reason. Take one off the list to allow it again.

Both lists have **Clear all**, which asks first.

## Fake and early releases

HolyArrs turns these away with a plain reason and takes the next good release.

| Check | Turned away when |
| --- | --- |
| Early | Posted before the movie's first date, beyond 24 h of grace for time zones. When you search yourself, you get a warning and decide. |
| Too small | Under 1 MB per minute of the movie, or under 20 MB when the length is unknown. |
| File kind | A program in the name (`movie.mkv.exe`), a password-locked archive, a sample or a disc image. |
| At import | An empty file, a file far too small for its length, or a download much smaller than its release said. The release is turned down and the next one taken. |

A file HolyArrs cannot read is not called a fake. It waits for you on [Downloads](downloads.md).

Some Sources give no size or post date. Automatic searches then show "size unknown" or "date unknown" and may still take the release. To decide on those yourself, turn on **Hold Releases of unknown size or date for you**.

## If it does not work

**Nothing is found.**
Cause: a Source is down, or the Pick rules turn everything away.
Fix: test your Sources on **Settings > Sources**, then use **Choose a release** to read why each release failed.

**The movie is never searched at night.**
Cause: **Keep up to date** is off, or the movie is not out.
Fix: turn **Keep up to date** on for its Version.

**A good release was turned away as early.**
Fix: raise **Hours a Release may be early**, or choose it yourself.

**HolyArrs keeps picking a bad release.**
Fix: press **Turn down** on it.

For downloads that fail or stall, see [Movie downloads](downloads.md).

## Settings

<details class="ha-more"><summary>Settings reference: Settings > Search</summary>

The page also has **Check feeds now** and **Find a better copy for all**, which start at once.

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| Titles to search each night | Longest unsearched first, 0 to 500. 0 turns it off. | 20 | Your Sources limit searches, or you have a large backlog. |
| Also find better copies each night | The nightly search also covers titles below Good enough. | Off | You want upgrades without waiting for feeds. |
| Check Source feeds every (minutes) | 10 to 120. 0 turns them off. | 15 | A Source limits requests. |
| Turn down Releases posted before release | The early check. | On | Rarely. |
| Hours a Release may be early | Grace for time zones, 0 to 720. | 24 | Good releases get turned away as early. |
| Turn down fakes by size and file type | Size and file kind checks, and the empty file check at import. | On | Rarely. |
| Check file sizes at import | Smallest sizes and the release size check at import. | On | Rarely. |
| Smallest video file to place (MB) | 0 uses 1 MB per minute of the movie. Any other value replaces that. | 0 | Short films get refused. |
| Hold Releases of unknown size or date for you | Automatic searches keep such releases for your choice. | Off | Your Sources often omit size or date. |

</details>

## Related

- [Movie downloads](downloads.md)
- [Organise movies](organise.md)
- [Search for shows](../shows/search.md)
