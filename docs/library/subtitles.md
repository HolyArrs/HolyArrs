---
title: Subtitles
description: Pick your subtitle languages, see which movies and episodes are missing one, and fix or replace subtitles.
level: Setup
sidebar: { order: 3 }
---

Pick your subtitle languages once. HolyArrs reads what your files already have and lists what is missing.

Only staff see subtitles. Changing settings needs the **Change settings** permission. Music has no subtitles.

:::note
Subtitle sites and subtitle search are held back in the first release. They stay hidden unless the `subtitle_sites` switch is on, see [Held-back features](../run/updates.md#held-back-features). Languages and the Missing subtitles list work without it.
:::

## Set your languages

1. Open **Settings > Subtitles**.
2. Under the default languages, add each language you want, such as English and Spanish.
3. For each language, set **Hearing-impaired** and **Forced only** (see [Languages](#languages)).
4. Click **Save**. You should see **Library > Missing subtitles** fill in. HolyArrs reads every file you have and asks no site.
5. If Subtitle sites are on for you, click **Add a Subtitle site**, pick the site, fill in **Your account** if it asks, and save. You should see the result of a test search right away.

Files brought in with **Import a library** are read the same way.

## See what is missing

Open **Library > Missing subtitles**. Each row is a movie or episode that lacks a subtitle, with the reason (not looked for yet, not found yet, or nothing over the Pass mark yet) and the next try. A title removed from the Library is left out, even when its files stay on disk.

- Filter by language, by movies or episodes, and by title. The page address keeps your filters, so you can bookmark a view.
- **Search all** searches every row in the filtered list and shows progress.
- **Search** on a row searches that one file.

## Fix one movie or episode

A movie page has a **Subtitles** card. A show page has one for the whole show. Each episode with a file has a **Subtitles** button on its row.

| Control | What it does |
|---|---|
| **Languages** | Shows what this title wants and where it comes from: the default languages, the show, or its own. **Change** switches to its own list. An empty own list means no subtitles. |
| **Look for subtitles** | Searches for what is missing now. |
| **Choose a subtitle** | Lists every result from every site with its score out of 100 and what matched. **Choose** saves it, whatever its score. |
| **Sync fix** | For subtitles HolyArrs saved. Shifts every line by up to 10 minutes, changes the frame rate (such as 25 to 23.976), or both. The old file goes to the holding folder. |
| **Turn down** | For subtitles HolyArrs saved. The file goes to the holding folder, that subtitle is never picked again for this file, and HolyArrs looks for another. |

Each subtitle shows its language, kind (normal, hearing-impaired or forced), source (inside the video, beside it, a site, or chosen by hand), score and file name. Everything that happens shows in the title's **History** under Subtitles, and [Alerts](../users/alerts.md) can tell you too.

## Choices

### Languages

| Option | Choices |
|---|---|
| **Hearing-impaired** | Normal (a hearing-impaired subtitle also counts), Hearing-impaired only (waits for one with sound descriptions), or Either. |
| **Forced only** | Only the lines for foreign speech and signs. |

Up to 10 languages. A language can appear twice only if one is full and the other forced only. With no languages set, HolyArrs looks for nothing.

### Where subtitles are saved

Next to the video: the video's name, the language, then `.hi` (or `.cc`, `.sdh`) or `.forced`, then the format.

```
Movie (2020).en.srt
Movie (2020).es.hi.srt
Movie (2020).fr.forced.ass
```

- HolyArrs never overwrites a subtitle file it did not save. That file counts as there.
- A subtitle HolyArrs replaces goes to the holding folder first.
- **Rename files** moves subtitles with their video. If the new name is taken, that subtitle stays and says why.
- Deleting a title with its files sends its subtitles to the holding folder too.

How scoring works, when HolyArrs searches, and the sites it uses are in [Subtitle scoring and sites](subtitle-scoring.md).

## If it does not work

**Symptom:** nothing is listed as missing.
**Cause:** no language was saved, or the title has an empty own list.
**Fix:** add a language and click **Save**.

**Symptom:** you can't see Subtitle sites or **Look for subtitles**.
**Cause:** they are held back, see [Held-back features](../run/updates.md#held-back-features).

**Symptom:** subtitles are out of time.
**Fix:** use **Sync fix**, or raise the Pass mark so only closer matches are saved. HolyArrs does not listen to the sound to line subtitles up.

**Symptom:** a subtitle is wrong.
**Fix:** use **Turn down**.

**Symptom:** an existing subtitle was not replaced.
**Cause:** HolyArrs never overwrites subtitle files it did not save.

## For experts

<details class="ha-more"><summary>Settings reference</summary>

| Setting | Default | Range |
|---|---|---|
| Default languages | none | up to 10 |
| **Hearing-impaired** file ending | `.hi` | `.hi`, `.cc`, `.sdh` |
| **Count subtitles inside the video** | On | On or Off |
| **Leave out picture subtitles** | Off | On or Off |

Scoring, schedule and Sync fix settings are in [Subtitle scoring and sites](subtitle-scoring.md).

</details>

<details class="ha-more"><summary>API</summary>

Every subtitle route is in `http://your-server:6262/api/v1/openapi.json`, under the Subtitles tag. Searches answer with the ids of their Jobs, so a script can follow them. See [API routes](../reference/api-routes.md).

</details>

## Related

- [Subtitle scoring and sites](subtitle-scoring.md)
- [Audio versions](audio-versions.md)
- [Held-back features](../run/updates.md#held-back-features)
