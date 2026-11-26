// Builds src/lib/core/i18n/messages/en/server-lines.json: the English lines the
// server sends that a person reads (API errors, labels, health lines, card
// lines, event summaries). serverWords() in lib/core/i18n/server-words.ts maps them
// back to keys so the page can show them in the picked language.
// Run: node scripts/server-words.mjs  (then review the diff by hand).
// Check: node scripts/server-words.mjs --check  writes nothing and fails when the
// server sends a line the file lacks (the i18n tests run it).
// Lines already in the file keep their keys (the other languages hold them);
// a run only adds keys for new lines. Reword an old line by editing its value by hand.
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join, relative, sep } from 'node:path';

const root = new URL('../../backend/internal/', import.meta.url).pathname.replace(/^\/(\w:)/, '$1');
const out = new URL('../src/lib/core/i18n/messages/en/server-lines.json', import.meta.url).pathname.replace(/^\/(\w:)/, '$1');
// Words the app never shows (scripts/check-words.mjs); a line holding one is left out.
const banned = ['Monitored', 'Activity', 'Wanted', 'Cutoff', 'Quality Profile', 'Root Folder', 'Download Client',
	'Interactive Search', 'Manual Import', 'Grabbed', 'Connections', 'Discover', 'Series'];
// Codes and names the scan picks up that no one reads as words (setting values,
// ids, kinds); they stay out of the file and the check.
const notWords = new Set(['dash', 'each', 'range', 'smart', 'remove', 'normal', 'key', 'movie', 'show', 'artist',
	// The audio codec and HDR labels a file name carries (mediafiles/naming_media.go): names, not words to translate.
	'AAC', 'AC3', 'EAC3', 'DTS', 'DTS-HD', 'TrueHD', 'FLAC', 'ALAC', 'MP3', 'Opus', 'Vorbis', 'PCM', 'DolbyVision', 'HDR10', 'HDR10+', 'HLG',
	// Device families in a Pick rules points line (mediafiles/device_play.go): product names.
	'Apple TV', 'Fire TV', 'Roku', 'Chromecast',
	// A release-name word quoted in a releasename comment, never sent.
	'COMPLETE.BLURAY',
	// Language names Gestdown is asked by (subtitles/sites/adapters.go langNames): sent to the site, never shown.
	'Bulgarian', 'Catalan', 'Chinese (Simplified)', 'Chinese (Traditional)', 'Croatian', 'Euskera', 'Galego', 'Portuguese (Brazilian)', 'Romanian', 'Serbian', 'Slovenian',
	'podnapisi', 'titlovi', 'gestdown', 'yifysubtitles', 'tvsubtitles', 'bsplayer', 'napiprojekt', 'napisy24', 'assrt', 'qBittorrent', 'Jackett', 'subtitles:languages', 'subtitle site ', 'tmdb:{v}', 'mbid:{v}',
	// migrate past_history.go: the old app's column names, never shown.
	'movieid', 'seriesid',
	// Answers only a plugin program reads (its token, hook and bridge routes) and scope names: for plugin authors, never shown in the app.
	"hands-off mode is on, so plugins can't change media files", "plugin token not accepted", "this acts for a person, and no one has linked this plugin to their account", "this plugin is calling too often; try again in a minute", "bad title id", "could not read the body", "could not read the people list", "downloads.read", "library.read", "method must be GET, POST, PUT or DELETE", "no such file", "no such plugin page", "not available", "only a running plugin calls this", "plugin pages are used from a signed-in session", "requests.read", "send {\"job\": \"...\", \"data\": ...} or {\"job\": \"...\", \"error\": \"...\"}", "send {\"method\": \"GET\", \"path\": \"/api/v1/...\"}", "system.health.read", "this link has ended", "this page has ended; open it again", "this plugin was not given the {scope} permission",
	"a plugin acts for a person only on requests and follows", "only you can remove your plugin links", "only you can see your plugin links", "send {\"external\": \"<who this is in your service>\"}", "that person may not do this", "this acts for a person: send {pluginforuserheader} for someone who linked this plugin", "plugins may only use folders the owner gave them",
	"this plugin has too many calls open; try again when one ends", "this plugin does not give remote access",
	"send {\"subject\": \"...\", \"text\": \"...\"}: up to 100 and 1000 characters, no markup", "the message could not be sent", "this plugin sent too many messages this hour",
	// migrate errNotChecked: a dry run's own marker, never shown (the page says how many shows are still being checked).
	'still being checked']);
