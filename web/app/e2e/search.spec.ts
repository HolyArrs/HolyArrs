import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// Search for movies against the fake Torznab Source in fakes.mjs (added by
// sources.spec.ts): Choose a release on the Movie page, the Held and
// Turned down lists, searches on Home, and Settings > Search. Each movie
// search finds three made-up Releases: 1080p and 720p web copies and a cam
// copy the default Pick rules turn away.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function openMovie(p: Page, name: string) {
	await p.goto('/library');
	await p.locator('.grid .item', { hasText: name }).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText(name);
}

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

const release = (p: Page, part: string) => p.getByTestId('release').filter({ has: p.locator('.rel-name', { hasText: part }) });

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('Choose a release shows every Release with its verdict and plain reasons; turn one down; force one that failed', async () => {
	await openMovie(page, 'Quiet Orchard');
	await page.getByRole('button', { name: 'Choose a release' }).click();
	await expect(page.getByTestId('search-running')).toContainText('Searching your Sources');
	await expect(page.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	await expect(page.getByTestId('search-running')).toHaveText('');

	// Why each one won or lost, in plain words.
	const best = release(page, '.1080p.');
	await expect(best.getByTestId('verdict')).toHaveText('Would be chosen');
	await expect(best).toContainText('Lab Torznab');
	await expect(best).toContainText('50 seeders');
	await expect(release(page, '.720p.').getByTestId('verdict')).toHaveText('Passed over');
	const cam = release(page, '.HDCAM.');
	await expect(cam.getByTestId('verdict')).toHaveText('Turned away by the rules');
	await expect(cam.locator('.reasons li.fail').first()).toBeVisible();
	await cam.locator('summary').click();
	await expect(cam.locator('details .reasons li').first()).toBeVisible();

	// Turn the 720p down with a reason.
	await release(page, '.720p.').getByRole('button', { name: /^Turn down/ }).click();
	const td = page.getByRole('dialog');
	await td.getByLabel('Why (optional)').fill('too small for the big screen');
	await td.getByRole('button', { name: 'Turn down' }).click();
	await expect(release(page, '.720p.').getByTestId('verdict')).toHaveText('Turned down');
	await expect(release(page, '.720p.')).toContainText('too small for the big screen');

	// The cam copy failed, so choosing it asks to force, with the reasons.
	await cam.getByRole('button', { name: /^Choose/ }).click();
	const dlg = page.getByRole('dialog');
	await expect(dlg.getByRole('heading', { name: 'Choose it anyway?' })).toBeVisible();
	await expect(dlg.getByTestId('choose-warnings')).toContainText('It did not pass');
	await dlg.getByRole('button', { name: 'Choose anyway' }).click();
	await expect(page.getByText(/^Chosen: .*HDCAM/)).toBeVisible();
	await expect(cam.getByTestId('verdict')).toHaveText('Chosen');
});

test('a movie not out yet: every Release is listed, and choosing one warns that the Release gate is skipped', async () => {
	await openMovie(page, 'The Lantern Keepers');
	await page.getByRole('button', { name: 'Choose a release' }).click();
	await expect(page.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	// A year past next year is not read as a year, so these match no Title;
	// they are still listed with that reason.
	const best = release(page, '.1080p.');
	await expect(best.getByTestId('verdict')).toHaveText('Turned away by the rules');
	await expect(best.locator('.reasons li.fail').first()).toContainText('no Title in the library');
	await best.getByRole('button', { name: /^Choose/ }).click();
	const dlg = page.getByRole('dialog');
	await expect(dlg.getByTestId('choose-warnings')).toContainText('Release gate is skipped');
	await expect(dlg.getByTestId('choose-warnings')).toContainText('It did not pass');
	await dlg.getByRole('button', { name: 'Cancel' }).click();
	await expect(dlg).toBeHidden();
});

test('a movie with two Versions: pick the Version to search for', async () => {
	await openMovie(page, 'Glass Meridian');
	const pick = page.getByLabel('Version', { exact: true });
	await expect(pick.locator('option')).toHaveCount(3);
	await pick.selectOption({ label: '4K' });
	await page.getByRole('button', { name: 'Choose a release' }).click();
	await expect(page.getByTestId('release').first()).toBeVisible({ timeout: 20_000 });
	await expect(page.getByTestId('release').first()).toContainText('4K');
});

test('the Turned down list: un-refuse one, and the Held list is empty', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: 'Turned down', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Turned down' })).toBeVisible();
	const row = page.getByTestId('listed-release').filter({ has: page.locator('.rel-name', { hasText: '.720p.' }) });
	await expect(row).toContainText('Quiet Orchard');
	await expect(row).toContainText('too small for the big screen');
	await row.getByRole('button', { name: /^Take off the list/ }).click();
	await expect(row).toHaveCount(0);
	await expect(page.getByText('Nothing is turned down')).toBeVisible();

	await page.getByRole('link', { name: 'Held', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Held' })).toBeVisible();
	await expect(page.getByText('Nothing is held')).toBeVisible();
});

test('clear all empties the Turned down list', async () => {
	await openMovie(page, 'Glass Meridian');
	await release(page, '.HDCAM.').first().getByRole('button', { name: /^Turn down/ }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Turn down' }).click();
	await expect(release(page, '.HDCAM.').first().getByTestId('verdict')).toHaveText('Turned down');
	await page.goto('/library/turned-down');
	await expect(page.getByTestId('listed-release')).toHaveCount(1);
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Clear all' }).click();
	await expect(page.getByText('Nothing is turned down')).toBeVisible();
});

test('Home shows a search running and then what it found, live', async ({ browser }) => {
	await page.goto('/');
	const home = page.getByTestId('home-searches');
	await expect(home).toBeVisible();
	const other = await fresh(browser);
	await signIn(other, owner());
	await openMovie(other, 'Northern Kettle');
	await other.getByRole('button', { name: 'Search now' }).click();
	// The fake Source takes a few seconds, so the search shows while it runs.
	await expect(home.getByTestId('search-running').filter({ hasText: 'Northern Kettle' })).toBeVisible({ timeout: 10_000 });
	await expect(home.getByTestId('search-found').filter({ hasText: 'Northern Kettle: found 3 Releases, 1 turned away, chose' })).toBeVisible({ timeout: 20_000 });
	await expect(home.getByTestId('search-running').filter({ hasText: 'Northern Kettle' })).toHaveCount(0);
	await other.context().close();
});

test('Settings > Search: the four settings are saved and a wrong interval is refused', async () => {
	await page.goto('/settings/search');
	await expect(page.getByRole('heading', { level: 1, name: 'Search' })).toBeVisible();
	await expect(page.getByLabel('Check Source feeds every (minutes)')).toHaveValue('15');
	await page.getByLabel('Check Source feeds every (minutes)').fill('5');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('alert')).toContainText('10 to 120 minutes');
	await page.getByLabel('Check Source feeds every (minutes)').fill('30');
	await page.getByLabel('Movies to search for each night').fill('7');
	await page.getByLabel('Also Find a better copy each night').check();
	await page.getByLabel('Try the next Downloader').uncheck();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved.')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Check Source feeds every (minutes)')).toHaveValue('30');
	await expect(page.getByLabel('Movies to search for each night')).toHaveValue('7');
	await expect(page.getByLabel('Also Find a better copy each night')).toBeChecked();
	await expect(page.getByLabel('Try the next Downloader')).not.toBeChecked();
});

test('Settings > Search: the early and fake switches are saved', async () => {
	await page.goto('/settings/search');
	await expect(page.getByRole('heading', { name: 'Early and fake Releases' })).toBeVisible();
	await expect(page.getByTestId('refuse-early')).toBeChecked();
	await expect(page.getByTestId('refuse-tiny')).toBeChecked();
	await expect(page.getByTestId('import-floors')).toBeChecked();
	await expect(page.getByTestId('hold-unknown')).not.toBeChecked();
	await page.getByLabel('Hours a Release may be early').fill('48');
	await page.getByLabel('Smallest video file to place (MB)').fill('0');
	await page.getByLabel('Episode length when not known (minutes)').fill('30');
	// The downloads in these tests are tiny stand-in files: the size floors
	// at import stay off from here on (the Go tests cover them).
	await page.getByTestId('import-floors').uncheck();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved.')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Hours a Release may be early')).toHaveValue('48');
	await expect(page.getByLabel('Episode length when not known (minutes)')).toHaveValue('30');
	await expect(page.getByTestId('import-floors')).not.toBeChecked();
	await page.getByLabel('Hours a Release may be early').fill('24');
	await page.getByLabel('Episode length when not known (minutes)').fill('20');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Search settings saved.').first()).toBeVisible();
});

