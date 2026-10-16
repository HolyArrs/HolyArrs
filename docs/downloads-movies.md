# Downloads and Needs a decision (movies)

When HolyArrs chooses a Release for a movie, it sends it to a **Downloader** (qBittorrent or SABnzbd) and watches it until the file is in your Library. The **Downloads** screen shows that, live. When HolyArrs cannot finish a movie on its own, the movie waits on **Needs a decision** for you. Only the owner and admins see these screens.

## The Downloads screen

Open **Downloads** in the menu. The screen has two parts at the top: **Downloads** and **Needs a decision**. A red number next to Downloads in the menu says how many movies need a decision.

**Now** lists every Download that is still moving. Each row shows:

- the movie, its Version and the Release name,
- a progress bar and the percent done,
- the speed and the time left, while it downloads,
- the Downloader it is on,
- what is happening, in plain words.

The rows follow the Downloader live: you never need to reload the page.

| Word on the row | Means |
|---|---|
| Waiting its turn | The Downloader has it and starts it when there is room. |
| Downloading | It is being fetched. |
| Stalled | No progress for a while (Settings, Downloaders, "Stalled after"). After the give-up time HolyArrs marks it failed by itself. |
| Paused in the Downloader | Someone paused it in the Downloader. HolyArrs waits. |
| Waiting to import | The Downloader is done; HolyArrs puts it in the Library next. |
| Importing | HolyArrs is putting the files in the Library. |
| Done | The movie is in the Library. |
| Failed | The Downloader gave up, the files were dangerous, or someone marked it failed. The reason is under it. |
| Vanished | It was removed in the Downloader, outside HolyArrs. |
| Removed | Someone removed it on this screen. |
| Needs a decision | The import could not finish on its own; see below. |

**Finished and stopped** lists the rest (done, failed, vanished, removed). **Imports** lists what HolyArrs placed in the Library.

### Mark failed

Press **Mark failed** on a row that is still moving. HolyArrs asks first. Then:

1. the Download fails with your name as the reason,
2. its Release goes on the **Turned down** list, so HolyArrs never picks it again for that movie,
3. it is removed from the Downloader with its files (setting "Remove failed downloads"),
4. HolyArrs searches for the next Release. When none passes the Pick rules, the movie goes to **Needs a decision**.

### Remove

Press **Remove** on a row that is still moving. A box asks what should happen:

- **Delete its files**: on, the Downloader deletes what it fetched; off, the files stay in the Downloader.
- **Add its Release to Turned down**: HolyArrs never picks that Release again for the movie.
- **Search again**: only with Turned down. HolyArrs looks for the next Release straight away. Without it, the movie goes back to waiting and the next feed check may find something.

Without Turned down, the movie goes back to waiting and the same Release may be picked again later.

The box starts from your settings: **Settings**, **Downloaders**, under "When you remove a download". Out of the box: delete the files on, Turned down off, search again on.

## Needs a decision

A movie lands here when HolyArrs cannot go on by itself:

- **It ran out of Releases.** A download failed and no other Release passes the Pick rules, or you searched again and nothing passed.
- **An import hit a conflict.** The files could not be placed on their own.
- **A download vanished** and the setting "Replace downloads removed outside HolyArrs" is off.

Each movie shows the reason in plain words, the last Release and how long it has waited. Pick one of:

| Choice | What happens |
|---|---|
| **Search again** | Searches all your Sources now. If a Release passes, it is sent to a Downloader. If not, the movie stays here with the new reason. |
| **Choose a release by hand** | Opens the movie page at the Release list. Pick any Release, also one that failed the Pick rules (HolyArrs says why it failed and asks first). |
| **Try another Version** (or **Add another Version**) | Opens the movie page at its Versions, to use other Pick rules (for example a 720p Version when no 1080p copy exists). |
| **Add to Turned down** | Puts the last Release on the Turned down list, then searches again. |
| **Stop looking** | Turns **Keep up to date** off for that Version. HolyArrs stops searching for it; files on disk are not touched. Turn Keep up to date on again on the movie page to start again. |

When nothing needs you, the screen says so and the red number in the menu goes away.

## Settings used here

All on **Settings**, **Downloaders**:

| Setting | Default | Used for |
|---|---|---|
| Stalled after (hours without progress) | 6 | When a row says Stalled |
| Give up and try the next Release after (hours) | 24 | When a stalled Download is marked failed by itself |
| Remove failed downloads from the Downloader | on | Mark failed and failures |
| Replace downloads removed outside HolyArrs | on | Off sends vanished downloads to Needs a decision |
| Delete its files (remove) | on | Where the Remove box starts |
| Add its Release to Turned down (remove) | off | Where the Remove box starts |
| Search again (remove) | on | Where the Remove box starts |
