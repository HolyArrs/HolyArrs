import { expect, test, type Route } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// First visit on a fresh install: the server can answer Explore's head
// before the hero's pictures are ready. The page shows the first titles of
// its rows as the hero (never an empty box) and swaps the server's hero in
// once it arrives, without a reload.

const tiny = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

const card = (i: number) => ({
	tmdb_id: String(9100 + i), kind: 'show', name: `Hero show ${i}`, year: 2026,
	poster: `https://image.tmdb.org/t/p/w500/h${i}.jpg`, backdrop: `https://image.tmdb.org/t/p/original/h${i}b.jpg`,
	card_logo: `https://image.tmdb.org/t/p/original/h${i}l.png`
});

test('Explore shows the hero on the first visit without a reload', async ({ page }) => {
	await page.route('https://image.tmdb.org/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await page.route('**/api/v1/artwork/tmdb/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await signIn(page, owner());
	let asks = 0;
	await page.route('**/api/v1/explore/head?**', (r: Route) => {
		asks++;
		const head = [{ id: 'row0', type: 'genre', title: 'Row 0', items: Array.from({ length: 8 }, (_, i) => card(20 + i)) }];
		// The first answer has no hero yet (its pictures are still being fetched).
		return r.fulfill({ json: { type: 'shows', building: false, seeds: 0, tail_cap: 0, tail_rows: 0, hero: asks === 1 ? [] : [card(1), card(2)], head } });
	});
	await page.goto('/explore?type=shows');
	await expect(page.getByTestId('explore-hero')).toBeVisible();
	// A mark on the page: a reload would wipe it.
	await page.evaluate(() => ((window as unknown as { heroMark: boolean }).heroMark = true));
	await expect(page.getByTestId('explore-hero')).toContainText(/Hero show [12](?!\d)/, { timeout: 15_000 });
	await expect(page.getByTestId('hero-placeholder')).toHaveCount(0);
	expect(asks).toBeGreaterThanOrEqual(2);
	expect(await page.evaluate(() => (window as unknown as { heroMark?: boolean }).heroMark)).toBe(true);
});
