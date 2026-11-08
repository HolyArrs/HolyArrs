# History

You want to know what HolyArrs did and why. History lists every step from the request to your media server, newest first. That covers adding a title, searches, the chosen Release, sending it to a Downloader, the download, the import, renames and deletes. Routine background work is left out.

## Open History

1. Open **Downloads**.
2. Open **History**.

The **Live** dot shows that new rows appear as they happen. When it is grey, HolyArrs is reconnecting.

## Find what you need

- **Search**: type a title, a person or a Source.
- **Filter**: narrow the list by **Type**, **Step**, **Who**, **Source** and **When**. For **When**, pick **Custom** to set **From** and **To** dates.
- **Clear filters** shows everything again.

The search and filters are part of the page address. Back works as you expect, and you can bookmark or share a filtered view.

## From a title's page

Each title's page has a History card with its latest rows. **All rows** opens History for just that title.

Example: to see why a movie was grabbed last night, open the movie, press **All rows**, and look at the rows for the chosen Release and the send to the Downloader.

## Troubleshooting

**A step I expected is missing.** Routine work, such as regular checks that changed nothing, is not listed. Clear the filters and search for the title.

**The list does not update.** Check the **Live** dot. If it stays grey, reload the page.

See also [Downloaders](downloaders.md).

For your own tools, `GET /api/v1/events` returns the same rows. The full list of filters is in `http://your-server:6262/api/v1/openapi.json`.
