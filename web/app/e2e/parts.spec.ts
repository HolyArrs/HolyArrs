import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// Parts (3.7.6): with the Wishlist and Music turned off through the API
// (the controls screen waits on its design), their menu entries and
// Settings tiles hide, a direct link says the part is off, the routes
// answer "this part is off", and everything is back when they are on.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function control(p: Page, path: string): Promise<number> {
	const me = (await (await p.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const headers = { 'X-CSRF-Token': me.csrf, 'Content-Type': 'application/json' };
	let res = await p.request.post('/api/v1' + path, { headers, data: {} });
	if (res.status() === 202) {
		const ask = (await res.json()) as { confirm: string };
		res = await p.request.post('/api/v1' + path, { headers, data: { confirm: ask.confirm } });
	}
	return res.status();
}

test.beforeAll(async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
	page = await ctx.newPage();
	await signIn(page, owner());
});

test.afterAll(async () => {
	await control(page, '/parts/music/on');
	await control(page, '/parts/wishlist/on');
});

test('a part that is off hides its pages and says so on a direct link', async () => {
	const main = page.getByRole('navigation', { name: 'Main' }).first();
	// The owner approves, so the Wishlist is Approvals in the Manage section.
	await expect(main.getByRole('link', { name: /^Approvals/ })).toBeVisible();
	expect(await control(page, '/parts/wishlist/off')).toBe(200);
	expect(await control(page, '/parts/music/off')).toBe(200);

	await page.goto('/library');
	await expect(main.getByRole('link', { name: 'Movies' })).toBeVisible();
	await expect(main.getByRole('link', { name: /^Approvals/ })).toHaveCount(0);
	await expect(main.getByRole('link', { name: 'Music' })).toHaveCount(0);

	await page.goto('/wishlist');
	const off = page.getByTestId('part-off');
	await expect(off).toBeVisible();
	await expect(off).toContainText('The Wishlist is turned off');
	await page.goto('/settings/music');
	await expect(off).toContainText('Music is turned off');
	const r = await page.request.get('/api/v1/settings/music');
	expect(r.status()).toBe(409);
	expect(((await r.json()) as { error: string }).error).toBe('this part is off');

	await page.goto('/settings');
	await expect(page.getByRole('main').getByRole('link').filter({ hasText: 'Sources' })).toBeVisible();
	await expect(page.getByRole('main').getByRole('link').filter({ hasText: 'MusicBrainz' })).toHaveCount(0);
	await expect(page.getByRole('main').getByRole('link').filter({ hasText: 'Weekly limits' })).toHaveCount(0);

	await page.goto('/wishlist');
	await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(axe.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id)).toEqual([]);
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(off).toBeVisible();
	await noSidewaysScroll(page);
	await page.setViewportSize({ width: 1280, height: 900 });
});

test('turned on again, every page is back with its data', async () => {
	expect(await control(page, '/parts/music/on')).toBe(200);
	expect(await control(page, '/parts/wishlist/on')).toBe(200);
	await page.goto('/wishlist');
	await expect(page.getByTestId('part-off')).toHaveCount(0);
	await expect(page.getByRole('heading', { level: 1, name: 'Wishlist' })).toBeVisible();
	await page.goto('/settings/music');
	await expect(page.getByTestId('part-off')).toHaveCount(0);
	expect((await page.request.get('/api/v1/settings/music')).status()).toBe(200);
});
