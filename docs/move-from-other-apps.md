# Move from other apps

HolyArrs can bring your setup over from the apps it replaces: Radarr,
Sonarr, Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent and
SABnzbd. You find it under **Settings > Move from other apps** (the owner
and admins only).

The move works in five steps, one at a time. The steps sit in a row at
the top: a step you finished takes you back to it, and the browser's Back
button works too. Each step has one main button at the bottom; when it
waits, the words next to it say why.

1. **Read.** **Add an app** reads one, through its address and API key
   (qBittorrent: its user name and password), or from a backup file on the
   HolyArrs server. **Check connection** asks the app first, in seconds.
   A read can take a few minutes; **Stop reading** stops it. Each read gets
   a switch: the newest read of each app starts on, and a read that a move
   already used starts off. See
   [Moving from the old apps: reading their setup](move-reading.md) for what
   is read and how keys are kept out.
2. **Choose.** What comes over, group by group: each group has a switch
   and **Show** for its items. Old folders that need a place here get a
   row each, and the old owner and admins get their own switches. Every
   change updates the list.
3. **Try it.** A test run against this server shows, group by group, what
   will be added, used, updated, left out, not found or already here.
   Nothing changes.
4. **Move.** **Move** asks once more, then the move runs in the background
   with its progress. You can leave the page; it keeps going. It runs
   exactly what the test showed: change anything and the test runs again
   first.
5. **Done.** What to do now, the results per group, the report as a file
   to download, and **Undo this move**.

**Earlier moves**, under the reads, lists every move with its state.
**Open** shows one at step 5 (or step 4 while it still runs).

Nothing in the old apps changes at any step. After the read, HolyArrs never
talks to them again, and it never writes to them.

## Path rules

The old apps often run in other containers and see your folders under
other names. In **Choose**, each old folder that is not a Library folder
here gets a row: pick the Library folder it is here, or **Other path…** to
type one. When an old folder is not on this server and a Library folder
with the same last name is, that folder is filled in and marked
**Suggested**. Under **Advanced** you can add other path rules: if Radarr
kept movies in `/movies` and HolyArrs sees the same folder as
`/data/movies`, add the rule `/movies` to `/data/movies`. The longest
matching rule wins.

Titles whose folder is not found are listed with the reason. Fix the rule
and move again: nothing is added twice.

## What moves

| From | Becomes in HolyArrs |
| --- | --- |
| Library folders (Radarr, Sonarr, Lidarr) | Library folders for movies, shows and music. |
| Movies, shows and artists | Titles, found by their TMDB, TVDB or MusicBrainz id, using **the folder where it is**: each title's folder and files stay exactly where they are. Its watch flag becomes **Keep up to date**; seasons that were not watched stay off. Nothing is searched for during the move. |
| Quality profiles | Each one is matched to the closest **Pick rules** preset (Good everyday, 4K, Lossless first, Lossy 320 or V0, Any music), with the reason shown in the plan. Titles use the preset of their profile. |
| Exclusions | The **Never-add** list (read live or from a backup). |
| Radarr's release stage of each movie, and its availability delay | Each moved movie's **Release gate** and gate delay (up to 365 days). This server's default for new movies stays as it is. |
| The untagged default wait rule (Radarr, Sonarr, Lidarr) | The preferred protocol and the Usenet and torrent waits of the ready-made **Pick rules** its quality profiles matched (up to 7 days; "skip the wait at the top quality" turns on **Skip the wait once good enough**). Only when the rule differs from the old app's own default, and only while those Pick rules still have their wait as they came. If two apps set different waits for the same Pick rules, neither is used; an app left at its own default counts as no wait. Try it shows the change and how many versions use those Pick rules. |
| Radarr TMDB lists, Trakt lists and Trakt watchlists | **Outside lists**, always added paused, with their movie Library folder, Pick rules, Keep up to date, search on add, Release gate and how often they are read (6 to 168 hours). A Trakt list needs your Trakt key and must be public. |
| Torznab, Newznab and RSS indexers (Radarr, Sonarr, Lidarr, Prowlarr) | **Sources**, always added paused and unable to reach your home network, with or without a key. An address is cut to its host and API path; an RSS feed address, extra parameters and anything that looks like a key in the path are typed again. |
| qBittorrent and SABnzbd, and the Transmission, Deluge and NZBGet clients set up in Radarr, Sonarr or Lidarr | **Downloaders**, always added paused and unable to reach your home network, even without a password. They use HolyArrs's own `holyarrs` category, so they never pick up the old apps' downloads. |
| Naming | HolyArrs naming settings, where a pattern word has a match. Nothing on disk is renamed. |
| Bazarr's default language profile, Podnapisi and Gestdown | Default subtitle languages and Subtitle sites. |
| Seerr and Overseerr people | People turned off until you invite them, as **members**, with the permissions that match and their weekly request limit. Their password stays behind. An admin there stays an admin only if you turn on **Keep as admin** in Choose; the Seerr owner is you only if you turn on **This is me**. An email that is not one plain address is left out. |
| Their open asks (waiting or approved) | **Requests** asks, with the right person. They do not count against anyone's weekly limit. An approved ask whose title did not come over waits for approval again. |
| Discord, webhook and email notifications | **Alerts**, turned off until you type the link, address or mail password. Links in notification settings (webhook addresses, servers, topics) never come over. |

## What never moves

- **Media files.** The move never moves, renames, copies or deletes a
  file. Titles stay where they are.
- **Keys, passwords, tokens and cookies.** They are never read into
  HolyArrs. The Move step and the report list each one under **Keys to
  type again**.
