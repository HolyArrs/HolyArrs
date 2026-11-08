---
title: Movie downloads
description: Follow movie downloads, fix stuck ones and settle movies that need your choice.
level: Setup
---

See what is downloading and fix what gets stuck. Only the owner and admins see this screen.

When HolyArrs chooses a release, it sends it to a **Downloader** (qBittorrent or SABnzbd, for example) and watches it until the file is in your Library.

## Watch your downloads

1. Open **Downloads** in the menu.
2. Pick a tab to filter the rows.

You should see each movie with its release, progress, speed, time left and Downloader. Rows update by themselves.

| Tab | Means |
| --- | --- |
| Queued | Starts when the Downloader has room. |
| Paused | Paused in the Downloader, or the row says "The Downloader can't use the files" (most often a share that mounted late). Once the share is there, use **Check again** on the row, or recheck it in the Downloader. After about 3 days the health check turns red. |
| Downloading | Taking in data, or stalled while it keeps trying. |
| Checking | Checking its data on disk, or moving to a new folder. |
| Importing | Done. Being checked and moved into your Library. |
| Failed | Failed, removed outside HolyArrs, or the files did not match. |
| Review | HolyArrs could not finish and needs you to choose. |
| Problems | Failed and Review rows together. |

A download with no progress for 6 h counts as stalled. After 24 h without progress HolyArrs tries the next release.

:::tip
A first download can sit in Queued or Downloading for a while. That is the Downloader at work, and HolyArrs keeps watching.
:::

## Mark a download failed

1. Open the row's menu and pick **Mark failed**.
2. Confirm.

HolyArrs turns the release down, removes it and its files from the Downloader, and searches for the next release. If nothing passes the Pick rules, the movie moves to **Review**.

## Remove a download

1. Open the row's menu and pick **Remove**.
2. Set the options:

   | Option | Does | Starts |
   | --- | --- | --- |
   | Also delete files | The Downloader deletes what it fetched. | On |
   | Add its Release to Turned down | HolyArrs never picks it again for this movie. | Off |
   | Search again | Looks for the next release at once. Needs Turned down on; without it the movie waits for the next feed check. | On |

3. Press **Remove**.

You should see the row leave the list.

## Settle a movie in Review

A movie lands in **Review** when HolyArrs cannot go on alone: it ran out of releases, an import hit a conflict, or a download was removed outside HolyArrs with replacing turned off. The row says why. Open it and pick one:

| Choice | What happens |
| --- | --- |
| Search again | Searches every Source now. If nothing passes, the movie stays with the new reason. |
| Choose a release by hand | Opens the movie's release list. See [Search for movies](search.md#choose-a-release-yourself). |
| Try another Version | Opens the movie's Versions so other Pick rules apply, for example 720p when no 1080p exists. Says **Add another Version** when the movie has only one. |
| Add to Turned down | Turns down the last release, then searches again. |
| Stop looking | Turns **Keep up to date** off for that Version. Files on disk stay. |

When the import is unsure of the edition, you set it here. See [Movie editions](editions-collections.md).

## If it does not work

**A row is paused and says the Downloader can't use the files.**
Cause: a share mounted after the Downloader started.
Fix: once the share is back, press **Check again** on the row. After about 3 days the health check turns red.

**Downloads never start.**
Fix: test the Downloader on **Settings > Downloaders**, then open the status page, for example `http://your-server:6262/health`.

**The same bad release keeps coming back.**
Fix: remove it with **Add its Release to Turned down** on.

## Settings

<details class="ha-more"><summary>Settings reference: Settings > Downloaders</summary>

| Setting | What it does | Default | Change it when |
| --- | --- | --- | --- |
| Stalled after (hours) | When a row counts as stalled, 1 to 168. Queued or paused time does not count. | 6 | Slow, rare releases get flagged too early. |
| Try the next Release after (hours) | When a stalled download gives way to the next release, counted from the last progress. At least the stalled time, at most 720. | 24 | You prefer to wait longer for rare releases. |
| Remove failed downloads from the Downloader | Removes failed downloads and their files. | On | You want to inspect failed downloads. |
| Fail downloads that hold programs or fakes | Fails a torrent as soon as its file list shows a program or a fake. | On | Rarely. |
| Replace downloads removed outside HolyArrs | Off sends those movies to Review. | On | You remove downloads by hand on purpose. |
| Try the next Downloader | When one Downloader fails, the next of the same kind takes the release. | On | You want one Downloader only. |
| Seconds between sends to one Downloader | Spacing between sends, 0 to 60. | 2 | A Downloader struggles with bursts. |
| When you remove a download | Where the Remove box starts: Delete its files, Turn down its Release, Search again. | On, Off, On | Your usual choice differs. |

</details>

## Related

- [Search for movies](search.md)
- [Show downloads](../shows/downloads.md)
- [Organise movies](organise.md)
