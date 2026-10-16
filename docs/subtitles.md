# Subtitles

This page covers how HolyArrs finds subtitles for movies and shows: which languages each title should have, how a match is scored against the Pass mark, the Subtitle sites it asks, the Missing subtitles list, and what you can do by hand on a movie or episode. Music has no subtitles. Only staff (people who manage the Library) see any of this; members never do. Changing settings and Subtitle sites needs the settings permission.

## The default languages

Open **Settings**, **Subtitles**. Under **Default languages**, **Add a language** for each language you want, for example English and Spanish, then **Save settings**. Every movie and show without languages of its own gets these. With no languages at all, HolyArrs looks for nothing.

Each language has two choices:

- **Hearing-impaired**: **Normal** (a hearing-impaired subtitle counts as there too), **Hearing-impaired only** (waits for one with sound descriptions), or **Either**.
- **Forced only**: only the lines for foreign speech and signs, not the whole film.

A language can be in the list twice only once as a full subtitle and once as forced only. At most 10 languages.

When you save new default languages, HolyArrs reads what every file already has (no site is asked) so the Missing subtitles list is right straight away. Files adopted with **Import a library** are read the same way.

## The Pass mark

Every subtitle a site offers gets a **match score** from 0 to 100. Points come from:

- the right name and year (for an episode, also the right season and episode),
- the same release group, source (Blu-ray, WEB, HDTV), resolution, video codec, audio and edition as your file; when the subtitle names exactly your file's release, all of these count,
- the hearing-impaired and forced choice you asked for,
- a trusted uploader,
- a frame rate hint: the subtitle's frame rate matches your video, or the site says it was corrected. This is worth one point only. HolyArrs does not measure your video's frame rate, so this hint rarely adds anything; a subtitle that is out of time is fixed by hand with **Sync fix** (below).

A subtitle for another title, year, season or episode scores 0.

A search saves a subtitle on its own only when it scores the **Pass mark** or more: **90 for episodes** (show, year, season and episode must match) and **70 for movies** (name, year and some release details must match). A lower mark takes looser matches, which are more often out of time; a higher mark waits longer for a close one. When you choose a subtitle by hand you may take any score.

## When HolyArrs looks

- A new file (an import or a better copy) is searched at once.
- **Look for missing subtitles every** (6 hours): every file still missing a subtitle is searched again, at most **Files per run** (100) at a time.
- **Slow down after three weeks**: a subtitle not found for three weeks is then looked for once a week.
- **Look for better subtitles** (every 12 hours, for subtitles saved in the last 7 days): a subtitle that scored under 99 is searched again; a new one must score higher.

**Count subtitles inside the video** (on) treats a language already inside the file as there. **Leave out picture subtitles** stops picture tracks from discs counting, since many players cannot show them. **Hearing-impaired files are named** picks `.hi`, `.cc` or `.sdh` in the file name.

## Where subtitles are saved

Next to the video, with the video's name, the language, then `.hi` (or `.cc`, `.sdh`) or `.forced`, then the format: `Movie (2020).en.srt`, `Movie (2020).es.hi.srt`, `Movie (2020).fr.forced.ass`.

- A subtitle file HolyArrs did not save is never overwritten; it counts as there.
- A subtitle HolyArrs saved that is replaced (a better one, a Sync fix, a Turn down) goes to the holding folder first.
- When you **Rename files**, the subtitle files beside a video follow it to its new name; the preview lists each one. One whose new name is taken stays where it is and says why.
- When you delete a movie or episode with its files, the subtitles beside it go to the holding folder with it.

## Subtitle sites

**Add a Subtitle site** under **Subtitle sites** on the same page. Pick the site, check its address, and save; HolyArrs tests it straight away and says what happened in plain words. **Test** asks it one search for a known title any time; a failed test never pauses it.

Each card says the site's state:

- **On**: asked on every search, lowest priority number first.
- **Paused**: you paused it; searches skip it until you **Turn on**.
- **Resting until** a time: it failed (did not answer, asked HolyArrs to slow down, reached its download limit for the day, refused the account details, or sent an answer HolyArrs could not read). HolyArrs tries it again after that time.
- **Needs your account**: see below.

**Limits** set the wait between calls, the most calls in a window, and downloads a day (0: no cap). **May reach the local network** is only for a site on your own machine or network; never a cloud address.

