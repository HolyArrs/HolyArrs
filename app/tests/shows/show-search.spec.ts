import { expect, test, type Browser, type Locator, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openFromSearch, openSeasonNamed, episodeAction, episodeState, inLibrary } from '../helpers/helpers.ts';
import { axe } from '../helpers/axe.ts';

// Searching shows (3.2.2 to 3.2.5) against the fake Torznab Source in
// fakes/: a show search answers per episode (a 1080p and a 720p web
// copy) or per season (a 1080p season pack and each aired episode).
// Choose a release for a season takes the pack for every episode with one
// download; an episode's Release turned down is never picked again for it;
// the Turned down list and Home name the show and the episode.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

const release = (scope: Locator, part: string) => scope.getByTestId('release').filter({ has: scope.page().locator('.rs-name', { hasText: part }) });
const verdict = (row: Locator) => row.locator('.rs-verdict');

async function openShow(p: Page, name: string) {
	await p.goto('/library?kind=show');
	await (await inLibrary(p, name)).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText(name);
}

// A season's row on the Library tab, opened so its episodes show.
async function openSeason(p: Page, name: string) {
	const s = await openSeasonNamed(p, name);
	await expect(s.getByRole('button', { name: /^Episode \d+:/ }).first()).toBeVisible();
	return s;
}

const states = (s: Locator) => s.locator('.lib-ep-st');

// The Pick a release sheet a season or episode button opened: Search now when nothing was searched yet, then All.
async function sheetReleases(p: Page, label: string): Promise<Locator> {
	const sheet = p.getByRole('dialog', { name: 'Choose a release' });
	await expect(sheet.getByTestId('release-sheet')).toBeVisible();
	await expect(sheet).toContainText(label);
	const now = sheet.getByRole('button', { name: 'Search now' });
	await expect(now.or(sheet.getByTestId('release').first())).toBeVisible({ timeout: 20_000 });
	if (await now.isVisible()) await now.click();
	await sheet.getByRole('tab', { name: /^All/ }).click();
	return sheet;
}

// The season rows are read when opened: reload to see what a search did to an episode.
async function stateAfterReload(p: Page, season: string, code: string, want: RegExp) {
	await expect(async () => {
		await p.reload();
		await openSeasonNamed(p, season);
		await expect(await episodeState(p, code)).toHaveText(want, { timeout: 1000 });
	}).toPass({ timeout: 30_000 });
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('the owner adds Moss Parade with every season kept', async () => {
	await openFromSearch(page, 'moss parade', 'Moss Parade', 'show');
	const add = page.getByRole('button', { name: 'Add Moss Parade' });
	if (await add.count()) {
		await add.click();
		const dialog = page.getByRole('dialog');
		await expect(dialog.getByTestId('no-show-search')).toContainText('Tick Search when added to start now');
		await dialog.getByLabel('Seasons to keep up to date').selectOption('all');
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(dialog).toBeHidden();
	}
	await openShow(page, 'Moss Parade');
	const s1 = await openSeason(page, 'Season 1');
	await expect(states(s1)).toHaveText(['Missing', 'Missing']);
	// Both can be looked for.
	await expect(s1.getByRole('button', { name: 'Look for episode 1' })).toBeEnabled();
	await expect(s1.getByRole('button', { name: 'Look for episode 2' })).toBeEnabled();
});

test('Choose a release for a season: the season pack fills both episodes with one download', async () => {
	await openShow(page, 'Moss Parade');
	const s1 = await openSeason(page, 'Season 1');
	await s1.getByRole('button', { name: 'Choose a release for season 1' }).click();
	const panel = await sheetReleases(page, 'Season 1');
	await expect(panel.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	await expect(panel.getByText('Asking your sources')).toHaveCount(0);

	const pack = release(panel, 'Moss.Parade.S01.1080p');
	await expect(pack.locator('.rs-rec')).toHaveText('Recommended');
	const single = release(panel, '.S01E01.');
	await expect(single.locator('.rs-rec')).toHaveCount(0);
	await expect(verdict(single)).toHaveCount(0);
	await expect(single.locator('.rs-why').filter({ hasText: 'lost to Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS on season pack' })).toBeVisible();
	await axe(page, 'choose a release for a season');

	await pack.getByRole('button', { name: 'Pick', exact: true }).click();
	await expect(panel).toBeHidden();
	await expect(page.getByRole('status')).toContainText('Sent to Downloader');
	// Both episodes go to the one download.
	await stateAfterReload(page, 'Season 1', 'S01E01', /Chosen|Downloading/);
	await expect(await episodeState(page, 'S01E02')).toHaveText(/Chosen|Downloading/);
});

test('a Release turned down for an episode is never picked again for it; Search now takes the next one', async () => {
	await openShow(page, 'Harbour Lights');
	const s2 = await openSeason(page, 'Season 2');
	await expect(await episodeState(page, 'S02E01')).toHaveText('Missing');
	await episodeAction(page, 'S02E01', 'Choose a release');
	const panel = await sheetReleases(page, 'S2 E1');
	await expect(panel.getByTestId('release')).toHaveCount(2, { timeout: 20_000 });
	const best = release(panel, '.S02E01.1080p.');
	await expect(best.locator('.rs-rec')).toHaveText('Recommended');

	await best.getByRole('button', { name: /^Never pick/ }).click();
	await expect(page.getByRole('status')).toContainText('Release turned down');
	await expect(verdict(best)).toHaveText('Turned down');
	await page.keyboard.press('Escape');
	await expect(panel).toBeHidden();

	// Search now looks again: the turned-down copy is refused, the 720p is taken.
	await expect(s2).toBeVisible();
	await episodeAction(page, 'S02E01', 'Search now');
	await stateAfterReload(page, 'Season 2', 'S02E01', /Chosen|Downloading/);

	// The Turned down list says what it was turned down for; take it off.
	await page.goto('/library/turned-down');
	const row = page.getByTestId('listed-release').filter({ hasText: 'Harbour.Lights.S02E01.1080p' });
	await expect(row.getByTestId('listed-for')).toHaveText('Harbour Lights S02E01');
	await axe(page, 'turned down with episodes');
	await row.getByRole('button', { name: /^Take off/ }).click();
	await expect(row).toHaveCount(0);
});

test('Settings > Search has the season pack choices', async () => {
	await page.goto('/settings/search');
	await expect(page.getByLabel('A season pack is taken when')).toHaveValue('all');
	await page.getByLabel('Season pack or single episodes').selectOption('always');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Search settings saved' })).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Season pack or single episodes')).toHaveValue('always');
	await page.getByLabel('Season pack or single episodes').selectOption('rank');
	await page.getByRole('button', { name: 'Save' }).click();
	// Scan once the saved note is in, not while it fades in.
	await expect(page.getByRole('status')).toContainText('Search settings saved');
	await axe(page, 'search settings');
});

test('the show page searches from a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await openShow(phone, 'Lantern Street');
	await expect(phone.getByRole('button', { name: 'More actions', exact: true })).toBeVisible();
	const s1 = await openSeason(phone, 'Season 1');
	await s1.getByRole('button', { name: 'Choose a release for season 1' }).click();
	const panel = await sheetReleases(phone, 'Season 1');
	await expect(panel.getByTestId('release').first()).toBeVisible({ timeout: 20_000 });
	await noSidewaysScroll(phone);
	await axe(phone, 'show releases on a phone');
	await ctx.close();
});
