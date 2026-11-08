# Follow your movie downloads

You want to see what is downloading, and fix the ones that get stuck. When HolyArrs chooses a release, it sends it to a Downloader such as qBittorrent or SABnzbd. It then watches it until the file is in your Library. Only the owner and admins see this screen.

## Watch your downloads

Open **Downloads** in the menu. Each row shows the movie, the release, a progress bar, the speed and time left, and the Downloader. Rows update by themselves.

The tabs at the top sort the rows:

| Tab | Means |
| --- | --- |
| Queued | Starts when the Downloader has room. |
| Paused | Paused in the Downloader. Or the row says "The Downloader can't use the files", most often a share that mounted late. Once the share is there, use Check again on the row, or recheck it in the Downloader. After about 3 days the health check turns red. |
| Downloading | Taking in data, or stalled while it keeps trying. |
| Checking | Checking its data on disk, or moving to a new folder. |
| Importing | Done downloading. Being checked and moved into your Library. |
| Failed | Failed, removed outside HolyArrs, or the files did not match. |
| Review | HolyArrs could not finish it and needs you to choose. |
| Problems | Failed rows and rows that need you, together. |

A download with no progress for 6 hours counts as stalled. After 24 hours HolyArrs marks it failed and tries the next release.

## Mark a download failed

1. Open the row's menu and pick **Mark failed**.
2. Confirm.

HolyArrs then puts the release on the Turned down list, removes it from the Downloader with its files, and searches for the next release. If nothing passes the Pick rules, the movie waits for you under **Review**.

## Remove a download

1. Open the row's menu and pick **Remove**.
2. Choose what happens:
   - **Also delete files**: the Downloader deletes what it fetched.
   - **Add its Release to Turned down**: HolyArrs never picks it again for this movie.
   - **Search again**: HolyArrs looks for the next release right away.
3. Confirm.

The box starts from your settings. Out of the box, delete files is on, Turned down is off, and search again is on.

## When a movie needs you

A movie lands under **Review** when HolyArrs cannot go on by itself. For example, it ran out of releases, an import hit a conflict, or a download was removed outside HolyArrs and replacing it is turned off. The row says why. Open it and pick one:

| Choice | What happens |
| --- | --- |
| Search again | Searches all your Sources now. If nothing passes, the movie stays here with the new reason. |
| Choose a release by hand | Opens the movie's release list. See [Find releases for your movies](search-movies.md). |
| Try another Version | Opens the movie's Versions, so you can use other Pick rules, for example 720p when no 1080p copy exists. It says **Add another Version** when the movie has only one. |
| Add to Turned down | Turns down the last release, then searches again. |
| Stop looking | Turns **Keep up to date** off for that Version. Files on disk are not touched. |

When the import is unsure about the edition, you can set it here and try again. See [Movie editions](movie-editions.md).

## Settings

Open **Settings**, then **Downloaders**.

| Setting | What it does | Default |
| --- | --- | --- |
| Stalled after (hours) | When a row counts as stalled. | 6 |
| Give up after (hours) | When a stalled download is marked failed. | 24 |
| Remove failed downloads | Removes failed downloads from the Downloader. | On |
| Fail downloads that hold programs or fakes | Fails a torrent as soon as its file list shows a program or a fake. | On |
| Replace downloads removed outside HolyArrs | Off sends those movies to Review instead. | On |
| Try the next Downloader | When one Downloader cannot take a release, it goes to the next of the same kind. | On |
| When you remove a download | Where the Remove box starts. | Delete files on, Turned down off, Search again on |

## Troubleshooting

- **A row is paused and says the Downloader cannot use the files.** Most often a share mounted late. Once the share is back, use **Check again** on the row.
- **Downloads never start.** Check the Downloader on **Settings**, **Downloaders**. Then open the status page, for example `http://your-server:6262/status`.
- **The same bad release keeps coming back.** Remove it with **Add its Release to Turned down** on.
