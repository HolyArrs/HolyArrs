# Subtitles

You want each movie and episode to have subtitles in the languages you watch. You pick the languages once. HolyArrs reads what your files already have and lists what is missing. With Subtitle sites turned on, it also finds and saves subtitles for you.

Music has no subtitles. Only staff see subtitles. Members never do. To change settings or Subtitle sites, you need the settings permission.

Subtitle sites and subtitle search are held back in the first release. They stay hidden unless the `subtitle_sites` switch is on. See [Held back](../help/held-back.md). Languages and the Missing subtitles list work without it.

## Set it up

1. Open **Settings**, then **Subtitles**.
2. Under the default languages, add each language you want, for example English and Spanish.
3. For each language, set **Hearing-impaired** and **Forced only** (explained below).
4. Click **Save**.
5. If Subtitle sites are on for you, click **Add a Subtitle site**, pick the site, fill in **Your account** if it asks, and save. HolyArrs tests it right away and tells you what happened.

When you save new default languages, HolyArrs reads every file you already have. It asks no site for this. The Missing subtitles list is right straight away. Files you bring in with **Import a library** are read the same way.

## Fix one movie or episode

A movie page has a **Subtitles** card. A show page has one for the whole show, with a link to its missing subtitles. Each episode with a file has a **Subtitles** button on its row.

- **Languages** shows what this title wants and where that comes from: the default languages, the show, or its own. **Change** lets you pick the default languages (for an episode, the same as the show) or its own languages. An empty own list means no subtitles for it.
- Each subtitle you have shows its language, its kind (normal, hearing-impaired or forced), where it came from (inside the video, found beside it, a site, or chosen by hand), its score and its file name.
- **Look for subtitles** searches for what is missing now.
- **Choose a subtitle** asks every Subtitle site and lists every result with its score out of 100 and what matched. **Choose** saves the one you pick, whatever its score.
- **Sync fix** works on subtitles HolyArrs saved. Shift every line earlier or later, by up to 10 minutes. Change the frame rate, for example from 25 to 23.976. Or do both. The old file goes to the holding folder. HolyArrs does not listen to the sound to line subtitles up.
- **Turn down** works on subtitles HolyArrs saved. The file goes to the holding folder. HolyArrs never picks that subtitle again for this file, and looks for another.

Everything that happens shows in the title's **History** under Subtitles. [Alerts](../alerts/alerts.md) can tell you about it too.

## The Missing subtitles list

Open **Library**, then **Missing subtitles**. It lists every movie and episode missing a subtitle it should have, and why: not looked for yet, not found yet, or nothing over the Pass mark yet. It shows when the next try is.

Narrow it by language, by kind (movies or episodes), and by movie or show. The page address keeps your filters, so you can bookmark a view.

- **Search all** searches every file in the list as it is filtered now, and shows progress.
- **Search** on a row searches that one file.

## Settings explained

### Languages

You can add up to 10 languages. A language can appear twice only if one is a full subtitle and the other is forced only.

- **Hearing-impaired**: Normal (a hearing-impaired subtitle also counts), Hearing-impaired only (waits for one with sound descriptions), or Either.
- **Forced only**: just the lines for foreign speech and signs, not the whole film.

With no languages set, HolyArrs looks for nothing.

### The Pass mark

Every subtitle a site offers gets a match score from 0 to 100. Points come from:

- the right name and year, and for an episode the right season and episode,
- the same release group, source, resolution, video codec, audio and edition as your file,
- the hearing-impaired and forced choice you asked for,
- a trusted uploader,
- a frame rate hint, worth one point. HolyArrs does not measure your video's frame rate, so this rarely adds anything.

A subtitle for another title, year, season or episode scores 0.

A search saves a subtitle on its own only when it reaches the Pass mark. The defaults are 90 for episodes and 70 for movies. You can set each from 0 to 100. A lower mark takes looser matches, which are more often out of time. A higher mark waits longer for a close match. When you choose by hand, any score is allowed.

### When HolyArrs looks

- A new file, from an import or a better copy, is searched at once.
- Files still missing a subtitle are searched again every 6 hours by default (1 to 168). **Files per run, at most** caps each pass at 100 by default (1 to 1000).
- **Slow down after three weeks**: a subtitle not found in three weeks is then looked for once a week.
- **Look for better subtitles**: every 12 hours by default, subtitles saved in the last 7 days that scored under 99 are searched again. A new one must score higher.

