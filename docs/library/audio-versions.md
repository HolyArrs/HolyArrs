---
title: Original and dubbed audio
description: Keep one copy of a title in its original language and another dubbed, using Versions with audio languages.
level: Setup
sidebar: { order: 5 }
---

Keep an original-language copy and a dubbed copy of the same movie or show, each in its own Version.

A Version is one copy of a title with its own folder, Pick rules and upgrades. HolyArrs checks the real audio tracks in a file before it counts for a Version.

## Keep an original and a dubbed copy

1. Open the title page.
2. Add a Version for each copy, such as "Original" and "German".
3. For each Version, enter the audio languages it needs as 2 or 3 letter codes, such as `en` or `deu`. Separate codes with commas or spaces.
4. Give each Version its own Library folder and Pick rules.
   You should see one group per Version in the **Copies** section.

HolyArrs never copies a file into a second Version on its own.

## How languages are matched

- Codes for one language are the same: `eng` matches `en`, `fra` matches `fr`.
- Every language you list must be among the file's audio tracks.
- A Version with no languages only looks at quality.
- On import, only tracks found in the file count. Release names, subtitles and language tags in a name do not. See [Codecs and media inspection](codecs-inspection.md).
- The title's original language describes the movie or show. It says nothing about a file's tracks.
- One file with several languages can meet several Versions. An import still fills only the Version the download was for.

This applies to movies and shows. Music is not checked this way.

## How search follows a Version's languages

Searches for a Version also read the language tags in each Release name, such as `FRENCH` or `GERMAN`.

- A name with no language tag counts as the title's original language.
- A name that clearly lacks a language the Version needs is refused.
- `MULTI`, or `DUAL` with no language named, may hold any language, so these names only get a warning.
- Subtitle tags such as `VOSTFR` or `SUBFRENCH` never count as audio.

The file's real tracks are still checked on import.

## The Copies section

Each Version's group shows the file, the audio found, the edition, and whether the file is missing.

| Action | What it does |
|---|---|
| **Protect this copy** | Better copies never replace it, see [Protected copies](protected-copies.md). |
| **Keep this copy and add another Version** | Adds a Version, such as 4K, beside the copy you have. |
| **Use an existing video** | Uses a video already in that Version's folder. |

## If it does not work

**Symptom:** an import waits for a decision.
**Cause:** the file's audio cannot meet the Version's languages. The copy you have stays.
**Fix:** compare the tracks found with the languages asked for, then pick another Release or change the Version.

**Symptom:** audio shows as unknown.
**Cause:** HolyArrs could not read the tracks, or a track has no language label. An unlabelled track never stands in for a missing language.

**Symptom:** a rescan changed nothing.
**Cause:** rescans skip files whose size and change time are the same. Editions and protection stay as they were.

## Related

- [Protected copies](protected-copies.md)
- [Codecs and media inspection](codecs-inspection.md)
- [Subtitles](../library/subtitles.md)
