---
title: Find a title in the Library
description: Search the movies, shows and artists you already added, with the matching rules and limits.
level: Setup
---

Find a movie, show or artist you already added. To find something new, use the **Search** page.

## Search the Library

1. Open **Library**.
2. Type in the search box (**Search library**, or **Search movies**, **Search TV shows**, **Search music** when a kind is picked). The list updates as you type.

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
**Fix:** Check the kind and state buttons, then try fewer words.

**Symptom:** An error shows on the page.
**Cause:** The search is over 256 characters or 16 words.
**Fix:** Shorten it.

## For experts

<details class="ha-more"><summary>API</summary>

`GET /api/v1/titles?q=amelie` uses the same rules. An empty `q` lists the whole Library. Limits: 256 characters, 16 words.

</details>

## Related

- [Sources](sources.md)
- [History](history.md)
