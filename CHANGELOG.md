# Changelog

All notable changes to HolyArrs are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [1.3.4] - 2026-11-26
### Added
- Outside links now sit higher on the title page. ([#745](https://github.com/HolyArrs/HolyArrs/issues/745))
- Music import shows which artist is which in the match list. ([#741](https://github.com/HolyArrs/HolyArrs/issues/741))
### Fixed
- 4K files with no quality in the name now show as 4K. ([#874](https://github.com/HolyArrs/HolyArrs/issues/874))
- Episode names from TheTVDB come in your language. ([#769](https://github.com/HolyArrs/HolyArrs/issues/769))
- Future-only show asks with nothing listed show Upcoming, not Processing. ([#716](https://github.com/HolyArrs/HolyArrs/issues/716))
- Deleted files leave the holding folder right on their keep time. ([#802](https://github.com/HolyArrs/HolyArrs/issues/802))
- Removing an artist stops its running refresh. ([#805](https://github.com/HolyArrs/HolyArrs/issues/805))
- Sync fix only retimes subtitles HolyArrs saved. ([#837](https://github.com/HolyArrs/HolyArrs/issues/837))
- Who watched what opens when two accounts share a name. ([#734](https://github.com/HolyArrs/HolyArrs/issues/734))
### Security
- Thanks to Andreas Døhl for reporting the signing-key gap fixed in 1.3.2. ([#839](https://github.com/HolyArrs/HolyArrs/pull/839))

## [1.3.3] - 2026-11-26
### Fixed
- Sources work through an HTTP proxy like Gluetun again. ([#768](https://github.com/HolyArrs/HolyArrs/issues/768))
- Pick rules in Check releases can be changed from Standard. ([#714](https://github.com/HolyArrs/HolyArrs/issues/714))
- SABnzbd downloads import when the job folder is the save folder. ([#861](https://github.com/HolyArrs/HolyArrs/issues/861))
- Knaben feed checks ask only for the kinds you want. ([#806](https://github.com/HolyArrs/HolyArrs/issues/806))
- Developer plugins with settings show the Settings button. ([#834](https://github.com/HolyArrs/HolyArrs/issues/834))
- Install on a developer plugin's card uses its folder. ([#835](https://github.com/HolyArrs/HolyArrs/issues/835))
- Owner hosts keep every port saved for a host name. ([#836](https://github.com/HolyArrs/HolyArrs/issues/836))
- History shows subtitle and cleanup details. ([#823](https://github.com/HolyArrs/HolyArrs/issues/823))
- Import takes a release the Pick rules chose as equal. ([#826](https://github.com/HolyArrs/HolyArrs/issues/826))
### Changed
- Cleanup says All languages, with a hint. ([#824](https://github.com/HolyArrs/HolyArrs/issues/824))

## [1.3.2] - 2026-11-26
### Added
- Import files by hand right from the Library Add menu. ([#692](https://github.com/HolyArrs/HolyArrs/issues/692))
- Movie files are checked before import: length, video and audio. ([#611](https://github.com/HolyArrs/HolyArrs/issues/611))
- Plugins can open a site's bot check for you. ([#865](https://github.com/HolyArrs/HolyArrs/issues/865)) ([#918](https://github.com/HolyArrs/HolyArrs/pull/918))
- Approved anime can go to its own folder. ([#855](https://github.com/HolyArrs/HolyArrs/pull/855))
- Delete a single movie file from the Library tab. ([#821](https://github.com/HolyArrs/HolyArrs/issues/821))
- Import a library keeps going with the page closed. ([#712](https://github.com/HolyArrs/HolyArrs/issues/712))
- Pick rules get an SDR 4K trait and catch more AI upscales. ([#610](https://github.com/HolyArrs/HolyArrs/issues/610))
- Pick anyway on a season search takes releases named for the show. ([#764](https://github.com/HolyArrs/HolyArrs/issues/764))
- YTS is built in as a movie search and feed. ([#817](https://github.com/HolyArrs/HolyArrs/pull/817))
- Titlovi joins the subtitle sites. ([#688](https://github.com/HolyArrs/HolyArrs/pull/688))
- The owner and an API key can come from files at start. ([#660](https://github.com/HolyArrs/HolyArrs/pull/660))
- Discord Alerts come as one card with the poster, and mark upgrades. ([#733](https://github.com/HolyArrs/HolyArrs/issues/733))
- The Alerts panel shows an alert's full text. ([#852](https://github.com/HolyArrs/HolyArrs/pull/852))
- Library alerts open their title; search alerts name show and episode. ([#830](https://github.com/HolyArrs/HolyArrs/issues/830))
- A plugin can bring its own download type. ([#864](https://github.com/HolyArrs/HolyArrs/pull/864))
- A plugin can list and remove its own links. ([#854](https://github.com/HolyArrs/HolyArrs/pull/854))
- The approve screen shows the remote-access warning. ([#699](https://github.com/HolyArrs/HolyArrs/issues/699))
- The No file lock tag explains what it means. ([#698](https://github.com/HolyArrs/HolyArrs/issues/698))
- Download counts show on first view and on Language Packs. ([#851](https://github.com/HolyArrs/HolyArrs/pull/851))
- A movie in a 1080p and a 4K Radarr becomes one title. ([#648](https://github.com/HolyArrs/HolyArrs/pull/648))
- Files keep scene name and group, and score with the original language. ([#783](https://github.com/HolyArrs/HolyArrs/issues/783))
- One Source per site when Prowlarr, Sonarr and Radarr are imported. ([#669](https://github.com/HolyArrs/HolyArrs/pull/669))
- Naming comes over with the Radarr-style words. ([#857](https://github.com/HolyArrs/HolyArrs/issues/857))
- Removing a copy's protection asks in the app's own dialog. ([#150](https://github.com/HolyArrs/HolyArrs/issues/150))
- Show settings get a Future seasons only option. ([#720](https://github.com/HolyArrs/HolyArrs/issues/720)) ([#721](https://github.com/HolyArrs/HolyArrs/issues/721))
### Fixed
- Back keeps you on the Explore or New & Popular tab. ([#753](https://github.com/HolyArrs/HolyArrs/pull/753))
- Each watch link shows once when a server is added twice. ([#717](https://github.com/HolyArrs/HolyArrs/issues/717))
- Rescan commands rescan the title, or the whole Library. ([#808](https://github.com/HolyArrs/HolyArrs/issues/808))
- Shows with hundreds of episodes on a network share now import. ([#682](https://github.com/HolyArrs/HolyArrs/issues/682))
- A show no longer takes another show's picture or episodes. ([#342](https://github.com/HolyArrs/HolyArrs/issues/342))
- Feed checks no longer time out on big libraries. ([#747](https://github.com/HolyArrs/HolyArrs/issues/747))
- SABnzbd speed goes only to jobs that are downloading. ([#838](https://github.com/HolyArrs/HolyArrs/issues/838))
- Empty Usenet job folders are deleted after a move import. ([#724](https://github.com/HolyArrs/HolyArrs/issues/724))
- Imports retry when files from a remote downloader arrive late. ([#829](https://github.com/HolyArrs/HolyArrs/pull/829))
- qBittorrent works behind proxies that compress answers, like qui. ([#846](https://github.com/HolyArrs/HolyArrs/pull/846))
- DUAL or MULTi names now say audio is checked at import. ([#797](https://github.com/HolyArrs/HolyArrs/issues/797))
- German DL keeps the original language in imported Pick rules. ([#858](https://github.com/HolyArrs/HolyArrs/issues/858))
- MULTi releases without your language are skipped before download. ([#750](https://github.com/HolyArrs/HolyArrs/pull/750))
- "amp" in a release name matches "&". ([#762](https://github.com/HolyArrs/HolyArrs/issues/762))
- Anime names with alt-titles or [Group][Title][Episode] read right. ([#763](https://github.com/HolyArrs/HolyArrs/issues/763))
- MKV audio with no language tag is read as English. ([#850](https://github.com/HolyArrs/HolyArrs/pull/850))
- IPTorrents sizes with odd spaces are read right. ([#833](https://github.com/HolyArrs/HolyArrs/pull/833))
- Single sign-on started from another address goes through. ([#708](https://github.com/HolyArrs/HolyArrs/issues/708))
- An expired sign-in goes to sign-in, not stuck on Reconnecting. ([#793](https://github.com/HolyArrs/HolyArrs/issues/793))
- Only warnings and errors light the Settings dot. ([#786](https://github.com/HolyArrs/HolyArrs/issues/786))
- Plugin limit labels stay whole and boxes stay on one line. ([#862](https://github.com/HolyArrs/HolyArrs/issues/862))
- The report says tags stay behind, not that HolyArrs has none. ([#672](https://github.com/HolyArrs/HolyArrs/issues/672))
- Forgetting saved keys while HolyArrs runs now says how. ([#676](https://github.com/HolyArrs/HolyArrs/issues/676))
- An empty show folder no longer breaks an import. ([#845](https://github.com/HolyArrs/HolyArrs/pull/845))
- Who watched what no longer breaks on two same-name accounts. ([#734](https://github.com/HolyArrs/HolyArrs/issues/734))
- Old import reports open, and plugin screens drop garbled symbols. ([#925](https://github.com/HolyArrs/HolyArrs/pull/925))
### Changed
- The key icon's bar starts at the ring's edge. ([#683](https://github.com/HolyArrs/HolyArrs/issues/683))
- Plain permission names, full-width own-server box, one-line TV shows button. ([#945](https://github.com/HolyArrs/HolyArrs/pull/945))
### Security
- Release builds keep signing keys apart and ship with provenance. ([#839](https://github.com/HolyArrs/HolyArrs/pull/839))

### Thanks

Thanks to [@dhruv1220](https://github.com/HolyArrs/HolyArrs/pull/812), [@NotCavnfox](https://github.com/HolyArrs/HolyArrs/pull/854), [@diblaze](https://github.com/HolyArrs/HolyArrs/pull/673), [@lancewilhelm](https://github.com/HolyArrs/HolyArrs/pull/840), [@mbartsch](https://github.com/HolyArrs/HolyArrs/pull/817), [@wai-lau](https://github.com/HolyArrs/HolyArrs/pull/772), [@wlatic](https://github.com/HolyArrs/HolyArrs/issues/783), [@wozniakpawel](https://github.com/HolyArrs/HolyArrs/pull/918), [u/Blegrand15](https://www.reddit.com/r/HolyArrs/), [@maaximal](https://github.com/HolyArrs/HolyArrs/pull/822), [@prantzos](https://github.com/HolyArrs/HolyArrs/pull/723), [@aguanitallc](https://github.com/HolyArrs/HolyArrs/pull/850), [@ansuehu](https://github.com/HolyArrs/HolyArrs/issues/717), [@aritmp](https://github.com/HolyArrs/HolyArrs/pull/750), [@bobberb](https://github.com/HolyArrs/HolyArrs/pull/660), [@gorpet](https://github.com/HolyArrs/HolyArrs/pull/688), [@sittingmongoose](https://github.com/HolyArrs/HolyArrs/pull/648), [@soybean-alien](https://github.com/HolyArrs/HolyArrs/pull/726), [@sunboyy](https://github.com/HolyArrs/HolyArrs/issues/683), [@toverux](https://github.com/HolyArrs/HolyArrs/issues/682), [@tydude001](https://github.com/HolyArrs/HolyArrs/issues/611), [@venzellucas](https://github.com/HolyArrs/HolyArrs/issues/611), to AlmostEpic89, chesh1r3k and Harybo on Discord, and to [xrandallx](https://community.holyarrs.com/t/398) on the forum.

## [1.3.1] - 2026-11-24
### Added
- Plugins can be told when titles are added, downloaded or requested. ([#707](https://github.com/HolyArrs/HolyArrs/issues/707))
- Developer mode runs the plugin you're building from a folder. ([#728](https://github.com/HolyArrs/HolyArrs/issues/728))
- Test versions let you try a plugin's next version first. ([#729](https://github.com/HolyArrs/HolyArrs/issues/729))
- A Cover art changed Alert, off until you turn it on. ([#730](https://github.com/HolyArrs/HolyArrs/issues/730))

## [1.3.0] - 2026-11-23
### Added
- A plugin system: add features in a sandbox. Tailscale is first. ([#114](https://github.com/HolyArrs/HolyArrs/issues/114))
- Sign in with Trakt, and see IMDb, Rotten Tomatoes and Metacritic scores. ([#556](https://github.com/HolyArrs/HolyArrs/issues/556)) ([#554](https://github.com/HolyArrs/HolyArrs/issues/554))
- Hands-off mode: HolyArrs sends the download and never touches a single file. ([#224](https://github.com/HolyArrs/HolyArrs/issues/224))
- Skip downloads on your streaming services, and spot ones to remove. ([#120](https://github.com/HolyArrs/HolyArrs/issues/120)) ([#143](https://github.com/HolyArrs/HolyArrs/issues/143))
- Requests say when releases were found but none fit the Pick rules. ([#501](https://github.com/HolyArrs/HolyArrs/issues/501))
- Outside lists can follow Trakt charts like trending and box office. ([#153](https://github.com/HolyArrs/HolyArrs/issues/153))
- Pick more than one quality when you ask for a title. ([#335](https://github.com/HolyArrs/HolyArrs/issues/335))
- Request a whole movie series in one go. ([#104](https://github.com/HolyArrs/HolyArrs/issues/104))
- Bigger browse-row arrows, and a setting to turn hover previews off. ([#541](https://github.com/HolyArrs/HolyArrs/issues/541))
- Rotating Home and Explore row titles in all 44 languages. ([#441](https://github.com/HolyArrs/HolyArrs/issues/441))
- Title pages list what a title contains, like nudity or gore. ([#122](https://github.com/HolyArrs/HolyArrs/issues/122))
- Owners and admins see which Explore rows get opened and requested. ([#155](https://github.com/HolyArrs/HolyArrs/issues/155))
- Library stats list watched titles you can clean up. ([#142](https://github.com/HolyArrs/HolyArrs/issues/142))
- Manual import: pick a folder, match files, then move or copy. ([#547](https://github.com/HolyArrs/HolyArrs/issues/547))
- Files on a streaming drive can be linked instead of copied. ([#548](https://github.com/HolyArrs/HolyArrs/issues/548))
- Movies can sit loose with no folders; Libraries can use letter folders. ([#379](https://github.com/HolyArrs/HolyArrs/issues/379)) ([#329](https://github.com/HolyArrs/HolyArrs/issues/329)) ([#419](https://github.com/HolyArrs/HolyArrs/issues/419))
- Watch a folder and import files dropped there. ([#133](https://github.com/HolyArrs/HolyArrs/issues/133))
- Default audio language for new titles, with bulk apply. ([#439](https://github.com/HolyArrs/HolyArrs/issues/439))
- Open a title in Plex, Jellyfin or Emby. ([#455](https://github.com/HolyArrs/HolyArrs/issues/455))
- {tmdb} and {tvdb} naming tokens, Plex-style braces and editions. ([#459](https://github.com/HolyArrs/HolyArrs/issues/459)) ([#514](https://github.com/HolyArrs/HolyArrs/issues/514))
- Full release dates on movie posters. ([#494](https://github.com/HolyArrs/HolyArrs/issues/494))
- Current file points shown next to good enough points. ([#465](https://github.com/HolyArrs/HolyArrs/issues/465))
- Library issues say what's wrong instead of To review. ([#517](https://github.com/HolyArrs/HolyArrs/issues/517))
- Big libraries can raise the write time limit with HOLYARRS_WRITE_CAP. ([#644](https://github.com/HolyArrs/HolyArrs/issues/644))
- Failed downloads reuse the search results already found. ([#127](https://github.com/HolyArrs/HolyArrs/issues/127))
- Better download controls on the Calendar. ([#404](https://github.com/HolyArrs/HolyArrs/issues/404))
- A close button on the Downloads detail pane. ([#588](https://github.com/HolyArrs/HolyArrs/issues/588))
- New guide: send downloads through a VPN with gluetun. ([#462](https://github.com/HolyArrs/HolyArrs/issues/462))
- Failed imports retry on their own, or tap Retry import. ([#602](https://github.com/HolyArrs/HolyArrs/issues/602))
- SABnzbd and NZBGet get a Remove after import switch. ([#662](https://github.com/HolyArrs/HolyArrs/issues/662))
- Choose a release shows every release's points and reasons. ([#539](https://github.com/HolyArrs/HolyArrs/issues/539)) ([#456](https://github.com/HolyArrs/HolyArrs/issues/456))
- Choose a release says when Pick rules changed since the search. ([#574](https://github.com/HolyArrs/HolyArrs/issues/574))
- Check how a release name is read, from a card or API. ([#538](https://github.com/HolyArrs/HolyArrs/issues/538))
- Pick rules can reorder quality steps. ([#386](https://github.com/HolyArrs/HolyArrs/issues/386))
- Pick rules can wait for a dub in your language. ([#128](https://github.com/HolyArrs/HolyArrs/issues/128))
- Pick rules can ask for French versions: VFF, VFQ, VFI, VF2. ([#544](https://github.com/HolyArrs/HolyArrs/issues/544))
- Releases with a resolution but no source are allowed as TV quality. ([#493](https://github.com/HolyArrs/HolyArrs/issues/493))
- 4K quality names say 2160p too. ([#454](https://github.com/HolyArrs/HolyArrs/issues/454))
- IPTorrents signs in with user ID and passkey. ([#522](https://github.com/HolyArrs/HolyArrs/issues/522))
- Alternative series titles are searched. ([#359](https://github.com/HolyArrs/HolyArrs/issues/359))
- New people get a short welcome on Home on their first visit. ([#500](https://github.com/HolyArrs/HolyArrs/issues/500))
- Single sign-on groups can set each person's role. ([#156](https://github.com/HolyArrs/HolyArrs/issues/156))
- Forgot password? gives a one-time code in the container log. ([#451](https://github.com/HolyArrs/HolyArrs/issues/451))
- Invite links can take several people, with no end date. ([#499](https://github.com/HolyArrs/HolyArrs/issues/499))
- Each schedule row shows its last run, time taken and result. ([#518](https://github.com/HolyArrs/HolyArrs/issues/518))
- Setup picks language, media server, old apps and folders; import runs alongside. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- Folders step: one column per part, with each folder's found count. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- Sources and Downloads show as rows, like Parts. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- Prowlarr and Jackett forms find the app and fetch its key. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- Setup finds Seerr and Overseerr, and retries before saying none. ([#596](https://github.com/HolyArrs/HolyArrs/issues/596))
- A "Tell me about new versions" switch next to the install mode. ([#387](https://github.com/HolyArrs/HolyArrs/issues/387))
- Unraid /config on /mnt/user starts on its own. ([#512](https://github.com/HolyArrs/HolyArrs/issues/512))
- The Mac start-up page shows how to skip the /config check. ([#210](https://github.com/HolyArrs/HolyArrs/issues/210))
- Release names in Choose a release open their page on the Source. ([#658](https://github.com/HolyArrs/HolyArrs/issues/658))
- Pick rules can block a release group, also from a release. ([#612](https://github.com/HolyArrs/HolyArrs/issues/612))
- Pick rules set a size per quality step, and replace files under the minimum. ([#614](https://github.com/HolyArrs/HolyArrs/issues/614))
- Pick rules can prefer releases your devices play without transcoding. ([#129](https://github.com/HolyArrs/HolyArrs/issues/129))
- Back from a movie keeps your Search words and results. ([#630](https://github.com/HolyArrs/HolyArrs/issues/630))
- Search prefers a pack that covers more missing seasons. ([#520](https://github.com/HolyArrs/HolyArrs/issues/520))
- Search by hand works on unreleased and same-day episodes. ([#546](https://github.com/HolyArrs/HolyArrs/issues/546))
- Episode Search says when a found release could not be sent. ([#578](https://github.com/HolyArrs/HolyArrs/issues/578))
- Import asks whether to keep your file names or rename them. ([#609](https://github.com/HolyArrs/HolyArrs/issues/609))
- Add an app finds old apps on your network, through Overseerr too. ([#596](https://github.com/HolyArrs/HolyArrs/issues/596))
- Bring your download history over from Sonarr and Radarr. ([#157](https://github.com/HolyArrs/HolyArrs/issues/157))
- Import full quality profiles from Sonarr and Radarr. ([#382](https://github.com/HolyArrs/HolyArrs/issues/382))
- Stop looking for deleted titles; the old apps' blocklist comes across. ([#562](https://github.com/HolyArrs/HolyArrs/issues/562))
- Big imports wait out the metadata server's limits and show how long. ([#607](https://github.com/HolyArrs/HolyArrs/issues/607))
- The old-app API takes Kometa's bulk add, edit and delete calls. ([#646](https://github.com/HolyArrs/HolyArrs/issues/646))
- Phone apps can search by hand and remove queue items. ([#650](https://github.com/HolyArrs/HolyArrs/issues/650))
- Clean subtitles after they're saved, per language, with Clean again. ([#331](https://github.com/HolyArrs/HolyArrs/issues/331))
- Polish and Chinese subtitle sites, and TVsubtitles finds results again. ([#560](https://github.com/HolyArrs/HolyArrs/issues/560))
- Default audio and subtitle language set in Plex or Jellyfin. ([#126](https://github.com/HolyArrs/HolyArrs/issues/126))
- History says what happened to each subtitle, and media servers refresh after a save. ([#624](https://github.com/HolyArrs/HolyArrs/issues/624))
- Navidrome and other Subsonic servers hear about new music. ([#125](https://github.com/HolyArrs/HolyArrs/issues/125))
- Each plugin gets only the access you approve when you install it. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- 87 permissions, and each one is refused outside its scope. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Hands-off mode keeps plugins from changing, moving or deleting files. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Link a plugin to your account to act for you. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Requests and History show "via plugin" when a plugin made them. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Download clients and Sources can be added "From a plugin". ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Plugin pages show on the plugin's card. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- A plugin update that asks for more waits for your approval. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- An activity log shows what each plugin did, allowed or refused. ([#431](https://github.com/HolyArrs/HolyArrs/issues/431))
- Remote access works for any tunnel plugin; change the address, or use plain http. ([#115](https://github.com/HolyArrs/HolyArrs/issues/115))
- arr API answers phone apps such as Helmarr. ([#552](https://github.com/HolyArrs/HolyArrs/issues/552))
- Kodi refreshes its library after each import. ([#652](https://github.com/HolyArrs/HolyArrs/issues/652))
- Add a Plex server by picking it from your Plex account. ([#618](https://github.com/HolyArrs/HolyArrs/issues/618))
- Automatic updates, and roll back an update in one click. ([#146](https://github.com/HolyArrs/HolyArrs/issues/146)) ([#154](https://github.com/HolyArrs/HolyArrs/issues/154))
- Title logos and taglines follow your display language. ([#513](https://github.com/HolyArrs/HolyArrs/issues/513))
- TheTVDB fields say which key to use, plus a Get a key link. ([#449](https://github.com/HolyArrs/HolyArrs/issues/449))
### Fixed
- hover previews no longer cover the row arrows. ([#542](https://github.com/HolyArrs/HolyArrs/issues/542))
- the Explore Add box always finishes loading. ([#447](https://github.com/HolyArrs/HolyArrs/issues/447)) ([#600](https://github.com/HolyArrs/HolyArrs/issues/600))
- Add from a hover card loads its defaults. ([#525](https://github.com/HolyArrs/HolyArrs/issues/525))
- request emails to several people go out one per person. ([#535](https://github.com/HolyArrs/HolyArrs/issues/535))
- Requests load past 200, and Approve all takes every one. ([#597](https://github.com/HolyArrs/HolyArrs/issues/597))
- Explore and Home fill in right after a fresh install. ([#586](https://github.com/HolyArrs/HolyArrs/issues/586))
- imports over a file row marked missing no longer fail. ([#670](https://github.com/HolyArrs/HolyArrs/issues/670))
- scans no longer freeze on two same-name folders. ([#452](https://github.com/HolyArrs/HolyArrs/issues/452)) ([#629](https://github.com/HolyArrs/HolyArrs/issues/629))
- collection packs import each movie in its own place. ([#505](https://github.com/HolyArrs/HolyArrs/issues/505)) ([#506](https://github.com/HolyArrs/HolyArrs/issues/506))
- moves check the whole file before removing the old one. ([#635](https://github.com/HolyArrs/HolyArrs/issues/635))
- adopting a movie folder never takes a trailer as the movie. ([#639](https://github.com/HolyArrs/HolyArrs/issues/639))
- a deleted title can be deleted again or added back. ([#613](https://github.com/HolyArrs/HolyArrs/issues/613))
- long Movies lists no longer hang Safari. ([#601](https://github.com/HolyArrs/HolyArrs/issues/601))
- the original-language chip shows a globe, not a speaker. ([#579](https://github.com/HolyArrs/HolyArrs/issues/579))
- files replaced by an upgrade show in Recently deleted. ([#489](https://github.com/HolyArrs/HolyArrs/issues/489))
- the Columns picker shows every column, Status included. ([#483](https://github.com/HolyArrs/HolyArrs/issues/483))
- folders are found on Linux whatever their letter case. ([#437](https://github.com/HolyArrs/HolyArrs/issues/437))
- AVI and M2TS files are read, not marked unreadable. ([#496](https://github.com/HolyArrs/HolyArrs/issues/496))
- big show imports no longer stop on "a write took too long". ([#446](https://github.com/HolyArrs/HolyArrs/issues/446))
- an https Downloader isn't switched to http by a 400 page. ([#667](https://github.com/HolyArrs/HolyArrs/issues/667))
- hybrid torrents in qBittorrent are no longer called removed. ([#649](https://github.com/HolyArrs/HolyArrs/issues/649))
- season packs no longer stick at Importing when an episode waits. ([#632](https://github.com/HolyArrs/HolyArrs/issues/632))
- Test on a Downloader checks what you just typed, address and password included, before you save. ([#628](https://github.com/HolyArrs/HolyArrs/issues/628)) ([#666](https://github.com/HolyArrs/HolyArrs/issues/666))
- other apps' downloads ask for a match only in your categories. ([#584](https://github.com/HolyArrs/HolyArrs/issues/584))
- long failure text stays inside its row. ([#585](https://github.com/HolyArrs/HolyArrs/issues/585))
- qBittorrent's temporary folder no longer breaks imports. ([#617](https://github.com/HolyArrs/HolyArrs/issues/617))
- aria2 magnets follow the real download, not the metadata step. ([#460](https://github.com/HolyArrs/HolyArrs/issues/460))
- rdt-client works again as a qBittorrent Downloader. ([#550](https://github.com/HolyArrs/HolyArrs/issues/550))
- qBittorrent closing an idle connection no longer stops grabs. ([#461](https://github.com/HolyArrs/HolyArrs/issues/461))
- seeding cleanup waits for the goal on paused torrents. ([#469](https://github.com/HolyArrs/HolyArrs/issues/469))
- partly imported torrents show what they wait for, and never hang. ([#521](https://github.com/HolyArrs/HolyArrs/issues/521))
- removed downloads leave the Downloads page. ([#508](https://github.com/HolyArrs/HolyArrs/issues/508))
- unpacked scene files named "group-title" import. ([#527](https://github.com/HolyArrs/HolyArrs/issues/527))
- a failed Move and Replace never puts the old file back. ([#637](https://github.com/HolyArrs/HolyArrs/issues/637))
- tag-routed folders on two disks stop giving false warnings. ([#504](https://github.com/HolyArrs/HolyArrs/issues/504))
- Movies and Shows on separate disks no longer warn about hardlinks. ([#664](https://github.com/HolyArrs/HolyArrs/issues/664))
- iPlayarr and other SABnzbd-like apps pass Test. ([#663](https://github.com/HolyArrs/HolyArrs/issues/663))
- accented titles like "Pokémon" find results. ([#440](https://github.com/HolyArrs/HolyArrs/issues/440)) ([#509](https://github.com/HolyArrs/HolyArrs/issues/509))
- apostrophes in show names no longer break Torznab searches. ([#271](https://github.com/HolyArrs/HolyArrs/issues/271))
- shows with a non-Latin original name search their other names too. ([#537](https://github.com/HolyArrs/HolyArrs/issues/537))
- releases with a local title plus the original title match. ([#484](https://github.com/HolyArrs/HolyArrs/issues/484))
- BRRip, WebHD and UHDBD names read the right quality. ([#543](https://github.com/HolyArrs/HolyArrs/issues/543)) ([#497](https://github.com/HolyArrs/HolyArrs/issues/497)) ([#453](https://github.com/HolyArrs/HolyArrs/issues/453)) ([#575](https://github.com/HolyArrs/HolyArrs/issues/575))
- French and Ukrainian language tags read right. ([#526](https://github.com/HolyArrs/HolyArrs/issues/526)) ([#468](https://github.com/HolyArrs/HolyArrs/issues/468))
- the all-reasons link no longer looks like a failure. ([#576](https://github.com/HolyArrs/HolyArrs/issues/576))
- the release sheet keeps its height while you filter. ([#577](https://github.com/HolyArrs/HolyArrs/issues/577))
- Choose a release with no Source says so at once. ([#530](https://github.com/HolyArrs/HolyArrs/issues/530))
- Test spots Prowlarr or Jackett on any port. ([#533](https://github.com/HolyArrs/HolyArrs/issues/533))
- a big Source can be deleted. ([#573](https://github.com/HolyArrs/HolyArrs/issues/573))
- big libraries open a title's release list fast. ([#470](https://github.com/HolyArrs/HolyArrs/issues/470))
- old search results are cleaned up daily. ([#532](https://github.com/HolyArrs/HolyArrs/issues/532))
- single sign-on with Google works. ([#568](https://github.com/HolyArrs/HolyArrs/issues/568))
- no more sign-outs about an hour after signing in. ([#599](https://github.com/HolyArrs/HolyArrs/issues/599))
- owners who set up with single sign-on can make protected changes. ([#570](https://github.com/HolyArrs/HolyArrs/issues/570))
- profile pictures up to 5 MB are accepted. ([#619](https://github.com/HolyArrs/HolyArrs/issues/619))
- the Plex sign-in window closes when sign-in ends. ([#595](https://github.com/HolyArrs/HolyArrs/issues/595))
- a new display name shows on Home. ([#540](https://github.com/HolyArrs/HolyArrs/issues/540))
- Jellyfin sign-in only says "account turned off" when it is. ([#529](https://github.com/HolyArrs/HolyArrs/issues/529))
- iPhone no longer zooms in when you tap a text field. ([#572](https://github.com/HolyArrs/HolyArrs/issues/572))
- container health checks pass with sign-in on, using the open /ping. ([#510](https://github.com/HolyArrs/HolyArrs/issues/510))
- coming back to a hidden tab reconnects at once. ([#480](https://github.com/HolyArrs/HolyArrs/issues/480))
- an imported Prowlarr key is reused only by the owner. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- even spacing in setup; the warning no longer touches buttons. ([#594](https://github.com/HolyArrs/HolyArrs/issues/594))
- the welcome page has no white strip on the right. ([#598](https://github.com/HolyArrs/HolyArrs/issues/598))
- Change edits an item where it sits in Settings. ([#592](https://github.com/HolyArrs/HolyArrs/issues/592))
- Tab moves from Password straight to Confirm password. ([#606](https://github.com/HolyArrs/HolyArrs/issues/606))
- German-only Pick rules turn away untagged English releases. ([#589](https://github.com/HolyArrs/HolyArrs/issues/589))
- upgrades follow points-then-quality for files you already have. ([#571](https://github.com/HolyArrs/HolyArrs/issues/571))
- RuTracker names with [year, ...] read the right title. ([#620](https://github.com/HolyArrs/HolyArrs/issues/620))
- Choose a release compares releases for the same episode only. ([#590](https://github.com/HolyArrs/HolyArrs/issues/590))
- episodes are freed after a failed season pack grab. ([#625](https://github.com/HolyArrs/HolyArrs/issues/625)) ([#616](https://github.com/HolyArrs/HolyArrs/issues/616))
- SABnzbd season packs in nested folders import every episode. ([#608](https://github.com/HolyArrs/HolyArrs/issues/608))
- Check again on a failed season pack skips a fresh download. ([#603](https://github.com/HolyArrs/HolyArrs/issues/603))
- an old two-episode file stays when one new episode fails. ([#633](https://github.com/HolyArrs/HolyArrs/issues/633))
- multi-episode files like 1x01-02 and S01E01-02 read as several episodes. ([#474](https://github.com/HolyArrs/HolyArrs/issues/474)) ([#495](https://github.com/HolyArrs/HolyArrs/issues/495))
- new episodes reuse an existing season folder. ([#549](https://github.com/HolyArrs/HolyArrs/issues/549))
- folder import finds shows with release details in the name. ([#536](https://github.com/HolyArrs/HolyArrs/issues/536))
- import keeps a Downloader that is already there, no second copy. ([#593](https://github.com/HolyArrs/HolyArrs/issues/593))
- import names the old folders this server can't reach. ([#587](https://github.com/HolyArrs/HolyArrs/issues/587))
- approved Seerr requests no longer come over as Waiting. ([#583](https://github.com/HolyArrs/HolyArrs/issues/583))
- Sonarr's Range and Prefixed Range episode styles are no longer swapped. ([#655](https://github.com/HolyArrs/HolyArrs/issues/655))
- episodes switched off in Sonarr stay off after import. ([#661](https://github.com/HolyArrs/HolyArrs/issues/661))
- format rules from Sonarr or Radarr backups keep their conditions. ([#654](https://github.com/HolyArrs/HolyArrs/issues/654))
- imported profiles with merged quality groups rank by points. ([#623](https://github.com/HolyArrs/HolyArrs/issues/623))
- Telegram, ntfy, Gotify and Pushover Alerts come across. ([#380](https://github.com/HolyArrs/HolyArrs/issues/380))
- shows, movies and music each keep their own naming. ([#485](https://github.com/HolyArrs/HolyArrs/issues/485))
- large Sonarr libraries import. ([#503](https://github.com/HolyArrs/HolyArrs/issues/503))
- slow lookups no longer stop an import. ([#486](https://github.com/HolyArrs/HolyArrs/issues/486))
- unmonitored Radarr movies get details and posters. ([#477](https://github.com/HolyArrs/HolyArrs/issues/477))
- ITA in a file name sets the language of untagged audio. ([#476](https://github.com/HolyArrs/HolyArrs/issues/476))
- imported SABnzbd and qBittorrent keep the right port and path. ([#375](https://github.com/HolyArrs/HolyArrs/issues/375))
- Chinese subtitles: pick Simplified, Traditional or Cantonese. ([#507](https://github.com/HolyArrs/HolyArrs/issues/507))
- Choose a subtitle always lists results. ([#511](https://github.com/HolyArrs/HolyArrs/issues/511))
- trailers no longer log a subtitle error. ([#471](https://github.com/HolyArrs/HolyArrs/issues/471))
- scanned music matches your artists' tracks; the rest is listed. ([#519](https://github.com/HolyArrs/HolyArrs/issues/519))
- music files can be ticked in Import files by hand. ([#406](https://github.com/HolyArrs/HolyArrs/issues/406))
- adopting one artist works. ([#490](https://github.com/HolyArrs/HolyArrs/issues/490))
- "Lisa Lisa" no longer scores 100% for "Lisa Knapp". ([#545](https://github.com/HolyArrs/HolyArrs/issues/545))
- backups over 2 GiB restore, and slow uploads finish. ([#642](https://github.com/HolyArrs/HolyArrs/issues/642))
### Changed
- a view picked on Movies, Shows or All applies everywhere. ([#564](https://github.com/HolyArrs/HolyArrs/issues/564))
- stalled downloads keep release and files; Status flags a stopped Downloader. ([#443](https://github.com/HolyArrs/HolyArrs/issues/443))
- a release that keeps stalling waits longer before each re-send. ([#580](https://github.com/HolyArrs/HolyArrs/issues/580))
- imported Downloaders keep the old apps' categories; missing ones are made. ([#591](https://github.com/HolyArrs/HolyArrs/issues/591))
- English is built in; Language packs adds the rest, picked in setup. ([#582](https://github.com/HolyArrs/HolyArrs/issues/582))

## 1.2.0

### Added

- Tap the red dot to see what needs attention. Fixed warnings clear. ([#409](https://github.com/HolyArrs/HolyArrs/issues/409)) ([#410](https://github.com/HolyArrs/HolyArrs/issues/410))
- Connection tests name the exact problem and what a name became. ([#411](https://github.com/HolyArrs/HolyArrs/issues/411)) ([#434](https://github.com/HolyArrs/HolyArrs/issues/434))
- Plex labels requested titles with your own label text. ([#407](https://github.com/HolyArrs/HolyArrs/issues/407))
- The owner's requests can wait for approval too. ([#421](https://github.com/HolyArrs/HolyArrs/issues/421))
- Add dialogs remember your last Search when added choice. ([#437](https://github.com/HolyArrs/HolyArrs/issues/437))
- New names can use your language, and {title_first} for first letters. ([#437](https://github.com/HolyArrs/HolyArrs/issues/437))
- File names can carry {audio} and {hdr}; dotted names lose double dots. ([#397](https://github.com/HolyArrs/HolyArrs/issues/397))
- Import files by hand works for music. ([#406](https://github.com/HolyArrs/HolyArrs/issues/406))
- Broken files are checked after download. ([#138](https://github.com/HolyArrs/HolyArrs/issues/138))
- See which Trakt list added a title. ([#124](https://github.com/HolyArrs/HolyArrs/issues/124))
- Shows can be kept apart from anime. ([#361](https://github.com/HolyArrs/HolyArrs/issues/361))
- A Watched mark on titles, and a Watched by anyone filter. ([#385](https://github.com/HolyArrs/HolyArrs/issues/385))
- Big libraries scan in minutes on first import. ([#425](https://github.com/HolyArrs/HolyArrs/issues/425))
- Send a Source's grabs to one chosen Downloader. ([#402](https://github.com/HolyArrs/HolyArrs/issues/402))
- Delete a person from Users. ([#378](https://github.com/HolyArrs/HolyArrs/issues/378))
- Connect apps over MCP with keys you control. ([#81](https://github.com/HolyArrs/HolyArrs/issues/81))
- Badges, dates and row titles show in all 44 languages. ([#352](https://github.com/HolyArrs/HolyArrs/issues/352))
- Say so when a router's proxy sends lookups to a fake address. ([#432](https://github.com/HolyArrs/HolyArrs/issues/432))
- Saving says why when HolyArrs is opened by an unknown name. ([#436](https://github.com/HolyArrs/HolyArrs/issues/436))
- Add your own certificate authority to the container. ([#389](https://github.com/HolyArrs/HolyArrs/issues/389))
- Run HolyArrs as a Proxmox LXC. ([#348](https://github.com/HolyArrs/HolyArrs/issues/348))
- Open a title in Jellyfin or Emby from its page. ([#415](https://github.com/HolyArrs/HolyArrs/issues/415))
- Outside lists can read MDBList lists. ([#417](https://github.com/HolyArrs/HolyArrs/issues/417))
- arr API works with more dashboards and phone apps. ([#333](https://github.com/HolyArrs/HolyArrs/issues/333))

### Changed

- A bracket in a file name drops out when all its words are empty, such as `[imdbid-{imdb}]` with no IMDb id. ([#397](https://github.com/HolyArrs/HolyArrs/pull/397))
- A torrent whose files are gone from the download folder waits in Review and names the missing file.
- The menu groups Downloads and History under Activity, and Status and Settings under System.
- Status has a Support tab with ways to help HolyArrs, and Support HolyArrs sits in the account menu.
- Phone layouts fit better: tighter tops on Home, Explore and New & Popular, and long tile names wrap.
- Small polish across Home, Library, Downloads, History and Settings: long labels wrap instead of being cut, one button and count style, a 24px Downloads menu target, and the More actions menu opens above the top bar.

### Fixed

- Requests no longer stay Failed after a movie is added back. ([#408](https://github.com/HolyArrs/HolyArrs/issues/408))
- Requests that came from Seerr can be removed. ([#365](https://github.com/HolyArrs/HolyArrs/issues/365))
- Movies reuse a folder that differs only by case or accents. ([#437](https://github.com/HolyArrs/HolyArrs/issues/437))
- Import a library looks in subfolders. ([#379](https://github.com/HolyArrs/HolyArrs/issues/379))
- Import a library handles huge libraries. ([#374](https://github.com/HolyArrs/HolyArrs/issues/374))
- Radarr imports link the files already on disk. ([#391](https://github.com/HolyArrs/HolyArrs/issues/391))
- Movie import uses title aliases, and Review can import the file. ([#308](https://github.com/HolyArrs/HolyArrs/issues/308))
- The Movies page opens fast. ([#363](https://github.com/HolyArrs/HolyArrs/issues/363))
- Poster pictures use the URL base. ([#336](https://github.com/HolyArrs/HolyArrs/issues/336))
- Cunk on Earth and similar shows import. ([#339](https://github.com/HolyArrs/HolyArrs/issues/339))
- Pick anyway on a movie sends it, with no episode error. ([#438](https://github.com/HolyArrs/HolyArrs/issues/438))
- NZBGet grabs work when a link redirects to another host. ([#423](https://github.com/HolyArrs/HolyArrs/issues/423))
- NZBGet gets post-processing settings in the right form. ([#422](https://github.com/HolyArrs/HolyArrs/issues/422))
- rdt-client imports with a leading slash work. ([#377](https://github.com/HolyArrs/HolyArrs/issues/377))
- The ghost torrent Downloader is gone. ([#413](https://github.com/HolyArrs/HolyArrs/issues/413))
- Downloads buttons work right after you allow changes. ([#395](https://github.com/HolyArrs/HolyArrs/issues/395))
- SABnzbd episodes import when the file name hides the episode. ([#312](https://github.com/HolyArrs/HolyArrs/issues/312))
- Shows with a country tag, like The Office (US), match releases. ([#424](https://github.com/HolyArrs/HolyArrs/issues/424))
- A REPACK ranks above the plain copy. ([#412](https://github.com/HolyArrs/HolyArrs/issues/412))
- 720p is no longer read as UHD; HUN reads as Hungarian. ([#414](https://github.com/HolyArrs/HolyArrs/issues/414))
- Show searches send IMDb and TMDB ids. ([#390](https://github.com/HolyArrs/HolyArrs/issues/390))
- A repost marker after the group is no longer the group. ([#400](https://github.com/HolyArrs/HolyArrs/issues/400))
- User invite settings are clearer. ([#337](https://github.com/HolyArrs/HolyArrs/issues/337))
- Make key in Linked apps works on phones. ([#384](https://github.com/HolyArrs/HolyArrs/issues/384))
- Explore waits when the metadata server says it is busy. ([#420](https://github.com/HolyArrs/HolyArrs/issues/420))
- Your own TMDB v3 key works. ([#403](https://github.com/HolyArrs/HolyArrs/issues/403))
- Turned-off backups stop making backups. ([#388](https://github.com/HolyArrs/HolyArrs/issues/388))
- The master key no longer reads missing after a restart. ([#366](https://github.com/HolyArrs/HolyArrs/issues/366))
- First-run Sonarr and Radarr import no longer signs the owner out. ([#345](https://github.com/HolyArrs/HolyArrs/issues/345))
- Allow local network blocks only local addresses. ([#373](https://github.com/HolyArrs/HolyArrs/issues/373))
- Hover previews stay behind the More like this popup. ([#394](https://github.com/HolyArrs/HolyArrs/issues/394))
- Test-only subtitle sites are gone from Settings. ([#383](https://github.com/HolyArrs/HolyArrs/issues/383))

### Docs

- Why an app name works or not when connecting. ([#433](https://github.com/HolyArrs/HolyArrs/issues/433))

### Thanks

Thanks to [@Yakouaho](https://github.com/HolyArrs/HolyArrs/issues/437), [@mrijke](https://github.com/HolyArrs/HolyArrs/pull/397), [@Jaylouisw](https://github.com/HolyArrs/HolyArrs/pull/405), [@biacz](https://github.com/HolyArrs/HolyArrs/pull/415), [@chadouming](https://github.com/HolyArrs/HolyArrs/pull/417), [@danielbaercgn](https://github.com/HolyArrs/HolyArrs/issues/308), [@krayhons](https://github.com/HolyArrs/HolyArrs/issues/308#issuecomment-6862553238), [@JulianAtkins](https://github.com/HolyArrs/HolyArrs/issues/325), [@johngully](https://github.com/HolyArrs/HolyArrs/issues/336), [@zaldev](https://github.com/HolyArrs/HolyArrs/issues/339), [@mimedo](https://github.com/HolyArrs/HolyArrs/issues/348#issuecomment-6862124869), [@stormbyte](https://github.com/HolyArrs/HolyArrs/issues/361), [@ArChAnG3LsFuRy](https://github.com/HolyArrs/HolyArrs/issues/363#issuecomment-6872787622), [@Cavamania](https://github.com/HolyArrs/HolyArrs/issues/373), [@riquecouto](https://github.com/HolyArrs/HolyArrs/issues/377), [@Nviviri](https://github.com/HolyArrs/HolyArrs/issues/390), [@chevalierbenoit](https://github.com/HolyArrs/HolyArrs/issues/390#issuecomment-6912724999), [@irwinthetapemeasure](https://github.com/HolyArrs/HolyArrs/issues/402), [@atocqueville](https://github.com/HolyArrs/HolyArrs/issues/413), [@levaib-git](https://github.com/HolyArrs/HolyArrs/issues/414), [@erichobson](https://github.com/HolyArrs/HolyArrs/issues/378), [@jorgytim](https://github.com/HolyArrs/HolyArrs/issues/379), [@michaeljmcauliffe-ctrl](https://github.com/HolyArrs/HolyArrs/issues/391), and to Nippo, 405 left hands, tekhop, Tharkys and GreyJoshy94 on Discord.

## 1.1.4

### Added

- Remove a request in any state, or clear all failed or finished ones. ([#365](https://github.com/HolyArrs/HolyArrs/issues/365))
- Friends' Plex watchlists are read again. ([#343](https://github.com/HolyArrs/HolyArrs/issues/343))

### Changed

- A folder's quality is a row on its card that shows the real value. ([#344](https://github.com/HolyArrs/HolyArrs/issues/344))
- Buttons, titles, switches and hints use one size across Settings, Explore and the Library.
- The image sizes in the README are the ones Docker Hub shows.

### Fixed

- An owner who set up with Plex, Jellyfin or Emby can make protected changes after signing in again. ([#364](https://github.com/HolyArrs/HolyArrs/issues/364))
- Setting a first password needs a fresh sign-in; an API key can no longer set one. ([#364](https://github.com/HolyArrs/HolyArrs/issues/364))
- Shows, movies and artists with an apostrophe in the name match their releases again. ([#369](https://github.com/HolyArrs/HolyArrs/issues/369))
- An episode airing later today no longer reads Yesterday for viewers ahead of the show's time zone. ([#368](https://github.com/HolyArrs/HolyArrs/issues/368))

### Thanks

Thanks to [@Nviviri](https://github.com/HolyArrs/HolyArrs/issues/369), who found the cause, and to roychassi and Lunim on Discord.

## 1.1.3

### Added

- Each Library folder can have its own quality. ([#344](https://github.com/HolyArrs/HolyArrs/issues/344))
- Import a library has Select all. ([#346](https://github.com/HolyArrs/HolyArrs/issues/346))

### Changed

- Library stats says "Unreadable files" and explains why.

### Fixed

- Skip sign-in at home: the password prompt has a way to a real sign-in. ([#353](https://github.com/HolyArrs/HolyArrs/issues/353))
- The size on the Library list drops when a smaller copy replaces a file. ([#347](https://github.com/HolyArrs/HolyArrs/issues/347))
- A show no longer picks up another show's picture, name or episodes. ([#342](https://github.com/HolyArrs/HolyArrs/issues/342))
- Shows from Sonarr with a wrong TMDB number import by their IMDb id or name. ([#339](https://github.com/HolyArrs/HolyArrs/issues/339))
- A season's Next date matches its episode in every time zone. ([#355](https://github.com/HolyArrs/HolyArrs/issues/355))
- A season counts every episode that has a file, and an episode with a file never says Not out yet. ([#358](https://github.com/HolyArrs/HolyArrs/issues/358))
- Undo this import counts only what that import added. ([#349](https://github.com/HolyArrs/HolyArrs/issues/349))
- Downloads names the same person in the list and the side pane. ([#350](https://github.com/HolyArrs/HolyArrs/issues/350))
- A toast over an open sheet stays readable. ([#351](https://github.com/HolyArrs/HolyArrs/issues/351))
- Sort & filter on the Library works in Safari. ([#340](https://github.com/HolyArrs/HolyArrs/issues/340))
- More toast and History lines show in your language. ([#352](https://github.com/HolyArrs/HolyArrs/issues/352))
- The request sheet turns off seasons that are already fully in the Library. ([#360](https://github.com/HolyArrs/HolyArrs/issues/360))

### Thanks

Thanks to [@zaldev](https://github.com/HolyArrs/HolyArrs/issues/339), [@ArBohe](https://github.com/HolyArrs/HolyArrs/issues/340), [@stormbyte](https://github.com/HolyArrs/HolyArrs/issues/358), [@johngully](https://github.com/HolyArrs/HolyArrs/issues/360), Lunim, Killjoy.69 and Bastien on Discord, and u/Agreeable_Mirror2257 on Reddit.

## 1.1.2

### Changed

- The image is smaller to download: about 16.1 MB, down from 18.6 MB.

### Fixed

- Members can request seasons of a show that is already in the Library. ([#338](https://github.com/HolyArrs/HolyArrs/issues/338))

### Thanks

Thanks to [@johngully](https://github.com/HolyArrs/HolyArrs/issues/338) for the report.

## 1.1.1

### Added

- Knaben, TorrentsCSV, SubsPlease and IPTorrents work without Prowlarr. ([#289](https://github.com/HolyArrs/HolyArrs/issues/289))
- Ask for shows in 4K, and pick a quality when you ask. ([#293](https://github.com/HolyArrs/HolyArrs/issues/293)) ([#335](https://github.com/HolyArrs/HolyArrs/issues/335))
- Profilarr can send its formats, profiles and settings to HolyArrs. ([#279](https://github.com/HolyArrs/HolyArrs/issues/279))
- Leftover files show up, and you can match them to episodes. ([#284](https://github.com/HolyArrs/HolyArrs/issues/284))
- Import a library has the Library's four views and big posters. ([#324](https://github.com/HolyArrs/HolyArrs/issues/324))
- Library stats sits in the Library menu. ([#321](https://github.com/HolyArrs/HolyArrs/issues/321))
- qBittorrent: category after import, others' torrents left alone, subtitles come along. ([#292](https://github.com/HolyArrs/HolyArrs/issues/292))
- Old-app formats come over with exact scores; tags pick each title's Sources. ([#291](https://github.com/HolyArrs/HolyArrs/issues/291))
- Set the listen port with HOLYARRS_PORT or HOLYARRS_ADDR. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))

### Fixed

- People from Seerr keep their Plex, Jellyfin or Emby sign-in. ([#334](https://github.com/HolyArrs/HolyArrs/issues/334))
- Import members reads people from Plex again. ([#341](https://github.com/HolyArrs/HolyArrs/issues/341))
- Requests from Seerr show real names, years and posters. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- An approval that adds nothing says why. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Posters view stays light in big libraries, Safari too. ([#328](https://github.com/HolyArrs/HolyArrs/issues/328))
- Rename files no longer reads every file first. ([#332](https://github.com/HolyArrs/HolyArrs/issues/332))
- Episodes count as missing only after they air. ([#320](https://github.com/HolyArrs/HolyArrs/issues/320))
- Movies under a translated name import; by-hand import can replace files. ([#308](https://github.com/HolyArrs/HolyArrs/issues/308))
- Downloads left on a removed Downloader re-link, or can be removed. ([#309](https://github.com/HolyArrs/HolyArrs/issues/309))
- A grab that never reaches a Downloader says so, never Sent. ([#310](https://github.com/HolyArrs/HolyArrs/issues/310))
- SABnzbd downloads import with the episodes they were grabbed for. ([#312](https://github.com/HolyArrs/HolyArrs/issues/312))
- A path pair with the same folder twice gets a note. ([#265](https://github.com/HolyArrs/HolyArrs/issues/265))
- A torrent the client already holds is followed when picked. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- A refused import shows why and offers Import files by hand. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Downloads keeps its bar and tabs while the list loads. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Season packs are found, likely fakes skipped, stuck choices replaceable. ([#307](https://github.com/HolyArrs/HolyArrs/issues/307))
- Feed checks grab date-named daily episodes and say what they skip. ([#314](https://github.com/HolyArrs/HolyArrs/issues/314))
- A failed Source test stays failed after a reload. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Backup alerts stop once backups are off. ([#318](https://github.com/HolyArrs/HolyArrs/issues/318))
- Metadata set to Off raises no alerts. ([#326](https://github.com/HolyArrs/HolyArrs/issues/326))
- A slow metadata server reads as slow, not down. ([#325](https://github.com/HolyArrs/HolyArrs/issues/325))
- Folders HolyArrs can't see come over as wanted, or stay behind. ([#238](https://github.com/HolyArrs/HolyArrs/issues/238))
- Confirming your password goes on with the import. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Settings tabs wrap on phones instead of hiding. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- The sign-in language button is bigger on touch screens. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Long toasts wrap instead of being cut off. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))
- Reworded hints are updated in all 44 languages. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))

### Changed

- Subtitle sites and subtitle searching start turned on. ([#322](https://github.com/HolyArrs/HolyArrs/issues/322))

### Thanks

Thanks to everyone who reported, tested and asked for these: [@eddoww](https://github.com/HolyArrs/HolyArrs/issues/289), [@JulianAtkins](https://github.com/HolyArrs/HolyArrs/issues/318), [@stormbyte](https://github.com/HolyArrs/HolyArrs/issues/284), GreyJoshy94 on Discord, [@mandopatriot](https://github.com/HolyArrs/HolyArrs/issues/279), [@mimedo](https://github.com/HolyArrs/HolyArrs/issues/291#issuecomment-6816138650), [@danielbaercgn](https://github.com/HolyArrs/HolyArrs/issues/308), [@stevelucky](https://github.com/HolyArrs/HolyArrs/issues/309), [@Nviviri](https://github.com/HolyArrs/HolyArrs/issues/314), [@zhu327](https://github.com/HolyArrs/HolyArrs/issues/325#issuecomment-6854023071), [@nevpeek](https://github.com/HolyArrs/HolyArrs/issues/324), [@p5y6ry52pt-hue](https://github.com/HolyArrs/HolyArrs/issues/328), [@sparecycles-dev](https://github.com/HolyArrs/HolyArrs/issues/332), Lunim, Mr_Mennens_Tech, roychassi and tekhop on Discord, and u/Blegrand15 on Reddit.

## 1.1.0

### Added

- HolyArrs speaks 44 languages, right-to-left included. ([#280](https://github.com/HolyArrs/HolyArrs/issues/280))
- aria2 works as a Downloader for torrents and magnets. ([#233](https://github.com/HolyArrs/HolyArrs/issues/233))
- Alerts reach Telegram, ntfy, Gotify, Pushover or a webhook. ([#151](https://github.com/HolyArrs/HolyArrs/issues/151)) ([#203](https://github.com/HolyArrs/HolyArrs/issues/203))
- Pulsarr and Plex friends' watchlists can become requests. ([#200](https://github.com/HolyArrs/HolyArrs/issues/200))
- Hide titles and genres you pick from Explore and search. ([#121](https://github.com/HolyArrs/HolyArrs/issues/121))
- Titles and synopses show in each person's language. ([#270](https://github.com/HolyArrs/HolyArrs/issues/270))
- Pick a show's episode order: aired, DVD or original. ([#204](https://github.com/HolyArrs/HolyArrs/issues/204))
- A Library stats page shows file types and who watched what. ([#214](https://github.com/HolyArrs/HolyArrs/issues/214))
- Change a Library folder's path without importing again. ([#275](https://github.com/HolyArrs/HolyArrs/issues/275))
- Library and Calendar views follow you to every device. ([#273](https://github.com/HolyArrs/HolyArrs/issues/273))
- Library import shows posters while it scans. ([#305](https://github.com/HolyArrs/HolyArrs/issues/305))
- Season packs download a few episodes at a time, in order. ([#193](https://github.com/HolyArrs/HolyArrs/issues/193))
- Remove torrents after seeding, with an optional dead torrent rule. ([#196](https://github.com/HolyArrs/HolyArrs/issues/196))
- Search follows each Version's audio languages and reads 18 more. ([#202](https://github.com/HolyArrs/HolyArrs/issues/202))
- Sources can be turned off if you add downloads by hand. ([#278](https://github.com/HolyArrs/HolyArrs/issues/278))
- Skip sign-in on your home network or listed addresses. ([#262](https://github.com/HolyArrs/HolyArrs/issues/262))
- Run without the HolyArrs metadata server. ([#206](https://github.com/HolyArrs/HolyArrs/issues/206))
- Use your own metadata keys or server. ([#222](https://github.com/HolyArrs/HolyArrs/issues/222))
- Backups can be turned off. ([#261](https://github.com/HolyArrs/HolyArrs/issues/261))
- The qBittorrent step takes an API key or a user name. ([#260](https://github.com/HolyArrs/HolyArrs/issues/260))
- A smaller download, even with 44 languages. ([#281](https://github.com/HolyArrs/HolyArrs/issues/281))
- An experimental native Synology package is attached to this release. ([#294](https://github.com/HolyArrs/HolyArrs/issues/294))
- Three more subtitle sites: YIFY Subtitles, TVsubtitles and BSPlayer. ([#290](https://github.com/HolyArrs/HolyArrs/issues/290))
- Translate subtitles with your own AI key. ([#218](https://github.com/HolyArrs/HolyArrs/issues/218))

### Fixed

- Tapping Explore on a phone no longer opens the keyboard. ([#282](https://github.com/HolyArrs/HolyArrs/issues/282))
- Clearly named episode files import. ([#272](https://github.com/HolyArrs/HolyArrs/issues/272))
- A second movie copy by quality imports. ([#274](https://github.com/HolyArrs/HolyArrs/issues/274))
- Music releases with disc counts in the name are read right. ([#269](https://github.com/HolyArrs/HolyArrs/issues/269))
- Adding a show links the files already in its folder. ([#298](https://github.com/HolyArrs/HolyArrs/issues/298))
- Shows added from TVMaze work. ([#296](https://github.com/HolyArrs/HolyArrs/issues/296))
- TVMaze posters load. ([#297](https://github.com/HolyArrs/HolyArrs/issues/297))
- Big Library imports, 2,700+ folders, show their results. ([#306](https://github.com/HolyArrs/HolyArrs/issues/306))
- RAR releases with a Sample folder are no longer called fake. ([#249](https://github.com/HolyArrs/HolyArrs/issues/249))
- Downloads let go once another app took the files. ([#257](https://github.com/HolyArrs/HolyArrs/issues/257))
- Failed Usenet clears after import are tried again. ([#276](https://github.com/HolyArrs/HolyArrs/issues/276))
- A by-hand move says copied when the file was copied. ([#277](https://github.com/HolyArrs/HolyArrs/issues/277))
- Searches drop apostrophes, so "Schitt's Creek" finds releases. ([#271](https://github.com/HolyArrs/HolyArrs/issues/271))
- A search with no title never reaches a Source. ([#307](https://github.com/HolyArrs/HolyArrs/issues/307))
- Key kit and backup downloads explain a missing password. ([#283](https://github.com/HolyArrs/HolyArrs/issues/283))
- Placeholders and turned-off controls are easier to read. ([#300](https://github.com/HolyArrs/HolyArrs/issues/300))
- Settings pages no longer squeeze their columns. ([#304](https://github.com/HolyArrs/HolyArrs/issues/304))
- Settings search no longer crashes. ([#268](https://github.com/HolyArrs/HolyArrs/issues/268))
- Subtitles stop looking for titles removed from the Library. ([#285](https://github.com/HolyArrs/HolyArrs/issues/285))

### Changed

- Big rTorrent seedboxes get much lighter background reads. ([#266](https://github.com/HolyArrs/HolyArrs/issues/266))

### Docs

- The install guide starts with one /data folder. ([#264](https://github.com/HolyArrs/HolyArrs/issues/264))
- Proxy variables and why HolyArrs ignores them. ([#267](https://github.com/HolyArrs/HolyArrs/issues/267))

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
