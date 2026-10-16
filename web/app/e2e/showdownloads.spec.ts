import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';
import { qbitPort } from './fakes.mjs';

// Show downloads on the Downloads screen and Needs a decision (4.2.3,
// 4.2.4). Runs after showsearch.spec.ts: the Moss Parade season pack fills
// S01E01 and S01E02 with one download and Harbour Lights S02E01 (720p) is
// downloading, both at 40% in the fake qBittorrent.
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;
const dl = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'downloads');

// The fake indexer's info hash for a Release name (fakes.mjs).
const hashOf = (name: string) => [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7).toString(16).padStart(8, '0').repeat(5);

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

const row = (name: string) => page.getByTestId('download-row').filter({ hasText: name }).first();

test('the Downloads screen names the show and the episodes each download covers', async () => {
	await page.goto('/downloads');
	const pack = row('Moss.Parade.S01.1080p');
	await expect(pack.getByRole('link', { name: 'Moss Parade', exact: true })).toBeVisible();
	await expect(pack.getByTestId('row-covers')).toHaveText('S01E01-E02, 2 episodes');
	await pack.getByTestId('row-episodes').locator('summary').click();
	await expect(pack.getByTestId('row-episode')).toHaveCount(2);
	await expect(pack.getByTestId('row-episode').first()).toContainText('S01E01');
	await expect(pack.getByTestId('row-episode').first()).toContainText('Green Start');
	await expect(pack.getByTestId('row-episode').nth(1)).toContainText('Downloading');
	await expect(row('Harbour.Lights.S02E01.720p').getByTestId('row-covers')).toHaveText('S02E01 Return of the Fog');
	await axe(page, 'downloads with shows');
});

test('a finished episode is imported into its episode and the row says so', async () => {
	const name = 'Harbour.Lights.S02E01.720p.WEB.H264-HOLYARRS';
	mkdirSync(join(dl, name), { recursive: true });
	writeFileSync(join(dl, name, `${name}.mkv`), 'episode');
	await fetch(`${qbit}/set?hash=${hashOf(name)}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(`${name}/${name}.mkv`)}`);
	const r = row('Harbour.Lights.S02E01.720p');
	await expect(r.getByTestId('row-state')).toHaveText('Done', { timeout: 30_000 });
	await expect(r.getByTestId('row-text')).toHaveText('In the library.');
});

test('a pack whose files cannot be placed waits on Needs a decision as one card', async () => {
	// The pack reports two files that are not on the disk: neither can be
	// placed, so both episodes need a decision.
	const name = 'Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS';
	const files = ['S01E01', 'S01E02'].map((e) => `${name}/Moss.Parade.${e}.1080p.WEB-DL.H264-HOLYARRS.mkv`).join('|');
	await fetch(`${qbit}/set?hash=${hashOf(name)}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(files)}`);
	const pack = row('Moss.Parade.S01.1080p');
	await expect(pack.getByTestId('row-state')).toHaveText('Needs a decision', { timeout: 30_000 });
	await expect(pack.getByTestId('row-text')).toHaveText('0 of 2 episodes in the library; S01E01, S01E02 need a decision.');

	await page.locator('main').getByRole('link', { name: /Needs a decision/ }).click();
	const card = page.getByTestId('decision-pack').filter({ hasText: 'Moss Parade' });
	await expect(card).toContainText('Season pack · 2 of the 2 episodes in this download need a decision');
	await expect(card).toContainText('Pack: Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS');
	await expect(card.getByTestId('decision')).toHaveCount(2);
	await expect(card.getByTestId('decision').first()).toContainText('Moss Parade S01E01 Green Start');
	await expect(card.getByTestId('decision').first().getByTestId('decision-why')).toHaveText('The import could not finish on its own.');
	for (const name of ['Search again for these episodes', 'Add the pack to Turned down', 'Stop looking for these episodes']) await expect(card.getByRole('button', { name })).toBeVisible();
	await expect(card.getByTestId('decision').first().getByRole('link', { name: 'Choose a release by hand' })).toHaveAttribute('href', /\/library\/\d+#h-rel$/);
	await axe(page, 'needs a decision with a pack');
});

test('Needs a decision works from a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/downloads/decide');
	await expect(phone.getByTestId('decision-pack').filter({ hasText: 'Moss Parade' })).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'needs a decision on a phone');
	await phone.goto('/downloads');
	await expect(phone.getByTestId('row-covers').first()).toBeVisible();
	await noSidewaysScroll(phone);
	await ctx.close();
});

test('one choice settles the whole pack: Add the pack to Turned down searches each episode again', async () => {
	const card = page.getByTestId('decision-pack').filter({ hasText: 'Moss Parade' });
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: 'Add the pack to Turned down' }).click();
	await expect(page.getByText('Turned down. Searching again for these episodes.')).toBeVisible();
	await expect(card).toHaveCount(0, { timeout: 30_000 });
	await page.goto('/library/turned-down');
	await expect(page.getByTestId('listed-release').filter({ hasText: 'Moss.Parade.S01.1080p' })).toBeVisible();
});

test('Settings > Naming and files shows the episode patterns with live examples', async () => {
	await page.goto('/settings/naming');
	await expect(page.getByRole('heading', { name: 'Show names' })).toBeVisible();
	const ex = page.getByTestId('episode-examples');
	await expect(ex).toContainText('Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv');
	await expect(ex).toContainText('S01E02-E03 - The Mill + Flood');
	await page.getByRole('textbox', { name: 'Episode file name pattern' }).fill('{title} {episode}');
	await expect(ex).toContainText('Copper Valley (2025)/Season 1/Copper Valley S01E02.mkv');
	await page.getByRole('textbox', { name: 'Episode file name pattern' }).fill('{title} {quality}');
	await expect(ex.getByRole('alert')).toContainText('must hold {episode}');
	await page.getByRole('textbox', { name: 'Episode file name pattern' }).fill('{title} - {episode} - {episode_title} {quality}');
	await expect(ex).toContainText('Copper Valley - S01E02 - The Mill 1080p WEB.mkv');
	await axe(page, 'naming with shows');
});
