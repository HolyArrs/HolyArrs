import { expect, type Browser, type Locator, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { tmp } from './data-folder.ts';

export const moviesDir = join(tmp, 'movies');
const ownerFile = join(tmp, 'owner.json');

export interface Owner {
	name: string;
	password: string;
}

export function newOwner(): Owner {
	const o = { name: 'luis', password: `e2e-${Math.random().toString(36).slice(2)}-pw` };
	writeFileSync(ownerFile, JSON.stringify(o));
	return o;
}

export function owner(): Owner {
	if (!existsSync(ownerFile)) throw new Error('run the desktop tests first; they make the owner');
	return JSON.parse(readFileSync(ownerFile, 'utf8'));
}

/**
 * finishFirstRun walks the first run after the owner: Start fresh, one movies
 * folder (Music on too when asked, with a folder next to it; TV shows off,
 * since Continue needs a folder for every part that is on), skips Downloader and Source, then opens
 * Home, or Users through Invite family (which also turns on Requests).
 */
export async function finishFirstRun(page: Page, moviesPath: string, o: { tv?: boolean; music?: boolean; invite?: boolean } = {}) {
	await expect(page.getByRole('heading', { name: 'Coming from other apps?', level: 1 })).toBeVisible();
	// The media step reads the saved folders as it opens and rebuilds its rows when they arrive; typing before that is lost.
	const loaded = page.waitForResponse((r) => r.url().includes('/api/v1/library-folders') && r.request().method() === 'GET');
	await page.getByRole('button', { name: /^Start fresh/ }).click();
	await expect(page.getByRole('heading', { name: /^Where.s your media\?$/, level: 1 })).toBeVisible();
	await loaded;
	// TV shows starts on; most own-server checks need Movies only. The shared setup keeps it on, with its own folder.
	if (o.tv) {
		const tvPath = join(dirname(moviesPath), 'tv');
		mkdirSync(tvPath, { recursive: true });
		await page.getByLabel('TV shows folder').fill(tvPath);
	} else await page.getByLabel('TV shows', { exact: true }).uncheck();
	if (o.music) {
		await page.getByLabel('Music', { exact: true }).check();
		const musicPath = join(dirname(moviesPath), 'music');
		mkdirSync(musicPath, { recursive: true });
		await page.getByLabel('Music folder').fill(musicPath);
	}
	await page.getByLabel('Movies folder').fill(moviesPath);
	await expect(page.getByLabel('Movies folder')).toHaveValue(moviesPath);
	await page.getByRole('button', { name: 'Continue' }).click();
	// Saving the parts restarts them, which can take a while on a slow or loaded machine.
	await expect(page.getByRole('heading', { name: 'Which app downloads?', level: 1 })).toBeVisible({ timeout: 20_000 });
	await page.getByRole('button', { name: 'Skip for now' }).click();
	await expect(page.getByRole('heading', { name: 'Where should we search?', level: 1 })).toBeVisible();
	await page.getByRole('button', { name: 'Skip for now' }).click();
	await expect(page.getByRole('heading', { name: 'HolyArrs is ready', level: 1 })).toBeVisible();
	if (o.invite) {
		await page.getByRole('button', { name: 'Invite family' }).click();
		await expect(page).toHaveURL(/\/settings\/users$/, { timeout: 20_000 });
	} else await page.getByRole('button', { name: 'Open Home' }).click();
}

/** plainForm leaves the remembered people (one person's step, or the list) for the name-and-password form. */
export async function plainForm(page: Page) {
	await expect(page.getByRole('heading', { name: /^(Sign in|Who's signing in\?)$/ })).toBeVisible();
	const other = page.getByRole('button', { name: 'Use another account' });
	const sw = page.getByRole('button', { name: 'Switch account' });
	if (await sw.isVisible()) {
		await sw.click();
		await other.click();
	} else if (await other.isVisible()) await other.click();
	// With other ways on, the card lists buttons first.
	const local = page.getByRole('button', { name: 'Use a HolyArrs account' });
	if (await local.isVisible()) await local.click();
}

export async function signIn(page: Page, o: Owner) {
	await page.goto('/login');
	await plainForm(page);
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(page).toHaveURL(/\/$/);
}

/** libraryStatus picks a Library status in the toolbar's Sort and filter panel (all, missing, not_out_yet, better, have). */
export async function libraryStatus(scope: Page | Locator, value: 'all' | 'missing' | 'not_out_yet' | 'better' | 'have') {
	const names = { all: /^All/, missing: /^Missing/, not_out_yet: /^Upcoming/, better: /^Better copy/, have: /^In library/ };
	await scope.getByRole('button', { name: /^Sort( &|)/ }).first().click();
	await scope.getByRole('region', { name: 'Filter' }).getByRole('button', { name: names[value] }).first().click();
	await ('keyboard' in scope ? scope : scope.page()).keyboard.press('Escape');
}

/** libraryView switches the Library between Banners, Posters and Overview. */
export async function libraryView(scope: Page | Locator, name: 'Banners' | 'Posters' | 'Overview') {
	await scope.getByRole('radiogroup', { name: 'View' }).getByRole('radio', { name, exact: true }).click();
}

/** openRequests reaches Requests as an approver on a phone: Manage tab, then Requests. */
export async function openRequests(page: Page) {
	await page.locator('.ui-tabbar').getByRole('button', { name: /^Manage/ }).click();
	await page.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: /^Requests/ }).click();
	await expect(page).toHaveURL(/requests/);
}

/** titleAction picks an item from a title page's hero ⋯ menu. The Library
 *  items (rename, delete, versions) joined More actions; 'Library' is kept as a name. */
export async function titleAction(page: Page, _menu: 'Library' | 'More actions', item: string) {
	await page.getByRole('button', { name: 'More actions', exact: true }).click();
	await page.getByRole('menuitem', { name: item, exact: true }).click();
}

/** openLibraryTab opens an owned title's Library tab: status, seasons or files,
 *  history and settings, then downloads, Versions, copies and subtitles. */
export async function openLibraryTab(page: Page): Promise<Locator> {
	const tab = page.getByTestId('own-tabs').getByRole('tab', { name: /^Library/ });
	await tab.click();
	await expect(tab).toHaveAttribute('aria-selected', 'true');
	await expect(page.getByTestId('lib-pane')).toBeVisible();
	return page.getByTestId('own-pane');
}

/**
 * libraryTool opens a Library tool. Recently deleted opens from the Library's Sort & filter panel;
 * the others are not on the Library page, so they open by address.
 */
export async function libraryTool(page: Page, name: string) {
	if (name === 'Recently deleted') {
		await page.getByRole('main').getByRole('button', { name: /^(Sort and filter|Sort & filter)/ }).first().click();
		await page.getByTestId('library-deleted').click();
		return;
	}
	const paths: Record<string, string> = { 'Import a library': '/library/import', 'Import files by hand': '/library/import-files', 'Turned down releases': '/library/turned-down', 'Missing subtitles': '/library/missing-subtitles', 'Rename files': '/library/rename', 'Video codecs': '/library/copies' };
	await page.goto(paths[name] ?? name);
}

// A show's seasons and episodes live on its Library tab: one row per season
// (newest first) that opens to its episodes. Each season row has a keep
// toggle, Get the missing episodes, Choose a release and Files, rename and
// history; each episode row a keep toggle, Look for episode N and Choose a
// release for episode N, and opens the episode sheet.

/** seasonRows opens the Library tab when needed and returns every season row. */
async function seasonRows(page: Page): Promise<Locator> {
	const pane = page.getByTestId('lib-pane');
	if (!(await pane.isVisible())) await openLibraryTab(page);
	const rows = pane.locator('section.lib-sr');
	await expect(rows.first()).toBeVisible();
	return rows;
}

/** seasonRow returns the row of "Season 1" or "Specials" (not opened). */
async function seasonRow(page: Page, name: string): Promise<Locator> {
	const rows = await seasonRows(page);
	const row = rows.filter({ has: page.locator('.lib-sr-main > b', { hasText: new RegExp(`^${name}$`) }) });
	await expect(row).toHaveCount(1);
	return row;
}

/** openRow opens a season row so its episodes load, and returns it. */
async function openRow(row: Locator): Promise<Locator> {
	const head = row.locator('.lib-sr-main');
	if ((await head.getAttribute('aria-expanded')) !== 'true') await head.click();
	await expect(head).toHaveAttribute('aria-expanded', 'true');
	await expect(row.locator('.lib-ep.skel')).toHaveCount(0);
	return row;
}

/** openSeason opens one season on a show's Library tab, counted from the lowest (0 is Specials when there are any), and returns its row. */
export async function openSeason(page: Page, index: number): Promise<Locator> {
	const rows = await seasonRows(page);
	const n = await rows.count();
	return openRow(rows.nth(n - 1 - index));
}

/** openSeasonNamed opens the season with this name ("Season 1", "Specials") and returns its row. */
export async function openSeasonNamed(page: Page, name: string): Promise<Locator> {
	return openRow(await seasonRow(page, name));
}

/** seasonCount counts the seasons on a show's Library tab. */
export async function seasonCount(page: Page): Promise<number> {
	return (await seasonRows(page)).count();
}

/** seasonKept says whether a season's keep toggle is on. */
export async function seasonKept(page: Page, season: string): Promise<boolean> {
	const keep = (await seasonRow(page, season)).locator('.lib-sr-head').getByRole('button', { name: /^Keep .* up to date$/ });
	return (await keep.getAttribute('aria-pressed')) === 'true';
}

/** toggleSeasonKept flips a season's keep toggle and waits for the saved state. */
export async function toggleSeasonKept(page: Page, season: string) {
	const keep = (await seasonRow(page, season)).locator('.lib-sr-head').getByRole('button', { name: /^Keep .* up to date$/ });
	const was = await keep.getAttribute('aria-pressed');
	await keep.click();
	await expect(keep).toHaveAttribute('aria-pressed', was === 'true' ? 'false' : 'true');
}

/** episodeRow opens the episode's season and returns its row ("S02E01"): keep toggle, name button, state and searches. */
export async function episodeRow(page: Page, code: string): Promise<Locator> {
	const m = /^S(\d+)E(\d+)$/.exec(code);
	if (!m) throw new Error(`not an episode code: ${code}`);
	const season = Number(m[1]);
	const row = await openSeasonNamed(page, season === 0 ? 'Specials' : `Season ${season}`);
	const ep = row.locator('.lib-ep').filter({ has: page.getByRole('button', { name: new RegExp(`^Episode ${Number(m[2])}:`) }) });
	await expect(ep).toHaveCount(1);
	return ep;
}

/** episodeAction does one thing from an episode's row: its keep toggle ("Keep S02E01 up to date"),
 *  "Search now" (Look for episode N), "Choose a release", or "History" (the episode sheet). */
export async function episodeAction(page: Page, code: string, item: string) {
	const ep = await episodeRow(page, code);
	const n = Number(/E(\d+)$/.exec(code)![1]);
	if (item === `Keep ${code} up to date`) {
		const keep = ep.getByRole('button', { name: /^Keep .* up to date$/ });
		const was = await keep.getAttribute('aria-pressed');
		await keep.click();
		await expect(keep).toHaveAttribute('aria-pressed', was === 'true' ? 'false' : 'true');
	} else if (item === 'Search now') await ep.getByRole('button', { name: `Look for episode ${n}` }).click();
	else if (item === 'Choose a release') await ep.getByRole('button', { name: `Choose a release for episode ${n}` }).click();
	else if (item === 'History') {
		await ep.getByRole('button', { name: /^Episode \d+:/ }).click();
		await expect(page.getByTestId('episode-sheet')).toBeVisible();
	} else throw new Error(`the Library tab has no "${item}" for ${code}`);
}

/** episodeKept says whether an episode's keep toggle is on. */
export async function episodeKept(page: Page, code: string): Promise<boolean> {
	const keep = (await episodeRow(page, code)).getByRole('button', { name: /^Keep .* up to date$/ });
	return (await keep.getAttribute('aria-pressed')) === 'true';
}

/** episodeState reads the state an episode row shows ("Missing", "Not out yet", "1080p WEB", a state label). */
export async function episodeState(page: Page, code: string): Promise<Locator> {
	return (await episodeRow(page, code)).locator('.lib-ep-st');
}

/** libraryKind opens one library the way the sidebar does (Movies, Shows, Music; Everything is the plain /library). */
export async function libraryKind(page: Page, name: RegExp) {
	const kind = name.test('Shows') ? 'show' : name.test('Music') ? 'artist' : name.test('Movies') ? 'movie' : '';
	await page.goto(kind ? `/library?kind=${kind}` : '/library');
}

/** signOut uses the account menu (sidebar) or, on a phone, the More sheet. */
export async function signOut(page: Page) {
	const account = page.getByRole('button', { name: 'Account and appearance' });
	if (await account.isVisible()) {
		await account.click();
		await page.getByRole('menuitem', { name: 'Sign out' }).click();
		return;
	}
	await page.locator('.ui-tabbar').getByRole('button', { name: /^(More|Manage)/ }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Sign out' }).click();
}

/** noSidewaysScroll fails when the page is wider than the screen. */
export async function noSidewaysScroll(page: Page) {
	const { sw, cw, wide } = await page.evaluate(() => {
		const cw = document.documentElement.clientWidth;
		// The deepest elements that stick out, to say what to fix.
		const wide = [...document.querySelectorAll('body *')]
			.filter((el) => el.getBoundingClientRect().right > cw + 1 && ![...el.children].some((c) => c.getBoundingClientRect().right > cw + 1))
			.slice(0, 3)
			.map((el) => `${el.tagName.toLowerCase()}.${[...el.classList].join('.')}`);
		return { sw: document.documentElement.scrollWidth, cw, wide };
	});
	expect(sw, `${page.url()} is ${sw}px wide on a ${cw}px screen (${wide.join(', ')})`).toBeLessThanOrEqual(cw);
}

/** openAdvanced opens every closed Advanced disclosure on a settings page (they start closed and are kept per person). */
export async function openAdvanced(scope: Page | Locator) {
	// A card can close again while the page finishes loading (it restores the saved
	// open state), so open every one and check they stay open.
	const closed = scope.locator('details.advanced:not([open]) > summary');
	await expect(async () => {
		// Opened in place (a hidden or restoring card can swallow a click); the page still gets its toggle event.
		await closed.evaluateAll((els) => els.forEach((s) => ((s.parentElement as HTMLDetailsElement).open = true)));
		await new Promise((r) => setTimeout(r, 300));
		expect(await closed.count()).toBe(0);
	}).toPass({ timeout: 10_000 });
}

/** openFromSearch searches and opens a result's page: movies and shows open
 *  their title page (Add or Ask lives there); Library titles open their Library page. */
export async function openFromSearch(page: Page, words: string, name: string, kind?: 'movie' | 'show') {
	await page.goto(`/search?q=${encodeURIComponent(words)}${kind ? `&kind=${kind}` : ''}`);
	const card = page.getByRole('main').getByRole('link', { name: `${name},` }).first();
	await expect(card).toBeVisible();
	await card.click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText(name);
}

/** addFromSearch opens a movie or show from Search and opens its Add dialog. */
export async function addFromSearch(page: Page, words: string, name: string, kind?: 'movie' | 'show') {
	await openFromSearch(page, words, name, kind);
	await page.getByRole('button', { name: `Add ${name}` }).click();
	return page.getByRole('dialog');
}

/**
 * ensureInLibrary adds a movie (not searched for) when an earlier project has
 * not added it already, so a project can also run on the setup project alone.
 * It uses its own desktop-sized browser context, so nothing it remembers (the
 * Add dialog's choices) reaches the caller's page.
 */
export async function ensureInLibrary(browser: Browser, words: string, name: string) {
	const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
	const page = await ctx.newPage();
	try {
		await signIn(page, owner());
		await page.goto('/library');
		const cards = page.locator('[data-testid="library-card"], [data-testid="library-row"]');
		await cards.or(page.getByRole('heading', { name: 'Nothing here yet' })).first().waitFor();
		if (await cards.filter({ hasText: name }).count()) return;
		const dialog = await addFromSearch(page, words, name);
		await dialog.getByLabel('Search when added').uncheck();
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(page).toHaveURL(/\/library\/\d+/);
	} finally {
		await ctx.close();
	}
}

/** openInLibrary steps from a music page (artist or album, found in Search or the Library) to its Library page. */
export async function openInLibrary(page: Page) {
	// A page reached from the Library (an album from the artist's Library page) is there already.
	if (/\/library\/\d+/.test(new URL(page.url()).pathname)) return;
	await page.getByRole('link', { name: 'Open in Library' }).or(page.getByRole('button', { name: 'Open in Library' })).first().click();
	await expect(page).toHaveURL(/\/library\/\d+/);
}

/** openSubtitles opens an owned title's Subtitles window from the Settings on its Library tab. */
export async function openSubtitles(page: Page): Promise<Locator> {
	await openLibraryTab(page);
	await page.getByTestId('subtitles-row').getByRole('button', { name: 'Subtitle options' }).click();
	await page.getByRole('menuitem', { name: 'Choose languages…' }).click();
	const card = page.getByTestId('subtitles');
	await expect(card).toBeVisible();
	return card;
}

/**
 * inLibrary finds a title's card or row on the Library page, scrolling down
 * until it is drawn: the Library only draws the cards near the screen.
 */
export async function inLibrary(page: Page, name: string, sel = '[data-testid="library-card"], [data-testid="library-row"]'): Promise<Locator> {
	const item = page.locator(sel, { hasText: name }).first();
	await page.locator('[data-testid="library-card"], [data-testid="library-row"]').or(page.getByRole('heading', { name: 'Nothing here yet' })).first().waitFor();
	await page.evaluate(() => window.scrollTo(0, 0));
	for (let i = 0; i < 60 && !(await item.count()); i++) {
		await page.mouse.wheel(0, 700);
		await page.waitForTimeout(120);
	}
	await item.scrollIntoViewIfNeeded().catch(() => undefined);
	return item;
}
