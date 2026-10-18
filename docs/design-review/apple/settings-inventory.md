# Settings inventory and the new map

Written 2026-10-18, before anything moved. It lists every setting on every Settings page as it was on `codex/apple-direction` at `093c0f2`.

How to read the tables:
- **Who** says who can open the page:
  - `settings`: people with the settings permission.
  - `users`: the owner and admins.
  - `self`: everyone who is signed in.
- **How often** is a guess at how often a person changes the setting: **once** (set during setup), **sometimes**, or **often**.
- **Adv** marks advanced settings: rarely changed, technical, or risky. In the new layout these sit behind the card's **Advanced** disclosure.
- **Default** is the value the page shows before the server answers. "server" means the page shows whatever the server sends.

## 1. Every setting, page by page (before)

### Library folders (`/settings/library-folders`, settings)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| New Library folder | Holds | select Movies / Shows / Music | Movies | once | |
| New Library folder | Folder path | text | – | once | |
| New Library folder | Name (optional) | text | – | once | |
| Folder row | Name (rename) | text | current | sometimes | |

Row actions: Check again, Rename, Remove (asks first). Folder health comes from the folder list itself.

### Naming and files (`/settings/naming`, settings). One form, one Save.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Movie names | File name pattern | text | server | once | |
| Movie names | Folder name pattern | text | server | once | |
| Movie names | A colon in a name becomes | select | smart | once | Y |
| Movie names | Other characters a disk cannot hold | select | replace | once | Y |
| Show names | Show folder pattern | text | server | once | |
| Show names | A folder for each season | checkbox | server | once | |
| Show names | Season folder pattern | text | server | once | |
| Show names | Folder for specials (season 0) | text | server | once | Y |
| Show names | Episode file name pattern | text | server | once | |
| Show names | For shows ordered by air day | text | server | rare | Y |
| Show names | For shows ordered by running number | text | server | rare | Y |
| Show names | A file holding several episodes | select | server | rare | Y |
| Show names | Air day | select | server | rare | Y |
| Show names | Digits in a running number | select | server | rare | Y |
| Music names | Artist folder pattern | text | server | once | |
| Music names | Album folder pattern | text | server | once | |
| Music names | Track file name pattern | text | server | once | |
| Music names | Track name on an album of more than one disc | text | server | rare | Y |
| Music names | A folder for each disc | checkbox | server | rare | Y |
| Music names | Disc folder pattern | text | server | rare | Y |
| Music names | Read a track's own tags when its name is not enough | checkbox | server | rare | Y |
| Importing | Rename files when they are imported | checkbox | on | once | |
| Where files go | Folders read per Import a library scan | number | 5000 | rare | Y |
| Where files go | Artist folders read per music scan | number | 200 | rare | Y |
| Where files go | Keep deleted files for (days) | number | 7 | sometimes | |
| Where files go | Always leave this much free space (MB) | number | 100 | rare | Y |
| Where files go | Skip the free-space check | checkbox | off | rare | Y |
| Holding folder | Empty old holding days now | action | – | sometimes | |

### Sources (`/settings/sources`, settings). An add/change form per Source.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Add / Change | Name, Kind of feed, Downloads, Address, Key | text / select / password | empty form | once | |
| Use it for | Newest list, Searches on its own, Search by hand | checkboxes | empty form | sometimes | |
| Media | Movies, TV shows, Music | checkboxes | empty form | once | |
| Add / Change | Priority | number 1–50 | empty form | sometimes | |
| Add / Change | Fewest seeders | number | empty form | sometimes | |
| Limits | Most searches per window, Window (hours), Least time between calls (ms), Give up after (seconds) | numbers | empty form | rare | Y |
| Proxy | Proxy address, Proxy password, The proxy may be on the local network | text / password / checkbox | empty form | rare | Y |
| Add / Change | May reach the local network | checkbox | empty form | once | Y |

Row actions: Test (own result line), Pause / Resume, Try again now, Health (opens `/settings/sources/{id}`), Change, Remove (asks first). Saving a new Source runs a test straight after the save. The health page is read-only.

