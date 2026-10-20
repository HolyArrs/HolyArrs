# Trailers

Explore and title pages can play a short, muted trailer over the art: on the
featured title at the top of Explore, and on the larger card that opens over a
poster or wide card. This page says where the trailers come from and what is
kept.

## Where a trailer comes from

1. The title's YouTube trailer or teaser is listed by the metadata server
   (an official Trailer first, then a Teaser, in the person's language before
   English). Clips never play.
2. **yt-dlp** looks up a direct video address on YouTube's video servers. It
   asks for the formats only and downloads nothing.
3. The browser plays that address in HolyArrs' own video. HolyArrs does not
   copy, store or pass the video through; only the address is kept, until
   shortly before it expires.

Playback tries these in order and stops at the first that works:

| Order | What plays |
| --- | --- |
| 1 | The adaptive HLS stream, in browsers that play it themselves (Safari, iOS). |
| 2 | The MP4 (H.264, up to 720p). Muxed with sound when YouTube offers it, otherwise picture only. |
| 3 | The privacy-enhanced YouTube embed (`youtube-nocookie.com`). |
| 4 | The moving backdrop pictures, with no video. |

Heroes also slowly zoom and pan through up to four backdrops from TMDB, with
or without a trailer.

An armv7 image has no yt-dlp build and goes straight to the embed.

## When a trailer starts

- The Explore hero starts after 1.5 seconds in view.
- A poster's card starts 0.8 seconds after the pointer rests on it, on a
  desktop pointer only. A wide card grows after 0.5 seconds first. Keyboard
  focus also opens a card, and Esc closes it.
- Only one video plays at a time; a new one stops the one before it.
- The server looks up the first trailers ahead of time (the hero first, then
  the first four cards of the first three rows) so they start quickly. A
  person may start 12 new lookups a minute, up to 4 run at once, and a title
  with no direct stream is not tried again for 10 minutes.

## Sound

Trailers start muted. A sound button at the bottom right turns sound on, and
that choice is remembered for the session. It shows only when the stream has
sound: a picture-only MP4 has no button. If the browser refuses sound
without a recent click, the trailer stays muted.

**Planned, not built yet:** passing the sound through HolyArrs, so a
picture-only stream can also play with sound. Until then, sound comes only
from the HLS stream, a muxed MP4, or the embed.

## Settings

Both are under **Settings > This app**, and apply to the whole household.

| Setting | What it does |
| --- | --- |
| **Autoplay trailers** ("Play trailers by themselves") | On by default. Off stops every autoplay and the lookups behind it. |
| **Keep yt-dlp up to date** | On by default. Once a week HolyArrs fetches yt-dlp's newest official release, checks its SHA-256 against the release's `SHA2-256SUMS`, and keeps the previous copy if the new one fails a test lookup. |

`GET` and `PUT /api/v1/settings/ui` read and change both
(`autoplay_trailers`, `keep_ytdlp_updated`).

yt-dlp's version and the last update's result show under **Health**. Health
warns "Trailer lookups failing: update yt-dlp" after 5 failed lookups in a
row. How HolyArrs finds yt-dlp (`HOLYARRS_YTDLP`, the checked copy in
`<config>/bin`, the bundled copy, then `PATH`) is in
[Portable packages](portable.md). The Docker image ships yt-dlp 2026.08.19 and
is built on `debian:12-slim`, because the yt-dlp release program needs glibc
(see [Install](install.md#the-image)).

## Kid accounts and reduced motion

- **Kid accounts never autoplay trailers.** The server answers 403 for them.
- **Reduce motion** (the browser or operating-system setting) turns off
  trailers, the grow on hover and the slow zoom. Still pictures stay.
- Trailers also stay off on slow connections (2G or 3G) and when the browser
  asks to save data.

## Privacy

- Lookups go from your HolyArrs server to YouTube. The browser then loads
  video from YouTube's video servers, as with any YouTube video, so YouTube
  can see the viewer's address.
- The embed uses `youtube-nocookie.com`, YouTube's privacy-enhanced domain.
- HolyArrs sends no name, ask or Library data to YouTube. It keeps only the
  video address (in memory and in its lookup cache, so a restart does not redo
  lookups) and never the video.
- Trailer addresses are not cached by the browser (`Cache-Control: private,
  no-store`).
