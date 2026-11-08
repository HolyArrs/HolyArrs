---
title: First title
description: Add one movie and watch HolyArrs search, download and import it into your Library.
level: Start
sidebar: { order: 5, label: "Step 5 of 5: First title" }
---

Add one movie and follow it from search to a file in your Library.

## Before you start

- [Connect](../connect/) done: a downloader showing **Working** and at least one source.
- A movies Library folder from [First run](../first-run/).

## Add a movie

1. Open **Search** and type the movie's name.
2. Open the movie, then press **Add**.
3. In the box, check the **Library folder** and **Pick rules**. Pick rules decide which releases count as good enough. The defaults are fine to start.
4. Leave **Search when added** ticked and press **Add to Library**.

   You should see the movie's Library page.

## Watch it happen

1. HolyArrs searches every source and picks the best release that passes the Pick rules.
2. Open **Downloads**. The movie moves through the tabs: **Queued**, **Downloading**, then **Importing**.
3. When the row leaves **Importing**, the file is in your movies folder, renamed.

   You should see the file in your Library folder and on the movie's page.

:::tip
A first download can take minutes to hours, depending on the release and your connection. A row with no progress for 6 hours shows as stalled; after 24 hours HolyArrs marks it failed and tries the next release.
:::

## Choices

### Pick a release yourself

On the movie, press **Choose a release**. HolyArrs lists every result with a label (**Would be chosen**, **Passed over**, **Turned away by the rules**, **Turned down**) and picks nothing. Press **Choose** on the one you want.

### Search later

Untick **Search when added** to add now and search later with **Search now** on the movie. Every day HolyArrs also searches for up to 20 missing titles by itself.

## If it does not work

| You see | Cause | Fix |
|---|---|---|
| Nothing is found | No source has the movie, or none passes the Pick rules | Press **Choose a release** to see why each result was turned away |
| Row stays in **Queued** | Downloader is busy or paused | Check the downloader's own queue |
| Row is in **Review** | HolyArrs could not finish the import | Open the row and choose what to do |
| Row is **Paused** with "The Downloader can't use the files" | A share mounted late | Press **Check again** on the row once the share is back |
| Import is slow and doubles disk use | Files are copied, not hardlinked | Use the layout in [Folders](../folders/) |

## For experts

<details class="ha-more"><summary>What runs by itself</summary>

| Job | When | What |
|---|---|---|
| Feed check | Every 15 min | Reads the newest releases from each source with its feed on and judges them against wanted titles |
| Nightly search | Once a day | Up to 20 missing titles, longest unsearched first; skips unreleased titles and ones with **Keep up to date** off |
| Better copies | Ongoing | A copy below **Good enough** keeps looking and is replaced when a better release appears |

**Mark failed** on a download turns that release down for the title, removes it from the downloader with its files, and searches again. A search started from the movie page skips the wait set in the Pick rules.

</details>

## Related

- [Find releases for movies](../../movies/search/)
- [Movie downloads](../../movies/downloads/)
- [Home and Settings](../home-and-settings/)