### Search (`/settings/search`, settings). One form, one Save.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Daily search | Check Source feeds every (minutes) | number 0–120 | 15 | rare | Y |
| Daily search | Movies to search for each night | number 0–500 | 20 | rare | |
| Daily search | Also Find a better copy each night | checkbox | off | sometimes | |
| Daily search | Search for missing episodes too | checkbox | on | once | |
| Season packs | A season pack is taken when | select | all | rare | Y |
| Season packs | Season pack or single episodes | select | rank | rare | Y |
| Season packs | Take Releases with several seasons | checkbox | off | rare | Y |
| Music | Search for missing albums too | checkbox | on | once | |
| Music | Take whole discographies | checkbox | off | rare | Y |
| Music | Tracks an album Release may lack | number | 0 | rare | Y |
| Early and fake Releases | Turn away Releases posted before it came out | checkbox | on | rare | Y |
| Early and fake Releases | Hours a Release may be early | number | 24 | rare | Y |
| Early and fake Releases | Turn away fakes by size and file kind | checkbox | on | rare | Y |
| Early and fake Releases | Check file sizes at import | checkbox | on | rare | Y |
| Early and fake Releases | Smallest video file to place (MB) | number | 0 | rare | Y |
| Early and fake Releases | Smallest track file to place (KB) | number | 0 | rare | Y |
| Early and fake Releases | Episode length when not known (minutes) | number | 20 | rare | Y |
| Early and fake Releases | Hold Releases of unknown size or date for you | checkbox | off | rare | Y |
| Downloaders | Try the next Downloader | checkbox | on | rare | Y |
| Search now | Check feeds now, Search every missing one, Find a better copy for all | actions | – | sometimes | |

### Subtitles (`/settings/subtitles`, settings). The settings form plus a site form.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Default languages | Language list | language editor | server | once | |
| Pass marks | Episodes, Movies | number 0–100 | server | rare | Y |
| Inside the video | Count subtitles inside the video | checkbox | server | once | |
| Inside the video | Leave out picture subtitles | checkbox | server | rare | Y |
| Inside the video | Hearing-impaired files are named | select | server | once | Y |
| When to look | Look for missing subtitles every (hours) | number | server | rare | Y |
| When to look | Files per run, at most | number | server | rare | Y |
| When to look | Slow down after three weeks | checkbox | server | rare | Y |
| When to look | Look for better subtitles | checkbox | server | sometimes | |
| When to look | Every (hours); For subtitles saved in the last (days) | numbers | server | rare | Y |
| Sync fix | Shift buttons move (ms), Frame rate from, Frame rate to | numbers | server | rare | Y |
| Subtitle site form | Which site, Name, Priority, Address, Username / Password / API key, On | radio / text / password / checkbox | – | once | |
| Subtitle site form, Limits | Wait between calls, Calls per window, Window, Downloads a day, May reach the local network | numbers / checkbox | – | rare | Y |

Site row actions: Test (own result line), Pause / Turn on, Change, Remove (asks first). Saving a site runs a test straight after the save.

### Downloaders (`/settings/downloaders`, settings). An add/change form per Downloader, plus a settings form.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Add / Change | Which app, Name, Address, Sign-in name, Password or API key | radio / text / password | empty form | once | |
| Add / Change | Category | text | empty form | once | Y |
| Add / Change | Priority | number 1–50 | empty form | sometimes | |
| Add / Change, Different folder names | Downloader's path / HolyArrs' path pairs | text pairs | – | once | Y |
| Add / Change | May reach the local network | checkbox | empty form | once | Y |
| When downloads go wrong | Stalled after (hours without progress) | number | 6 | rare | Y |
| When downloads go wrong | Give up and try the next Release after (hours) | number | 24 | rare | Y |
| When downloads go wrong | Remove failed downloads from the Downloader | checkbox | on | rare | |
| When downloads go wrong | Fail downloads that hold programs or fakes | checkbox | on | rare | Y |
| When downloads go wrong | Replace downloads removed outside HolyArrs | checkbox | on | rare | Y |
| When downloads go wrong | Delete its files / Add its Release to Turned down / Search again (defaults when removing a download) | checkboxes | on / off / on | rare | |
| When downloads go wrong | Try the next Downloader | checkbox | on | rare | Y |
| When downloads go wrong | Seconds between sends to one Downloader | number | 2 | rare | Y |

Row actions: Test (own result line), Pause / Resume, Change, Remove (asks first), Abandon this send. Saving a Downloader runs a test straight after the save. "Try the next Downloader" shows on both Search and Downloaders; it is the same server value.

### Outside lists (`/settings/outside-lists`, settings)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| New / Change list | Name, Kind, Which list, Library folder | text / select | – | once | |
| New / Change list | Pick rules | select | the folder's own | sometimes | |
| New / Change list | Read every (hours) | number 6–168 | 12 | rare | Y |
| New / Change list | Keep up to date, Search when added, On | checkboxes | on / off / on | sometimes | |
| Trakt key | Paste / Replace the Trakt key | password (masked) | – | once | Y |
| Never-add list | TMDB number, Name, Why | text | – | sometimes | |

Row actions: Read now, Change, Remove (asks first), Take off (Never-add).

