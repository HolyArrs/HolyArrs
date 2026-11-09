---
title: History
description: See every step HolyArrs took for a title, from the ask to the media server, and set how long rows are kept.
level: Start
sidebar: { order: 4 }
---

See what HolyArrs did and why, newest first.

History lists adding a title, searches, the chosen Release, the send to a Downloader, the download, the import, renames and deletes.

## Open History

1. Open **Downloads**.
2. Open **History**.

   You should see the **Live** dot lit. New rows appear as they happen.

## Find a step

1. Type a title, a person or a Source in **Search**.
2. Narrow the list with the filters: **Type**, **Step**, **Who**, **Source** and **When**. For **When**, pick **Custom** to set **From** and **To**.
3. Press **Clear filters** to see everything again.

Search and filters live in the page address. Back works, and you can bookmark a filtered view.

### From a title's page

Each title page has a History card with its latest rows. **All … rows** (it shows the count, like **All 12 rows**) opens History for that title only. The title menu also has **All history**.

Example: to see why a movie was grabbed last night, open the movie, press **All … rows**, and read the rows for the chosen Release and the send.

## How long rows are kept

Open **Settings > This app** and find the **History** card.

| Setting | Covers | Default |
|---|---|---|
| **Keep routine Events** | Searches, feed checks, refreshes, scans | 30 days |
| **Keep other Events** | Downloads, imports, better copies, problems | Forever |

Choices: Forever, 7, 14, 30, 90 or 180 days, or 1 year. Sign-ins and settings changes are kept 1 year whatever you pick.

Rows past their keep time are gone for good. Keep **Keep other Events** on Forever if you want a record of every import.

## If it does not work

**Symptom:** A step you expected is missing.
**Cause:** Routine checks that changed nothing are not listed on the page.
**Fix:** Clear the filters and search for the title.

**Symptom:** The list does not update.
**Cause:** The **Live** dot is grey, so HolyArrs is reconnecting.
**Fix:** Wait a few seconds. If it stays grey, reload the page.

## For experts

<details class="ha-more"><summary>API and cleanup</summary>

| What | Detail |
|---|---|
| Rows | `GET /api/v1/events` returns the same rows as the page |
| Filters | Full list in `http://your-server:6262/api/v1/openapi.json` |
| Cleanup | A daily job removes rows past their keep time in small batches, so the database is never locked for long |
| Sign-ins and settings changes | Always kept 1 year |

</details>

## Related

- [Downloaders](../downloaders/)
- [Sources](../sources/)
- [Troubleshooting](../../help/troubleshooting/)