### Sites that need your account

Some sites only work with your own account with them. They are marked **Needs your account** and HolyArrs refuses to save or use one until you fill in what it asks for (a user name and password, or an API key). The fields are masked; the details are kept encrypted and never shown again. Leave them empty when you change the site to keep the stored ones. Moving the site to another address needs them again. Removing the site forgets them.

The sites HolyArrs knows:

| Site | Covers | Account | Notes |
|---|---|---|---|
| OpenSubtitles.com | movies and shows | API key, user name and password | Searches use only the key; downloads sign in with your user name and password (the sign-in is reused for a day and redone when the site refuses it). Searches match by the video's file hash, then by IMDb or TMDB id, then by name. Machine-made and AI-made translations are left out. Your account has a daily download count: when it runs out the site rests until the time the site gives for its reset. |
| subdl | movies and shows | API key | Searches by TMDB id, IMDb id or name; downloads are zip files from subdl's download server. Its download count resets at midnight UTC; when it runs out the site rests until then. |
| Gestdown | shows | none | Addic7ed's show subtitles. |
| Podnapisi | movies and shows | none | Starts turned off: the site name does not resolve right now. Turn it on when it answers again. |
| Addic7ed (direct) | shows | user name and password | HolyArrs signs in itself with your user name and password (kept in the vault; the session cookie only in memory) and reads the site's pages: it finds the show by name, reads the season page (language, version names, how complete, hearing impaired) and downloads with the show page as referrer. Unfinished subtitles are left out. Download links must stay on the site's own address. Used gently: at least 3 seconds between calls and at most 100 calls a day by default (never under 3 s; a hard cap of 200 a day), and a refused sign-in rests it 12 hours. When the site shows an anti-bot or captcha check, it rests 12 hours with that reason; HolyArrs never tries to get past one. |

When a site refuses your account details (wrong key, user name or password), it rests for 12 hours and shows **Resting until** with "refused the account details"; fix them and save, which turns it back on. A site that sends something HolyArrs cannot read, a link off the site that is not https, or a file that is not a subtitle is refused the same way as with every site. Account details never appear in the log, in Events or in error messages.

These three account sites were built and tested only against fake servers that copy each site's public answer shape. The first real search with your account is yours to make, with the Test button.

## Missing subtitles

**Library**, **Missing subtitles** lists every movie and episode missing a subtitle it should have, with why (not looked for yet, not found yet, or none over the Pass mark yet, and when the next try is). Narrow it by **Language**, **Kind** (movies or episodes) and **Movie or show**; the page address keeps the filters, so a view can be bookmarked.

- **Search all** searches every file in the list as filtered now and shows how far it got.
- **Search** on a row searches that one file.

## On a movie or episode

A movie page has a **Subtitles** card. A show page has one for the whole show (with a link to its missing subtitles); each episode with a file has a **Subtitles** button on its row.

- **Languages**: what this title or episode wants and where that comes from (the default languages, the show, or its own). **Change** picks **The default languages** (or, for an episode, **The same as the show**) or **Its own languages**. An empty own list means no subtitles for it.
- Each subtitle there now shows its language, kind (normal, hearing-impaired, forced), where it came from (inside the video, found beside it, a site, or chosen by hand), its score and the file name.
- **Look for subtitles** looks for what is missing at once.
- **Choose a subtitle** asks every Subtitle site and lists every result for each wished language with its score out of 100 and its reasons (what matched and what did not). **Choose** saves that one whatever its score.
- **Sync fix** (only for subtitles HolyArrs saved): shift every line earlier or later (up to 10 minutes), change the frame rate (for example from 25 to 23.976), or both. The old file goes to the holding folder. The starting step and frame rates come from **Sync fix** in Settings > Subtitles. HolyArrs does not listen to the sound to line subtitles up.
- **Turn down** (only for subtitles HolyArrs saved): the file goes to the holding folder, HolyArrs never chooses that subtitle again for this file, and looks for another.

Everything that happens shows in the title's **History** under **Subtitles**, and **Alerts** can speak for it.

## For scripts

Every subtitle route is in the API description (`GET /api/v1/openapi.json`), under the Subtitles tag: settings, languages, a title's subtitles and wish, searches, the missing list, choices and choose, sync, turn down, and Subtitle sites with their test. Searches answer the ids of their Jobs, so a script can follow them.
