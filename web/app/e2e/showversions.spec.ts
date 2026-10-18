import AxeBuilder from '@axe-core/playwright';
import { devices, expect, test, type Browser, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openSeasonNamed } from './helpers.ts';

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

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

async function openShow(p: Page) {
	await p.goto('/library');
	await p.locator('.grid .item', { hasText: 'Tidal Atlas' }).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText('Tidal Atlas');
}

async function openSeason(p: Page) {
	const s = await openSeasonNamed(p, 'Season 1');
	await expect(s.getByTestId('episode').first()).toBeVisible();
	return s;
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('add Tidal Atlas with a second 4K Version', async () => {
	await page.goto('/search?kind=show');
	await page.getByLabel('Show name or id').fill('tidal atlas');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.getByTestId('show-hit').filter({ hasText: 'Tidal Atlas' });
	await hit.getByRole('button', { name: 'Add' }).click();
	const dialog = page.getByRole('dialog');
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
	await expect(s1.getByTestId('episode').first().getByTestId('episode-state')).toHaveText('Main: Missing · 4K: Missing');
});

test('Search now on the season fills each Version with its own pack; History names the Version', async () => {
	await openShow(page);
	const s1 = await openSeason(page);
	await s1.getByTestId('search-season').click();
	const first = s1.getByTestId('episode').first().getByTestId('episode-state');
	// One pill names both Versions: neither may still be missing.
	await expect(first).not.toContainText('Main: Missing', { timeout: 30_000 });
	await expect(first).not.toContainText('4K: Missing', { timeout: 30_000 });
	await page.reload();
	await expect(page.getByTestId('title-history').getByTestId('event-version').filter({ hasText: '4K' }).first()).toBeVisible({ timeout: 20_000 });
	await page.goto('/downloads/history');
	await expect(page.getByTestId('history-version').filter({ hasText: '4K' }).first()).toBeVisible();
	// A download's steps fold into one row with a step line; expanding lists each step.
	const grouped = page.getByTestId('history-row').filter({ has: page.getByTestId('history-steps') }).first();
	await expect(grouped.getByTestId('history-steps').locator('li')).toHaveCount(3);
	await expect(grouped.getByTestId('history-steps').locator('[data-state="done"]').first()).toBeVisible();
	await grouped.getByRole('button', { name: /^Every step \(\d+\)$/ }).click();
	expect(await grouped.getByTestId('history-trail').locator(':scope > li').count()).toBeGreaterThan(1);
	await expect(grouped.getByRole('button', { name: 'Hide steps' })).toHaveAttribute('aria-expanded', 'true');
});

test('change, add and remove Versions; a busy Version stays', async () => {
	await openShow(page);
	const card = page.getByTestId('show-versions');
	await card.getByRole('button', { name: 'Change the 4K Version' }).click();
	const vd = page.getByRole('dialog').filter({ hasText: 'Change the 4K Version' });
	await vd.getByLabel('Keep up to date').uncheck();
	await vd.getByRole('button', { name: 'Save' }).click();
	await expect(vd).toBeHidden();
	await expect(card.getByTestId('version').nth(1)).toContainText('not kept up to date');

	await card.getByRole('button', { name: 'Add a Version' }).click();
	const ad = page.getByRole('dialog').filter({ hasText: 'Add a Version of Tidal Atlas' });
	await ad.getByLabel('Version name').fill('Small');
	await ad.getByRole('button', { name: 'Add Version' }).click();
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
