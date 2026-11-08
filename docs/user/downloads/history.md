---
title: History
description: See every step HolyArrs took for a title, from the ask to the media server, and set how long it is kept.
level: Start
---

See what HolyArrs did and why, newest first.

History lists adding a title, searches, the chosen Release, the send to a Downloader, the download, the import, renames and deletes.

## Open History

1. Open **Downloads**.
2. Open **History**. You should see the **Live** dot lit: new rows appear as they happen.

## Find a step

- **Search**: type a title, a person or a Source.
- **Filter**: narrow by **Type**, **Step**, **Who**, **Source** and **When**. For **When**, pick **Custom** to set **From** and **To**.
- **Clear filters** shows everything again.

Search and filters live in the page address, so Back works and you can bookmark a filtered view.

## From a title's page

Each title page has a History card with its latest rows. **All rows** opens History for that title only.

Example: to see why a movie was grabbed last night, open the movie, press **All rows**, and read the rows for the chosen Release and the send.

## How long rows are kept

Set this in **Settings > This app**, on the **History** card.

| Setting | Covers | Default |
|---|---|---|
| **Keep routine Events** | Searches, feed checks, refreshes, scans | 30 days |
| **Keep other Events** | Downloads, imports, better copies, problems | Forever |

Choices: Forever, 7, 14, 30, 90 or 180 days, or 1 year. Sign-ins and settings changes are kept 1 year whatever you pick.

## If it does not work

**Symptom:** A step you expected is missing.
**Cause:** Routine checks that changed nothing are not listed on the page.
**Fix:** Clear the filters and search for the title.

**Symptom:** The list does not update.
**Fix:** Check the **Live** dot. Grey means HolyArrs is reconnecting. If it stays grey, reload the page.

## For experts

<details class="ha-more"><summary>API and cleanup</summary>

`GET /api/v1/events` returns the same rows. The full filter list is in `http://your-server:6262/api/v1/openapi.json`. A daily job removes rows past their keep time in small batches, so the database is never locked for long.

</details>

## Related

- [Downloaders](downloaders.md)
- [Sources](sources.md)
