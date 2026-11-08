import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Locator, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openAdvanced, openLibraryTab, libraryTool } from '../helpers/helpers.ts';

// Search for movies against the fake Torznab Source in fakes/ (added by
// sources.spec.ts): Choose a release on the Movie page, the Held and
// Turned down lists, searches on Home, and Settings > Search. Each movie
// search finds three made-up Releases: 1080p and 720p web copies and a cam
// copy the default Pick rules turn away.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function openMovie(p: Page, name: string) {
	await p.goto('/library');
	await p.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: name }).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText(name);
}

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

// Choose a release sits in the title's More menu; it opens the Pick a release sheet.
async function chooseRelease(p: Page): Promise<Locator> {
	await p.getByRole('button', { name: 'More actions' }).click();
	await p.getByRole('menuitem', { name: 'Choose a release' }).click();
	const sheet = p.getByRole('dialog', { name: 'Choose a release' });
	await expect(sheet.getByTestId('release-sheet')).toBeVisible();
	return sheet;
}

// The sheet lists what the last search found; Search now asks the Sources, and All adds the ones that did not pass.
async function searchAll(sheet: Locator, count: number) {
	await sheet.getByRole('button', { name: 'Search now' }).click();
	await sheet.getByRole('tab', { name: /^All/ }).click();
	await expect(sheet.getByTestId('release')).toHaveCount(count, { timeout: 20_000 });
}

const release = (scope: Locator, part: string) => scope.getByTestId('release').filter({ has: scope.page().locator('.rs-name', { hasText: part }) });
// The verdict a row says out loud ("Turned down", "Chosen"); a Release that passed and lost has none.
const verdict = (row: Locator) => row.locator('.rs-verdict');

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('Choose a release shows every Release with its verdict and plain reasons; turn one down; force one that failed', async () => {
	await openMovie(page, 'Quiet Orchard');
	const sheet = await chooseRelease(page);
	// Nothing searched yet: Search now asks the Sources and says so while they answer.
	await sheet.getByRole('button', { name: 'Search now' }).click();
	await expect(sheet.getByText('Asking your sources')).toBeVisible();
	// Good matches holds the two that pass; All adds the one the rules turn away.
	await expect(sheet.getByTestId('release')).toHaveCount(2, { timeout: 20_000 });
	await sheet.getByRole('tab', { name: /^All/ }).click();
	await expect(sheet.getByTestId('release')).toHaveCount(3);
	await expect(sheet.getByText('Asking your sources')).toHaveCount(0);

	// Why each one won or lost, in plain words.
	const best = release(sheet, '.1080p.');
	await expect(best.locator('.rs-rec')).toHaveText('Recommended');
	await expect(best).toContainText('Lab Torznab');
	await expect(best).toContainText('50 seeders');
	const runnerUp = release(sheet, '.720p.');
	await expect(runnerUp.locator('.rs-rec')).toHaveCount(0);
	await expect(verdict(runnerUp)).toHaveCount(0);
	const cam = release(sheet, '.HDCAM.');
	await expect(verdict(cam)).toHaveText('Turned away by the rules');
	await expect(cam.locator('.rs-why.fail').first()).toBeVisible();

	// Turn the 720p down (the sheet asks no reason).
	await runnerUp.getByRole('button', { name: /^Never pick/ }).click();
	await expect(page.getByRole('status')).toContainText('Release turned down');
	await expect(verdict(runnerUp)).toHaveText('Turned down');

	// The cam copy failed, so Pick asks once more before it is forced.
	await cam.getByRole('button', { name: 'Pick', exact: true }).click();
	await cam.getByRole('button', { name: 'Pick anyway' }).click();
	await expect(page.getByRole('status')).toContainText('Sent to Downloader');
	await expect(sheet).toBeHidden();
	const again = await chooseRelease(page);
	await again.getByRole('tab', { name: /^All/ }).click();
	await expect(verdict(release(again, '.HDCAM.'))).toHaveText('Chosen');
	await expect(release(again, '.HDCAM.')).toContainText('Sent');
	await page.keyboard.press('Escape');
	await expect(again).toBeHidden();
});