### Music (`/settings/music`, settings). One form, one Save.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| A new artist | Albums to keep up to date | select | all | once | |
| A new artist | New albums | select | all | once | |
| A new artist | Album kinds | checkboxes | album, EP | once | |
| A new artist | Extra kinds | checkboxes | studio | rare | Y |
| A new artist | Pressings that count | checkboxes | official | rare | Y |
| Pressings | Which Pressing an album follows | select | common | rare | Y |
| MusicBrainz | MusicBrainz address (optional) | text | empty | rare | Y |
| MusicBrainz | Keep answers for (hours) | number | 24 | rare | Y |

### Wishlist (`/settings/wishlist`, settings). One form, one Save.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Weekly limits | Movie asks, Show asks, Music asks | numbers 0–1000 | 10 each | sometimes | |
| Approved asks | Library folder for approved movie / show / music asks | selects | the first folder | once | |
| Approved asks | Pick rules for approved movie / show / music asks | selects | the folder's own | once | Y |
| Approved asks | Search when approved | checkbox | on | once | |

### Alerts (`/settings/alerts`, settings). An add/change form per Alert.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Add / Change | Name, Kind, Discord link, From address, Staff addresses, On | text / select / email / checkbox | Discord, on | once | |
| Add / Change | Webhook link, Secret, What the secret does | text / password / select | sign | once | Y |
| Add / Change | Mail server, Port, Security, Mail sign-in name, Mail password | text / number / select / password | 587, STARTTLS | once | Y |
| Add / Change | When to send (Wishlist, Library moments) | checkbox groups with All / None | all on | sometimes | |
| Add / Change | May reach the local network | checkbox | off | once | Y |
| Season packs | One Alert for the pack / One Alert per episode | radio, saves on change | one | once | |

Row actions: Send a test (own result line), Change, Remove (asks first).

### Media servers (`/settings/media-servers`, settings)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Add / Change | Name, Kind, Address, On | text / select / checkbox | Jellyfin, on | once | |
| Add / Change | Plex token / API key | password | – | once | Y |
| Add / Change | Tell it after an import / a rename / a delete | checkboxes | all on | once | |
| Add / Change | HolyArrs path, Same folder on the server | text | – | once | Y |
| Add / Change | May reach the local network | checkbox | on | once | Y |

Row actions: Test (own result), Change, Remove (asks first). Saving a server runs a test straight after the save.

### TMDB key (`/settings/tmdb`, settings, needs a recent password)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Read access token | Paste / Replace the key | password (masked, Saved / Not set) | – | once | Y |

### Backups (`/settings/backups`, settings)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| How often | Balanced / Light / Careful | radio, saves on change | balanced | once | |
| Saved backups | Restore from a file | file picker (asks first) | – | rare | Y |

Row actions: Make a backup now, Download, Delete (asks first).

### Sign-in (`/settings/sign-in`, settings; some rows owner only)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Local network | Skip sign-in on my local network | switch, saves on change | off | once | Y |
| Two-factor sign-in | Admins must use two-factor sign-in (owner) | switch, saves on change | off | once | Y |
| Passwords | Shortest password allowed (owner) | number 4–64, own Save | 4 | once | Y |
| Sign in with Plex | Show the Plex button; Server id people must share | switch / text, own Save | off | once | |
| Sign in with Jellyfin / Emby | Server address; Who may join; Names or emails | text / select / textarea, own Save | nobody | once | |
| Trusted proxies | Text only (set with environment values) | – | – | – | Y |
| API keys | Key name; Make a key; Copy; Revoke (asks first) | text + actions | – | sometimes | Y |

### Users (`/settings/users`, users)
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Invite someone | Role (Admin is owner only), Preset, May…, Note for yourself, Days the link lasts | radio / checkboxes / text / number | Member, 7 days | often | |
| People | Devices, Change, Reset two-factor (asks first), Turn off / on (asks first) | actions | – | sometimes | |
| Change dialog | Role, Preset, May…, Weekly asks (movie / show / music) | radio / checkboxes / numbers | current | sometimes | |
| Invite links | Take back | action | – | sometimes | |

### Move from other apps (`/settings/move`, users)
Steps: read an app (App, How, Use the saved key, Address, User name, Password, API key, home network, Backup path), see the plan (path rules, groups, people to merge), try it, then move, undo, forget or download the report. All once. The path rules are advanced. Moving, undoing and forgetting each ask first.

### Parts and controls (`/settings/parts`, users)
No fields. Turn a part on or off, restart a part, pause / resume / restart a stage, reconnect or test a linked app. The whole app can be restarted, restarted in safe mode or shut down; the server asks a second time for each one.

