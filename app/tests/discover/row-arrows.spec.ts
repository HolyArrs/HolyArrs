import { expect, test, type Locator, type Page, type Route } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Browse-row arrows use the large icon button (44 px) on a desktop pointer,
// and My account > Hover previews (On / Slower / Off) stops posters growing on hover.

const tiny = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

function head(route: Route) {
	const rows = Array.from({ length: 2 }, (_, r) => ({
		id: `row${r}`, type: 'genre', title: `Row ${r}`,
		items: Array.from({ length: 20 }, (_, i) => ({
			tmdb_id: String(7000 + r * 100 + i), kind: 'movie', name: `row${r} ${i}`, year: 2021,
			poster: `https://image.tmdb.org/t/p/w500/a${r}${i}.jpg`, backdrop: `https://image.tmdb.org/t/p/original/a${r}${i}b.jpg`
		}))
	}));
	return route.fulfill({ json: { type: 'all', building: false, seeds: 0, tail_cap: 0, tail_rows: 0, hero: [], head: rows } });
}

async function explore(page: Page) {
	await page.route('https://image.tmdb.org/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await page.route('**/api/v1/artwork/tmdb/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await page.route('**/api/v1/explore/head?**', head);
	await page.goto('/explore');
	await expect(page.locator('.ui-shelf .row a').first()).toBeVisible();
}

// Rest on a card the way a person does: move onto it, then pause (a scroll just before holds previews back).
async function rest(page: Page, card: Locator) {
	await card.scrollIntoViewIfNeeded();
	await page.waitForTimeout(400);
	const b = (await card.boundingBox())!;
	await page.mouse.move(b.x + 5, b.y + 5);
	await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 8 });
}

test.use({ viewport: { width: 1600, height: 900 } });

test('row arrows are the large size, and Off in Hover previews keeps posters still', async ({ page }) => {
	await signIn(page, owner());
	await explore(page);
	const shelf = page.locator('.ui-shelf').filter({ has: page.getByRole('heading', { name: 'Row 0', exact: true }) });
	await shelf.scrollIntoViewIfNeeded();
	await shelf.hover();
	const fwd = shelf.locator('.arrow.right .ui-ibtn');
	await expect(fwd).toBeVisible();
	const box = await fwd.boundingBox();
	expect(Math.round(box!.width)).toBe(44);
	expect(Math.round(box!.height)).toBe(44);

	// Default (On): resting on a poster grows it.
	await rest(page, shelf.locator('.row a').nth(1));
	await expect(page.locator('.ui-grow')).toBeVisible();
	await page.mouse.move(5, 5);
	await expect(page.locator('.ui-grow')).toHaveCount(0);

	// Off, then back on: twice, so the choice really toggles.
	for (let round = 0; round < 2; round++) {
		await page.goto('/account');
		const row = page.locator('#previews-me');
		await expect(row).toContainText('Hover previews');
		await row.getByRole('radio', { name: 'Off' }).click();
		await explore(page);
		await rest(page, shelf.locator('.row a').nth(1));
		await page.waitForTimeout(1200);
		await expect(page.locator('.ui-grow')).toHaveCount(0);

		await page.goto('/account');
		await page.locator('#previews-me').getByRole('radio', { name: 'On' }).click();
		expect(await page.evaluate(() => localStorage.getItem('holyarrs.preview.hover'))).toBeNull();
	}
});

async function setPref(page: Page, name: 'On' | 'Slower' | 'Off') {
	await page.goto('/account');
	await page.locator('#previews-me').getByRole('radio', { name }).click();
}

test('Off: a fast sweep grows nothing and fetches no trailer; keyboard focus still opens the preview', async ({ page }) => {
	await signIn(page, owner());
	await setPref(page, 'Off');
	const trailerCalls: string[] = [];
	// The hero (the first title, as this head sends none) plays its own trailer: Hover previews is about the cards.
	page.on('request', (r) => { if (/trailer/i.test(r.url()) && !r.url().includes('/trailer/batch') && !r.url().includes('/trailer/movie/7000?')) trailerCalls.push(r.url()); });
	await explore(page);
	const shelf = page.locator('.ui-shelf').first();
	await shelf.scrollIntoViewIfNeeded();
	await page.waitForTimeout(400);
	const cards = shelf.locator('.row a');
	for (let i = 0; i < 6; i++) {
		const b = (await cards.nth(i).boundingBox())!;
		await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 3 });
	}
	await page.waitForTimeout(1200);
	await expect(page.locator('.ui-grow')).toHaveCount(0);
	expect(trailerCalls).toEqual([]);
	// Keyboard: focus a tile with Tab so it is :focus-visible.
	await page.mouse.move(5, 5);
	await cards.nth(0).focus();
	await page.keyboard.press('Tab');
	await expect(page.locator('.ui-grow')).toBeVisible({ timeout: 3000 });
	await page.keyboard.press('Escape');
	await setPref(page, 'On');
});

test('Slower waits about a second; the arrows mirror and still page in right-to-left', async ({ page }) => {
	await signIn(page, owner());
	await setPref(page, 'Slower');
	await explore(page);
	const shelf = page.locator('.ui-shelf').first();
	await rest(page, shelf.locator('.row a').nth(1));
	await page.waitForTimeout(500);
	await expect(page.locator('.ui-grow')).toHaveCount(0);
	await expect(page.locator('.ui-grow')).toBeVisible({ timeout: 1500 });
	await page.mouse.move(5, 5);
	await setPref(page, 'On');

	await explore(page);
	await page.evaluate(() => (document.documentElement.dir = 'rtl'));
	const s = page.locator('.ui-shelf').first();
	await s.scrollIntoViewIfNeeded();
	await s.hover({ position: { x: 300, y: 10 } });
	const row = (await s.locator('.row').boundingBox())!;
	const fwd = s.locator('.arrow.right .ui-ibtn');
	await expect(fwd).toBeVisible();
	const fb = (await fwd.boundingBox())!;
	expect(fb.x + fb.width / 2).toBeLessThan(row.x + row.width / 2);
	expect(Math.round(fb.width)).toBe(44);
	const before = await s.locator('.row').evaluate((el) => el.scrollLeft);
	await fwd.click();
	await expect.poll(() => s.locator('.row').evaluate((el) => el.scrollLeft)).not.toBe(before);
});

test.describe('phone 390', () => {
	test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
	test('no row arrows on touch, and My account fits with the new row', async ({ page }) => {
		await signIn(page, owner());
		await explore(page);
		await expect(page.locator('.ui-shelf .arrow .ui-ibtn').first()).toBeHidden();
		await page.goto('/account');
		const row = page.locator('#previews-me');
		await row.scrollIntoViewIfNeeded();
		await expect(row).toBeVisible();
		expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
		const rb = (await row.boundingBox())!;
		expect(rb.x + rb.width).toBeLessThanOrEqual(390);
	});
});