test('a movie not out yet: every Release is listed, and one that did not pass is only sent when forced', async () => {
	await openMovie(page, 'The Lantern Keepers');
	const sheet = await chooseRelease(page);
	await searchAll(sheet, 3);
	// A year past next year is not read as a year, so these match no Title;
	// they are still listed with that reason.
	const best = release(sheet, '.1080p.');
	await expect(verdict(best)).toHaveText('Turned away by the rules');
	await expect(best.locator('.rs-why.fail').filter({ hasText: 'no Title in the library' })).toBeVisible();
	await best.getByRole('button', { name: 'Pick', exact: true }).click();
	await expect(best.getByRole('button', { name: 'Pick anyway' })).toBeVisible();
	// Closed without forcing it: nothing was sent.
	await page.keyboard.press('Escape');
	await expect(sheet).toBeHidden();
	const again = await chooseRelease(page);
	await again.getByRole('tab', { name: /^All/ }).click();
	await expect(again.getByTestId('release')).toHaveCount(3);
	await expect(again.getByText('Sent', { exact: true })).toHaveCount(0);
	await page.keyboard.press('Escape');
});

test('a movie with two Versions: Choose a release searches it', async () => {
	await openMovie(page, 'Glass Meridian');
	await openLibraryTab(page);
	await expect(page.getByTestId('version-card')).toHaveCount(2);
	// The sheet has no Version choice: one search covers the movie.
	const sheet = await chooseRelease(page);
	await sheet.getByRole('button', { name: 'Search now' }).click();
	await sheet.getByRole('tab', { name: /^All/ }).click();
	// Each Version is searched: at least the three made-up Releases come back.
	await expect(sheet.getByTestId('release').nth(2)).toBeVisible({ timeout: 20_000 });
	await expect(release(sheet, '.1080p.').locator('.rs-rec').first()).toHaveText('Recommended');
	await page.keyboard.press('Escape');
});