### This app (`/settings/this-app`, settings to change, everyone who may open Settings to read)
Read-only: version and database facts, updates (Check for updates now), Health checks (Check again), Stages, Linked apps, Storage (needs a recent password), Logs links.
| Card | Label | Control | Default | How often | Adv |
|---|---|---|---|---|---|
| Health check settings | Check every (minutes) | number 1–1440 | server | rare | Y |
| Health check settings | Alert after this many runs in a row | number 1–20 | server | rare | Y |
| Health check settings | Warn when free space is under (MB) | number | server | sometimes | Y |
| Health check settings | Check once a day for a newer HolyArrs | checkbox | server | once | |
| Health check settings | Release feed | text | the project feed | rare | Y |
| Health check settings | The release feed may be on the local network | checkbox | server | rare | Y |
| Health check settings | Tell me about test versions too | checkbox | server | rare | Y |

### Schedules (`/settings/schedules`, settings)
One row per timed job: Run now, Change → Every (unit) number + Save, and turning automatic refresh off or on (movie refresh only). How often: sometimes. Advanced: yes.

### API for other apps (`/settings/api`, settings)
Read-only: base address (Copy), scopes, example calls (Copy), every route with Find a route. No settings.

### Logs (`/settings/logs`, users)
Filters only (Level, Text, From, To). These are not saved settings. Actions: Show, Clear, Newer, Older, Download the log.

### My account (`/settings/account` and `/account`, self)
| Card | Label | Control | How often |
|---|---|---|---|
| Confirm local sign-in | HolyArrs password, Two-factor code | password / text | sometimes |
| Linked sign-in | Link single sign-on, Link trusted proxy sign-in | actions | once |
| Two-factor sign-in | Set up, code, Turn on, Make new backup codes / Turn off | text / radio / actions | once |
| Alerts for you | Email address, Discord user id | email / text | once |
| Password | Current, New, New again | password | rare |
| Signed-in devices | Sign out one, Sign out all other devices | actions | sometimes |

### Settings home (`/settings`)
Appearance: Theme (Light / Dark / Automatic), stored in this browser. Search filtered whole pages only.

**Counts before:** 22 pages, 4 groups (Library 7 pages, Linked apps 6, Access 4, System 5), about 75 cards and about 190 settings. These counts leave out read-only facts and row actions.

## 2. What is advanced

These are the settings marked **Adv = Y** above. In short:
- naming edge cases and scan limits;
- feed timing, season packs, discographies and the early and fake guards;
- subtitle Pass marks, timing and Sync fix;
- the Downloader stall and retry timing;
- music Pressings and MusicBrainz;
- Pick rules for approved asks;
- the health check timing and release feed;
- source and site limits, proxies and local-network switches;
- path maps, API keys, the Trakt and TMDB keys, and restoring from a file.

Restore, shut down and safe mode are advanced and also risky. They live in Danger zone.

## 3. The new map (old place → new place)

**Group names.** The brief asked for a group called "Connections". That word is on the glossary's _Avoid_ list, and the build's word check fails on it, so the group keeps the existing label **Linked apps**.

| Old place | New place |
|---|---|
| Settings home › Appearance card | **General › Appearance** (new page `/settings/appearance`) |
| System › This app (version, updates, stages, linked apps, storage) | **General › This app**. Health checks shrink to a one-line summary with a link. |
| System › Parts and controls (parts, stages, linked apps, controls used) | **General › Parts and controls** |
| System › Parts and controls › The whole app (restart, safe mode, shut down) | **Maintenance › Danger zone** (new page `/settings/danger`, always last, red outline, every action asks first) |
| Library › Schedules | **General › Schedules** |
| Library › Library folders | **Library › Library folders** |
| (no page; chosen in the add dialog, and remembered per browser) | **Library › Movies and TV defaults** (new page `/settings/title-defaults`, added later) |
| Library › Naming and files | **Library › Naming and files**. Edge cases move under Advanced. |
| Library › Music | **Library › Music defaults** (what a new artist gets, Pressings, MusicBrainz) |
| Library › Subtitles | **Library › Subtitles** |
| Linked apps › TMDB key | **Library › Metadata** (same route `/settings/tmdb`; MusicBrainz stays on Music defaults and the search finds it from Metadata words) |
| Linked apps › Outside lists | **Library › Outside lists** |
| Linked apps › Sources | **Getting copies › Sources** |
| Library › Search | **Getting copies › Search** |
| Linked apps › Downloaders | **Getting copies › Downloaders** |
| (no page; Pick rules were only picked from lists) | **Getting copies › Pick rules** (new page `/settings/pick-rules`; at first read-only, later with an editor for one set at `/settings/pick-rules/{id}`) |
| Library › Wishlist | **Wishlist and people › Wishlist rules** |
| Access › Users (invites, people, invite links) | **Wishlist and people › Users and roles**. Invites stay on that page as their own cards, and the search finds them. |
| Access › Sign-in (methods, two-factor rule, local network, passwords, API keys) | **Sign-in and security › Sign-in and security** |
| Linked apps › Media servers | **Linked apps › Media servers** |
| Linked apps › Alerts | **Linked apps › Alerts** |
| Access › API for other apps | **Linked apps › API for other apps** |
| System › Move from other apps | **Linked apps › Move from other apps** |
| System › Backups | **Maintenance › Backups and restore** |
| System › Logs | **Maintenance › Logs** |
| System › This app › Health checks and Health check settings | **Maintenance › Health checks** (new page `/settings/health`) |
| Access › My account | **My account**. It stays open to everyone, sits on its own below the groups, and also opens from the account menu (`/account`). |

