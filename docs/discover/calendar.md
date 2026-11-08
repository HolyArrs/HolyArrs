---
title: Calendar
description: See when movies, shows and music in your Library come out, and follow titles.
level: Start
---

See release dates for the movies, shows and music in your Library.

Open **Calendar** in the menu (`http://your-server:6262/calendar`).

## See what is coming

1. Pick **Movies**, **TV shows**, **Music** or **All** at the top.
2. Move through the weeks with the arrows. Press **Today** to come back.
3. Press a title to open its page.

To jump to a day, press **Pick a day**. To link a month, add `?month=YYYY-MM`, for example `?month=2026-03`.

## Follow a title

Press **Follow** on its row. Press **Unfollow** to stop. Asking for a title follows it for you.

## Choices

### Which dates

| Option | Shows |
|---|---|
| **My follows** | What you follow |
| **Everyone's follows** | What anyone on the server follows, with who. Only when the server has more than one person |
| **Whole Library** | Every date in the Library |

If you follow nothing yet, the calendar opens on **Whole Library**.

### Layout

- **Cards** or **List** sets how each day is drawn. The pick is kept in the address, so a shared link opens the same way.
- **Show month** and **Hide month** toggle the small month at the side.
- A movie shows one date: its home release, or its cinema date if that is all there is.
- Each row's dot means **In the Library**, **Missing** or **Coming up**.

## If it does not work

**Symptom:** "Nothing coming up."
**Cause:** your Library has no dates yet. **Fix:** add a movie, show or artist.

**Symptom:** "Nothing you follow here."
**Fix:** press **Show the Whole Library**, or follow more titles.

**Symptom:** "Calendar could not be loaded."
**Fix:** reload. If it repeats, open **Health** (`http://your-server:6262/health`).

## Related

- [Explore](explore.md)
- [Ask for a title](../requests/asking.md)
