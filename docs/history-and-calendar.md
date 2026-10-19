# History and the calendar

## History

Everything HolyArrs does leaves a row in History: adding a movie, each search and what it found, holding and choosing a Release, sending it to a Downloader, the download's progress and failure, the import and where each file went, a better copy replacing an old one, renames, deletes, the daily scan of the Library, health checks and every movie that needs a decision. Rows keep their reason and their paths.

Open **Downloads**, then **History**. Newest rows come first, 50 to a page (**Newer** and **Older**).

- **Movie**: only one movie's rows.
- **From** and **To**: a date range (both days included).
- **What happened**: tick one or more kinds, such as **Sent to download**, **Imported**, **Renamed** or **Health**.
- **Why and where** on a row shows its reasons and paths: why a search chose a Release, which Downloader got it and where it downloaded, where an import placed each file.

The filters are part of the page address, so you can bookmark or share a filtered view. **Show everything** clears them.

Each movie's page has a **History** card with its latest rows; **All rows** opens History filtered to that movie. **Home** shows the latest rows and links to History.

Example: to see why a movie was grabbed last night, open the movie, then **All rows** in its History card, and look at the **Chosen** and **Sent to download** rows with **Why and where**.

For your own tools, `GET /api/v1/events` takes the same filters: `title`, `kind` (a comma list; `import_*` is every import outcome), `from`, `to`, `limit` and `offset`.

## Calendar

**Calendar** shows the cinema, digital and disc dates of the movies in your Library.

- **Month** shows the month as a grid, Monday first; today is underlined. On a phone each date is a coloured dot; tap it to open the movie.
- **List** shows the month's dates day by day.
- The arrows move a month back or ahead; **Today** comes back to this month. A month can be linked directly, such as `/calendar?month=2026-10`.
- **In cinemas**, **Digital release** and **Disc release** turn each kind of date on or off.

Each entry of `GET /api/v1/calendar` carries the title's pictures when it has
them: `poster` (a link such as `/api/v1/artwork/<name>`; for an album, the
artist's picture), `backdrop` (the wide picture) and, for an episode, `still`
when known. Fields without a picture are left out.

### Phone calendar

Your phone's calendar app can show the same dates through a feed link.

1. In **Phone calendar** at the bottom of the Calendar page, press **Make a feed link**.
2. Copy the link straight away, or press **Open in a calendar app**. HolyArrs shows it only this once: it keeps only a fingerprint of its key.
3. Add it to your phone as a subscribed calendar (on an iPhone: Settings, Calendar, Accounts, Add Account, Other, Add Subscribed Calendar; on Android: add it in Google Calendar on the web, From URL).

The link has its own key. It can only read this calendar; it cannot sign in or do anything else. The feed holds the last 7 days and the next 28, one all-day event per date. Adding `&future_days=90`, `&past_days=0` or `&kinds=cinema,digital` to the link changes that.

**Make a new link** ends the old one on every phone that uses it. **Turn the feed off** ends every link. Only people with the settings permission can make or end links.

The phone must reach HolyArrs at the address in the link: on your home network, or over your VPN when away.
