# Trailers

You want short trailers to play over the art on Explore and title pages, or you want them off. This page shows how to turn them on or off, what plays where, and what to do when a trailer won't play.

## Turn trailers on or off

1. Open **Settings > Appearance**.
2. In the **For everyone** card, switch **Play trailers by themselves**.

It is on by default and applies to everyone on the server. When it is off, nothing plays by itself and HolyArrs stops looking trailers up.

Trailers never play by themselves for:

- Kid accounts.
- Browsers or systems set to **Reduce motion**. The grow on hover and the slow zoom stop too. Still pictures stay.
- Slow connections (2G or 3G), and browsers that ask to save data.

## Where trailers play

- **The Explore hero.** It starts after 1.5 seconds in view. Heroes also slowly zoom through up to four backdrops, with or without a trailer.
- **Poster cards.** A card opens 0.8 seconds after your mouse rests on it. Wide cards grow first. Keyboard focus opens a card too, and Esc closes it.
- **Artist pages.** One of the artist's official music videos plays the same way.

Only one video plays at a time. Trailers start muted. A sound button at the bottom right turns sound on, and the browser remembers that until you close it. If the browser blocks sound without a recent click, the trailer stays muted.

## Where a trailer comes from

1. The metadata server lists the title's YouTube trailers. An official trailer comes first, then a teaser, in your language before English. Clips never play.
2. HolyArrs reads the video's direct address from YouTube. It downloads nothing and never signs in.
3. Your browser plays that address. When your browser can't play it directly, or needs it for sound, HolyArrs passes the stream through without storing it.

If that fails, HolyArrs tries the privacy-enhanced YouTube embed, then falls back to the moving backdrop pictures.

## When lookups happen

Lookups run on a schedule, never when you open a page. Every 5 minutes HolyArrs looks up to 20 trailers, a few seconds apart. The first round starts a few minutes after HolyArrs starts. Addresses about to expire are renewed first. Then come titles people looked at that had nothing yet, then the rest of Explore for each person who is not a Kid account.

Right after a new setup, the first pass over everything can take a few hours. Until a round reaches a title, the YouTube embed plays instead. A title with no trailer that plays here is tried again after 2 hours.

Artist music videos follow the same schedule. An artist page you open is kept ready for a week after anyone last looked. Pressing play on a video in the **Music videos** row is the one exception: that video is looked up right away.

### Where a music video starts

Music videos start at the most replayed moment, with the opening left out of that choice. If the picture barely moves there, a moving moment nearby is used instead. A video behind a sign-in or an age check starts 30% in.

## Health

**Status > Updates** and **Health** show the last good lookup. When YouTube turns HolyArrs away for 5 different titles in a row, Health warns with the reason:

| Reason | What it means |
| --- | --- |
| YouTube asks to confirm you're not a bot | YouTube is checking this server. |
| YouTube changed how its videos are reached | A HolyArrs update will fix it. |
| YouTube turned the lookups away | YouTube answered with an error. |
| YouTube could not be reached | No connection. |
| Trailers are not being found | Trailers that played stopped playing. An update may be needed. |

Videos that are age checked, need a sign-in or sit behind a content warning don't count, since that is about the video. The warning count survives restarts. While trailers are off, Health leaves this out.

## Privacy

- Lookups go from your server to YouTube. Your browser then loads the video from YouTube's video servers, so YouTube sees the viewer's address, as with any YouTube video.
- The embed uses `youtube-nocookie.com`.
- HolyArrs sends no names, requests or Library data to YouTube. It keeps only the video address, never the video.

## Troubleshooting

**A trailer never plays.** YouTube may require a sign-in or an age check, the video may be removed, private, blocked in your country, a premiere not started yet, or behind a content warning. HolyArrs does not get around these. It skips such a video for 7 days and tries the title's next trailer, up to four more. A blocked video or a premiere is tried again within the hour. When none plays, you see the moving backdrop.

**The full-screen player says it can't play.** It tells you why when HolyArrs knows, for example that YouTube plays it only for signed-in adults. The Trailer button still opens the player so you can read why.

**Nothing plays anywhere.** Check **Play trailers by themselves** is on, your account is not a Kid account, and **Reduce motion** is off. Then check **Health** for a trailer warning.

**Trailers stopped after working for a while.** Look at **Health**. If it says YouTube changed how its videos are reached, update HolyArrs.
