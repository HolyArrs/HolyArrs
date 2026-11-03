import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18741';
// The first-run test, run alone as the setup project.
const firstRun = /first run makes the owner, adds the key and folder, then returns Home/;

// The tests drive a real HolyArrs (built from this repo, fresh config) that
// serves the built app from web/dist; run `npm run build` first.
export default defineConfig({
	testDir: 'e2e',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: {
		baseURL: `http://127.0.0.1:${port}`,
		trace: 'retain-on-failure'
	},
	webServer: [
		{
			command: 'node e2e/server.mjs',
			url: `http://127.0.0.1:${port}/readyz`,
			reuseExistingServer: false,
			timeout: 180_000,
			stdout: 'ignore',
			stderr: 'pipe'
		},
		// The same app under /holyarrs behind a small reverse proxy (3.5.10).
		{
			command: 'node e2e/urlbase-server.mjs',
			url: `http://127.0.0.1:${process.env.HOLYARRS_E2E_BASE_PROXY_PORT ?? '18735'}/holyarrs/readyz`,
			reuseExistingServer: false,
			timeout: 180_000,
			stdout: 'ignore',
			stderr: 'pipe'
		}
	],
	projects: [
		// setup is the first app.spec test alone (first run: the owner, the parts, the Movies folder); every project builds on it.
		{ name: 'setup', testMatch: 'app.spec.ts', grep: firstRun, use: { ...devices['Desktop Chrome'] } },
		{ name: 'desktop', testMatch: 'app.spec.ts', grepInvert: firstRun, use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'musicplaceholder', testMatch: 'music-placeholder.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'phone', testMatch: 'phone.spec.ts', use: { ...devices['Pixel 7'] }, dependencies: ['desktop'] },
		{ name: 'accounts', testMatch: 'accounts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'a11y', testMatch: 'a11y.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'movies', testMatch: 'movies.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'lists', testMatch: 'lists.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// The Requests page at phone width: a member asks, the owner approves from a phone.
		{ name: 'requests', testMatch: 'requests.spec.ts', use: { ...devices['Pixel 7'] }, dependencies: ['setup'] },
		// Sources: add, test, a wrong key, pause, the health page, an rss feed.
		{ name: 'sources', testMatch: 'sources.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Search: Choose a release, Held and Turned down, Home, Settings > Search.
		{ name: 'search', testMatch: 'search.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['movies', 'sources'] },
		{ name: 'ai', testMatch: 'ai.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Downloaders: SABnzbd and qBittorrent fakes, the settings, Mark failed on a movie, phone width and accessibility.
		{ name: 'downloaders', testMatch: 'downloaders.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['sources'] },
		// Downloads screen and Needs a decision: live progress, Remove with its choices, the badge, phone width and accessibility.
		{ name: 'downloads', testMatch: 'downloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['downloaders'] },
		// Organise: import an existing library, rename, delete, naming settings; phone width, accessibility, members see none of it.
		{ name: 'organise', testMatch: 'organise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// History, health, the calendar and its phone feed, webhook Alerts and media servers; phone width and accessibility.
		{ name: 'history', testMatch: 'history.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['organise'] },
		// Shows: a show folder, add a show, seasons and episodes on and off, shows in the Library, a member asks for seasons from a phone and the owner approves from a phone.
		{ name: 'shows', testMatch: 'shows.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['movies'] },
		// Show search: Choose a release for a season (one pack fills both episodes), an episode turned down and searched again, Home and the Turned down list name the episode, a phone.
		{ name: 'showsearch', testMatch: 'showsearch.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['shows', 'downloaders'] },
		// Show downloads: the Downloads screen names the episodes a download covers, an episode imports, a stuck pack waits on Needs a decision as one card and is settled with one choice, a phone, the episode naming settings.
		{ name: 'showdownloads', testMatch: 'showdownloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showsearch', 'search'] },
		// Organise for shows: import a show folder (nothing moves), rename its episodes, delete an episode of a multi-episode file and a season, an episode's History, this week's episode on the Calendar, pack Alerts, a phone.
		{ name: 'showorganise', testMatch: 'showorganise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showdownloads'] },
		// Music: a music folder, Settings > Music, add an artist from Search, its albums, kinds and Pressings, artists in the Library, a member asks for one single from a phone and the owner approves from a phone.
		{ name: 'music', testMatch: 'music.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Music search: Choose a release on the album page (verdicts, one turned down, FLAC chosen for every track), Home and the Turned down list name Artist – Album, Search now on the artist page, the music settings, a phone.
		{ name: 'musicsearch', testMatch: 'musicsearch.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['music', 'downloaders'] },
		// Music downloads: the Downloads screen names Artist – Album and its tracks, an album is placed track by track, a stuck track waits on Needs a decision, a phone.
		{ name: 'musicdownloads', testMatch: 'musicdownloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['musicsearch', 'search'] },
		// Organise for music: music naming with a live example, rename an album's tracks from its page, import an artist folder (nothing moves, a single left out), delete a track to the holding folder, an album's History, album days on the Calendar, a phone, delete an artist with Never-add.
		{ name: 'musicorganise', testMatch: 'musicorganise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['musicdownloads'] },
		// Subtitles: English and Spanish as the default languages, the Pass marks, Subtitle sites (the lab's fake site, one that needs your account), every movie missing Spanish with Search all, a movie's and an episode's subtitles (languages, Choose a subtitle, Sync fix, Turn down), History, a phone, accessibility, members see none of it.
		{ name: 'subtitles', testMatch: 'subtitles.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['shows'] },
		// Several Versions per show: add a show with a 4K Version, both Versions searched with their own pack, History names the Version, change, add and remove Versions, a busy Version stays, a phone.
		{ name: 'showversions', testMatch: 'showversions.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['shows', 'downloaders'] },
		// Parts: Requests and Music turned off hide their menu entries and Settings tiles, a direct link says the part is off, its routes answer 409, and on again everything is back.
		{ name: 'parts', testMatch: 'parts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Move from other apps: read a fake old Radarr and Seerr, the plan with a suggested path rule, a dry run, the move and its report, a second try adds nothing, undo, a phone and accessibility.
		{ name: 'move', testMatch: 'move.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Parts and controls, and the first-run step: a part off and on with the confirm step, a refused switch, a stage paused and resumed, a linked app reconnected, the whole-app restart asked and cancelled, "Just movies" in one click, a phone and accessibility.
		{ name: 'controls', testMatch: 'controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// The rest of the app frame: signed-in devices, This app, Schedules, the log viewer, the API page and the update notice; a phone and accessibility.
		// The whole app under a URL base behind a reverse proxy: its own HolyArrs, no other project needed.
		{ name: 'urlbase', testMatch: 'urlbase.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'schedules', testMatch: 'schedules.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'copychoices', testMatch: 'copy-choices.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Explore memory: heap, DOM nodes and listeners back to the start after 20 Home, Explore and title rounds.
		{ name: 'home', testMatch: 'home.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['desktop'] },
		{ name: 'exploremem', testMatch: 'explore-memory.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['desktop'] },
		{ name: 'communitybrowse', testMatch: 'community-browse.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		{ name: 'appframe', testMatch: 'appframe.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Settings: the seven groups, setting search with a deep link and highlight, the save bar and its leave guard, Advanced kept per person, Danger zone, a phone and accessibility.
		{ name: 'settings', testMatch: 'settings.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Pick rules: copy Good everyday, change a step and a trait, save and reload, a mistake named on its field, remove the copy (asked first), a ready-made set cannot be removed, the music steps, a phone and accessibility.
		{ name: 'pickrules', testMatch: 'pick-rules.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['setup'] },
		// Kid accounts: invite a Kid (PG), join, the Library shows only titles rated up to PG, an R title and an ask for it are refused in plain words, no Music.
		{ name: 'kid', testMatch: 'kid.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['desktop'] },
		// Library tools: Movie series, Recently deleted and Import files by hand from the Add menu; a phone.
		{ name: 'librarytools', testMatch: 'library-tools.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['search', 'kid'] }
	]
});
