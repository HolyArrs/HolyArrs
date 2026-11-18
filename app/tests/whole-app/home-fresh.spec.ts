import { expect, test, type Page } from '@playwright/test';
import { join } from 'node:path';
import { noSidewaysScroll, signIn, setUpFresh } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

// Home before setup: the owner finished the wizard with only a Movies folder
// and nothing in the Library yet. Home shows the compact welcome card, then
// its own grid and Explore's picture rows, with even gaps.
// Own fresh app: npx playwright test -c ../tests/setups/home-fresh.config.ts
// HOME_SHOTS=<folder> also saves 1440, 1280, 1100, 900 and 390, light and dark.
test.describe.configure({ mode: 'serial' });

const tmp = dataDir('phone');
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
const owner = () => ({ name: 'luis', password: pw });

const item = (id: number) => ({ kind: id % 2 ? 'movie' : 'show', tmdb_id: 9000 + id, name: `Pick ${id}`, year: 2026, poster: `/p${id}.jpg`, backdrop: `/b${id}.jpg` });
// A loadable picture for the tile art (the fake paths above never load).
const art = (a: string, b: string) => `data:image/svg+xml,${encodeURIComponent(`<svg xmlns='http://www.w3.org/2000/svg' width='600' height='900'><linearGradient id='g' x2='1' y2='1'><stop stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient><rect width='600' height='900' fill='url(#g)'/><circle cx='420' cy='260' r='170' fill='rgb(255 255 255 / 18%)'/></svg>`)}`;
const row = (id: string, title: string) => ({ id, type: 'trending', title, wide: true, items: Array.from({ length: 8 }, (_, i) => item(i + 1)) });
const head = { type: 'all', building: false, seeds: 0, tail_cap: 0, tail_rows: 0, head: [row('trending_now', 'Trending now'), row('trending_week', 'Trending this week')] };