test('a member sees none of the search screens', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByLabel('Note for yourself').fill('Search viewer');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const m = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-sv`;
	await m.goto(new URL(link).pathname);
	await m.getByLabel('Name').fill('searchviewer');
	await m.getByLabel('Password', { exact: true }).fill(pw);
	await m.getByLabel('Password again').fill(pw);
	await m.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(m).toHaveURL(/\/library$/);
	await expect(m.getByRole('link', { name: 'Held', exact: true })).toHaveCount(0);
	await m.locator('.grid .item', { hasText: 'Quiet Orchard' }).click();
	await expect(m.getByRole('heading', { level: 1 })).toContainText('Quiet Orchard');
	for (const b of ['Search now', 'Choose a release']) await expect(m.getByRole('button', { name: b })).toHaveCount(0);
	await expect(m.getByTestId('release')).toHaveCount(0);
	for (const url of ['/library/held', '/library/turned-down', '/settings/search', '/']) {
		await m.goto(url);
		await expect(m).toHaveURL(/\/library$/);
	}
});

test('no serious accessibility issues on the filled search screens', async () => {
	const scan = async (where: string) => {
		await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
		const r = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
		const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
		expect.soft(bad.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')}`), where).toEqual([]);
	};
	await openMovie(page, 'Quiet Orchard');
	await expect(page.getByTestId('release').first()).toBeVisible();
	await scan('movie page with Releases');
	await release(page, '.1080p.').getByRole('button', { name: /^Choose/ }).click();
	await expect(page.getByRole('dialog')).toBeVisible();
	await scan('choose dialog');
	await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
	await page.goto('/');
	await expect(page.getByTestId('search-found').first()).toBeVisible();
	await scan('Home with searches');
});

test('phone width: the search screens fit', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
	const p = await ctx.newPage();
	await signIn(p, owner());
	await openMovie(p, 'Quiet Orchard');
	await expect(p.getByTestId('release').first()).toBeVisible();
	await noSidewaysScroll(p);
	for (const url of ['/', '/library/held', '/library/turned-down', '/settings/search']) {
		await p.goto(url);
		await expect(p.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(p);
	}
	await ctx.close();
});
