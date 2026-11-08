---
title: Old-app API limits
description: What Seerr, Bazarr, Prowlarr and other old-app clients can't do through HolyArrs, and how to fix their errors.
level: Expert
sidebar: { order: 3 }
---

What old-app clients can't do through HolyArrs, and the fix for each common error.

## Supported apps

Seerr, Overseerr, Bazarr, Unpackerr, Prowlarr and the Homepage Sonarr widget. Recyclarr, Maintainerr, Cleanuparr and other apps that talk to Sonarr or Radarr are not supported.

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| Every call answers 404 | The feature is off, or the address is wrong | Switch on **Answer Seerr and Overseerr** in **Settings > API**. Copy the address from the card, URL base included. |
| 401 | Key wrong or replaced | Make a new key and paste it into the app. |
| 403 | The key's owner is turned off or can't manage the Library. Prowlarr also needs permission to change settings. | Make a new key as someone with both permissions. |
| A `hak_` key is refused | HolyArrs API keys don't work here | Use the keys from the **Requests from Seerr and Overseerr** card. |
| Files stay after removing in Seerr | **Let these apps delete files** is off | Turn it on if you want removal to delete. |
| A Source you deleted by hand comes back wrong in Prowlarr | Prowlarr still knows the old one | Run **Sync App Indexers** in Prowlarr. |
| Bazarr logs connection errors | HolyArrs has no live feed | Turn Bazarr's live feed off. Its scheduled sync still works. |

## For experts

<details class="ha-more"><summary>What these apps can't do</summary>

| Area | Behaviour |
|---|---|
| Versions | Apps see one copy per title, the main Version. A 4K Version beside it is invisible. |
| Library folder | Choosing another folder for a title is ignored. Move titles in HolyArrs. |
| Quality | Choosing other pick rules switches the main Version's pick rules, which can start a search for a better copy. |
| Show type | Changing the series type of a show already in the Library is ignored. |
| Settings calls | Pick rules, naming, notifications and backups are not answered. Indexers are answered only the way Prowlarr writes them, one at a time. |
| Unanswered calls | Return an error. HolyArrs never pretends a call worked. |
| Tags | Not attached to titles, so lists filtered by tag are not narrowed. |
| Languages | Carry only their name. No known original language reads "Unknown". |
| Version number | Status reports `4.0.99.0` on the Sonarr side and `5.0.99.0` on the Radarr side, because clients choose their calls from it. |

</details>

## Related

- [Old-app API](../reference/old-app-api/)
- [Old-app API clients](../reference/old-app-api-clients/)
