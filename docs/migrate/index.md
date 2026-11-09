---
title: Move from other apps
description: Copy your setup from Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent or SABnzbd, then undo it if you want.
level: Start
sidebar: { order: 1 }
---

Bring your old apps' setup into HolyArrs without typing it again.

HolyArrs only reads the old apps. Nothing in them changes, and your media files never move.

## Before you start

- Sign in as the owner or an admin.
- Have each old app's address and API key ready.
- Make sure HolyArrs can see your media folders. See [Folders and hardlinks](../start/folders.md).

## Move your setup

1. Open **Settings > Move from other apps**. You should see five steps in a row at the top.
2. **Read:** click **Add an app** for each old app. You should see each app listed with a switch.
3. **Choose:** turn on the groups you want to bring over.
4. **Try it:** check what will be added. Nothing changes yet.
5. **Move:** click **Move** and confirm. You can leave the page while it runs.
6. **Done:** you should see the results per group and **Undo this move**.

Keys and passwords never come over. See [After the move](#after-the-move).

## Order to do it

| Order | App | Why this order |
| --- | --- | --- |
| 1 | Radarr, Sonarr | Library folders and titles come first. See [Sonarr and Radarr](sonarr-radarr.md). |
| 2 | Lidarr, Prowlarr, Bazarr | Music, Sources and subtitle languages. See [Lidarr, Bazarr and Prowlarr](lidarr-bazarr-prowlarr.md). |
| 3 | qBittorrent, SABnzbd | Downloaders. Read them in the same move as the apps above. |
| 4 | Seerr, Overseerr | People and requests, matched to titles already here. See [Request apps](requests-apps.md). |

You can read every app and move them all at once. The order matters only if you move in batches.

## After the move

Each item below comes over paused or off. Type its secret, check it, then turn it on.

| Item | Where | What to do |
| --- | --- | --- |
| Sources | **Settings > Sources** | Type the key, then resume. |
| Downloaders | **Settings > Downloaders** | Type the password or key, then resume. |
| Alerts | **Settings > Alerts** | Type the link or mail password, then turn on. |
| Outside lists | **Settings > Outside lists** | Check folder and Pick rules, then turn on. A Trakt list needs your Trakt key first. |
| People | **Settings > Users and roles** | Send invites. |

A Source, Downloader or Alert that had tags in the old app now serves every title of its kind. Check it before you turn it on.

Sources and Downloaders arrive with **May reach the local network** off. Turn it on only for one on your home network. Link-local and cloud metadata addresses stay blocked either way.

To type many keys at once, see [Import keys from a file](import-secrets.md).

## What stays behind

- **Media files.** The move never moves, renames, copies or deletes a file.
- **Keys, passwords, tokens and cookies.** The Move step and the report list each under **Keys to type again**.
- **Settings with no match here.** Each one is on the **not moved** list with its reason.
- **History.** The old apps' history is not read. The person who asked for a title does come over.

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| Titles show as not found | Their folder does not match a Library folder here. | Add a path rule under **Advanced** in **Choose**, then move again. |
| Move says the plan changed | Something changed since the test. | Run **Try it** again, then move. |
| The move button is greyed out | A person needs a choice, or HolyArrs is in safe mode. | Read the text next to the button. |
| Some items failed | For example, the metadata server was down. | Wait, then move again. Your old-app keys are kept. |

## For experts

<details class="ha-more"><summary>Move again, undo, parts and safe mode</summary>

**Move again.** A move remembers what it added. Moving again adds only what is new and says **already here** for the rest. One move or undo runs at a time. If HolyArrs stops mid-move, the move is marked failed at the next start, or resumes if its job is still queued. It never blocks the next move.

**Earlier moves**, under the reads, lists every move and its state. **Open** shows a move at step 5, or step 4 while it runs.

**Undo.** Click **Undo this move** at step 5 (for an older move, **Open** it first). The undo runs in the background and removes only what that move added, newest first:

- titles leave the Library; their files stay where they are,
- Sources, Downloaders, Alerts, subtitle sites, people, requests and Never-add entries it added are removed,
- Outside lists it added are removed while still paused,
- naming, default subtitle languages and waits on Pick rules go back to what they were.

What you set up or changed after the move stays, and the undo report lists it: a secret you typed, a person or Outside list you turned on, naming, languages or waits you changed. Library folders that held titles stay, because the removed titles keep their History.

**Parts.** A part that is off is left out. Movies off: no movie folder, movie or Outside list. Subtitles off: no subtitle setting. Requests off: no requests.

**Safe mode.** You can read, choose and try, but not move or undo.

</details>

<details class="ha-more"><summary>API</summary>

API keys need the **admin** scope.

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/plan` | The plan for the picked reads, with counts and suggested path rules. |
| `POST /api/v1/migrate/dry-run` | The test report and its `plan_hash`. Nothing changes. A person who needs a choice has `choose` set to `same_name`, `numeric_id` or `merge_gone`. |
| `POST /api/v1/migrate/moves` | Starts a move. Needs the test's `plan_hash`. Answers `409` when the plan changed. |
| `GET /api/v1/migrate/moves` | Lists moves, newest first. |
| `GET /api/v1/migrate/moves/{id}` | One move, its report and progress. |
| `GET /api/v1/migrate/moves/{id}/report` | The report as a Markdown file. |
| `POST /api/v1/migrate/moves/{id}/undo` | Queues the undo (`202`). Follow it with `GET /api/v1/migrate/moves/{id}`. |

Body of plan, dry-run and moves (moves also take `"plan_hash"`):

```json
{
  "snapshots": [1, 2],
  "path_rules": [{"from": "/data/media/movies", "to": "/movies"}],
  "skip": ["alerts"],
  "people": {"person:seerr:1": "owner", "person:seerr:3": "admin", "person:seerr:2": "merge"}
}
```

A person choice is `owner` (old app's owner only), `admin`, `merge` or `skip`. Groups you can skip: `folders`, `pick_rules`, `titles`, `never_add`, `outside_lists`, `sources`, `downloaders`, `naming`, `subtitles`, `people`, `wishlist`, `alerts`.

</details>

## Pages in this section

| Page | Use it to |
| --- | --- |
| [Sonarr and Radarr](sonarr-radarr.md) | Bring over folders, shows, movies and their rules. Do this first. |
| [Lidarr, Bazarr and Prowlarr](lidarr-bazarr-prowlarr.md) | Bring over music, subtitle languages and Sources. |
| [Seerr and Overseerr](requests-apps.md) | Bring over people and open requests. |
| [Read your old apps](what-is-read.md) | See exactly what HolyArrs reads and keeps. |
| [Import keys from a file](import-secrets.md) | Type many keys at once. |

## Related

- [First run](../start/first-run.md)
- [Glossary](../help/glossary.md)
