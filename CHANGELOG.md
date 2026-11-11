# Changelog

All notable changes to HolyArrs are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## 1.0.19

### Added

- rTorrent and ruTorrent work as a Downloader, seedboxes too. ([#199](https://github.com/HolyArrs/HolyArrs/issues/199))

### Fixed

- Finished Usenet downloads move into the Library, and copies write each file once. ([#240](https://github.com/HolyArrs/HolyArrs/issues/240))
- Downloads tabs count every torrent, and Show more works under each filter. ([#245](https://github.com/HolyArrs/HolyArrs/issues/245))
- Free space on Downloads matches what qBittorrent and SABnzbd report. ([#246](https://github.com/HolyArrs/HolyArrs/issues/246))
- A shared SABnzbd no longer asks you to match other apps' downloads. ([#248](https://github.com/HolyArrs/HolyArrs/issues/248))

## 1.0.18

### Added

- A Minimal look with small posters and no big photos or autoplay. ([#119](https://github.com/HolyArrs/HolyArrs/issues/119))

### Fixed

- Anime that TMDB keeps as one long season gets its second half too. ([#234](https://github.com/HolyArrs/HolyArrs/issues/234))
- Grabs from sites behind Cloudflare, like NZBFinder, go through. ([#231](https://github.com/HolyArrs/HolyArrs/issues/231))
- A Library folder can be removed once no title uses it. ([#235](https://github.com/HolyArrs/HolyArrs/issues/235))
- Changing good enough on a quality set re-checks your Library. ([#236](https://github.com/HolyArrs/HolyArrs/issues/236))
- The Home downloads card counts torrents your old apps added. ([#237](https://github.com/HolyArrs/HolyArrs/issues/237))
- Importing from old apps checks folder paths and brings over wanted titles. ([#238](https://github.com/HolyArrs/HolyArrs/issues/238))
- Rescan links files in a show's folder by season and episode. ([#241](https://github.com/HolyArrs/HolyArrs/issues/241))
- An empty Library shows the right add button on each page. ([#208](https://github.com/HolyArrs/HolyArrs/issues/208))

## 1.0.17

### Added

- The Library sorts by Score, and shows each title's score while you do. ([#139](https://github.com/HolyArrs/HolyArrs/issues/139))

## 1.0.16

### Added

- Pick rules can score a release by its size, and TRaSH Guides size entries now sync. ([#201](https://github.com/HolyArrs/HolyArrs/issues/201))

### Fixed

- Importing two copies of the same app, like two Sonarrs, keeps both instead of replacing the first. ([#165](https://github.com/HolyArrs/HolyArrs/issues/165))

### Docs

- A new page lists every outside address HolyArrs contacts, what for, and how to turn it off or point it elsewhere. ([#222](https://github.com/HolyArrs/HolyArrs/issues/222))

## 1.0.15

### Added

- Library folders can mark copying as expected, so a setup without hardlinks stops asking for attention. ([#223](https://github.com/HolyArrs/HolyArrs/issues/223))

### Fixed

- Grabs from Newznab sites like NZBgeek keep the site's own download key. ([#216](https://github.com/HolyArrs/HolyArrs/issues/216))
- Sources send a HolyArrs User-Agent, so sites behind Cloudflare stop blocking searches. ([#209](https://github.com/HolyArrs/HolyArrs/issues/209))
- Torrents from magnet-only indexers through Prowlarr or Jackett reach the downloader as magnets. ([#194](https://github.com/HolyArrs/HolyArrs/issues/194))
- The Docker Compose example uses a lowercase project name, so it starts. ([#195](https://github.com/HolyArrs/HolyArrs/issues/195))

## 1.0.14

### Fixed

- Sources imported from Prowlarr get Prowlarr's API key, so they search instead of failing with 401. ([#221](https://github.com/HolyArrs/HolyArrs/issues/221))

## 1.0.13

### Fixed

- The Settings search box reads as an action in every language, for example "Cerca nelle impostazioni" in Italian.
- Card labels such as New movie, New show, New season and New episodes are translated in all 16 languages.
- Italian reads more naturally: word order and many literal translations fixed.

## 1.0.12

### Fixed

- Importing from other apps no longer stops with "the plan changed since its dry run". ([#205](https://github.com/HolyArrs/HolyArrs/issues/205))
- A download finishes when the same file is already in place, for example after Sonarr or Radarr got there first. ([#207](https://github.com/HolyArrs/HolyArrs/issues/207))
- The poster wall shows trending movies while your Library has no posters yet. ([#215](https://github.com/HolyArrs/HolyArrs/issues/215))

## 1.0.11

### Added

- HolyArrs now speaks 16 languages, the same as HolyArrs.com and the docs. Italian, Dutch, Polish and Turkish are new.
- More server messages show in your language: import results, History rows and Music row captions.

### Fixed

- Deleting a title never asks you to type its name; deleted files always go to the holding folder. ([#187](https://github.com/HolyArrs/HolyArrs/issues/187))

## 1.0.10

### Added

- Titles on a Plex watchlist become requests. The owner can turn it on for everyone who signs in with Plex. ([#30](https://github.com/HolyArrs/HolyArrs/issues/30))
- Each downloader can use its own category for movies, shows and music. ([#184](https://github.com/HolyArrs/HolyArrs/issues/184))

### Fixed

- A new install no longer logs "Metadata server is down" while Explore fills in. ([#191](https://github.com/HolyArrs/HolyArrs/issues/191))

## 1.0.9

### Added

- Library > Add > Import from a folder opens the page to match a title by hand. ([#183](https://github.com/HolyArrs/HolyArrs/issues/183))

### Fixed

- A Source that searches through Prowlarr or Jackett says when that app isn't answering. ([#179](https://github.com/HolyArrs/HolyArrs/issues/179))
- Sources from Prowlarr and Jackett read the right feed address. ([#180](https://github.com/HolyArrs/HolyArrs/issues/180))
- Settings > Subtitles no longer shows "404 page not found". ([#182](https://github.com/HolyArrs/HolyArrs/issues/182))
- Files a scan saved before the episodes were known can now be imported by hand. ([#185](https://github.com/HolyArrs/HolyArrs/issues/185))
- Home hides the Requests box while Requests is turned off. ([#186](https://github.com/HolyArrs/HolyArrs/issues/186))
- Deleting a title asks you to type its name only when the files go for good. ([#187](https://github.com/HolyArrs/HolyArrs/issues/187))
- Importing a large Lidarr library works. ([#188](https://github.com/HolyArrs/HolyArrs/issues/188))
- Drop-down lists are readable in dark mode. ([#190](https://github.com/HolyArrs/HolyArrs/issues/190))

## 1.0.8

### Added

- HolyArrs now speaks 12 languages; pick yours in Settings or on the sign-in screen. ([#106](https://github.com/HolyArrs/HolyArrs/issues/106))

### Changed

- "Move from other apps" is now "Import from other apps". Your files stay where they are.

## 1.0.7

### Added

- Pick rules can take their release scores from TRaSH Guides, and sync them again later without duplicates. ([#172](https://github.com/HolyArrs/HolyArrs/issues/172))

### Fixed

- Opening HolyArrs by a name it won't save on now shows how to allow that name, on every page, before you try to save. ([#178](https://github.com/HolyArrs/HolyArrs/issues/178))

## 1.0.6

### Added

- Moving over brings the quality Sonarr and Radarr recorded for each file. Already moved? Read the old app again and move again. ([#174](https://github.com/HolyArrs/HolyArrs/issues/174))

### Fixed

- The last step before a move now says your files stay where they are. ([#176](https://github.com/HolyArrs/HolyArrs/issues/176))
- A refused or unreachable metadata server is logged once every few minutes, not once per row. ([#175](https://github.com/HolyArrs/HolyArrs/issues/175))

## 1.0.5

### Fixed

- Movies moved over from Radarr keep their real quality: `WEBDL-1080p` and `Bluray-720p` names are read right. ([#171](https://github.com/HolyArrs/HolyArrs/issues/171))
- Words in titles, like "Charlotte's Web" or "Cam", are no longer read as quality. ([#171](https://github.com/HolyArrs/HolyArrs/issues/171))
- Episodes moved over from Sonarr with no quality in their names are judged by the file's own resolution, so they stop asking for a better copy. ([#171](https://github.com/HolyArrs/HolyArrs/issues/171))
- Libraries moved over before this version are checked again once after the update. ([#171](https://github.com/HolyArrs/HolyArrs/issues/171))

## 1.0.4

### Changed

- Build tools updated to clear a reported advisory; nothing in the app itself changes. ([#169](https://github.com/HolyArrs/HolyArrs/issues/169))

## 1.0.3

### Added

- Docker images are signed, with a build record and a contents list you can check yourself. ([#166](https://github.com/HolyArrs/HolyArrs/issues/166))

### Fixed

- Requests ready check no longer keeps retrying and using a CPU core on a large library. ([#168](https://github.com/HolyArrs/HolyArrs/issues/168))
- Security: updates the SSO sign-in library to fix a denial-of-service hole (CVE-2026-116587). ([#167](https://github.com/HolyArrs/HolyArrs/issues/167))

## 1.0.2

### Changed

- The metadata server's address is now `api.holyarrs.com`. The old one keeps working. ([#164](https://github.com/HolyArrs/HolyArrs/issues/164))

### Fixed

- First-run setup can be read in light mode. ([#161](https://github.com/HolyArrs/HolyArrs/issues/161))
- An install the metadata server no longer knows gets a new key by itself. ([#162](https://github.com/HolyArrs/HolyArrs/issues/162))
- Moving over: the screens now say your media files are never moved. ([#163](https://github.com/HolyArrs/HolyArrs/issues/163))

## 1.0.1

### Added

- Libraries and downloads made of symlinks work when you set `HOLYARRS_FOLLOW_LINKS=1`. Linked files are never read in full or copied. ([#158](https://github.com/HolyArrs/HolyArrs/issues/158))

### Fixed

- Setup no longer ends on an error page after moving over two downloaders of the same kind. ([#160](https://github.com/HolyArrs/HolyArrs/issues/160))
- A Newznab or Torznab Source works with just the site address, and its key field says **API key**. ([#159](https://github.com/HolyArrs/HolyArrs/issues/159))

## 1.0.0

First release. HolyArrs runs movies, shows and music requests, downloads and your library in one app.

### Explore and search

- A start page shows what's new, what's downloading and what needs you.
- Browse movies, shows and music like a streaming app (music rows are still being built). Pick a genre for its own rows, or hover a title for a quick look.
- Explore builds rows from your library and what your family and friends request. It all happens on your server.
- Popular page with the top new shows and movies, what's trending and what's coming soon, plus a Music tab with this week's biggest albums and tracks.
- Trailers play on cards and title pages and start fast.
- Search finds movies, shows, artists, albums and people as you type. You can also search only what you already have.
- Describe a title in your own words ("that space movie with the robot from 2014") and get the right one.

### Movies

- One page per movie with ratings, the trailer, details and what's already on disk, with a request button.
- Choose when a movie counts as out (announced, in theaters, digital or disc). HolyArrs starts looking from then, plus extra days if you want.
- Add a TMDB list and HolyArrs adds its movies and shows for you.
- Open an actor or director and request the movies you're missing.
- Remove a whole movie series in one step, after you review it. Files stay on disk unless you choose to delete them too.
- Keep HD, 4K, a dub or a director's cut of one title, each in its own folder.

### Shows

- One page per show with every season and episode, ratings and a request button.
- Keep only the seasons and episodes you want up to date, and choose which episode order a show follows.
- Long-running anime numbered straight through, like episode 1090, lands on the right season and episode.

### Music

- Artist pages with albums, top tracks, music videos and a follow button. Album pages list every track.
- Ask for an album from the same place you ask for a movie or show.
- A lossless album is never replaced by a worse copy.

### Requests

- Ask for a movie, a show or an album from one page.
- Approve only the seasons, episodes, albums or songs you want, or turn a request down with an optional reason. Undo any slip.
- Watch a request move from searching to downloading to importing as it happens.
- Get an alert when a request is approved, turned down or ready.
- Want 4K? It arrives as a second version of the same movie and you approve it on its own.
- Set a weekly request limit for each person.
- Report bad audio, wrong subtitles or a broken file.
- Follow a show, artist or upcoming movie and hear when something new reaches your library. Following never makes a request.

### Finding releases

- Add a source once and it serves movies, shows and music. HolyArrs watches it for new releases.
- Keep your Jackett or Prowlarr. HolyArrs finds it, checks your key once and adds every indexer as a source.
- Pick the quality, languages and file sizes you want for each title. Favor or rule out HDR, Atmos, release groups or words in the name. HolyArrs swaps in better copies until it's good enough.
- Hold a release for a set time so a better copy that shows up in the meantime gets picked.
- See why each release won or lost. Releases HolyArrs turned down are listed too, and you can clear them to try again.
- Fake releases and malware are blocked: posted before the title came out, too small, or carrying a file like .exe or .lnk.

### Downloads

- Keep the downloader you already run: qBittorrent, SABnzbd, NZBGet, Transmission or Deluge.
- Tag titles to choose which downloader or source they use.
- See every download on one page and set seeding rules for each qBittorrent torrent.
- A download with no progress for 6 hours is marked stalled. After 24 hours HolyArrs grabs the next best release. You can change both.
- RAR, 7z and zip archives are unpacked and imported on their own.
- Finished files are hardlinked so they don't take space twice. Library folders warns you first when a folder can't.
- Map a downloader's folder to the one HolyArrs sees, as in Docker.
- Once a Usenet download is in your library, HolyArrs clears it from SABnzbd or NZBGet.

### Library

- Your movies, shows and music on one page with posters, filters and search.
- Movies, shows and music each get their own folders. Move titles between folders later.
- Point HolyArrs at the folders you already have. Nothing is moved or renamed.
- Choose how files and folders are named and preview every change before anything is renamed.
- Select a batch of titles and change their quality or folder, search for them or remove them in one go.
- Search for everything you're still waiting on at once.
- Import a download by hand when HolyArrs couldn't match it.
- Deleted files wait in a holding folder for 7 days by default, so you can get them back.
- See why anything happened: every search, pick, download, import, rename and delete, with the reason.
- A calendar shows upcoming episodes, movies and albums as cards or a list.
- Plex, Jellyfin and Emby hear about every file HolyArrs adds, renames or removes, so new titles show up without a manual scan.

### Accounts

- Sign in with a Plex or Jellyfin account. Import the people already on your server, or let them join on first sign-in.
- Single sign-on through a login service you already run, like Authentik or Authelia.
- Two-factor sign-in with an authenticator app.
- Invite family and friends. If someone forgets a password, you set a new one for them.
- Kid accounts only see and ask for titles at or under the age limit you set. Restricted Plex and Jellyfin accounts join as kids.
- On a shared TV or tablet, the sign-in page remembers who used it, so switching person is a tap and a password.
- Movie and show artwork fills the screen behind the sign-in panel.
- Each person gets their own account page for their password, two-factor sign-in, profile picture, alerts, follows and signed-in devices.
- Skip sign-in on the home networks you list. Everywhere else still asks.

### Settings and alerts

- Search every setting by typing a word.
- Every setting has a clear name, a sensible default and a one-line explanation.
- Get alerts on Discord or by email.
- Turn off Movies, Shows, Music or Requests and they disappear from view. Turn them back on anytime.
- An API for your scripts and other apps. Each key gets only the access you give it.

### Status and updates

- Health checks for library folders, free space, sources, downloaders, media servers and backups, plus a heads-up when an update is out.
- Download a log you can post for help, with keys and passwords already hidden. See every timed job and run any of them now.
- HolyArrs backs up your data before every update. Back up and restore any time from Settings.

### Moving over

- Bring your library and setup from Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr or Overseerr. Try it first and undo it later. No file is moved or renamed.
- Seerr, Prowlarr, Bazarr, Unpackerr and Homepage's Sonarr widget keep working, because HolyArrs answers them at /sonarr and /radarr. This stays off until you turn it on.

### Install

- A step-by-step guide for Docker and common NAS boxes.
- A guided first run: pick movies, shows or music and choose your library folder.
- Run HolyArrs behind a reverse proxy at a path like /holyarrs.
- Works on your phone. Every page fits and works at phone width.
- The app download is about 15 MB.

### Privacy and security

- Metadata (titles, posters, cast, ratings, music details) works with no keys to set up.
- A plain page lists what HolyArrs sends to the metadata server. Your files and accounts stay on your server.
- Every page asks for a sign-in. Passwords are hashed with Argon2id and keys are kept in an AES-256-GCM vault.
- Sources on your home network stay blocked unless you allow them.
- Wrong passwords and codes are slowed down. Setting a new password stops that person's old keys and linked sign-ins.
- Server folders and file names stay hidden from anyone who doesn't manage the library. Only the owner can download a backup.
- Passkeys inside your source addresses are encrypted.

### Speed and size

- Pages open in under a second, and long scrolls through Explore stay smooth.
- HolyArrs idles under 250 MB with 5,000 movies and 50,000 episodes. On a box with about 2.5 GB of memory or less, it trims its own caches and background jobs.

### Not in 1.0

These are built or planned but are not part of this release.

- Connecting AI apps through MCP.
- Plex watchlists that turn into requests.
- Trakt lists and reviews.
- Alerts on Telegram, ntfy, Gotify, Pushover or a webhook.
- Subtitle sites and subtitle search.
- Sign in with Emby.
- Going back to the previous version in one click.
- Fixing a copy's edition by hand and protecting a copy from upgrades.
- Requesting a whole movie series at once.
- Sites with no feed of their own. Use Jackett or Prowlarr for those for now.
- Translations, starting with German.
- Built-in downloads, books and audiobooks, native phone apps and TV apps.

Features that are built but off can be turned on with the `HOLYARRS_FLAGS` setting, at your own risk. See [Held-back features](https://docs.HolyArrs.com/run/updates/#held-back-features) in the docs.
