# Move from other apps

You want to bring your setup over from Radarr, Sonarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent or SABnzbd, without setting everything up again. HolyArrs reads each app, shows you what will come over, tests it, then moves it. You can undo a move later.

Nothing in the old apps changes. HolyArrs only reads them, and after the read it never talks to them again. Your media files never move.

## Before you start

- You need to be the owner or an admin, signed in with your password.
- Have the address and API key of each old app ready, or its backup.
- Make sure HolyArrs can see your media folders.

## Move your setup

Open **Settings > Move from other apps** (at `http://your-server:6262/settings/move`). The move has five steps. They sit in a row at the top. Click a step you finished to go back to it. The browser's Back button works too. Each step has one main button at the bottom. When it is waiting, the text next to it tells you why.

1. **Read.** Click **Add an app** for each old app. See [Read your old apps](move-reading.md). Each read has a switch. The newest read of each app starts on. A read an earlier move already used starts off.
2. **Choose.** Pick what comes over, group by group. Each group has a switch and **Show** for its items. Old folders that need a place here get a row each (see [Folders](#folders)). Seerr's owner and admins get their own switches.
3. **Try it.** HolyArrs runs a test against this server. It shows, group by group, what will be added, used, updated, left out, not found or already here. Nothing changes.
4. **Move.** Click **Move** and confirm. The move runs in the background and shows its progress. You can leave the page. It runs exactly what the test showed. If you change anything, the test runs again first.
5. **Done.** You see what to do next, the results per group, the report as a file to download, and **Undo this move**.

**Earlier moves**, under the reads, lists every move and its state. **Open** shows a move at step 5, or step 4 while it still runs.

## Folders

Old apps often run in other containers and see your folders under other names. In **Choose**, each old folder that is not a Library folder here gets a row. Pick the Library folder it matches, or pick **Other path…** and type one.

When an old folder is not on this server but a Library folder with the same last name is, HolyArrs fills it in and marks it **Suggested**.

Under **Advanced** you can add your own path rules. Say Radarr kept movies in `/data/media/movies` and HolyArrs sees the same folder as `/movies`. Add the rule `/data/media/movies` to `/movies`. The longest matching rule wins.

Titles whose folder is not found are listed with the reason. Fix the rule and move again. Nothing is added twice.

## What comes over

| From | In HolyArrs |
| --- | --- |
| Library folders (Radarr, Sonarr, Lidarr) | Library folders for movies, shows and music. |
| Movies, shows and artists | Titles, matched by TMDB, TVDB or MusicBrainz id. Each title keeps its folder and files where they are. A watched title gets **Keep up to date**. Seasons that were not watched stay off. Nothing is searched for during the move. |
| Quality profiles | Each one is matched to the closest **Pick rules** preset (Standard, 4K, Lossless first, Lossy 320 or V0, Any music). The plan shows why. Titles use the preset of their profile. |
| Exclusions | The **Never-add** list. |
| Radarr's release stage per movie, and its availability delay | Each moved movie's **Release gate** and gate delay, up to 365 days. Your default for new movies stays as it is. |
| The untagged default wait rule (Radarr, Sonarr, Lidarr) | The preferred protocol and the Usenet and torrent waits on the Pick rules its profiles matched, up to 7 days. See [Waits](#waits). |
| Radarr TMDB lists, Trakt lists and Trakt watchlists | **Outside lists**, always added paused, with their folder, Pick rules, Keep up to date, search on add, Release gate and how often they are read (6 to 168 hours). A Trakt list needs your Trakt key and must be public. Trakt lists are held back in this version, so they stay off until that feature is switched on. |
| Torznab, Newznab and RSS indexers (Radarr, Sonarr, Lidarr, Prowlarr) | **Sources**, always added paused and blocked from your home network. |
| qBittorrent, SABnzbd, and the Transmission, Deluge and NZBGet clients set up in the arr apps | **Downloaders**, always added paused and blocked from your home network. They use HolyArrs's own `holyarrs` category, so they never pick up the old apps' downloads. |
| Remote path mappings (Radarr, Sonarr, Lidarr) | **Different folder names** on the Downloader at that host, up to 100 pairs. A Downloader that was already here is left alone, and its pairs are listed for you to add by hand. A pair whose folder HolyArrs cannot see yet is still added and marked "not visible here yet (check your mounts)". |
| Naming | HolyArrs naming settings, where a pattern word has a match. Nothing on disk is renamed. |
| Bazarr's default language profile, Podnapisi and Gestdown | Default subtitle languages and Subtitle sites. Subtitle sites are held back in this version. |
| Seerr and Overseerr people | People, turned off until you invite them, as **members** with matching permissions and their weekly request limit. Their password stays behind. An admin there stays an admin only if you turn on **Keep as admin** in Choose. The Seerr owner becomes you only if you turn on **This is me**. An email that is not one plain address is left out. |
| Their waiting or approved requests | **Requests**, with the right person. They do not count against anyone's weekly limit. An approved request whose title did not come over waits for approval again. |
| Discord, webhook and email notifications | **Alerts**, turned off until you type the link, address or mail password. Webhook alerts are held back in this version. Discord and email work now. |

### Waits

The old app's default wait rule comes over only when it differs from that app's own default, and only while those Pick rules still have the waits they came with. "Skip the wait at the top quality" turns on **Skip the wait once good enough**. If two apps set different waits for the same Pick rules, neither is used. **Try it** shows the change and how many versions use those Pick rules.

## What stays behind

- **Media files.** The move never moves, renames, copies or deletes a file.
- **Keys, passwords, tokens and cookies.** They are never read into a move. The Move step and the report list each one under **Keys to type again**.
- **Settings with no match here.** For example custom formats, tag-scoped wait rules, release word rules, language and metadata profiles, tags, import lists for shows and music, list kinds HolyArrs does not read (IMDb, Plex, another Radarr and the like), metadata writers, Prowlarr's app links, sync profiles and proxies, download categories, usenet servers, and other notification kinds. Each one is on the **not moved** list with its reason. Tag-scoped wait rules are listed with their waits, so you can set them under Pick rules.
- **Not read at all.** Auto-tag rules, Seerr's request tags, and the old apps' history. The person who asked for a title does come over.

## After the move

Type these again, then turn each thing on:

- each Source's key in **Settings > Sources**, then resume it,
- each Downloader's password or key in **Settings > Downloaders**, then resume it,
- each Alert's link or mail password in **Settings > Alerts**, then turn it on,
- each Outside list: check its folder and Pick rules, then turn it on in **Settings > Outside lists** (add your Trakt key first for a Trakt list),
- invites for the people who came over, in **Settings > Users and roles**.

A Source, Downloader or Alert that had tags in the old app now serves every title of its kind. Check it before you turn it on.

Sources and Downloaders come over paused even when they need no key, with **May reach the local network** off. Turn that switch on only for one that lives on your home network. Link-local and cloud metadata addresses stay blocked either way.

## People with the same name

When someone here already has the name of a person from Seerr, the move never joins them by itself. **Try it** lists that person under **Needs a choice** with **Same person** and **Leave out**.

- **Same person** gives their requests to the person here.
- **Leave out** brings their requests over as yours.

The same choice shows up when HolyArrs cannot tell if a person is the one an earlier move brought over, and when the person you picked is gone. Your choice runs the test again right away. The row stays, so you can change it. Until you choose, that person stays behind. **Done** tells you how many still need a choice.

## Move again

A move remembers what it added. Moving again, for example after you fix a path rule, adds only what is new and says **already here** for the rest. Only one move or undo runs at a time. If HolyArrs stops during a move, the move is marked failed at the next start, or picks up again if its job is still there. It never blocks the next move.

## Undo a move

Click **Undo this move** at step 5. For an older move, **Open** it under **Earlier moves** first. Confirm, and the undo runs in the background with its own progress. It removes only what that move added, newest first:

- titles leave the Library, and **their files stay where they are**,
- Sources, Downloaders, Alerts, subtitle sites, people, requests and Never-add entries it added are removed,
- Outside lists it added are removed while they are still paused,
- naming, default subtitle languages and the waits on Pick rules go back to what they were.

What you set up or changed after the move stays, and the undo report says so. That covers a Source, Downloader or Alert whose secret you typed, a person you turned on, an Outside list you turned on, and naming, languages or waits you changed. Library folders that held titles stay too, because the removed titles keep their History. Remove what stays by hand if you want.

## Parts and safe mode

A part that is turned off is left out. With Movies off, no movie folder, movie or Outside list comes over. With Subtitles off, no subtitle setting. With Requests off, no requests.

When HolyArrs runs in safe mode, you can read, choose and try, but you cannot move or undo.

## Troubleshooting

- **Titles show as not found.** Their folder does not match a Library folder here. Add a path rule under **Advanced** in Choose, then move again.
- **Move says the plan changed.** Something changed since the test. Run **Try it** again, then move.
- **A path pair says "not visible here yet".** HolyArrs cannot see that folder. Check your container mounts.
- **The move button is greyed out.** Read the text next to it. A person may still need a choice, or HolyArrs may be in safe mode.
- **Some items failed.** For example the metadata server was down. Your old-app keys are kept, so wait a bit and move again.

## API

API keys need the **admin** scope.

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/plan` | The plan for the picked reads, with counts and suggested path rules. |
| `POST /api/v1/migrate/dry-run` | The test run's report and its `plan_hash`. Nothing changes. A person who needs a choice has `choose` set to `same_name`, `numeric_id` or `merge_gone`. |
| `POST /api/v1/migrate/moves` | Starts a move. Needs the test run's `plan_hash`. Answers 409 when the plan changed. |
| `GET /api/v1/migrate/moves` | Lists moves, newest first. |
| `GET /api/v1/migrate/moves/{id}` | One move, its report and its progress. |
| `GET /api/v1/migrate/moves/{id}/report` | The report as a Markdown file. |
| `POST /api/v1/migrate/moves/{id}/undo` | Queues the undo (202). Follow it with `GET /api/v1/migrate/moves/{id}`. |

The body of plan, dry-run and moves looks like this, plus `"plan_hash"` for moves:

```json
{
  "snapshots": [1, 2],
  "path_rules": [{"from": "/data/media/movies", "to": "/movies"}],
  "skip": ["alerts"],
  "people": {"person:seerr:1": "owner", "person:seerr:3": "admin", "person:seerr:2": "merge"}
}
```

A person choice is `owner` (only for the old app's owner), `admin`, `merge` or `skip`. The groups you can skip are `folders`, `pick_rules`, `titles`, `never_add`, `outside_lists`, `sources`, `downloaders`, `naming`, `subtitles`, `people`, `wishlist` and `alerts`.