// Lines the scan cannot put together: a control's Event starts with the person's name (setup ControlEvent)
// and takes its verb from a map; a removal's reason may name who removed it.
const handLines = { 'Because you have {name}': 'suggest', 
	// suggest rows.go RowTitle/TabTitle, build.go and titles.go fixedSubtitle: the plain row titles and head-row subtitles,
	// put together from a genre (lower case), a decade or a name. server-words.ts only fills a {genre} with a known genre.
	...Object.fromEntries(['Top picks for you', 'Trending this week everywhere', 'Because you asked for {name}', 'More from {name}', '{genre} picks for you',
		'Popular {genre}', 'Popular {decade}s {genre}', '{decade}s {genre}', 'New on your services', 'Best new releases', 'Popular in your household', 'Hidden gems',
		'Trending on streaming', 'Finish the collection', 'Coming to cinemas and streaming', 'New seasons and premieres', 'Returning soon', 'Classics',
		'Trending movies this week', 'Trending shows this week', 'Trending movies on streaming', 'Trending shows on streaming',
		'Matched to your Library and asks', 'This week, mostly {genre}', 'What people watch this week', 'The parts missing from collections you have',
		'In the next 90 days', 'Shows you have that are airing, then new shows', 'Back in the next two months'].map((l) => [l, 'suggest'])),
	// discover ttl.go Titles and explore_genre_routes.go: Explore row titles; explore_new.go NewFresh and home.go: card badges ({date} is "Jan 2").
	...Object.fromEntries(['Trending this week', 'Popular', 'Because you have it', 'More in this genre', 'On your services',
		'Trending {genre}', 'Best new {genre}', 'Top-rated {genre} classics',
		'Out {date}', 'Added {n}d ago', 'Season {season}', 'S{season}E{episode} · {date}'].map((l) => [l, 'discover'])),
	// TMDB departments, a person's line on the people page.
	...Object.fromEntries(['Acting', 'Directing', 'Writing', 'Production', 'Art', 'Camera', 'Editing', 'Costume & Make-Up', 'Crew', 'Visual Effects',
		'Lighting', 'Creator'].map((l) => [l, 'metadata'])),
	
	// The folder tags note (the scan misreads its two apostrophes) and its older wording, still in stored History.
	"No Source this search can use has this folder's tags, so Sources without the folder's tags were asked.": 'sources',
	"No Source this search can use has this folder's tags, so Sources without tags were asked too.": 'sources', 'No Downloader has confirmed this Release yet. You can choose another one.': 'sources', 'Deleted by hand': 'downloads', 'Marked bad': 'downloads',
	'{name} paused the stage {stage}': 'accounts', '{name} resumed the stage {stage}': 'accounts',
	'{name} restarted the stage {stage}': 'accounts', 'Download removed': 'downloads', 'Download removed by {name}': 'downloads',
	// metadata ErrTraktNoSecret: sent through traktFail as err.Error(), so the scan cannot see it.
	"Add your Trakt app's client secret in Settings first; Trakt needs it to keep you signed in.": 'metadata',
	// outbound.FakeIPMessage: a name that resolved to 198.18.x.x (Clash or Surge fake-IP mode).
	'Your router or VPN is sending HolyArrs to a fake address (198.18.x.x). This usually comes from Clash or Surge. Add {host} to its direct list.': 'metadata',
	// A finished download removed by hand: the files it kept, and why a remove is refused (downloads remove_done.go).
	'Download removed by {name}; its files were kept: {why}': 'downloads', 'Download removed; its files were kept: {why}': 'downloads',
	// Why the files stayed (seed_cleanup.go keepFilesWhy, remove_done.go doneKeepWhy), said inside the line above.
	'{path} is a Library folder or is in or around one': 'downloads', '{path} is a whole save folder': 'downloads',
	'{name} did not say where they are': 'downloads', 'HolyArrs could not read its Library folders': 'downloads',
	'HolyArrs could not check whether another torrent uses them': 'downloads', 'another torrent in {name} uses {path}': 'downloads',
	'HolyArrs could not reach {name}': 'downloads',
	// requests streaming.go: My streaming services, and an ask turned down because it streams on one of them.
	'pick the country your streaming services are in': 'requests', 'pick up to 30 streaming services': 'requests',
	'pick at least one streaming service under My streaming services, then save': 'requests',
	"Not downloaded: it's on {name}. Approve it again to download it anyway.": 'requests',
	"It's on {name}, one of the streaming services set in Requests settings, so it was not downloaded. Approve it again to download it anyway.": 'requests',
	"HolyArrs is putting this download's files in the library; remove it once that is done": 'downloads',
	'this Downloader is read-only: turn on "HolyArrs can change this Downloader" in its settings to remove it from here': 'downloads',
	// migrate seasonsNote: a show found only by the old app's TMDB number, with and without files to place.
	"{d}; found only by the old app's TMDB number, as {other}: check that it is the same show. If it is, its seasons are numbered differently here, so its files wait in Leftover files for you to place; if it is not, remove it. Keep up to date is off and the old app's season choices were not used, so nothing is downloaded until you turn it on": 'migrate',
	"{d}; found only by the old app's TMDB number, as {other}: check that it is the same show, and remove it if it is not. Its seasons may be numbered differently here, so no file is placed on an episode by its numbers. Keep up to date is off and the old app's season choices were not used, so nothing is downloaded until you turn it on": 'migrate',
	// metadata.ErrServiceSlow, answered through library.ServiceError.
	'The metadata server is slow right now. This comes back on its own.': 'metadata',
	// metadata.BusyLine: the server's reason for asking HolyArrs to wait, answered by discover.Fail and library.ServiceError.
	'The metadata server is not signing up more installs from this address right now. HolyArrs asks again on its own; keep the /config folder between restarts so this install keeps its sign-up.': 'metadata',
	'The metadata server has no room for new installs right now. HolyArrs asks again on its own within the hour.': 'metadata',
	'This address sent the metadata server too many requests. HolyArrs waits and asks again on its own.': 'metadata',
	'This install sent the metadata server too many requests. HolyArrs waits and asks again on its own.': 'metadata',
	'The metadata server is busy right now. HolyArrs asks again on its own in a moment.': 'metadata',
	// requests.ErrNotAddedSlow, answered when an approval could not add its title.
	'The metadata server did not answer in time. It is back in Waiting; approve it again.': 'requests',
	// requests views.go: the reason a request whose title was deleted from the Library will not arrive.
	'It was deleted from the Library after it was approved.': 'requests',
	// importer.PlaceError, the refusal of a folder Import by hand may not read.
	"Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders, and none is set up yet. Add one in Settings > Library folders.": 'importer',
	"Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders in Settings > Library folders: {all}": 'importer',
	// importer refusedMedia: the file name and its refusal, put together at run time.
	'HolyArrs refused {name}: the Downloader reported a file outside the download folder': 'importer',
	// place_why.go: why a file was not placed (targetTakenWhy, noWriteWhy, noReadWhy, notThereWhy) and the lead for the system's own words.
	'a file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again': 'importer',
	'HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again': 'importer',
	'the file could not be placed: {why}': 'importer',
	'HolyArrs may not read the downloaded file; give the user HolyArrs runs as (PUID and PGID) read access to the file and the folder it is in, then import again': 'importer',
	'the file could not be placed': 'importer',
	// place_why.go: what follows that lead when the file was gone or could not be read to check (notInFolderWhy, unreadCheckWhy).
	'the file is not in the download folder any more': 'importer', 'could not read the file to check it': 'importer',
	// files_pending.go: a listed file that has not reached the local download folder yet.
	'the file is not present yet; HolyArrs will try importing it again': 'importer',
	// importer.go place: what else follows that lead; a slot holds the system's own words. shows_import.go: the same lead naming the episode or track.
	'the source is not a plain file (a symlink or something else)': 'importer', 'a file appeared at the target': 'importer',
	'hardlink failed ({a}), copy failed ({b})': 'importer', 'hardlink not possible: {why}': 'importer',
	'the file for {episode} could not be placed: {why}': 'importer',
	// manual.go: why Import this file was refused, kept on the card after that lead (review_import.go).
	'the current file is protected; turn its protection off first': 'importer', 'the new path would leave the Library folder': 'importer',
	'the new path would be too long ({n} characters, the limit here is {max})': 'importer', 'this is the file it already has': 'importer',
	'this already has a file; replace it or pick another Version': 'importer', 'already a file in the Library': 'importer',
	'the current file could not be moved to the holding folder: {why}': 'importer',
	// import_job.go downloadGoneWhy, and downloads.ErrNotWaiting answered to Import again.
	'the download HolyArrs was importing is gone; search again or import the files by hand': 'importer',
	'this download does not wait for an import': 'downloads',
	"HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again": 'importer',
	// unread_reason.go: no file list and no way to read the download's folder, and the lines when no file matched.
	"HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and {name} has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again": 'importer',
	"HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and the Downloader has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again": 'importer',
	'no file in the download matched this movie; match the files by hand': 'importer',
	'no file in the download matched an episode; match the files by hand': 'importer',
	'no file in the download matched a track; match the files by hand': 'importer',
	// place_why.go SlowReadWhy: Import again read the download folder for too long.
	'reading the download folder took too long; try again': 'importer',
	// fake.go: why a file looks like a fake; a slot holds a size, a file type or a resolution.
	'the file is empty; likely a fake': 'importer', 'the file is {n} KB, too small to be a track; likely a fake': 'importer',
	'the file is {size}, too small to be the full video; likely a fake': 'importer',
	'a {ext} file where the Release said {p}p; likely a fake': 'importer',
	'the Release said {claimed} but holds {held} of video or audio; likely a fake': 'importer',
	// Two lines of a Release's reasons (mediafiles judge, sources search_shows), shown in the Release lists.
	'holds {covered}, not what was searched for': 'sources', 'no Title in the library is called {value}': 'mediafiles',
	// mediafiles judge.go: a release from a group blocked in the Pick rules.
	'the group {group} is blocked in these Pick rules': 'mediafiles',
	// mediafiles judge.go and judge_steps.go: a Release's step, points and good-enough reasons; a {step} gap holds a
	// ladder step as the Pick rules page names it (StepWord), and server-words.ts shows it in the picked language.
	'{step} is allowed': 'mediafiles', '{step} is not allowed in these Pick rules': 'mediafiles',
	"{step} is lower than the current file's {step2}": 'mediafiles', 'reaches good enough, which starts at {step}': 'mediafiles',
	'below good enough, which starts at {step}': 'mediafiles', 'below good enough, which needs {n} points': 'mediafiles',
	'{n} points': 'mediafiles', '{n} points (needs at least {min})': 'mediafiles', '{n} points, needs at least {min}': 'mediafiles',
	// mediafiles judge.go seedersLine: a torrent's seeders against the Pick rules' minimum, "seeder" for one.
	'{n} seeder, at least {min}': 'mediafiles', '{n} seeders, at least {min}': 'mediafiles',
	'{n} seeder, needs at least {min}': 'mediafiles', '{n} seeders, needs at least {min}': 'mediafiles',
	// sources name_check.go reasonsOf and routes_search.go: a reason line with the points it gives or takes ("+10").
	'{line} ({n} points)': 'mediafiles',
	// mediafiles ShowYearMisfit: the year after a show's name is another show's.
	'the name says {year}; this show began in {began}': 'mediafiles',
	// requests: the Quality row's refusals and the approver's note (qualities.go, approve.go CopyKept).
	'a quality name can be at most 30 characters': 'requests', 'keep at least one quality shown for movies and one for shows': 'requests',
	'give every quality you added a name': 'requests', 'movies and shows can have at most 8 qualities each': 'requests',
	'leave at least one quality this person can pick for movies and one for shows': 'requests',
	'that quality cannot be asked for': 'requests', 'someone already asked for this movie in that quality; decide their ask instead': 'requests',
	'The quality you picked was not used: this title already has that copy.': 'requests',
	// accounts hosts.go hostRefusedMsg: the name and the env name are joined in.
	"Saving is blocked because HolyArrs doesn't know the name {host}. Open HolyArrs at its local address and add this name under Settings > Sign-in and security > Allowed host names, or add it to HOLYARRS_ALLOWED_HOSTS.": 'accounts',
	// sources ask_sources.go folderTagsNote: a search that passed over its Library folder's tags.
	"No Source this search can use has this folder's tags, so Sources without tags were asked too.": 'sources',
	// sources copy_sources.go: a folder kept to tagged Sources (a Slot's reason and a search's line, with its three causes), and ask_sources.go noAnswer.
	"not sent: its Source is not one this folder's tags allow": 'sources',
	"no Source takes this title's tags: {failures}": 'sources',
	"This folder only takes Sources tagged {tags}. None can be searched right now ({sources}), so nothing was asked.": 'sources',
	"{source} only has a feed": 'sources',
	"{source} has search turned off": 'sources',
	"{source} is paused": 'sources',
	"{source} is paused by you": 'sources',
	"{source}: its tags ({tags}) are not on this title": 'sources',
	"{source}: its tags ({tags}) are not in the Pick rules' tags ({rules})": 'sources',
	// downloads grab.go: why no Downloader takes a Release now (its causes are lines of their own), and ask_sources.go noTagsWhy.
	"no {protocol} Downloader can take it now: {skipped}": 'downloads',
	"{source}: it has no tags, and the Pick rules only take Sources tagged {rules}": 'sources',
	// downloads client_choice.go order: a Downloader that rests after failing.
	"{source} is resting until {time} after failing": 'downloads',
	// sources choose.go: a forced pick's reasons, each ending in the verdict text.
	// doctor.CategoryLine, the Category line with the missing category's name.
	'Category missing: {name}. Add it in the download app, or pick another one here.': 'core',
	// doctor.NameLine, a failed test line that shows what the address's name turned into.
	"{name} → not found. HolyArrs can't find this name. Use the IP instead.": 'core',
	'{name} → {ip} → refused on port {port}. The name works, but nothing answers on that port. Check the port.': 'core',
	'{name} → {ip} → no answer on port {port}. The name works, but nothing answers in time. Check the port, and that no firewall is in the way.': 'core',
	'it was turned down: {d}': 'sources', 'it was turned down for this episode: {d}': 'sources',
	// library adopt_why.go: why an imported folder was not brought in, and the scan's line while lookups fail.
	'The server HolyArrs gets details from is not answering. Scan again in a few minutes to bring this folder in.': 'library',
	'The server HolyArrs gets details from is not answering, so some folders could not be looked up. Scan again in a few minutes.': 'library',
	'{name} is already in the Library in another folder': 'library',
	// updates: in-app update refusals and failures, shown in the Status block and in toasts.
	...Object.fromEntries([
		'the download link is not allowed', 'not enough free space in the config folder for the {size} MB download',
		'the download does not match the update list, so it was deleted', "the download's signature could not be read",
		'the download is not signed by HolyArrs, so it was deleted', 'the new version could not open your data, so it was not installed: {err}',
		'the download stopped part way', 'the download has the wrong size, so it was deleted', 'too many redirects',
		'the download moved to a host that is not allowed', 'the download server did not answer', 'the download server answered {status}',
		'no update is downloaded and checked yet', 'this build is updated by its package manager', 'a download is already running',
		"this is the image's own version; there is nothing to go back to", 'still busy with {what}; try again when it finishes', '{n} imports',
		'the database could not be copied, so nothing was changed: {err}',
		'this version changed the database and no copy from before it is kept, so going back is not safe',
		'switching versions in place only works on Linux', 'HOLYARRS_UPDATES sets the update mode, so it cannot be changed here',
		'the update mode is off, ask or auto', 'no newer version', 'this version was withdrawn', 'you went back from this version, so it is skipped',
		'this version needs a new image: pull it to update', 'update to {version} first (pull that image), then this one',
		'this version keeps its data in a new format: pull the new image to update', 'this version has no download for {platform}'
	].map((line) => [line, 'updates'])),
	// accounts: what a Jellyfin or Emby address Test answers (media_server_people.go) and the first-run refusal of an address (setup_outside.go).
	'The server did not answer at this address.': 'accounts', 'Something answered, but it does not look like {word}.': 'accounts',
	'that address did not answer like {word}; check it and try again': 'accounts',
	// accounts mediaOffLine: the account is turned off on the Jellyfin or Emby server.
	'Your account is turned off on the {word} server. Ask the person who runs it to turn it back on.': 'accounts' };