test('the Turned down list: un-refuse one, and the Held list is empty', async () => {
	await page.goto('/library');
	await libraryTool(page, 'Turned down releases');
	await expect(page.getByRole('heading', { level: 1, name: 'Turned down' })).toBeVisible();
	const row = page.getByTestId('listed-release').filter({ has: page.locator('.rel-name', { hasText: '.720p.' }) });
	await expect(row).toContainText('Quiet Orchard');
	await row.getByRole('button', { name: /^Take off/ }).click();
	await expect(row).toHaveCount(0);
	await expect(page.getByText('Nothing turned down.')).toBeVisible();

	await page.locator('#main').getByRole('link', { name: 'Held', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Held' })).toBeVisible();
	await expect(page.getByText('Nothing held.')).toBeVisible();
});

test('clear all empties the Turned down list', async () => {
	await openMovie(page, 'Glass Meridian');
	const sheet = await chooseRelease(page);
	await sheet.getByRole('tab', { name: /^All/ }).click();
	// Each Version has its own cam copy; turn one down.
	await release(sheet, '.HDCAM.').first().getByRole('button', { name: /^Never pick/ }).click();
	await expect(release(sheet, '.HDCAM.').filter({ has: page.locator('.rs-verdict', { hasText: 'Turned down' }) })).toHaveCount(1);
	await page.goto('/library/turned-down');
	await expect(page.getByTestId('listed-release')).toHaveCount(1);
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Clear all' }).click();
	await expect(page.getByText('Nothing turned down.')).toBeVisible();
});

test('Settings > Search: the three settings are saved and a wrong interval is refused', async () => {
	await page.goto('/settings/search');
	await expect(page.getByRole('heading', { level: 1, name: 'Search' })).toBeVisible();
	await openAdvanced(page);
	await expect(page.getByLabel('Check Source feeds every (minutes)')).toHaveValue('15');
	await page.getByLabel('Check Source feeds every (minutes)').fill('5');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('alert')).toContainText('10 to 120');
	await page.getByLabel('Check Source feeds every (minutes)').fill('30');
	await page.getByLabel('Titles to search each night').fill('7');
	await page.getByLabel('Also find better copies each night').check();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Check Source feeds every (minutes)')).toHaveValue('30');
	await expect(page.getByLabel('Titles to search each night')).toHaveValue('7');
	await expect(page.getByLabel('Also find better copies each night')).toBeChecked();
});

test('Settings > Search: the early and fake switches are saved', async () => {
	await page.goto('/settings/search');
	await expect(page.getByRole('heading', { name: 'Early and fake Releases' })).toBeVisible();
	await expect(page.getByTestId('refuse-early')).toBeChecked();
	await expect(page.getByTestId('refuse-tiny')).toBeChecked();
	await expect(page.getByTestId('import-floors')).toBeChecked();
	await openAdvanced(page);
	await expect(page.getByTestId('hold-unknown')).not.toBeChecked();
	await page.getByLabel('Hours a Release may be early').fill('48');
	await page.getByLabel('Smallest video file to place (MB)').fill('0');
	await page.getByLabel('Episode length when not known (minutes)').fill('30');
	// The downloads in these tests are tiny stand-in files: the size floors
	// at import stay off from here on (the Go tests cover them).
	await page.getByTestId('import-floors').uncheck();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Hours a Release may be early')).toHaveValue('48');
	await expect(page.getByLabel('Episode length when not known (minutes)')).toHaveValue('30');
	await expect(page.getByTestId('import-floors')).not.toBeChecked();
	await page.getByLabel('Hours a Release may be early').fill('24');
	await page.getByLabel('Episode length when not known (minutes)').fill('20');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved').first()).toBeVisible();
});

test('a member sees none of the search screens', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	await page.getByLabel('Note for yourself').fill('Search viewer');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const m = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-sv`;
	await m.goto(new URL(link).pathname);
	await m.getByLabel('Name').fill('searchviewer');
	await m.getByLabel('Password', { exact: true }).fill(pw);
	await m.getByLabel('Confirm password').fill(pw);
	await m.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(m).toHaveURL(/\/$/);
	await m.goto('/library');
	await expect(m.getByRole('link', { name: 'Held', exact: true })).toHaveCount(0);
	await expect(m.getByRole('link', { name: 'Library tools' })).toHaveCount(0);
	await m.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Quiet Orchard' }).click();
	await expect(m.getByRole('heading', { level: 1 })).toContainText('Quiet Orchard');
	for (const b of ['Search now', 'Choose a release']) await expect(m.getByRole('button', { name: b })).toHaveCount(0);
	await expect(m.getByTestId('release')).toHaveCount(0);
	// Staff pages send a member to their own Home, which has no searches panel;
	// a Settings page they may not open says so in place instead.
	await m.goto('/settings/search');
	await expect(m.locator('#settings-blocked')).toContainText("You can't change this");
	for (const url of ['/library/held', '/library/turned-down', '/']) {
		await m.goto(url);
		await expect(m).toHaveURL(/\/$/);
	}
	await expect(m.getByTestId('home-searches')).toHaveCount(0);
});

test('no serious accessibility issues on the filled search screens', async () => {
	const scan = async (where: string) => {
		await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
		const r = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
		const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
		expect.soft(bad.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')}`), where).toEqual([]);
	};
	await openMovie(page, 'Quiet Orchard');
	await scan('movie page');
	const sheet = await chooseRelease(page);
	await sheet.getByRole('tab', { name: /^All/ }).click();
	await expect(sheet.getByTestId('release').first()).toBeVisible();
	await scan('Pick a release sheet');
	await page.keyboard.press('Escape');
	await expect(sheet).toBeHidden();
	await page.goto('/');
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Home');
	await scan('Home');
});

test('phone width: the search screens fit', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
	const p = await ctx.newPage();
	await signIn(p, owner());
	await openMovie(p, 'Quiet Orchard');
	// On a phone the Pick a release sheet opens from the More menu too.
	const sheet = await chooseRelease(p);
	await expect(sheet.getByTestId('release').first()).toBeVisible();
	await noSidewaysScroll(p);
	await p.keyboard.press('Escape');
	for (const url of ['/', '/library/held', '/library/turned-down', '/settings/search']) {
		await p.goto(url);
		await expect(p.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(p);
	}
	await ctx.close();
});
