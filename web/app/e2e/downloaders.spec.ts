import AxeBuilder from '@axe-core/playwright';
import { devices, expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';
import { qbitPort, sabKey, sabPort } from './fakes.mjs';

// Settings > Downloaders against the fake qBittorrent and SABnzbd in
// fakes.mjs, then a movie whose download is marked failed: the next
// Release is sent by itself. Runs after search.spec.ts (the Lab Torznab
// Source is there).
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

const row = (name: string) => page.getByTestId('downloader').filter({ hasText: name });

async function addDownloader(kind: 'qBittorrent' | 'SABnzbd', name: string, url: string, secret = '', category = '') {
	await page.getByRole('button', { name: 'Add a Downloader' }).first().click();
	await page.getByRole('radio', { name: new RegExp(kind) }).check();
	await page.getByLabel('Name', { exact: true }).fill(name);
	await page.getByLabel('Address', { exact: true }).fill(url);
	if (secret) await page.getByLabel(kind === 'SABnzbd' ? 'API key' : 'Password', { exact: true }).fill(secret);
	if (category) await page.getByLabel('Category', { exact: true }).fill(category);
	await page.getByRole('button', { name: 'Save Downloader' }).click();
}

test('a SABnzbd is added and tested, a wrong key is said in plain words, it pauses and resumes', async () => {
	await page.goto('/settings/downloaders');
	await expect(page.getByRole('heading', { level: 1, name: 'Downloaders' })).toBeVisible();

	await addDownloader('SABnzbd', 'Lab SABnzbd', `http://127.0.0.1:${sabPort}`, sabKey, 'movies');
	const r = row('Lab SABnzbd');
	await expect(r.getByTestId('downloader-test')).toHaveText('It answered: sabnzbd 5.1.3.');
	await expect(r.getByTestId('downloader-state')).toHaveText('Working');
	await expect(r).toContainText('Usenet');
	await expect(r).not.toContainText(sabKey);

	// A wrong key.
	await r.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('API key', { exact: true }).fill('not-the-key');
	await page.getByRole('button', { name: 'Save Downloader' }).click();
	await expect(r.getByTestId('downloader-test')).toContainText('turned down the key');
	await expect(r.getByTestId('downloader-state')).toHaveText('Sign-in refused');

	// A category SABnzbd does not have.
	await r.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('API key', { exact: true }).fill(sabKey);
	await page.getByLabel('Category', { exact: true }).fill('holyarrs');
	await page.getByRole('button', { name: 'Save Downloader' }).click();
	await expect(r.getByTestId('downloader-test')).toContainText('category does not exist in SABnzbd');
	await r.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('Category', { exact: true }).fill('movies');
	await page.getByRole('button', { name: 'Save Downloader' }).click();
	await expect(r.getByTestId('downloader-state')).toHaveText('Working');

	await r.getByRole('button', { name: 'Pause' }).click();
	await expect(r.getByTestId('downloader-state')).toHaveText('Paused');
	await expect(r.getByTestId('downloader-text')).toContainText('Paused by you');
	await r.getByRole('button', { name: 'Resume' }).click();
	await expect(r.getByTestId('downloader-state')).toHaveText('Working');
});

test('the settings for downloads that go wrong are saved and checked', async () => {
	await page.goto('/settings/downloaders');
	await expect(page.getByLabel('Stalled after (hours without progress)')).toHaveValue('6');
	await expect(page.getByLabel('Give up and try the next Release after (hours)')).toHaveValue('24');
	await page.getByLabel('Stalled after (hours without progress)').fill('8');
	await page.getByLabel('Give up and try the next Release after (hours)').fill('4');
	await page.getByRole('button', { name: 'Save settings' }).click();
	await expect(page.getByRole('alert')).toContainText('no sooner than it counts as stalled');
	await page.getByLabel('Give up and try the next Release after (hours)').fill('30');
	await page.getByRole('button', { name: 'Save settings' }).click();
	await expect(page.getByText('Downloader settings saved.')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Stalled after (hours without progress)')).toHaveValue('8');
	await page.getByLabel('Stalled after (hours without progress)').fill('6');
	await page.getByLabel('Give up and try the next Release after (hours)').fill('24');
	await page.getByRole('button', { name: 'Save settings' }).click();
	await expect(page.getByText('Downloader settings saved.')).toBeVisible();
});

test('a movie downloads through qBittorrent; Mark failed sends the next Release by itself', async () => {
	await page.goto('/settings/downloaders');
	await addDownloader('qBittorrent', 'Lab qBittorrent', qbit);
	await expect(row('Lab qBittorrent').getByTestId('downloader-test')).toHaveText('It answered: qbittorrent v5.2.4.');

	// Add a movie and let it search now.
	await page.goto('/search');
	await page.getByLabel('Movie name or id').fill('iron lullaby');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.locator('li.hit', { hasText: 'Iron Lullaby' });
	await hit.getByRole('button', { name: 'Add' }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Search now').check();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await hit.getByRole('link', { name: 'In your Library' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Iron Lullaby');

	const dl = page.getByTestId('download');
	await expect(dl.getByTestId('download-state')).toHaveText('Downloading', { timeout: 30_000 });
	await expect(dl).toContainText('40%');
	await expect(dl.getByTestId('download-text')).toHaveText('40% done.');
	let state = await (await fetch(`${qbit}/state`)).json();
	expect(state.added).toHaveLength(1);
	const first = state.added[0];

	page.once('dialog', (d) => d.accept());
	await dl.getByRole('button', { name: 'Mark failed' }).click();
	await expect(page.getByText('Marked failed. HolyArrs looks for the next Release.')).toBeVisible();
	// The failed one leaves qBittorrent and the 720p copy is sent instead.
	await expect.poll(async () => (await (await fetch(`${qbit}/state`)).json()).added.length, { timeout: 30_000 }).toBe(2);
	state = await (await fetch(`${qbit}/state`)).json();
	expect(state.deleted).toContain(first);
	expect(state.added[1]).not.toBe(first);
	await expect(dl.getByTestId('download-state')).toHaveText('Downloading', { timeout: 30_000 });

	// The failed Release is on the Turned down list with the reason.
	await page.goto('/library/turned-down');
	const td = page.getByTestId('listed-release').filter({ hasText: 'Iron.Lullaby.2015.1080p' });
	await expect(td).toBeVisible();
	await expect(td).toContainText('marked failed by');
});

test('the Downloaders page and a downloading movie pass the accessibility scan and fit a phone', async ({ browser }) => {
	for (const scheme of ['light', 'dark'] as const) {
		const ctx = await browser.newContext({ colorScheme: scheme });
		await ctx.addInitScript((t) => localStorage.setItem('holyarrs-theme', t), scheme);
		const p = await ctx.newPage();
		await signIn(p, owner());
		for (const where of ['/settings/downloaders', 'movie']) {
			if (where === 'movie') {
				await p.goto('/library');
				await p.locator('.grid .item', { hasText: 'Iron Lullaby' }).click();
				await expect(p.getByTestId('download')).toBeVisible();
			} else {
				await p.goto(where);
				await expect(p.getByTestId('downloader').first()).toBeVisible();
			}
			await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
			const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
			const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
			expect.soft(bad.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')}`), `${where} (${scheme})`).toEqual([]);
		}
		await ctx.close();
	}
	const phone = await browser.newContext({ ...devices['Pixel 7'] });
	const p = await phone.newPage();
	await signIn(p, owner());
	await p.goto('/settings/downloaders');
	await expect(p.getByTestId('downloader').first()).toBeVisible();
	await noSidewaysScroll(p);
	await p.getByRole('button', { name: 'Add a Downloader' }).click();
	await noSidewaysScroll(p);
	await p.goto('/library');
	await p.locator('.grid .item', { hasText: 'Iron Lullaby' }).click();
	await expect(p.getByTestId('download')).toBeVisible();
	await noSidewaysScroll(p);
	await phone.close();
});
