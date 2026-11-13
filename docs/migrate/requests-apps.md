---
title: Seerr and Overseerr
description: Import people, permissions and open requests from Seerr or Overseerr into HolyArrs.
level: Setup
sidebar: { order: 4 }
---

Bring over the people who ask for titles, their limits and their open requests, so nobody asks twice.

## Before you start

- Import Radarr and Sonarr first, or in the same import. Requests point at titles. See [Sonarr and Radarr](sonarr-radarr.md).
- Have Seerr's or Overseerr's address and API key, or its config folder.

## Import people and requests

1. In **Settings > Import from other apps**, click **Add an app** and pick **Seerr** or **Overseerr**.
2. Read it. You should see a count of people and requests.
3. In **What to import**, turn on **This is me** if the Seerr owner is you. Turn on **Keep as admin** for each admin who should stay one.
4. Go to **Import**. HolyArrs checks what will be added. Answer any row under **Needs a choice** (see [People with the same name](#people-with-the-same-name)).
5. Click **Import**. Then invite each person from **Settings > Users and roles**. You should see them listed, turned off until invited.

## What comes over

| From Seerr or Overseerr | In HolyArrs |
| --- | --- |
| People | People, turned off until you invite them, as **members** with matching permissions and their weekly request limit. Passwords stay behind. |
| Admins | Members, unless you turn on **Keep as admin**. |
| The owner | You, only if you turn on **This is me**. |
| Waiting or approved requests | **Requests**, with the right person. They do not count against anyone's weekly limit. |

An email that is not one plain address is left out. An approved request whose title did not come over waits for approval again.

Stays behind: request tags, avatars and finished or turned-down requests (only counted).

## Choices

### People with the same name

When someone here already has the name of a person from Seerr, the import never joins them by itself. The **Import** step lists that person under **Needs a choice**:

| Choice | Result |
| --- | --- |
| **Same person** | Their requests go to the person here. |
| **Leave out** | Their requests come over as yours. |

The same choice shows when HolyArrs cannot tell if a person is one an earlier import brought, and when the person you picked is gone. Your choice reruns the check right away. The row stays, so you can change it. Until you choose, that person stays behind; **Done** says how many still need a choice.

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| The Import button is greyed out | A person still needs a choice. | Answer every row under **Needs a choice**. |
| A request waits for approval again | Its title did not come over. | Import the Radarr or Sonarr that holds it, then approve. |
| A person is missing | Their email was not one plain address. | Invite them by hand. |

## For experts

<details class="ha-more"><summary>API person choices</summary>

In the `people` map of `POST /api/v1/migrate/plan`, `dry-run` and `moves`, each person takes `owner` (the old app's owner only), `admin`, `merge` or `skip`. A person who needs a choice has `choose` set to `same_name`, `numeric_id` or `merge_gone` in the dry-run report. See the API block on [Import from other apps](index.md#for-experts).

</details>

## Related

- [Import from other apps](index.md)
- [Read your old apps](what-is-read.md)
