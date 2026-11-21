---
title: YouTube
description: Download YouTube videos, playlists and channels to your server, watch them in HolyArrs and save files to your device.
level: Start
---

Paste a YouTube link and HolyArrs downloads the video to your server. Follow a playlist or channel and new videos arrive by themselves. Each person sees only their own videos.

YouTube is held back in this release. It needs the `youtube` switch and two programs on the server.

## Turn it on

1. Add `youtube` to `HOLYARRS_FLAGS` and restart the container. See [Held-back features](../run/updates.md#held-back-features).
2. Put yt-dlp and ffmpeg on the server. See [Programs](#programs).
3. The owner opens **Settings > YouTube** and sets the **YouTube folder**, for example `/data/youtube`. Videos download only once it is set.
4. Give people the **Use the YouTube library** permission in **Settings > Users and roles**. The owner always has it.

You should see **YouTube** in the menu, under **Library**.

### Programs

yt-dlp downloads the videos. ffmpeg joins sound and picture. The HolyArrs image does not include them, so bring your own: mount the binaries into the container or build your own image.

HolyArrs looks for them in this order:

1. `HOLYARRS_YTDLP` and `HOLYARRS_FFMPEG`, full paths to the programs.
2. `yt-dlp` and `ffmpeg` on the container's `PATH`.

Keep yt-dlp in a folder HolyArrs can write to, such as `/config/bin/yt-dlp`. HolyArrs then lets it update itself once a day. YouTube changes often, and an old yt-dlp stops working.

**Settings > YouTube** shows where each program was found, its version and the last self-update.

### The YouTube folder

- It must exist and HolyArrs must be able to write to it.
- It must not be inside a Library folder or a downloads folder.
- It cannot change once it holds videos.
- Only the owner changes it. Admins see it.

## Add a link

1. Open **YouTube**.
2. Select **Add** at the end of the toolbar.
3. Paste a link to a video, playlist or channel and select **Add**.
4. Check the card that opens: title, channel and how many videos.
5. Select **Download** for a video, **Download all and follow** for a playlist, or **Follow** for a channel.

You should see the video as **Queued**, then **Downloading** with a percentage.

Links from `youtube.com`, `www.youtube.com`, `m.youtube.com`, `music.youtube.com` and `youtu.be` work. Channel links work as `@name`, `/channel/...`, `/c/...` or `/user/...`, with or without a tab such as `/videos`.

Following something you already follow shows **You already follow this**, with a link to it.

## Follows and the first download

A follow checks for new videos every 6 hours and downloads them.

| Follow | What it covers | First download |
|---|---|---|
| Playlist | Every video in the playlist | All of it |
| Mirrored playlist | Every video in the playlist, and nothing that left it. See [Mirror a playlist](#mirror-a-playlist) | All of it |
| Channel | Videos and past live streams. Shorts only with **Include Shorts** | **Newest 20**, **Newest…** (you pick how many) or **All** |

**All** on a channel downloads every video it has. On a big channel this takes days and a lot of space.

After the first check, only new videos download. Live streams and premieres wait until they end, then download.

Manage your follows under **Following > Manage**:

- Change quality, **Keep only the newest** and **Include Shorts**.
- Turn **Mirror this playlist** on or off for a playlist.
- **Pause** stops checks; **Resume** starts them again.
- **Unfollow** asks whether to keep or delete the videos already downloaded. See [What a follow deletes](#what-a-follow-deletes).
- See when the follow was last checked and why the last check failed.

To check a follow now, use **Run now** on **Check YouTube follows** in **Settings > Schedules**. That checks everyone's follows.

## A video in more than one playlist

A video is stored once per person, however many of your playlists and channels have it. When a follow lists a video you already have, HolyArrs links it to that follow and does not download it again.

- The list view names every playlist a video is in.
- Filtering by a playlist or channel shows the video under each one that has it.
- A video you added by its own link is yours to keep: no follow ever deletes it. That holds even when a follow brought the video first and you pasted its link later.

## Keep only the newest

Turn on **Keep only the newest** and pick how many. When a new video arrives, HolyArrs deletes the oldest beyond that number, file first. The follow never downloads them again.

Off by default: every video stays. A mirrored playlist keeps every video, so the two cannot be on together.

## Mirror a playlist

A mirrored playlist is kept the same as the playlist on YouTube. Turn on **Mirror this playlist** when you follow a playlist, or later under **Following > Manage**. Turning it on later asks first. It is off by default, and only playlists have it.

- Every video in the playlist downloads, whatever the first download took.
- A video that is no longer in the playlist is deleted here: file, thumbnail and its place in your list. That takes two checks in a row that both find it gone, so about 12 hours with the usual schedule. If it is back in the playlist at the second check, it stays.
- A video you added by its own link, or that another playlist or channel you follow also has, stays. It only leaves this playlist.
- A video you removed yourself stays removed while it is still in the playlist.

A check only counts towards deleting when YouTube's listing looks complete:

- A check that fails does not count. That covers a playlist that went private or was deleted.
- A listing that yt-dlp warned about does not count. yt-dlp warns when part of a long playlist could not be read.
- A listing that is empty, or shorter than the number of videos YouTube says the playlist has, does not count. YouTube leaves private and deleted videos out of a listing but still counts them, so a playlist holding such a video removes nothing until you take that video out of the playlist on YouTube.
- A paused follow is not checked.

Mixes and YouTube's own lists (Watch later, Liked videos, Uploads) cannot be mirrored: they change by themselves or cannot be read in full.

Turning the mirror off deletes nothing, and turning it on again starts the two checks afresh.

## What a follow deletes

Three things delete videos on their own: **Unfollow** with **Delete videos**, **Keep only the newest**, and a mirrored playlist. Each deletes a video's files only when both are true:

- no other playlist or channel you follow has the video, and
- you did not add the video by its own link.

Otherwise the video stays and only leaves that follow. **Unfollow** with **Keep videos** keeps every video, and no other follow deletes those later.

## Quality

Each follow downloads up to a height you pick: 2160p, 1440p, 1080p, 720p, 480p or 360p. The default is 1080p. HolyArrs takes H.264 with AAC sound at or below it first, so every browser plays the file. Without that, it takes the best picture at or below the height. It never re-encodes.

A single video downloads at 1080p.

## Watch

Select a video to play it. HolyArrs saves where you stopped every 10 seconds and when you pause or leave. At 90% it counts as **Watched**.

## Find your videos

The YouTube page works like the Library page:

- **Search** finds words in a title or channel name.
- **Grid** shows thumbnails. **List** shows a table: title, channel, playlists, uploaded, length, size and status. Select a heading to sort by it.
- **Sort & filter** sorts by added, uploaded, title, channel, length or size, and filters by status, watched or unwatched, playlist and channel.
- Filters that are on show as chips under the toolbar. **Clear** removes them all.
- A name on the **Following** line shows that follow's videos.

The view and the sort are remembered in your browser.

**Remove** in a video's menu deletes its file. A follow never downloads a removed video again. Paste the video's link on the YouTube page to download it once more. Nobody else sees your videos or follows, not even the owner. **Settings > YouTube** shows only totals for everyone together.

## Where files live

Each person gets their own folder inside the YouTube folder:

```
<YouTube folder>/<user id>-<name>/<channel or playlist>/<YYYY-MM-DD> <title> [<video id>].mp4
```

A `.jpg` thumbnail with the same name sits next to each video. A followed video goes in a folder named after the playlist, or after the channel as it was named when followed. A video in several follows goes in the folder of the one that had it first. Any other video goes in a folder named after its channel.

When the last video in a channel or playlist folder is removed, HolyArrs removes the folder too. It never removes a person's own folder or the YouTube folder, and it leaves a folder that holds files of your own.

Deleting a person deletes their YouTube videos and follows too.

## Save to your device

With the **Save files to their device** permission, a person saves a copy of a movie or an episode to their own device:

- On a movie's page, select **Save to device** next to its file.
- On a show's page, select the download button next to an episode.

The owner always has this permission. Admins and members get it only when ticked. Kid accounts never do.

Your own YouTube videos save from the video's menu or its page with **Save to device**. That needs only the YouTube permission.

## Bring in videos you already have

The owner can copy videos already on disk, for example a yt-dlp or Pinchflat folder, into a person's YouTube library. There is no page for it yet; use the API below.

- Files are copied, never moved. The source folder stays as it was.
- The YouTube id comes from a `.nfo` file next to the video, an `.info.json` file, or `[<id>]` in the file name.
- A video already in the library is not copied again.
- A video that is in the folder twice is copied once. Without `playlists`, the second copy is skipped.
- Thumbnails come from a `.jpg` or `-thumb.jpg` next to the video. HolyArrs converts a `.webp` or `.png`, or takes a frame from the video.
- Subtitle files such as `.en.srt` are copied too.
- No follows are made, unless `playlists` names a playlist link for a folder. The key is the name of the folder that holds the videos, not a path. That follow takes only videos added after it.
- With `playlists`, the videos of a folder are linked to that folder's follow. A video in two such folders is copied once and linked to both follows. A video already in the library is linked too.
- A video with no follow, or one YouTube no longer has, counts as added by its own link: no follow deletes it.

```sh
curl -X POST http://holyarrs:6262/api/v1/youtube/adopt \
  -H "X-Api-Key: <key>" -H "Content-Type: application/json" \
  -d '{"user_id": 1, "path": "/data/old-videos", "dry_run": true}'
```

To follow a playlist for one folder:

```sh
curl -X POST http://holyarrs:6262/api/v1/youtube/adopt \
  -H "X-Api-Key: <key>" -H "Content-Type: application/json" \
  -d '{"user_id": 1, "path": "/data/old-videos", "playlists": {"Crash Course": "https://www.youtube.com/playlist?list=<id>"}, "dry_run": true}'
```

`"dry_run": true` lists what would be copied, where to and why anything is skipped. Send it again without `dry_run` to start the copy as a job.

## If it does not work

**Symptom:** the YouTube page says it needs yt-dlp and ffmpeg.
**Cause:** HolyArrs cannot find one of them. The panel names which.
**Fix:** set `HOLYARRS_YTDLP` or `HOLYARRS_FFMPEG` to the full path, or put the program on `PATH`. Restart the container. **Settings > YouTube** shows where each was found.

**Symptom:** videos stay **Queued** and the page says downloads are paused.
**Cause:** the disk holding the YouTube folder has less free space than **Free space to keep (GB)**, 10 GB by default.
**Fix:** free some space, or lower the number in **Settings > YouTube**. Downloads go on by themselves.

**Symptom:** the page says **Choose a YouTube folder**.
**Cause:** the owner has not set the YouTube folder.
**Fix:** the owner sets it in **Settings > YouTube**. Queued videos then download.

**Symptom:** videos stay **Queued** with no message.
**Cause:** the `youtube` part is off in **Settings > Parts**.
**Fix:** turn it on. See [Parts and safe mode](../run/parts-safe-mode.md).

**Symptom:** a video shows **Failed**.
**Cause:** it failed 3 times. The card shows yt-dlp's reason, for example a private, removed or age-checked video.
**Fix:** select **Retry**. If many videos fail at once, check the yt-dlp version in **Settings > YouTube**; an old one stops working when YouTube changes.

**Symptom:** a follow says the last check failed.
**Cause:** yt-dlp could not list it, for example a deleted playlist. Other follows still run.
**Fix:** open the link in a browser. If it is gone, unfollow it.

**Symptom:** a video you took out of a mirrored playlist is still here.
**Cause:** it takes two checks in a row to delete a video. Or the video is also in another follow, was added by its own link, or YouTube's listing was not complete.
**Fix:** wait for two checks, or run **Check YouTube follows** in **Settings > Schedules** twice. If the playlist holds a private or deleted video, remove that one from the playlist on YouTube. See [Mirror a playlist](#mirror-a-playlist).

**Symptom:** **Skipped** videos on a channel.
**Cause:** older than the first download you picked, or older Shorts when **Include Shorts** was turned on later.
**Fix:** none needed.

**Symptom:** someone cannot see **YouTube**.
**Cause:** the `youtube` switch is off, they lack the permission, or it is a Kid account. Kid accounts never use YouTube.
**Fix:** add the switch, or tick **Use the YouTube library** in **Settings > Users and roles**.

## For experts

<details class="ha-more"><summary>Settings and schedule</summary>

| Item | Value |
|---|---|
| Switch | `youtube` in `HOLYARRS_FLAGS` |
| Programs | `HOLYARRS_YTDLP`, `HOLYARRS_FFMPEG`, else `PATH` |
| Settings | **Settings > YouTube**. Admins can view it; only the owner changes it. Stored in the `this_app` group as `youtube_folder` and `youtube_min_free_gb` |
| Free space to keep | 10 GB by default |
| Check YouTube follows | Every 6 h, 1 h to 7 days in **Settings > Schedules** |
| yt-dlp self-update | At most once a day, during a check |
| Download attempts | 3 |
| Permissions | `youtube`, `save_files`. Owner always; admins and members when ticked; Kid accounts never |

</details>

<details class="ha-more"><summary>Privacy</summary>

- Every video and follow belongs to one person. No route shows them to anyone else.
- Jobs, History and the log carry row ids only, never titles, channels, links or video ids. The one exception is a folder copy the owner starts: its job holds the folder path and any playlist links.
- yt-dlp only ever gets a link HolyArrs rebuilt from a checked id, never the pasted text.

</details>

## Related

- [Held-back features](../run/updates.md#held-back-features)
- [Users and roles](../users/roles.md)
- [Schedules](../run/schedules.md)
- [Environment variables](../reference/env-vars.md)
