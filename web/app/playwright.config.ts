import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18741';

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
			url: `http://127.0.0.1:${port}/healthz`,
			reuseExistingServer: false,
			timeout: 180_000,
			stdout: 'ignore',
			stderr: 'pipe'
		},
		// The same app under /holyarrs behind a small reverse proxy (3.5.10).
		{
			command: 'node e2e/urlbase-server.mjs',
			url: 'http://127.0.0.1:18735/holyarrs/healthz',
			reuseExistingServer: false,
			timeout: 180_000,
			stdout: 'ignore',
			stderr: 'pipe'
		}
	],
	projects: [
		{ name: 'desktop', testMatch: 'app.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'phone', testMatch: 'phone.spec.ts', use: { ...devices['Pixel 7'] }, dependencies: ['desktop'] },
		{ name: 'accounts', testMatch: 'accounts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['phone'] },
		{ name: 'a11y', testMatch: 'a11y.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['accounts'] },
		{ name: 'movies', testMatch: 'movies.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['a11y'] },
		{ name: 'lists', testMatch: 'lists.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['movies'] },
		// The Wishlist at phone width: a member asks, the owner approves from a phone.
		{ name: 'wishlist', testMatch: 'wishlist.spec.ts', use: { ...devices['Pixel 7'] }, dependencies: ['lists'] },
		// Sources: add, test, a wrong key, pause, the health page, an rss feed.
		{ name: 'sources', testMatch: 'sources.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['wishlist'] },
		// Search: Choose a release, Held and Turned down, Home, Settings > Search.
		{ name: 'search', testMatch: 'search.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['sources'] },
		// Downloaders: SABnzbd and qBittorrent fakes, the settings, Mark failed on a movie, phone width and accessibility.
		{ name: 'downloaders', testMatch: 'downloaders.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['search'] },
		// Downloads screen and Needs a decision: live progress, Remove with its choices, the badge, phone width and accessibility.
		{ name: 'downloads', testMatch: 'downloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['downloaders'] },
		// Organise: import an existing library, rename, delete, naming settings; phone width, accessibility, members see none of it.
		{ name: 'organise', testMatch: 'organise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['downloads'] },
		// History, health, the calendar and its phone feed, webhook Alerts and media servers; phone width and accessibility.
		{ name: 'history', testMatch: 'history.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['organise'] },
		// Shows: a show folder, add a show, seasons and episodes on and off, shows in the Library, a member asks for seasons from a phone and the owner approves from a phone.
		{ name: 'shows', testMatch: 'shows.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['history'] },
		// Show search: Choose a release for a season (one pack fills both episodes), an episode turned down and searched again, Home and the Turned down list name the episode, a phone.
		{ name: 'showsearch', testMatch: 'showsearch.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['shows'] },
		// Show downloads: the Downloads screen names the episodes a download covers, an episode imports, a stuck pack waits on Needs a decision as one card and is settled with one choice, a phone, the episode naming settings.
		{ name: 'showdownloads', testMatch: 'showdownloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showsearch'] },
		// Organise for shows: import a show folder (nothing moves), rename its episodes, delete an episode of a multi-episode file and a season, an episode's History, this week's episode on the Calendar, pack Alerts, a phone.
		{ name: 'showorganise', testMatch: 'showorganise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showdownloads'] },
		// Music: a music folder, Settings > Music, add an artist from Search, its albums, kinds and Pressings, artists in the Library, a member asks for one single from a phone and the owner approves from a phone.
		{ name: 'music', testMatch: 'music.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showorganise'] },
		// Music search: Choose a release on the album page (verdicts, one turned down, FLAC chosen for every track), Home and the Turned down list name Artist – Album, Search now on the artist page, the music settings, a phone.
		{ name: 'musicsearch', testMatch: 'musicsearch.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['music'] },
		// Music downloads: the Downloads screen names Artist – Album and its tracks, an album is placed track by track, a stuck track waits on Needs a decision, a phone.
		{ name: 'musicdownloads', testMatch: 'musicdownloads.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['musicsearch'] },
		// Organise for music: music naming with a live example, rename an album's tracks from its page, import an artist folder (nothing moves, a single left out), delete a track to the holding folder, an album's History, album days on the Calendar, a phone, delete an artist with Never-add.
		{ name: 'musicorganise', testMatch: 'musicorganise.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['musicdownloads'] },
		// Subtitles: English and Spanish as the default languages, the Pass marks, Subtitle sites (the lab's fake site, one that needs your account), every movie missing Spanish with Search all, a movie's and an episode's subtitles (languages, Choose a subtitle, Sync fix, Turn down), History, a phone, accessibility, members see none of it.
		{ name: 'subtitles', testMatch: 'subtitles.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['musicorganise'] },
		// Several Versions per show: add a show with a 4K Version, both Versions searched with their own pack, History names the Version, change, add and remove Versions, a busy Version stays, a phone.
		{ name: 'showversions', testMatch: 'showversions.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['subtitles'] },
		// Parts: the Wishlist and Music turned off hide their menu entries and Settings tiles, a direct link says the part is off, its routes answer 409, and on again everything is back.
		{ name: 'parts', testMatch: 'parts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['showversions'] },
		// Move from other apps: read a fake old Radarr and Seerr, the plan with a suggested path rule, a dry run, the move and its report, a second try adds nothing, undo, a phone and accessibility.
		{ name: 'move', testMatch: 'move.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['parts'] },
		// Parts and controls, and the first-run step: a part off and on with the confirm step, a refused switch, a stage paused and resumed, a linked app reconnected, the whole-app restart asked and cancelled, "Just movies" in one click, a phone and accessibility.
		{ name: 'controls', testMatch: 'controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['move'] },
		// The rest of the app frame: signed-in devices, This app, Schedules, the log viewer, the API page and the update notice; a phone and accessibility.
		// The whole app under a URL base behind a reverse proxy: its own HolyArrs, no other project needed.
		{ name: 'urlbase', testMatch: 'urlbase.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'schedules', testMatch: 'schedules.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['controls'] },
		{ name: 'copychoices', testMatch: 'copy-choices.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['controls'] },
		{ name: 'communitybrowse', testMatch: 'community-browse.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['desktop'] },
		{ name: 'appframe', testMatch: 'appframe.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['controls'] }
	]
});
