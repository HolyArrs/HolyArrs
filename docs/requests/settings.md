---
title: Weekly limits and Request rules
description: Set how much each person can ask for and where approved titles go.
level: Setup
---

Limit how much people ask for and set where approved titles go. Only the owner and admins change these.

Open **Settings > Request rules** (`http://your-server:6262/settings/requests`).

## Set weekly limits

1. Under **Weekly limits**, set the number for movies, shows and music.
2. Save.

Each limit covers a rolling 7 days and counts one kind only. Approvers have no limit.

| Limit | Default | Range |
|---|---|---|
| Movies | 10 | 0 to 1000, 0 = no limit |
| Shows | 10 | 0 to 1000, 0 = no limit |
| Music | 10 | 0 to 1000, 0 = no limit |

## Choices

### One person's own limit

Open **Settings > Users and roles**, change the person and set their weekly asks. An empty box uses the default.

### Where approved titles go

These apply when an approver leaves folder or quality on **Default**.

| Setting | What it does | Default |
|---|---|---|
| **Movie folder**, **Movie quality** | Folder and quality rules for approved movies | First movie Library folder, and that folder's quality |
| **Show folder**, **Show quality** | The same for shows | First show Library folder, and that folder's quality |
| **Music folder**, **Music quality** | The same for artists and albums | First music Library folder, and that folder's quality |
| **Search when approved** | Search as soon as a request is approved. Movies at once, shows at the next feed check | On |

## If it does not work

**Symptom:** saving fails on a limit.
**Cause:** the value is outside 0 to 1000.

**Symptom:** someone still hits a limit after you raised it.
**Cause:** they have their own limit. **Fix:** clear it in **Settings > Users and roles**.

## For experts

<details class="ha-more"><summary>How asks count</summary>

| Ask | Counts |
|---|---|
| A movie | 1 |
| A show, any number of seasons | 1 |
| An artist or album request, any number of albums | 1 |
| More seasons or albums on a waiting request | 1 more |
| Joining someone else's request | 0 |
| A declined request | 0 |

</details>

## Plex watchlists

With **Request from Plex watchlists** on, HolyArrs reads the Plex watchlist of people who signed in with Plex every few minutes and asks for what is new on it, as that person. Their weekly limits, auto-approve and age-rating limits apply like any other request. Each entry is acted on once, and taking a title off the watchlist changes nothing.

By default each person turns it on for themselves in **My account**, for movies, shows or both.

Turn on **Turn on for everyone who signs in with Plex** to read everyone's watchlist without asking them first. People who signed in another way are left out. Anyone can still turn it off for themselves in **My account**, and HolyArrs keeps their choice.

## Related

- [Approve or decline](approving.md)
- [Ask for a title](asking.md)