- **Settings with no HolyArrs match.** Custom formats, tag-scoped wait
  rules (each one listed with its waits, to set under Pick rules), protocol
  on/off switches and score-based wait skips, release word rules, language
  and metadata profiles, tags, import lists for shows and music and list
  kinds HolyArrs does not read (IMDb, Plex, another Radarr, Trakt watched or
  collection lists and the like), metadata writers, Prowlarr's app links,
  sync profiles and indexer proxies, download categories and usenet servers,
  Bazarr's other profiles and sites HolyArrs does not have yet, and
  notification kinds other than Discord, webhook and email. Each one is on
  the **not moved** list with its reason. Not read at all: auto-tag rules
  (they only add tags) and Seerr's request tags; the person who asked does
  come over.

## Needs your input afterwards

After the move, type again:

- each Source's key (**Settings > Sources**), then resume it;
- each Downloader's password or key (**Settings > Downloaders**), then
  resume it;
- each Alert's link or mail password (**Settings > Alerts**), then turn it on;
- each Outside list that came over: check its folder and Pick rules, then
  turn it on (**Settings > Outside lists**); for a Trakt list add your Trakt
  key there first;
- invites for the people who came over (**Settings > Users and roles**).

A Source, Downloader or Alert that had tags now serves every title of its
kind; check it before you resume it or turn it on. An import list's sign-in
is not asked for: an Outside list reads public lists with this server's own
keys, and a list that did not come over says why on the **not moved** list.

Sources and Downloaders come over paused even when they need no key, and
their **May reach the local network** switch is off. Check each one, turn the
switch on only for one that lives on your home network (link-local and
cloud metadata addresses stay blocked either way), then resume it.

## People with the same name

When someone here already has the name of a person from Seerr, the move
never joins them on its own. **Try it** lists that person under **Needs a
choice** with **Same person** and **Leave out**, and nothing picked. Same
person makes their requests belong to the person here; Leave out brings
their requests over as yours. The same choice shows when HolyArrs cannot
tell if a person is the one an earlier move brought over (the old app gave
only a number for them), and when the person you picked is no longer here.
A choice runs the test again at once, and the row stays, so you can change
it. Until you choose, the person stays behind; **Done** says how many still
need a choice and takes you back to choose.

## Moving twice

A move remembers what it added: each thing and its mark are written
together, so nothing is added without one. Moving again (for example after
adding a path rule) adds only what is new and says **already here** for
the rest. Only one move or undo runs at a time. A move cut off by a stop
of HolyArrs is marked failed at the next start (or resumed when its Job is
still there), so it never blocks the next one.

## Undo a move

**Undo this move** (at step 5; **Open** a move under **Earlier moves** to
get there) asks once more, then runs in the background like the move, with
its own progress, and removes only what that move added, newest first:

- titles leave the Library, and **their files stay where they are**;
- Sources, Downloaders, Alerts, subtitle sites, people, asks and Never-add
  entries it added are removed;
- Outside lists it added are removed while they are still paused;
- naming, the default subtitle languages and the waits on ready-made Pick
  rules go back to what they were, unless you changed them after the move.

Things you set up or changed after the move stay, and the undo report
says so: a Source, Downloader or Alert whose secret you typed, a person you
turned on, an Outside list you turned on, naming, languages or a wait you
changed. A row is removed only while it
is still the one the move made (its creation time and name): if you
removed a moved thing and made your own, which got the same number, yours
stays. Remove what stays by hand if you want. Library folders that held
titles stay too, because the removed titles keep their History.

## Parts and safe mode

A part that is off is left out of the move: with Movies off, no movie
folder, movie or Outside list comes over, and no wait that only Radarr set;
with Subtitles off, no subtitle setting; with
Requests off, no asks. HolyArrs started in safe mode refuses to move or
undo (reading, choosing and Try it still work).

## Who may move

The owner and admins, signed in with their password. API keys need the
**admin** scope.

## API

| Route | What it does |
| --- | --- |
| `POST /api/v1/migrate/plan` | The plan for the picked snapshots, with counts and suggested path rules. |
| `POST /api/v1/migrate/probe` | Asks an app for its name and version, without reading it. |
| `POST /api/v1/migrate/dry-run` | The test run's report and its `plan_hash`. Nothing changes. A People line that needs **Same person** or **Leave out** has `choose`: `same_name`, `numeric_id` or `merge_gone`; it stays after a choice is made. |
| `POST /api/v1/migrate/moves` | Approves and starts a move (a Job); needs the test run's `plan_hash` (409 when the plan changed since). |
| `GET /api/v1/migrate/moves` | Lists moves, newest first. |
| `GET /api/v1/migrate/moves/{id}` | One move, its report and its progress. |
| `GET /api/v1/migrate/moves/{id}/report` | The report as a Markdown file. |
| `POST /api/v1/migrate/moves/{id}/undo` | Queues the undo of a move (a Job; 202). Follow it with `GET /api/v1/migrate/moves/{id}`. |

The body of plan, dry-run and moves is
`{"snapshots": [1, 2], "path_rules": [{"from": "/movies", "to": "/data/movies"}], "skip": ["alerts"], "people": {"person:seerr:1": "owner", "person:seerr:3": "admin", "person:seerr:2": "merge"}}`,
plus `"plan_hash"` for moves. A person choice is `owner` (only for the
person the old app marks as its owner), `admin`, `merge` or `skip`.
The groups you can skip are `folders`, `pick_rules`, `titles`,
`never_add`, `outside_lists`, `sources`, `downloaders`, `naming`,
`subtitles`, `people`, `wishlist` and `alerts`.
