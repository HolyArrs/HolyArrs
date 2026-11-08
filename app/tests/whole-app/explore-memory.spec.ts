import { expect, test, type CDPSession, type Page, type Route } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Explore memory: after 20 Home -> Explore -> title -> Home rounds the
// heap, DOM nodes and event listeners come back to where they started
// (within 10%). Discover rows are full (20 cards each) so a leak shows.

type Metrics = { heap: number; nodes: number; listeners: number };

const tiny = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

function discoverRow(route: Route) {
	const row = new URL(route.request().url()).searchParams.get('row') ?? 'trending';
	const items = Array.from({ length: 20 }, (_, i) => ({
		tmdb_id: String(9000 + i), kind: i % 2 ? 'show' : 'movie', name: `${row} ${i}`, year: 2020 + (i % 5),
		poster: `https://image.tmdb.org/t/p/w500/${row}${i}.jpg`, backdrop: `https://image.tmdb.org/t/p/original/${row}${i}b.jpg`
	}));
	return route.fulfill({ json: { row, items } });
}

// Explore reads one server-built head now: six full rows, the first card of
// each a Library title (so a card opens /library/<id>).
function exploreHead(route: Route, libraryId: number) {
	const head = Array.from({ length: 6 }, (_, r) => ({
		id: `row${r}`, type: 'genre', title: `Row ${r}`,
		items: Array.from({ length: 20 }, (_, i) => ({
			tmdb_id: String(9000 + r * 100 + i), kind: i % 2 ? 'show' : 'movie', name: `row${r} ${i}`, year: 2020 + (i % 5),
			poster: `https://image.tmdb.org/t/p/w500/r${r}i${i}.jpg`, backdrop: `https://image.tmdb.org/t/p/original/r${r}i${i}b.jpg`,
			in_library: i === 0, ...(i === 0 ? { title_id: libraryId } : {})
		}))
	}));
	return route.fulfill({ json: { type: 'all', building: false, seeds: 0, tail_cap: 0, tail_rows: 0, hero: [], head } });
}

async function metrics(cdp: CDPSession): Promise<Metrics> {
	for (let i = 0; i < 3; i++) await cdp.send('HeapProfiler.collectGarbage');
	const { metrics } = await cdp.send('Performance.getMetrics');
	const get = (n: string) => metrics.find((m) => m.name === n)?.value ?? 0;
	return { heap: get('JSHeapUsedSize'), nodes: get('Nodes'), listeners: get('JSEventListeners') };
}

async function round(page: Page) {
	const nav = page.getByRole('navigation', { name: 'Main' }).first();
	await nav.getByRole('link', { name: 'Explore', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Explore' })).toBeVisible();
	const card = page.locator('main :is(.ui-poster, .ui-wide) a.hit[href*="/library/"]').first();
	await expect(card).toBeVisible();
	await card.click();
	await expect(page).toHaveURL(/\/library\/\d+/);
	await nav.getByRole('link', { name: 'Home', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();
}

test('Explore gives memory back after 20 rounds', async ({ browser }) => {
	test.setTimeout(240_000);
	const context = await browser.newContext();
	const page = await context.newPage();
	await page.route('**/api/v1/discover?**', discoverRow);
	await page.route('https://image.tmdb.org/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await page.route('**/api/v1/artwork/tmdb/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await signIn(page, owner());
	const libraryId: number = await page.evaluate(async () => (await (await fetch('/api/v1/titles')).json()).titles[0].id);
	await page.route('**/api/v1/explore/head?**', (r) => exploreHead(r, libraryId));
	const cdp = await context.newCDPSession(page);
	await cdp.send('Performance.enable');

	// Warm up first: code chunks, caches and the engine's compiled code
	// count in the baseline, so only what the rounds keep shows as growth.
	for (let i = 0; i < 10; i++) await round(page);
	await page.waitForTimeout(500);
	const base = await metrics(cdp);

	for (let i = 0; i < 20; i++) await round(page);
	await page.waitForTimeout(500);
	const after = await metrics(cdp);
	const table = (['heap', 'nodes', 'listeners'] as const).map((k) => `${k}: ${Math.round(base[k])} -> ${Math.round(after[k])} (${(((after[k] - base[k]) / base[k]) * 100).toFixed(1)}%)`);
	console.log(`explore memory\n${table.join('\n')}`);
	for (const k of ['heap', 'nodes', 'listeners'] as const) expect(after[k], `${k}: ${table.join('; ')}`).toBeLessThanOrEqual(base[k] * 1.1);

	// A slow answer must not keep the page that asked: Explore is left while
	// its "new on your services" request is still open (held 3 s here), and the
	// page is gone at once, not only when the answer comes (each open request
	// used to keep a whole Explore page, about 7000 nodes).
	await page.route('**/api/v1/explore/new/services**', async (r) => {
		await new Promise((f) => setTimeout(f, 3000));
		await r.continue().catch(() => {});
	});
	await round(page);
	await page.waitForTimeout(300);
	const open = await metrics(cdp);
	await page.waitForTimeout(3500);
	const settled = await metrics(cdp);
	expect(open.nodes - settled.nodes, `nodes with a slow request open ${open.nodes}, after it answered ${settled.nodes}`).toBeLessThan(2000);

	// Row cards ask the artwork cache for a picture sized to the card
	// (w92-w342 posters and logos, w300 or w780 backdrops), never a full-size one.
	await page.getByRole('navigation', { name: 'Main' }).first().getByRole('link', { name: 'Explore', exact: true }).click();
	const tmdbImg = page.locator('main img[src*="/artwork/tmdb/"]').first();
	await expect(tmdbImg).toBeAttached();
	const srcs = await page.locator('main img[src*="/artwork/tmdb/"]').evaluateAll((els) => els.map((e) => (e as HTMLImageElement).getAttribute('src') ?? ''));
	expect(srcs.filter((s) => !/\/artwork\/tmdb\/(w92|w154|w185|w300|w342|w780)\//.test(s))).toEqual([]);

	await context.close();
});