const today = () => {
	const d = new Date();
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

// The grid has no holes: its tiles cover every cell of its rows.
async function gridFull(page: Page) {
	const r = await page.getByTestId('home-board').evaluate((g) => {
		const cols = getComputedStyle(g).gridTemplateColumns.split(' ').length;
		const box = g.getBoundingClientRect();
		const gap = parseFloat(getComputedStyle(g).rowGap);
		const tiles = [...g.children].map((c) => c.getBoundingClientRect());
		const unitW = (box.width - gap * (cols - 1)) / cols;
		const unitH = parseFloat(getComputedStyle(g).gridAutoRows);
		const cells = tiles.reduce((t, b) => t + Math.round((b.width + gap) / (unitW + gap)) * Math.round((b.height + gap) / (unitH + gap)), 0);
		const rows = Math.round((box.height + gap) / (unitH + gap));
		return { cells, all: cols * rows };
	});
	expect(r.cells).toBe(r.all);
}

async function freshHome(page: Page) {
	await page.route('**/api/**/explore/head**', (r) => r.fulfill({ json: head }));
	// New & Popular's first page gives the New & Popular and Calendar tiles their art.
	await page.route((u) => u.pathname.endsWith('/explore/new'), (r) => r.fulfill({ json: { type: 'all', page: 0, more: false, building: false, hero: [{ ...item(3), backdrop: art('#7a2e3a', '#1d2a44') }], rail: [{ day: today(), items: [7, 8].map((i) => ({ ...item(i), poster: art('#3b4f7a', '#111'), what: 'Premiere', following: false, upcoming: true })) }, { day: '2099-12-01', items: [4, 5, 6].map((i) => ({ ...item(i), poster: art(['#2f5d62', '#6b3e75', '#8a5a2b'][i - 4], '#111'), what: 'Premiere', following: false, upcoming: true })) }], rows: [] } }));
	await signIn(page, owner());
	await page.goto('/');
	await expect(page.getByTestId('home-welcome')).toBeVisible();
}

test.beforeAll(async ({ browser }) => {
	test.setTimeout(240_000);
	const page = await browser.newPage();
	await setUpFresh(page, owner(), join(tmp, 'movies'));
	await page.context().close();
});

test('home before setup: welcome card on top, then the grid and the picture rows', async ({ page }) => {
	await page.setViewportSize({ width: 1280, height: 900 });
	await freshHome(page);
	const welcome = page.getByTestId('home-welcome');
	const grid = page.getByTestId('home-overview');
	await expect(grid).toBeVisible();
	// The Library's place points to the step still missing, not an empty gap.
	const lib = page.getByTestId('home-setup-library');
	await expect(lib).toContainText('Not set up yet');
	await expect(lib).toHaveAttribute('href', /\/settings\/(sources|downloaders|library-folders)$/);
	await expect(page.getByTestId('home-attention')).toHaveCount(0);
	await expect(page.getByTestId('home-pick')).toHaveCount(2);
	// Compact: the card is shorter than the grid, and sits above it.
	const w = (await welcome.boundingBox())!;
	const g = (await grid.boundingBox())!;
	expect(w.y).toBeLessThan(g.y);
	expect(w.height).toBeLessThan(260);
	// One gap between every block on the page.
	const gaps = await page.evaluate(() => {
		const kids = [...document.querySelectorAll<HTMLElement>('[data-testid="home-welcome"], [data-testid="home-overview"], [data-testid="home-pick"] > .ui-shelf')]
			.map((e) => e.getBoundingClientRect())
			.sort((a, b) => a.top - b.top);
		return kids.slice(1).map((r, i) => Math.round(r.top - kids[i].bottom));
	});
	expect(Math.max(...gaps) - Math.min(...gaps)).toBeLessThanOrEqual(1);
});

test('home before setup: Coming this week borrows New & Popular, opens the Calendar, and the Calendar stays two rows tall', async ({ page }) => {
	await page.setViewportSize({ width: 1100, height: 900 });
	await freshHome(page);
	const week = page.getByTestId('home-week');
	await expect(week).toBeVisible();
	await expect(week).toHaveAttribute('href', /\/calendar$/);
	await expect(week).toContainText('Coming this week');
	await expect(week).toContainText('2');
	await expect(week).toContainText('Popular releases landing this week');
	const cal = page.locator('[data-testid="home-board"] a[href$="/calendar"]').filter({ hasText: "What's next" });
	const h = await cal.evaluate((el) => el.getBoundingClientRect().height);
	const wh = await week.evaluate((el) => el.getBoundingClientRect().height);
	expect(h).toBeGreaterThan(wh * 1.8);
	await gridFull(page);
});

test('home before setup: Add on a picture opens the missing setup step', async ({ page }) => {
	await page.setViewportSize({ width: 1280, height: 900 });
	await freshHome(page);
	const card = page.getByTestId('home-pick').first().getByRole('link', { name: /Pick 1/ }).first();
	await expect(async () => {
		await page.mouse.move(0, 0);
		await card.hover();
		await expect(page.getByRole('link', { name: 'Finish setup to add Pick 1' })).toBeVisible({ timeout: 2_000 });
	}).toPass({ timeout: 20_000 });
	await expect(page.getByRole('link', { name: 'Finish setup to add Pick 1' })).toHaveAttribute('href', /\/settings\/(sources|downloaders|library-folders)$/);
});

for (const width of [1440, 1280, 1100, 900, 390]) {
	for (const scheme of ['light', 'dark'] as const) {
		test(`home before setup at ${width}px, ${scheme}: no sideways scroll`, async ({ browser }) => {
			const ctx = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme });
			const page = await ctx.newPage();
			await freshHome(page);
			const dir = process.env.HOME_SHOTS;
			if (dir) {
				await page.waitForLoadState('networkidle');
				await page.screenshot({ path: join(dir, `${process.env.HOME_SHOTS_TAG ?? 'after'}-${width}-${scheme}.png`), fullPage: true });
			}
			await expect(page.getByTestId('home-overview')).toBeVisible();
			await gridFull(page);
			await noSidewaysScroll(page);
			await ctx.close();
		});
	}
}
