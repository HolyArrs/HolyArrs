---
title: Explore
description: Browse rows built from your Library and asks, and find titles to ask for or add.
level: Start
---

Find titles to ask for or add, in rows built from your own Library and asks.

Open **Explore** in the menu (`http://your-server:6262/explore`).

## Find something to ask for

1. Pick **All**, **Movies**, **Shows** or **Music** at the top.
2. Scroll. More rows load as you go.
3. Press a card to open the title, then press **Ask** or add it.

**Music** shows only when the Music part is on. Your type pick is kept in the address (`?type=`) and on this device.

Explore plays nothing and tracks no watching. It reads your Library, your asks and charts from the [metadata server](../library/metadata.md).

## Choices

### Rows that need something first

| Row | Shows when |
|---|---|
| **Top picks for you** | You have 3 or more titles in your Library or asks |
| **Because you asked for** | You have an open ask |
| **Popular in your household** | 2 or more people on the server asked for things lately |

A new server with an empty Library still gets a full page: charts and popular genre and decade rows.

### Other rows

| Row | Holds |
|---|---|
| **On your services** | Titles on the streaming services you pick in the row header, kept on this device |
| **Finish the collection** | Movies missing from a collection you started |
| **Returning soon** | Library shows with a new episode in the next 2 months |
| **Browse by genre** | Genre cards, your household's top genres first |

Hidden gems, decade rows and rows about people in your favourite titles fill the rest. **Music** has its own rows, such as new albums from artists you have, trending artists and top albums. Music rows are still being built.

### Cards

- A strip can say **New Episode**, **New Season**, **Recently Added**, **Trending**, **Coming** or **New Release**.
- A round mark at the top right says if the title is in the Library, partly there, downloading, asked for or turned down.
- Under **All**, a label says Movie, Show or Music.

On a computer, rest the pointer on a card or focus it with the keyboard: it grows and plays the trailer. Esc closes it. On a phone, a tap opens the title. With Reduce motion on, cards do not grow. See [Trailers](../library/trailers.md).

### Who asked and new episodes

A title page can say "You asked" or "Asked by Ana and 2 others". Press the line to see everyone and when. Cards never show names.

If you asked for a show, you get a notice when new episodes arrive, at most once an hour per show. Turn it off under **My account**. The owner can also send it through [Alerts](../users/alerts.md).

### Kid accounts

Kid accounts see fewer rows, each checked against their age limit. A title with no rating for your region is hidden. See [Kid accounts](../users/kids.md).

## If it does not work

**Symptom:** the page has few rows.
**Cause:** HolyArrs knows little about you yet. **Fix:** add or ask for a few titles.

**Symptom:** a row from yesterday is gone.
**Cause:** rows show only when they have enough titles. It comes back when it fills.

**Symptom:** **Music** is missing.
**Fix:** turn the Music part on in Settings.

**Symptom:** cards show only names.
**Cause:** the metadata server could not match the title. It still counts in the row.

## For experts

<details class="ha-more"><summary>How the page is built</summary>

- Each person gets their own rows and order.
- The first rows load fast and show each title once: the charts, **Top picks for you**, **Because you have**, **Best new releases**, **Because you asked for** and **Popular in your household**.
- The order changes a little each day and stays fixed all day.
- Lower rows can get a short title that fits the moment, such as the time of day or a holiday, only when it is true for the row. Kid accounts get kid titles only. The first rows keep plain titles.

| Your titles and asks | Rows at most |
|---|---|
| Fewer than 3 | 8 |
| 3 to 9 | 20 |
| 10 or more | 40 |

</details>

## Building

The owner table of row title performance is not in this release.

## Related

- [New & Popular and People](popular-people.md)
- [Calendar](calendar.md)
- [Ask for a title](../requests/asking.md)
