import { expect, test, type Page, type Request } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Music off leaks nothing: with Music turned off in Settings > Parts and
// controls, no screen shows music, the music pages and routes give the
// parts-off answer, and browsing never asks a music endpoint.
test.describe.configure({ mode: 'serial' });

let page: Page;
const seen: string[] = [];
const suggestKinds: string[] = [];

const MUSIC_URL = /\/api\/v1\/(explore\/music|explore\/new\/music|music-trending|music\/|artists\/lookup|albums\/|metadata\/album\/)/;
const musicHits = () => seen.filter((u) => MUSIC_URL.test(u) || /[?&]kind=music\b/.test(u));
const ID = '0383dadf-2a4e-4d10-a46a-e9e041da8eb3';

test.beforeAll(async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
	page = await ctx.newPage();
	page.on('request', (r: Request) => void seen.push(r.url()));
	page.on('response', async (r) => {
		if (!r.url().includes('/api/v1/search/suggest') || !r.ok()) return;
		const a = (await r.json().catch(() => null)) as { items?: { kind: string }[] } | null;
		for (const it of a?.items ?? []) suggestKinds.push(it.kind);
	});
	await signIn(page, owner());
	await page.goto('/settings/parts');
	const music = page.locator('[data-testid="part"][data-part="music"]');
	await expect(music).toBeVisible();
	if ((await music.getAttribute('data-state')) !== 'off') {
		await music.getByRole('button', { name: 'Turn Music off' }).click();
		await page.getByTestId('confirm').getByRole('button', { name: /^Yes, turn music off/ }).click();
	}
	await expect(music).toHaveAttribute('data-state', 'off');
});

test.afterAll(async () => {
	await page.goto('/settings/parts');
	const music = page.locator('[data-testid="part"][data-part="music"]');
	await music.getByRole('button', { name: 'Turn Music on' }).click();
	await expect(music).not.toHaveAttribute('data-state', 'off');
	await page.context().close();
});

const main = () => page.getByRole('main');
const musicRadio = () => main().getByRole('radio', { name: 'Music', exact: true });
const musicLinks = () => main().locator('a[href*="/music/"]');

async function visit(path: string) {
	await page.goto(path);
	await page.waitForLoadState('networkidle');
}

test('the main navigation has no Music item', async () => {
	await visit('/');
	const nav = page.getByRole('navigation', { name: 'Main' }).first();
	await expect(nav).toBeVisible();
	await expect(nav.getByRole('link', { name: /music|artists?/i })).toHaveCount(0);
});

test('Home shows no music asks or rows', async () => {
	await visit('/');
	await expect(main().getByRole('heading', { name: /music|artist|album/i })).toHaveCount(0);
	await expect(musicLinks()).toHaveCount(0);
});

test('Explore has no Music tab and no music rows', async () => {
	await visit('/explore');
	await expect(musicRadio()).toHaveCount(0);
	await expect(musicLinks()).toHaveCount(0);
});

test('Explore opened with type=music falls back and shows no music rows', async () => {
	await visit('/explore?type=music');
	await expect(musicRadio()).toHaveCount(0);
	await expect(musicLinks()).toHaveCount(0);
});

test('New & Popular has no Music tab', async () => {
	await visit('/popular');
	await expect(musicRadio()).toHaveCount(0);
});

test('Search has no music scope and no music genre tiles', async () => {
	await visit('/search');
	await expect(musicRadio()).toHaveCount(0);
	await expect(main().locator('a[href*="/music/genre/"]')).toHaveCount(0);
});

test('Search results show no artists or albums', async () => {
	await visit('/search?q=salt');
	await expect(main().getByRole('heading', { name: 'Music', exact: true })).toHaveCount(0);
	await expect(musicLinks()).toHaveCount(0);
});

test('the live search dropdown offers no artists, albums or songs', async () => {
	await visit('/search');
	await page.getByTestId('search-box').first().fill('the');
	await expect.poll(() => seen.some((u) => u.includes('/api/v1/search/suggest')), { timeout: 5_000 }).toBe(true);
	await page.waitForLoadState('networkidle');
	expect(suggestKinds.filter((k) => k === 'artist' || k === 'album' || k === 'song')).toEqual([]);
});

test('the Library has no Music', async () => {
	await visit('/library');
	await expect(main().getByRole('radio', { name: /^Music/ })).toHaveCount(0);
	await expect(main().getByRole('link', { name: /Add an artist|Search for an artist/ })).toHaveCount(0);
	await visit('/library?kind=artist');
	await expect(main().locator('a[href*="/music/artist/"]')).toHaveCount(0);
});

test('Calendar offers no music', async () => {
	await visit('/calendar');
	await expect(page.getByTestId('cal-titles')).toBeVisible();
	await expect(page.getByTestId('cal-titles').getByRole('radio', { name: 'Music', exact: true })).toHaveCount(0);
});

test('Downloads shows no music', async () => {
	await visit('/downloads');
	await expect(musicRadio()).toHaveCount(0);
	await expect(musicLinks()).toHaveCount(0);
});

test('no music endpoint is asked while browsing Home, Explore, New & Popular, Search and Library', async () => {
	seen.length = 0;
	for (const path of ['/', '/explore', '/popular', '/search', '/search?q=salt', '/library']) await visit(path);
	expect(musicHits()).toEqual([]);
});

test('direct visits to artist and album pages say Music is off', async () => {
	for (const path of [`/music/artist/${ID}`, `/music/album/${ID}`]) {
		await page.goto(path);
		await expect(page.getByTestId('part-off'), path).toBeVisible();
	}
});

test('the music routes give the parts-off answer', async () => {
	for (const path of ['/api/v1/explore/music', '/api/v1/explore/new/music', '/api/v1/music-trending', `/api/v1/music/artists/${ID}`, `/api/v1/music/albums/${ID}`, '/api/v1/artists/lookup?term=salt', `/api/v1/music-trending/art/artist/${ID}`]) {
		const r = await page.request.get(path);
		expect(r.status(), path).toBe(409);
		expect(((await r.json()) as { part: string }).part, path).toBe('music');
	}
});
