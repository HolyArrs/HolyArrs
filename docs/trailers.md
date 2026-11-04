# Trailers

Explore and title pages can play a short, muted trailer over the art: on the
featured title at the top of Explore, and on the larger card that opens over a
poster or wide card. This page says where the trailers come from and what is
kept.

## Where a trailer comes from

1. The title's YouTube trailer or teaser is listed by the metadata server
   (an official Trailer first, then a Teaser, in the person's language before
   English). Clips never play.
2. HolyArrs' own **trailer reader** looks up a direct video address on
   YouTube's video servers. It reads the video's page and its list of
   formats, downloads nothing, and never signs in.
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

## When a trailer will not play

Some videos YouTube does not play here: it asks to sign in to confirm the
viewer's age, the video was removed or made private, it is blocked in this
country, it is a premiere that has not started yet, or it sits behind a
content warning. HolyArrs never gets around those. Instead:

- A video that asks for a sign-in, was removed or made private, or sits
  behind a content warning is skipped for 7 days, for every title that
  lists it. A video that is blocked here or not on yet is tried again on the
  next lookup round (within 5 minutes), so a premiere plays soon after it
  starts.
- The title's next trailer is tried, in the same order as above, up to four
  more in one lookup. The one that plays is shown meanwhile; once the first
  is no longer skipped, it is tried first again.
- When none of them plays, the page shows the moving backdrop pictures and
  no YouTube embed, since the embed would only show YouTube's refusal.

When YouTube refuses HolyArrs as a whole (its "confirm you're not a bot"
check, its "try again later", or YouTube could not be reached), the title
is not marked and no other trailer is tried: that round of lookups stops,
the same trailer is tried again on the next round, and the embed may still
play it meanwhile.

`GET /api/v1/trailer/{kind}/{tmdb_id}` then answers 404 with `reason` (for
example `age_restricted` or `bot_check`) and `key`, the video the embed may
still play, or `""` when none is left. For a title not looked up yet it
answers 404 with only `key`, the title's YouTube trailer when known, so the
embed plays it.

When the full-screen player cannot play a trailer either, it says why in
plain words when HolyArrs knows: YouTube plays it only for signed-in
adults, the video was removed, it is private or for members only, YouTube won't play it
here (blocked in a region, embedding turned off, or a live stream not on),
it sits behind a content warning, or trailers can't be reached right now.
Otherwise it says the trailer can't play here. When HolyArrs already knows
none of a title's trailers plays here, the title page's Trailer button
still opens the player, and it says why at once.

## When a trailer starts

- The Explore hero starts after 1.5 seconds in view.
- A poster's card starts 0.8 seconds after the pointer rests on it, on a
  desktop pointer only. A wide card grows after 0.5 seconds first. Keyboard
  focus also opens a card, and Esc closes it.
- Only one video plays at a time; a new one stops the one before it.

Trailers are looked up on a fixed schedule, never when a page opens. Every
5 minutes (the first round a few minutes after HolyArrs starts) the server
looks up to 20 trailers, a few seconds apart. Addresses that would expire
before the next round are renewed first. Then come the titles people have
looked at that had nothing yet, oldest first (seeing one again keeps its
place): any card that stayed on screen (a genre's rows and New on your
services too), a person's page, the Requests banner, New & Popular's
featured titles, each in the language it was asked in. Then the other
renewals, then everything Explore shows for each household member who is
not a Kid account: the
featured titles first, then every card of every row, in up to three
languages pages asked for lately. An address is looked up again shortly
before it expires. A title with no trailer, or none that plays here, is
tried again after 2 hours. A title not looked up yet plays the YouTube
embed until a round reaches it. Right after HolyArrs is first set up, the
first round of every title can take a few hours; after that the rounds
keep up.

## Where a music video starts

For artists' music videos, the metadata server picks where each one starts.
It reads YouTube's "most replayed" graph and the video's storyboard (the
small preview pictures YouTube shows along the timeline) with HolyArrs' own
trailer reader, the same one that finds trailers. Nothing is downloaded, and
there is no sign-in. When the metadata server has no start for a video,
HolyArrs reads the most replayed moment itself in the same way, with the
opening left out.

- The start is the most replayed moment. The first part of the video is left
  out of that choice, because every play starts there, so a start is not
  0:00 just for that.
- If the storyboard shows the picture barely moving from there, a moving
  moment nearby is used instead.
- A video YouTube keeps behind a sign-in or an age check gets no start, and
  the app starts it 30% in. When YouTube refuses the server as a whole (its
  "confirm you're not a bot" check), the server stops asking for 30 minutes.

## Sound

Trailers start muted. A sound button at the bottom right turns sound on, and
that choice is remembered for the session. It shows only when the stream has
sound: a picture-only MP4 has no button. If the browser refuses sound
without a recent click, the trailer stays muted.

**Planned, not built yet:** passing the sound through HolyArrs, so a
picture-only stream can also play with sound. Until then, sound comes only
from the HLS stream, a muxed MP4, or the embed.

## Settings

It is under **Settings > This app**, and applies to the whole household.

| Setting | What it does |
| --- | --- |
| **Autoplay trailers** ("Play trailers by themselves") | On by default. Off stops every autoplay and the lookups behind it. |

`GET` and `PUT /api/v1/settings/ui` read and change it (`autoplay_trailers`).
The old switch that kept a separate trailer tool up to date is gone: the
trailer reader is part of HolyArrs and gets its fixes with HolyArrs updates.
Older scripts that still send that switch keep working: it is ignored.

The trailer reader's state shows on **Status > Updates** and under
**Health**: the last good lookup, and while YouTube turns the reader away,
why and for how many different titles in a row. A trailer that played here
and then stops playing (removed or blocked) counts too, and is named only
once they add up to a warning. A trailer that never played here does not
count, and neither does one behind an age check, a sign-in or a content
warning, since that is about the video.
After 5 different titles fail with no good lookup in between, Health warns
with the reason in plain words: usually YouTube changed something and a
HolyArrs update is needed. The count, and a warning already shown, are kept
across restarts. While trailer autoplay is off
nothing is looked up, and Health leaves the trailer reader out.

| Reason | Health says |
| --- | --- |
| The bot check | YouTube asks to confirm you're not a bot |
| YouTube changed how its videos are reached | YouTube changed how its videos are reached; a HolyArrs update will fix it |
| An error answer from YouTube | YouTube turned the lookups away |
| No connection | YouTube could not be reached |
| Trailers that played stop playing across many titles | Trailers are not being found; a HolyArrs update may be needed |

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
  video address (in memory and in its SQLite lookup cache, so a restart does
  not redo lookups) and never the video. A kept address is checked again when
  it is read back: it must still be on YouTube's video servers and not expired.
- Trailer addresses are not cached by the browser (`Cache-Control: private,
  no-store`).
