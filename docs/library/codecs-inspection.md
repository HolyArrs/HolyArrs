---
title: Codecs and media inspection
description: Find movies by the video codec read from the file, and search again for better copies.
level: Expert
---

Find movies in an old or large codec and look for better copies.

A codec is the way the video is compressed, such as H.264 or AV1. Newer codecs give the same picture in a smaller file. You need permission to manage the Library.

## Find copies by codec

1. Open **Library > Video codecs**.
2. Type in **Search movies** to match the start of a title.
3. Pick a codec in **Codec**.
4. Pick an order in **Sort**: **Title**, **Codec** or **Added**.

The list shows main movie files that are present, with their Version, 50 rows a page. Your ticks stay while you change pages.

## Search again for better copies

1. Tick the copies you want.
2. Select **Review**. HolyArrs checks each copy again and shows what it will search.
3. Select **Start**. You should see results saved for each Version.

HolyArrs searches each Version with its current Pick rules. It does not pick, grab, replace or delete anything, and it does not change your rules. A better Release may not exist.

## Where the codec comes from

HolyArrs reads the part of the file that lists its tracks, when it imports a movie or show, takes over a file already on disk, or rescans. The release name never counts. Anything it cannot read shows **Unknown**.

| It reads | It records |
|---|---|
| Matroska (`.mkv`), WebM, MP4 or QuickTime (`.mp4`, `.mov`) | **Container** |
| One of H.264, HEVC, AV1, VP9, MPEG-2, Xvid, VC-1 | **Video codec** |
| Each audio language, cut to its base code (`en-US` counts as `en`) | **Audio languages** |
| Count of audio tracks | **Audio tracks** |
| A track with no language, `und` or a bad value | **Unknown audio** |
| How far reading got | **Status**: complete, partial or unsupported |

Audio languages also decide whether a file meets a Version. See [Original and dubbed audio](audio-versions.md).

Inspection does not play or decode the file, so it cannot prove every frame works. It uses no network and runs no other programs. Music files are not inspected.

## If it does not work

**Symptom:** A copy cannot be ticked.
**Cause:** It is protected. See [Protected copies](protected-copies.md).

**Symptom:** Codec shows **Unknown**.
**Cause:** Another container, a codec outside the list, or an unreadable file.

**Symptom:** A file is reported as damaged.
**Cause:** Its track details are cut short or do not add up.
**Fix:** Get a fresh copy.

**Symptom:** **Review** refuses a copy.
**Cause:** The file moved, is missing, is not a main movie file, left that Version, or is protected.

**Symptom:** The search says it is out of date.
**Cause:** Something changed after you reviewed.
**Fix:** Select **Review** again.

## For experts

<details class="ha-more"><summary>API</summary>

These routes need the manage Library permission:

| Route | Does |
|---|---|
| `GET /api/v1/library/copies?q=&codec=&sort=&limit=&offset=` | Lists copies |
| `POST /api/v1/library/copies/search/preview` | Reviews a selection, returns `review_token` |
| `POST /api/v1/library/copies/search` | Runs the search |

Preview body:

```json
{"selections":[{"file_id":12,"version_id":7}]}
```

Send the same selections plus `review_token` to the search route. A change since review returns `409 Conflict`.

</details>

## Related

- [Protected copies](protected-copies.md)
- [Original and dubbed audio](audio-versions.md)