// mediaservers service.go: why the Test of a media server failed.
for (const line of ['the server did not answer', 'the server is on the local network, which it may not reach', 'reading the server answer failed',
	"the server's answer was too big to read", 'the server turned down the token', 'the server answered {code}', 'the token could not be read',
	"paste the server's token (Plex) or API key (Jellyfin, Emby)", 'the address must be an http or https link with a host, like http://192.168.1.10:32400'])
	handLines[line] = 'mediaservers';
// Files whose fmt.Sprintf lines are display text (card lines, labels).
const display = /(accounts[\\/]events|discover[\\/](home|explore_np|title_view)|shows[\\/]episode_labels|requests[\\/](episodes|follows\w*|reports)|health[\\/](health|routes_settings_status)|migrate[\\/]plan_\w+|metadata[\\/]moods)\.go$/;

function walk(dir, files = []) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) walk(p, files);
		else if (p.endsWith('.go') && !p.endsWith('_test.go')) files.push(p);
	}
	return files;
}

// Reads one Go expression from i: up to a top-level , ) } or a line end not after +.
function expr(src, i) {
	let depth = 0, j = i, last = '';
	for (; j < src.length; j++) {
		const c = src[j];
		if (c === '"' || c === '`') {
			j = str(src, j).end - 1;
			last = '"';
			continue;
		}
		if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) {
			if (depth === 0) break;
			depth--;
		} else if (c === ',' && depth === 0) break;
		else if (c === '\n' && depth === 0 && last !== '+') break;
		if (!/\s/.test(c)) last = c;
	}
	return src.slice(i, j).trim();
}