### Other choices

- **Count subtitles inside the video** (on by default): a language already inside the file counts as there.
- **Leave out picture subtitles**: picture tracks from discs do not count, since many players can't show them.
- The hearing-impaired file name ending: `.hi` (default), `.cc` or `.sdh`.
- **Sync fix**: the starting step (500 ms by default) and the frame rates the Sync fix dialog starts with (25 to 23.976).

## Where subtitles are saved

Next to the video, with the video's name, then the language, then `.hi` (or `.cc`, `.sdh`) or `.forced`, then the format:

- `Movie (2020).en.srt`
- `Movie (2020).es.hi.srt`
- `Movie (2020).fr.forced.ass`

A few rules:

- HolyArrs never overwrites a subtitle file it did not save. That file counts as there.
- When HolyArrs replaces a subtitle it saved, the old one goes to the holding folder first.
- When you **Rename files**, the subtitles beside a video follow it. The preview lists each one. If the new name is taken, that subtitle stays put and says why.
- When you delete a movie or episode with its files, the subtitles beside it go to the holding folder too.

## Subtitle sites

Held back in the first release, see above.

Sites are asked lowest priority number first. Each card shows a state:

- **On**: asked on every search.
- **Paused**: you paused it. Click **Turn on** to bring it back.
- **Resting until** a time: it failed, and HolyArrs tries again after that time.
- **Needs your account**: it will not run until you fill in your details.

**Test** runs one search for a known title. A failed test never pauses the site.

**Limits** set the wait between calls, the most calls in a window, and downloads a day (0 means no cap). **Reach the local network** is only for a site running at home or on your own network.

### Sites that need your account

Some sites only work with your own account. HolyArrs will not save or use one until you fill in what it asks for, a user name and password or an API key. The fields are masked. Your details are stored encrypted and never shown again. Leave the fields empty when you edit the site to keep the stored ones. Moving the site to another address asks for them again. Removing the site forgets them. Account details never appear in the log, in Events or in error messages.

| Site | Covers | Account | Notes |
|---|---|---|---|
| OpenSubtitles.com | movies and shows | API key, user name and password | Searches use the key. Downloads sign in with your user name and password. Matches by file hash, then IMDb or TMDB id, then name. Machine translations are left out. |
| subdl | movies and shows | API key | Searches by TMDB id, IMDb id or name. The download count resets at midnight UTC. |
| Gestdown | shows | none | Show subtitles from Addic7ed. |
| Podnapisi | movies and shows | none | Starts turned off because the site does not answer right now. Turn it on when it does. |
| Addic7ed (direct) | shows | user name and password | Used gently: at least 3 seconds between calls and 100 calls a day by default. Unfinished subtitles are left out. HolyArrs never tries to get past a captcha. |

The account sites have only been tested against stand-in servers. Use **Test** for your first real search.

## Troubleshooting

**Nothing is listed as missing.** Check that you set at least one language and clicked **Save**. A title with an empty own list wants nothing.

**I can't see Subtitle sites or Look for subtitles.** They are held back. See [Held back](../help/held-back.md).

**A site says Resting until.** Here is how long it rests:

| Why | Rest |
|---|---|
| Did not answer | 20 minutes, doubling up to 6 hours |
| Asked HolyArrs to slow down | the time the site gives, up to a day, or 1 hour |
| Reached its download limit | until the site's reset, or midnight UTC |
| Sent an answer HolyArrs could not read | 6 hours |
| Refused your account details, or showed an anti-bot check | 12 hours |

If it refused your details, fix them and save. That turns it back on.

**Subtitles are found but out of time.** Use **Sync fix** on the title. Or raise the Pass mark so only closer matches are saved.

**A subtitle is wrong.** Use **Turn down**. HolyArrs will not pick it again for that file.

**An existing subtitle was not replaced.** HolyArrs never overwrites subtitle files it did not save.

## For scripts

Every subtitle route is in the API description at `http://your-server:6262/api/v1/openapi.json`, under the Subtitles tag. Searches answer with the ids of their Jobs, so a script can follow them. See [API](../arrapi/README.md).
