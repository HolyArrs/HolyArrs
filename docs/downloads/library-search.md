---
title: Library search
description: Find a movie, show or artist you already added, using the search box on the Library page.
level: Start
sidebar: { order: 3 }
---

Find a movie, show or artist you already added. To find something new, use the **Search** page.

## Search the Library

1. Open **Library**.
2. Type in the search box. It says **Search library**, or **Search movies**, **Search TV shows** or **Search music** when you picked a kind.

   You should see the list narrow as you type.
3. Clear the box to see everything again.

The kind and state buttons still apply. Pick **Shows** and **Missing**, then type part of a name.

Searching never adds a title, starts a download or changes a file. Removed titles never show.

## How matching works

| You type | It finds | Rule |
|---|---|---|
| `amelie` | Amélie | Accents are optional |
| `star` | Stargate | The start of a word matches |
| `star 1994` | Stargate (1994) | Every word must match. Years count |

It matches names, original names, other names from metadata, and years. `OR` and `*` are plain text.

## If it does not work

**Symptom:** Nothing shows.
**Cause:** The kind or state buttons hide the title.
**Fix:** Check the buttons, then try fewer words.

**Symptom:** An error shows on the page.
**Cause:** The search is over 256 characters or 16 words.
**Fix:** Shorten it.

## For experts

<details class="ha-more"><summary>API and limits</summary>

| What | Detail |
|---|---|
| Request | `GET /api/v1/titles?q=amelie` uses the same rules |
| Empty `q` | Lists the whole Library |
| Limits | 256 characters, 16 words |

</details>

## Related

- [Sources](../sources/)
- [History](../history/)
