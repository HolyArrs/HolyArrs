# Find releases for your movies

You want a movie, and you want HolyArrs to get a good copy of it. Most of the time it does this by itself. This page shows how it looks, how to search yourself, and how to pick a release by hand. Only the owner and admins see these screens.

## What HolyArrs does by itself

- **Feed checks.** Every 15 minutes HolyArrs reads the newest releases from each Source that has its feed on. A release that fits a movie you want is judged by that movie's Pick rules. The best one is chosen or held. Releases for movies you do not have are dropped.
- **The nightly search.** Once a day HolyArrs searches for up to 20 missing titles, the ones searched longest ago first. It skips movies that are not out yet, movies already getting a copy, and movies with **Keep up to date** off.
- **Better copies.** A movie whose copy is below **Good enough** keeps looking. When a better release turns up, it replaces the old copy.

## Search for one movie

1. Open the movie.
2. Press **Search now**. HolyArrs searches every Source and picks the best release.

If the movie has more than one Version, for example HD and 4K, pick the Version first. The search then looks for that Version only.

## Choose a release yourself

1. Open the movie.
2. Press **Choose a release**. HolyArrs searches every Source but picks nothing.
3. Read the list. The best release is at the top. Each one has a label:
   - **Would be chosen**: the one HolyArrs would pick.
   - **Passed over**: it passed the Pick rules but lost to a better one.
   - **Turned away by the rules**: it failed the Pick rules, or matched no movie.
   - **Turned down**: someone said never to pick it for this movie.
4. Press **Choose** on the one you want and confirm.

Under each release you see the Source, size, seeders, points and the reasons that decided it.

The confirm box warns you when the release failed the Pick rules or was turned down. The button then says **Choose anyway**. It also warns you when the movie is not out yet, or when the Version already has a good enough copy that the new one would replace.

A search you start from the movie page skips the wait set in the Pick rules. Each release says so in its reasons.

To make sure HolyArrs never picks a release for this movie, use **Turn down** on it. You can add a reason.

## Search for everything missing

Open the **Library**, filter to **Missing**, and press **Search all missing**. HolyArrs asks first and tells you how many titles it will search.

## Held and Turned down

Open the **Library** and use **Held** or **Turned down**.

- **Held** lists releases that were picked but are waiting out the wait in their Pick rules. A better release found in the meantime takes the place. **Stop holding** sends the movie back to looking.
- **Turned down** lists releases HolyArrs will never pick again for that movie, with the reason. Take one off the list to let HolyArrs pick it again.

Both lists have **Clear all**, which asks first.

## Fake and early releases

Some releases are fakes. HolyArrs turns these away with a plain reason and takes the next good one.

- **Posted before the movie came out.** A release posted before the movie's first date is turned away. A day of grace covers time zones. When you search yourself, you see a warning instead, and you decide.
- **Far too small.** Less than 1 MB for each minute of the movie, or 20 MB when the length is not known.
- **Programs and locked archives.** A program in the name (such as `movie.mkv.exe`), a password-locked archive, a sample or a disc image is always turned away.
- **At import.** An empty file, a file far too small for its length, or a download much smaller than its release said is refused. The release is turned down and the next one is taken. A file HolyArrs cannot read is not called a fake. It waits for you on the [Downloads](downloads-movies.md) screen.

Some sites do not say how big a release is or when it was posted. An automatic search then shows "size unknown" or "date unknown" and may still take it. Turn on **Hold Releases of unknown size or date for you** if you want to decide on those yourself.

## Settings

Open **Settings**, then **Search**. The same page has **Check feeds now** and **Find a better copy for all**, which start right away.

| Setting | What it does | Default |
| --- | --- | --- |
| Check Source feeds every (minutes) | How often feeds are read. 10 to 120, or 0 to turn feed checks off. | 15 |
| Titles to search for each night | How many missing titles the nightly search looks for. 0 turns it off. | 20 |
| Also find a better copy each night | The nightly search also looks for better copies. | Off |
| Turn away Releases posted before it came out | The early check. | On |
| Hours a Release may be early | Grace for time zones, 0 to 720. | 24 |
| Turn away fakes by size and file kind | The size check, and the empty file check at import. | On |
| Check file sizes at import | The least sizes and the release size check at import. | On |
| Smallest video file to place (MB) | 0 uses 1 MB a minute of the movie. Another value replaces it. | 0 |
| Hold Releases of unknown size or date for you | Automatic searches keep such releases for your choice. | Off |

## Troubleshooting

- **Nothing is found.** Check that your Sources work on **Settings**, **Sources**. Then check the movie's Pick rules. Use **Choose a release** to see why each release was turned away.
- **The movie never gets searched at night.** Check that **Keep up to date** is on and that the movie is out.
- **A good release was turned away as early.** Raise **Hours a Release may be early**, or choose it yourself.
- **HolyArrs keeps picking a bad release.** Use **Turn down** on it.

When a download fails or needs you, see [Follow your movie downloads](downloads-movies.md).