### What could not be placed as asked
- **Movies and TV shows defaults.** Placed later, once the server kept them: **Library › Movies and TV defaults** (`/settings/title-defaults`) sets the Library folder, Pick rules, Keep up to date and Search when added for a new movie and a new show, with the Release gate and extra days (movies) and the seasons, new seasons, episode order and details source (shows) behind Advanced. The add dialogs start from it; the browser now remembers only the Release gate. Music defaults keep their own page.
- **Appearance: density and reduced motion.** The app has no density setting. Reduced motion follows the operating system only. The Appearance page holds Theme alone; adding the others needs a decision (see backend-needs.md).
- **Sessions in Sign-in and security.** No setting covers all sessions. People sign out their own devices in My account, and admins see each person's devices in Users and roles. The search finds both.
- **Reset in Danger zone.** HolyArrs has no reset action in the app; the owner reset is a command-line tool. Danger zone holds Restart, Restart in safe mode and Shut down.
- **Editing Pick rules.** Placed later, once the server could change them: the Pick rules page has New Pick rules, and per set Change, Make a copy and Remove (asked first; the server refuses a set something uses and says what). Change opens the editor (`/settings/pick-rules/{id}`): Name and kinds, Quality steps with Good enough, Traits and points, Must have and Never, Size (GB per hour for every step), Waiting and Language, with which wins, upgrade points, fewest points and fixed copies behind Advanced. Ready-made sets keep their name and kinds and cannot be removed.

## 4. Before and after (counted after the move)

A card is a SettingsCard or a card section. Settings are fields; actions and read-only facts are not counted.

| | Before | After |
|---|---|---|
| Groups | 4, plus Appearance on the home | 7, plus My account |
| Pages | 22 | 28 (new: Appearance, Pick rules, Health checks, Danger zone; later Movies and TV defaults and the Pick rules editor) |
| Cards | about 70 | 94 (big forms are split into cards with a heading and a hint) |
| Settings behind Advanced | 0 | about 71 |

Pages, cards and settings per group, after the move:

| Group | Pages | Cards | Settings |
|---|---|---|---|
| General | 4 | 16 | Theme, plus one interval per timed job |
| Library | 7 | 21 | 93 |
| Getting copies | 5 | 20 | 78 |
| Wishlist and people | 2 | 5 | 19 |
| Sign-in and security | 1 | 6 | 11 |
| Linked apps | 4 | 12 | 38 |
| Maintenance | 4 | 7 | 9 |
| My account | 1 | 7 | 11 |

The same settings per group before the move: Library 95, Linked apps 79, Access 31, System 21, and the home 1 (Theme).

**Clicks from Settings home to the 10 most-used settings.** Each search result goes straight to its field.

| Setting | Before | After (browse) | After (search) |
|---|---|---|---|
| Invite someone | 1 | 1 | type + 1 |
| Weekly ask limits | 1 | 1 | type + 1 |
| Add a Library folder | 2 | 2 | type + 1 |
| Keep deleted files for | 1, then scroll past three cards | 1 | type + 1, lands on the field |
| Movies to search each night | 1 | 1 | type + 1 |
| Look for better subtitles | 1, then scroll | 1 | type + 1 |
| Add a Source | 2 | 2 | type + 1 |
| Add a Downloader | 2 | 2 | type + 1 |
| How often backups are made | 1 | 1 | type + 1 |
| Theme | 0 (on the home) | 1 | type + 1 |

Advanced settings, such as feed check minutes or free space, take one more click (Advanced) when browsing. From search they still take type + 1, because the link opens Advanced by itself.
