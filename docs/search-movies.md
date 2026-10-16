# Searching for movies

This page covers how HolyArrs finds Releases for your movies: by itself, when you press a button, and when you choose a release yourself. It also covers the Held and Turned down lists and the search settings. Only the owner and admins (people who may manage the Library) see any of this; members never do.

## How HolyArrs looks by itself

- **Feed checks.** Every 15 minutes HolyArrs reads the newest Releases from each Source that has its feed turned on. A Release that fits a movie that is missing, wants a better copy or is holding one is judged by that movie's Pick rules; the best one is chosen or held. Releases for movies you do not have are dropped.
- **The nightly search.** Once a day HolyArrs searches for up to 20 missing movies, the ones searched longest ago first. It skips movies that are not out yet, movies already busy getting a copy, and movies with **Keep up to date** off.
- **Find a better copy.** A movie whose copy is below **Good enough** keeps looking. When a better Release turns up, it replaces the old copy.

## The Movie page

**Search now** searches every Source and lets HolyArrs pick the best Release by itself.

**Choose a release** searches every Source but picks nothing. The Releases card then lists every Release found, best first:

- **Would be chosen**: the one HolyArrs would pick.
- **Passed over**: it passed the Pick rules but lost to a better one.
- **Turned away by the rules**: it failed the Pick rules, or matched no movie.
- **Turned down**: you, or HolyArrs, said never to pick it for this movie.
- **Chosen** and **Held**: what happened to it.

Under each Release is a short line (Source, size, seeders, points and rank) and the reasons that decided it, in plain words. **All reasons** opens the full list. The list shows 25 Releases at a time; use **Back** and **More** for the rest.

A movie with more than one Version (for example HD and 4K) has a **Version** box. Pick a Version and **Search now** and **Choose a release** look for that Version only.

**Choose** asks you to confirm first. The box warns you when:

- the Release failed the Pick rules or was turned down. The button then says **Choose anyway**, and the reasons it failed are kept on the Event;
- the movie does not count as out yet, so the Release gate is skipped;
- the Version already has a good enough copy, which the new one replaces.

A search from the Movie page skips the Release gate and the wait in the Pick rules. Each Release says so in its reasons.

**Turn down** puts a Release on the Turned down list. You can add why; the reason shows on the list. HolyArrs never picks that Release again for that movie.

## Held and Turned down

Open **Library** and press **Held** or **Turned down**.

- **Held** lists Releases that were picked but are waiting out the wait set in their Pick rules, with when each one is sent. A better Release found meanwhile takes its place. **Stop holding** sends the movie back to looking.
- **Turned down** lists Releases HolyArrs will never pick again for their movie, with the reason. **Take off the list** lets HolyArrs pick it again.

Both lists show 25 at a time and have **Clear all**, which asks first.

## Searches on Home

The **Searches** block on Home shows each search while it runs, with how far it is and which Source it is asking, and then what it found: how many Releases, how many were turned away, and which one was chosen or held. It updates by itself.

## Settings > Search

| Setting | What it does | Default |
| --- | --- | --- |
| Check Source feeds every (minutes) | How often feeds are read. 10 to 120, or 0 to turn feed checks off. | 15 |
| Movies to search for each night | How many missing movies (and episodes, see [Searching for shows](search-shows.md)) the nightly search looks for. 0 to 500; 0 turns it off. | 20 |
| Also Find a better copy each night | The nightly search also looks for better copies of movies below Good enough. | Off |
| Try the next Downloader | When a Downloader cannot take a Release, hand it to the next one of the same kind. | On |

The same page has **Check feeds now**, **Search every missing one** (movies, and episodes unless that is turned off) and **Find a better copy for all**, which start at once instead of waiting for the timer.

## Early and fake Releases

Some Releases are fakes: posted before the movie came out, far too small to hold it, or carrying a program instead of a video. HolyArrs turns these away with a plain reason under the Release, and takes the next good one. The same checks run for episodes and albums.