function str(src, i) {
	const q = src[i];
	let j = i + 1;
	while (j < src.length && src[j] !== q) j += src[j] === '\\' && q === '"' ? 2 : 1;
	const raw = src.slice(i + 1, j);
	const text = q === '`' ? raw : raw.replace(/\\(["\\nt])/g, (_, c) => (c === 'n' ? '\n' : c === 't' ? '\t' : c));
	return { text, end: j + 1 };
}

function parts(e) {
	const ps = [];
	let depth = 0, start = 0;
	for (let j = 0; j < e.length; j++) {
		const c = e[j];
		if (c === '"' || c === '`') j = str(e, j).end - 1;
		else if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) depth--;
		else if (c === '+' && depth === 0) {
			ps.push(e.slice(start, j).trim());
			start = j + 1;
		}
	}
	ps.push(e.slice(start).trim());
	return ps;
}

function slotName(e) {
	if (/\b(?:byName|userName)\(/.test(e)) return 'name';
	const inner = e.match(/\(([^()]*)\)\s*$/);
	const ids = (inner && inner[1].trim() ? inner[1] : e).match(/[A-Za-z_]\w*/g) ?? ['x'];
	return ids[ids.length - 1].toLowerCase();
}

function sprintf(e) {
	const m = e.match(/^fmt\.Sprintf\(\s*("(?:[^"\\]|\\.)*"|`[^`]*`)\s*,?([\s\S]*)\)$/);
	if (!m) return null;
	const fmt = str(m[1], 0).text;
	const args = [];
	let rest = m[2].trim();
	while (rest) {
		const a = expr(rest + ')', 0);
		args.push(a);
		rest = rest.slice(a.length).replace(/^\s*,/, '').trim();
		if (!a) break;
	}
	let k = 0;
	return fmt.replace(/%%|%[-+# 0]*\d*(?:\.\d+)?[dsvqfx]/g, (v) => (v === '%%' ? '%' : `{${slotName(args[k++] ?? 'x')}}`));
}

// The English template of an expression, or null when it holds no words.
function template(e) {
	let text = '', words = false;
	for (const p of parts(e)) {
		if (/^("(?:[^"\\]|\\.)*"|`[^`]*`)$/.test(p)) {
			text += str(p, 0).text;
			words = true;
		} else if (p.startsWith('fmt.Sprintf(')) {
			const s = sprintf(p);
			if (s == null) return null;
			text += s;
			words = true;
		} else {
			text += `{${slotName(p)}}`;
		}
	}
	if (!words) return null;
	// Each slot name once, so a line can be filled back by name.
	const seen = new Map();
	return text.trim().replace(/\{(\w+)\}/g, (_, n) => {
		const c = (seen.get(n) ?? 0) + 1;
		seen.set(n, c);
		return `{${c > 1 ? n + c : n}}`;
	});
}

function readable(t, error = false) {
	if (!t || t.includes('\n') || t.length > 300) return false;
	if (migrateExtra.includes(t)) return true;
	if (!(error ? /^[A-Za-z{]/ : /^[A-Z{]/).test(t)) return false;
	if ((t.replace(/\{\w+\}/g, '').match(/[A-Za-z]/g) ?? []).length < 3) return false;
	if (/%[wdsvq]|\{x\}|^\{\w+\}$|\b[a-z]+_[a-z_]+\b|\b(sql|SELECT|json|nil)\b/.test(t)) return false;
	return !banned.some((w) => t.includes(w));
}


// The person-facing lines of internal/migrate: result details, reasons, notes, next steps.
const migrateFile = /[\\/]internal[\\/]migrate[\\/][^\\/]+\.go$/;
// Words kept for slots (a line's gap is filled with these) and a few bare labels.
const migrateExtra = ['(missing)', '(no name)', '(none supported)', 'announced', 'in cinemas', 'released', "this server's default"];

// The top-level arguments of the call or literal whose opening bracket sits just before i.
function args(src, i) {
	const out = [];
	let depth = 0, start = i, j = i;
	for (; j < src.length; j++) {
		const c = src[j];
		if (c === '"' || c === '`') {
			j = str(src, j).end - 1;
			continue;
		}
		if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) {
			if (depth === 0) break;
			depth--;
		} else if (c === ',' && depth === 0) {
			out.push(src.slice(start, j).trim());
			start = j + 1;
		}
	}
	out.push(src.slice(start, j).trim());
	return out.filter(Boolean);
}

function migratePass(file, src, addLine) {
	const at = (re, f) => {
		for (const m of src.matchAll(re)) f(m.index + m[0].length);
	};
	const one = (i) => addLine(template(expr(src, i)));
	// Result words next to a result: return ResX, "..."  result(err, ResX, "...")  r.line(..., ResX, "...", "")
	at(/\bRes[A-Za-z]+,\s*(?=["`]|fmt\.Sprintf|[a-z]\w*\s*\+)/g, one);
	// A not-imported line: what it is, why it is left out.
	at(/\bb\.not\(/g, (i) => {
		const a = args(src, i);
		if (a.length < 2) return;
		addLine(template(a[a.length - 1]));
		const name = template(a[a.length - 2]);
		if (name && (name.replace(/\{\w+\}/g, '').match(/[A-Za-z]/g) ?? []).length >= 8) addLine(name);
	});
	// Fields and locals that hold a sentence.
	at(/\b(?:Detail|Why|Name|Needs)\s*:\s*(?=["`]|fmt\.Sprintf|[a-z]\w*\s*\+)/g, one);
	at(/\b(?:detail|why|reason|check|was|needs|values|d|it\.Needs|ap\.Note)\s*:?=\s*(?=["`]|fmt\.Sprintf)/g, one);
	at(/\bwhat\s*:?=\s*(?=["`])/g, one);
	at(/\b(?:scores|out)\s*=\s*(?=fmt\.Sprintf)/g, one);
	// Next steps and notes: lists and appends.
	at(/\b(?:Next|Notes)\s*(?::|=)\s*\[\]string\{/g, (i) => args(src, i).forEach((a) => addLine(template(a))));
	at(/\bappend\(\s*(?:out|rep\.Next|\*?\w+\.notes|\w+\.Notes|\w+\.NeedsReentry|b\.p\.Notes|s\.Notes|out\.notes|\*t\.notes)\s*,\s*/g, one);
	// Errors a person reads.
	at(/\b(?:Err|err)[A-Z]\w*\s*=\s*errors\.New\(/g, one);
	at(/\bconst\s+\w+Note\s*=\s*/g, one);
	at(/\bkept[A-Z]\w*\s*=\s*/g, one);
	// A music issue: the third part is the sentence.
	at(/\bmusicIssue\{/g, (i) => {
		const a = args(src, i);
		if (a.length === 3) addLine(template(a[2]));
	});
	// return "label", "sentence"   and   return "", "", "sentence"
	at(/\breturn\s+"[^"]*",\s*(?=["`])/g, one);
	at(/\breturn\s+"",\s*"",\s*(?=")/g, one);
	// Small tables: a map literal indexed in place, and the gate words.
	for (const m of src.matchAll(/map\[string\]string\{([^}]*)\}\[/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) addLine(str(v[1], 0).text.trim());
	for (const m of src.matchAll(/var\s+gateWords\s*=\s*map\[[^\]]+\]string\s*\{([^}]*)\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) addLine(str(v[1], 0).text);
	// The group names in lower case ("you left library folders out").
	if (/plan_types\.go$/.test(file))
		for (const m of src.matchAll(/Group\w+:\s*("(?:[^"\\]|\\.)*")/g)) {
			const w = str(m[1], 0).text;
			if (w.toLowerCase() !== w) addLine(w.toLowerCase());
		}
	// The address-read failures.
	if (/fetch\.go$/.test(file)) at(/\breturn\s+(?=")/g, one);
}

// The lenient test for migrate lines: lower case starts are fine, SQL and code are not.
function migrateKeep(t) {
	if (!t || t.includes('\n') || /\b(SELECT|INSERT|UPDATE|DELETE|WHERE|Job|job queue)\b|^[?, ]+$/.test(t)) return false;
	return migrateExtra.includes(t) || /[a-z]{3}/.test(t.replace(/\{\w+\}/g, ''));
}

const found = new Map(); // text -> module
function add(file, t, error = false) {
	const mod = relative(root, file).split(sep)[0];
	// arrapi answers Sonarr/Radarr-style apps, not people.
	if (mod === 'arrapi' || notWords.has(t) || !readable(t, error) || found.has(t)) return;
	found.set(t, mod);
}

for (const file of walk(root)) {
	const src = readFileSync(file, 'utf8');
	const at = (re, f) => {
		for (const m of src.matchAll(re)) f(m.index + m[0].length);
	};
	// API errors.
	at(/\b(?:fail|webserver\.Fail\w*)\(\s*w\s*,\s*[^,]+,\s*/g, (i) => add(file, template(expr(src, i)), true));
	// Event summaries (the bell and history).
	at(/\bSummary:\s*/g, (i) => add(file, template(expr(src, i))));
	at(/\bsummary\s*:?=\s*/g, (i) => add(file, template(expr(src, i))));
	// The line of an Event written through a helper: a.event(r, by, kind, line), accountEvent(...), h.Event(...), m.event(...).
	at(/\b(?:a\.event|accountEvent|h\.Event|m\.event)\(/g, (i) => {
		const a = args(src, i);
		if (a.length >= 4) add(file, template(a[3]));
	});
	// Labels.
	at(/\b\w*Label:\s*(?=")/g, (i) => add(file, template(expr(src, i))));
	for (const m of src.matchAll(/var\s+\w*(?:Labels?|Words|Names)\s*=\s*map\[[^\]]+\]string\s*\{([\s\S]*?)\n\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) add(file, str(v[1], 0).text);
	// Why a synced trait was left out, and the test kinds named with it.
	at(/\bReason:\s*(?=")/g, (i) => add(file, template(expr(src, i)), true));
	for (const m of src.matchAll(/var\s+\w*Kinds\s*=\s*map\[[^\]]+\]string\s*\{([\s\S]*?)\n\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) add(file, str(v[1], 0).text, true);
	for (const v of src.matchAll(/\bbad\["([a-z][a-z ]+)"\]\s*=\s*true/g)) add(file, v[1], true);
	// What a pick warns about (the toast and the pick sheet) and why a send is refused (a stored reason shown in History).
	at(/\bWarnings\s*=\s*append\(\s*[\w.]+,\s*/g, (i) => add(file, template(expr(src, i)), true));
	if (/[\\/]downloads[\\/]/.test(file)) {
		at(/\breleaseRefused\{\s*/g, (i) => add(file, template(expr(src, i)), true));
		at(/\bvar\s+ErrOutsideTorrent\s*=\s*errors\.New\(\s*/g, (i) => add(file, template(expr(src, i)), true));
	}
	// The reasons inside a pick's "sent although it failed" warning.
	if (/[\\/]sources[\\/]choose\.go$/.test(file)) {
		at(/\boverridden\s*=\s*append\(\s*overridden,\s*(?=")/g, (i) => add(file, template(expr(src, i)), true));
		at(/\bwhy\s*(?::=|\+=)\s*(?=")/g, (i) => add(file, template(expr(src, i)), true));
	}
	// Health lines.
	at(/\bStatusItem\{\s*[^,]+,\s*[^,]+,\s*/g, (i) => add(file, template(expr(src, i))));
	if (migrateFile.test(file)) migratePass(file, src, (t) => t && migrateKeep(t) && add(file, t, true));
	if (display.test(file)) {
		at(/(?=fmt\.Sprintf\()/g, (i) => add(file, template(expr(src, i))));
		at(/\breturn\s+(?=")/g, (i) => add(file, template(expr(src, i))));
		if (/moods\.go$/.test(file)) for (const v of src.matchAll(/"([A-Z][A-Za-z &'-]+)"/g)) add(file, v[1]);
	}
}

for (const [t, mod] of Object.entries(handLines)) if (!found.has(t)) found.set(t, mod);

// The rotating row titles (suggest/titles/*.json, key|text|flags). A decade fills as "1990s", so the line keeps
// its number gap ("{decade}s"); a genre gap is a lower-case genre word. server-words.ts reads these as shaped lines.
for (const name of readdirSync(join(root, 'suggest', 'titles')).filter((n) => n.endsWith('.json')))
	for (const line of JSON.parse(readFileSync(join(root, 'suggest', 'titles', name), 'utf8')).templates) {
		const text = line.split('|')[1].replace(/\{decade\}/g, '{decade}s');
		if (!found.has(text)) found.set(text, 'titles');
	}
// The music row titles (music/rowtpl catalog.go, the fifth argument of t() and the .vars() wordings). A style gap
// ({genre}, {genre2}, {substyle}) holds a music style name, kept as written unless it is a known genre; a season
// or holiday name is {holiday}, as {season} is a season number in other lines.
{
	const src = readFileSync(join(root, 'music', 'rowtpl', 'catalog.go'), 'utf8');
	const lines = [...src.matchAll(/\bt\(\s*"[A-Z]\d+",\s*"\w+",\s*[\d.]+,\s*"\w*",\s*("(?:[^"\\]|\\.)*")/g)].map((m) => str(m[1], 0).text);
	for (const m of src.matchAll(/\.vars\(([^)]*)\)/g)) for (const v of m[1].matchAll(/"(?:[^"\\]|\\.)*"/g)) lines.push(str(v[0], 0).text);
	for (const l of lines) {
		// A title that is only a gap ("{mix}") has no words to translate.
		// A mood or season gap holds a name from the music data that no language knows yet, so those titles stay English.
		if (/^\{\w+\}$/.test(l) || /\{(mood|season)\}/.test(l)) continue;
		const text = l.replace(/\{genre\}/g, '{style}').replace(/\{genre2\}/g, '{style2}').replace(/\{substyle\}/g, '{style3}').replace(/\{name\}/g, '{style}').replace(/\{season\}/g, '{holiday}').replace(/\{decade\}/g, '{decade}s');
		if (!found.has(text)) found.set(text, 'musictitles');
	}
	// What fills a music title's {place}, {area} and {for} gaps (page_text.go place, page.go areaName and forOf).
	for (const l of ['worldwide', 'in {country}']) if (!found.has(l)) found.set(l, 'musictitles');
	const page = readFileSync(join(root, 'music', 'rowtpl', 'page.go'), 'utf8');
	for (const v of page.matchAll(/var (?:areaName|forOf) = map\[string\]string\{([^\n]*)\}/g))
		for (const w of v[1].matchAll(/:\s*"([^"]+)"/g)) if (!found.has(w[1])) found.set(w[1], 'musictitles');
}

// Keep every line already in the file under its key; give new lines new keys.
const words = existsSync(out) ? JSON.parse(readFileSync(out, 'utf8')) : {};
const keys = new Set(Object.keys(words));
const known = new Set(Object.values(words));
const fresh = [...found].filter(([text]) => !known.has(text)).sort((a, b) => a[1].localeCompare(b[1]) || a[0].localeCompare(b[0]));
if (process.argv.includes('--check')) {
	if (!fresh.length) {
		console.log(`server words: every line is in the file (${keys.size} lines)`);
		process.exit(0);
	}
	console.log(`server words: ${fresh.length} lines the server sends are missing from ${relative(process.cwd(), out)}:`);
	for (const [text, mod] of fresh) console.log(`  ${mod}: ${text}`);
	console.log('Run node scripts/server-words.mjs, then add the new keys to every language (or list a code in notWords).');
	process.exit(1);
}
let added = 0;
for (const [text, mod] of fresh) {
	let slug = text.replace(/\{\w+\}/g, ' ').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim().split(' ').slice(0, 6).join('-') || 'line';
	// A key ending in a secret word looks like a stored secret to the commit scan.
	if (/(password|passwd|token|secret|key)$/.test(slug)) slug += '-line';
	let key = `server.${mod}.${slug}`;
	for (let n = 2; keys.has(key); n++) key = `server.${mod}.${slug}-${n}`;
	keys.add(key);
	words[key] = text;
	added++;
}
writeFileSync(out, JSON.stringify(words, null, '\t') + '\n');
console.log(`server words: ${keys.size} lines (${added} new) -> ${relative(process.cwd(), out)}`);
for (const t of known) if (!found.has(t)) console.log(`not found in the Go text any more: ${t}`);
