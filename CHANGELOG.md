# Changelog

All notable changes to HolyArrs are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

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