- **Posted before it came out.** A Release posted before the movie's first date (cinema, digital or disc), before the last episode it holds aired, or before the album's release day is turned away ("posted 2024-01-15, before it came out; likely a fake"). A day of grace covers time zones. When you search yourself, it is shown with a warning instead, and you decide.
- **Far too small.** Less than 1 MB a minute of video, 20 MB for a video whose length is not known, or 512 KB a track, whatever the Pick rules allow. A season pack whose episode lengths are not known counts 20 minutes an episode.
- **Size or date not known.** Some sites do not say how big a Release is or when it was posted, and some movies have no dates yet. The checks above cannot run then: an automatic search shows "size unknown" or "date unknown" under the Release and may still take it. Turn on **Hold Releases of unknown size or date for you** to keep such Releases for your own choice.
- **Programs and locked archives.** A program in the name (`movie.mkv.exe`, `.lnk`), a password-locked archive, a sample or a whole disc image are always turned away.
- **In the Downloader.** As soon as a torrent lists its files, a program, a password note, or archives with no video or audio fail the download (Settings, Downloaders, **Fail downloads that hold programs or fakes**). A web shortcut (`.url`) fails it only when there is no video or audio beside it; many ordinary torrents carry one, so beside the media it is only noted in the Events.
- **At import.** An empty file, a file far too small for its length (the same least sizes as above), a download much smaller than its Release said (a 4 GB Release holding a 3 MB video), or an old `.wmv` or `.asf` file under an HD name, is refused, the Release is turned down and the next one is taken. A file HolyArrs cannot read is not called a fake: the import waits for you in Needs a decision.

The switches are on Settings > Search, **Early and fake Releases**.

| Setting | What it does | Default |
| --- | --- | --- |
| Turn away Releases posted before it came out | The early check above. | On |
| Hours a Release may be early | Grace for time zones and early air times, 0 to 720. | 24 |
| Turn away fakes by size and file kind | The size check, and the empty and old-container checks at import. | On |
| Check file sizes at import | The least sizes and the Release-size check at import. Off, only empty files and the sizes below are refused. | On |
| Smallest video file to place (MB) | 0 uses 1 MB a minute of the video's length (20 MB when not known); another value replaces it. | 0 |
| Smallest track file to place (KB) | 0 uses 512 KB; another value replaces it. | 0 |
| Episode length when not known (minutes) | Used for the size check of a season pack, 1 to 240. | 20 |
| Hold Releases of unknown size or date for you | Automatic searches keep such Releases for your choice instead of taking them with a warning. | Off |

## For scripts: the API

Every call needs the **manage the Library** permission, except the search settings, which need **settings**. The full list, with every field, is in `/api/v1/openapi.json`.

| Call | What it does |
| --- | --- |
| `POST /api/v1/titles/{id}/search` | Search for a movie. `{"choose": false}` only judges and stores; `{"slot_id": n}` searches one Version. Answers with the search ids. |
| `GET /api/v1/titles/{id}/releases` | The Releases of the newest search, with verdicts and reasons. `slot_id`, `limit`, `offset`. |
| `POST /api/v1/releases/{id}/choose` | Choose a Release. `{"force": true}` for one that failed; without it the answer is 409 with the reasons. |
| `POST /api/v1/releases/{id}/turn-down` | Turn a Release down, with an optional `{"reason": "..."}`. |
| `GET /api/v1/held`, `GET /api/v1/turned-down` | The two lists, paged. |
| `DELETE /api/v1/held/{id}`, `DELETE /api/v1/turned-down/{id}` | Take one off a list. Without an id, clear the list. |
| `GET /api/v1/searches` | Searches running now and the newest finished ones with what they found. |
| `POST /api/v1/search/missing`, `/better`, `/feed-check` | Start a search of every missing movie, every movie below Good enough, or every feed. |
| `GET` and `PUT /api/v1/settings/search` | The four search settings. |
