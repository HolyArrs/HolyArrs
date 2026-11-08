import { devices, expect, test, type Browser, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openSeasonNamed, addFromSearch, openLibraryTab, inLibrary } from '../helpers/helpers.ts';
import { axe } from '../helpers/axe.ts';

// Several Versions per show (4.8.11): Tidal Atlas is added with a second
// 4K Version from the add dialog; the show page lists both Versions and
// every episode's state per Version; Search now on the season takes the
// 1080p pack for the main Version and the 2160p pack for 4K; History names
// the Version; a Version is changed, a third added and removed, and a
// Version with a download under way is not removed; a phone.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

// Tidal Atlas on its Library tab (seasons, Versions and History live there).
async function openShow(p: Page) {
	await p.goto('/library?kind=show');
	await (await inLibrary(p, 'Tidal Atlas')).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText('Tidal Atlas');
	await openLibraryTab(p);
}

async function openSeason(p: Page) {
	const s = await openSeasonNamed(p, 'Season 1');
	await expect(s.getByRole('button', { name: /^Episode \d+:/ }).first()).toBeVisible();
	return s;
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('add Tidal Atlas with a second 4K Version', async () => {
	const dialog = await addFromSearch(page, 'tidal atlas', 'Tidal Atlas', 'show');
	await dialog.getByLabel('Seasons to keep up to date').selectOption('all');
	await dialog.getByLabel('Also keep a second Version').check();
	await expect(dialog.getByLabel('Version name')).toHaveValue('4K');
	await expect(dialog.getByLabel('Pick rules for this Version').locator('option:checked')).toHaveText('4K');
	await expect(dialog.getByText('the show gets its own folder named with')).toBeVisible();
	await axe(page, 'add a show with a second Version');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(dialog).toBeHidden();

	await openShow(page);
	const versions = page.getByTestId('show-versions').getByTestId('version');
	await expect(versions).toHaveCount(2);
	await expect(versions.nth(0)).toContainText('(main)');
	await expect(versions.nth(1)).toContainText('Pick rules: 4K');
	await expect(versions.nth(1)).toContainText('[4K]');
	const s1 = await openSeason(page);
	await expect(s1.locator('.lib-ep-st').first()).toHaveText('Missing');
});

test('Search now on the season fills each Version with its own pack; History names the Version', async () => {
	await openShow(page);
	const s1 = await openSeason(page);
	await s1.getByRole('button', { name: 'Get the missing episodes of season 1' }).click();
	await expect(page.getByRole('status')).toContainText('Searching for season 1');
	// The season rows are read when opened: reload until the first episode is no longer missing.
	await expect(async () => {
		await page.reload();
		const s = await openSeason(page);
		await expect(s.locator('.lib-ep-st').first()).not.toHaveText('Missing', { timeout: 1000 });
	}).toPass({ timeout: 30_000 });
	// The Library tab's History shows what happened.
	await expect(page.getByTestId('lib-pane').getByTestId('history-row').first()).toBeVisible({ timeout: 20_000 });
	await page.goto('/downloads/history');
	await expect(page.getByTestId('history-version').filter({ hasText: '4K' }).first()).toBeVisible();
	// A download's steps fold into one row; picking it shows each step in the details panel.
	await page.locator('[data-testid="history-row"][data-steps]').first().click();
	const pane = page.getByTestId('look-pane');
	await expect(pane.getByTestId('history-steps').locator('li')).toHaveCount(3);
	await expect(pane.getByTestId('history-steps').locator('[data-state="done"]').first()).toBeVisible();
});

test('change, add and remove Versions; a busy Version stays', async () => {
	await openShow(page);
	const card = page.getByTestId('show-versions');
	// One row shows the main Version; See all lays out every Version.
	await expect(card.getByRole('button', { name: 'Change the 4K Version' })).toBeHidden();
	await card.getByRole('button', { name: 'See all 2' }).click();
	await expect(card.getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true');
	await card.getByRole('button', { name: 'Change the 4K Version' }).click();
	const vd = page.getByRole('dialog').filter({ hasText: 'Change the 4K Version' });
	await vd.getByLabel('Keep up to date').uncheck();
	await vd.getByRole('button', { name: 'Save' }).click();
	await expect(vd).toBeHidden();
	await expect(card.getByTestId('version').nth(1)).toContainText('not kept up to date');

	await card.getByRole('button', { name: 'Add a Version' }).click();
	const ad = page.getByRole('dialog').filter({ hasText: 'Add a Version of Tidal Atlas' });
	await ad.getByLabel('Version name').fill('Small');
	await ad.getByRole('button', { name: 'Add version' }).click();
	await expect(card.getByTestId('version')).toHaveCount(3);
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: 'Version actions for Small' }).click();
	await page.getByRole('menuitem', { name: 'Remove Small' }).click();
	await expect(card.getByTestId('version')).toHaveCount(2);
	await expect(card.getByRole('button', { name: 'Remove Main' })).toHaveCount(0);
	await expect(card.getByRole('button', { name: /^Version actions for Main/ })).toHaveCount(0);

	// The 4K pack is downloading or placed: the Version is not removed.
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: 'Version actions for 4K' }).click();
	await page.getByRole('menuitem', { name: 'Remove 4K' }).click();
	await expect(page.getByText(/download or import under way|still has files/)).toBeVisible();
	await expect(card.getByTestId('version')).toHaveCount(2);
	await axe(page, 'show page with two Versions');
});

test('the Versions card on a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ ...devices['Pixel 7'] });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await openShow(phone);
	await expect(phone.getByTestId('show-versions').getByTestId('version')).toHaveCount(2);
	await noSidewaysScroll(phone);
	await axe(phone, 'show Versions on a phone');
	await ctx.close();
});
