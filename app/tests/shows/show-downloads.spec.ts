import { expect, test, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { qbitPort } from '../fakes/index.mjs';
import { axe as scan } from '../helpers/axe.ts';
import { tmp } from '../helpers/data-folder.ts';

// Show downloads on the Downloads screen and Needs a decision (4.2.3,
// 4.2.4). Runs after show-search.spec.ts: the Moss Parade season pack fills
// S01E01 and S01E02 with one download and Harbour Lights S02E01 (720p) is
// downloading, both at 40% in the fake qBittorrent.
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;
const dl = join(tmp, 'downloads');

// The fake indexer's info hash for a Release name (fakes/).
const hashOf = (name: string) => [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7).toString(16).padStart(8, '0').repeat(5);

const axe = (p: Page, where: string) => scan(p, where, { waitAll: true });
test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

// A table row by its Release name (the second line shows the episodes; the
// Release name is its tooltip), and the row's details panel (it opens in
// place under the row; one panel is open at a time; Escape closes it).
const row = (name: string) => page.getByTestId('dl-table-row').filter({ has: page.locator(`.rel[title*="${name}"]`) }).first();
const details = () => page.getByTestId('dl-panel');
const state = (r: ReturnType<typeof row>) => r.locator('[data-col="status"]');
async function open(r: ReturnType<typeof row>) {
	await r.getByTestId('dl-line').click();
	await expect(details()).toBeVisible();
}
async function close() {
	// Escape from the row closes its details (the redesign has no row toggle).
	await page.keyboard.press('Escape');
	await expect(details()).toBeHidden();
}

test('the Downloads screen names the show and the episodes each download covers', async () => {
	await page.goto('/downloads');
	const pack = row('Moss.Parade.S01.1080p');
	await expect(pack.locator('.tt')).toContainText('Moss Parade');
	await expect(pack.locator('.rel')).toContainText('S01E01-E02, 2 episodes');
	await expect(row('Harbour.Lights.S02E01.720p').locator('.rel')).toContainText('S02E01 Return of the Fog');
	// The details open in place and name the download.
	await open(pack);
	await expect(details()).toContainText('Moss.Parade.S01.1080p');
	await close();
	await axe(page, 'downloads with shows');
});

test('a finished episode is imported into its episode and the row says so', async () => {
	const name = 'Harbour.Lights.S02E01.720p.WEB.H264-HOLYARRS';
	mkdirSync(join(dl, name), { recursive: true });
	writeFileSync(join(dl, name, `${name}.mkv`), 'episode');
	await fetch(`${qbit}/set?hash=${hashOf(name)}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(`${name}/${name}.mkv`)}`);
	const r = row('Harbour.Lights.S02E01.720p');
	// A finished download leaves the Active tab for Done.
	await page.goto('/downloads?show=done');
	await expect(state(r)).toContainText('Done', { timeout: 30_000 });
});

test('a pack whose files cannot be placed waits on Needs a decision as one card', async () => {
	// The pack reports two files that are not on the disk: neither can be
	// placed, so both episodes need a decision.
	const name = 'Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS';
	const files = ['S01E01', 'S01E02'].map((e) => `${name}/Moss.Parade.${e}.1080p.WEB-DL.H264-HOLYARRS.mkv`).join('|');
	await fetch(`${qbit}/set?hash=${hashOf(name)}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(files)}`);
	// The last test left the Done tab open: a pack that needs a decision is under Problems › Review.
	await page.goto('/downloads?show=problems.decide');
	const pack = row('Moss.Parade.S01.1080p');
	// The row says Review with what stopped it ("Review · import blocked · import failed: …").
	await expect(state(pack)).toContainText('Review', { timeout: 30_000 });
	await open(pack);
	// The choices are in the same panel; the panel stays open for the next test.
	const card = details().getByTestId('decision-pack');
	await expect(card).toContainText('2 of the 2 episodes in this download need a decision');
	await expect(card).toContainText('Season pack');
	// The pack's own Why? names the Release.
	await card.getByTestId('decision-why').first().click();
	await expect(card.getByTestId('decision-why-box').first()).toContainText('Release: Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS');
	await expect(card.getByTestId('decision')).toHaveCount(2);
	await expect(card.getByTestId('decision').first()).toContainText('Moss Parade S01E01 Green Start');
	await card.getByTestId('decision').first().getByTestId('decision-why').click();
	await expect(card.getByTestId('decision').first().getByTestId('decision-why-box')).toContainText('The import could not finish on its own.');
	await expect(card.getByRole('button', { name: 'Search again for these episodes' })).toBeVisible();
	await card.getByRole('button', { name: /^More/ }).first().click();
	for (const name of ['Add the pack to Turned down', 'Stop looking for these episodes']) await expect(page.getByRole('menuitem', { name })).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(card.getByTestId('decision').first().getByRole('link', { name: 'Choose a release by hand' })).toHaveAttribute('href', /\/library\/\d+\?tab=library&pick=1#h-rel$/);
	await axe(page, 'needs a decision with a pack');
});

test('Needs a decision works from a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/downloads?show=problems.decide');
	await phone.getByTestId('dl-list').locator('li').filter({ hasText: 'Moss Parade' }).getByTestId('dl-line').click();
	const sheet = phone.getByTestId('dl-panel');
	await expect(sheet.getByTestId('decision-pack')).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'needs a decision on a phone');
	await phone.goto('/downloads');
	// A phone row shows its short title (the release line is for wider screens).
	await expect(phone.getByTestId('dl-list').getByTestId('dl-line').first()).toBeVisible();
	await noSidewaysScroll(phone);
	await ctx.close();
});

test('one choice settles the whole pack: Add the pack to Turned down searches each episode again', async () => {
	const card = details().getByTestId('decision-pack');
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: /^More/ }).first().click();
	await page.getByRole('menuitem', { name: 'Add the pack to Turned down' }).click();
	await expect(page.getByText('Searching again for these episodes')).toBeVisible();
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
	// The page shows one problem in one place (under Movie names); the examples point there.
	await expect(page.getByRole('alert').filter({ hasText: 'must hold {episode}' })).toBeVisible();
	await expect(ex).toContainText('See the error under Movie names');
	await page.getByRole('textbox', { name: 'Episode file name pattern' }).fill('{title} - {episode} - {episode_title} {quality}');
	await expect(ex).toContainText('Copper Valley - S01E02 - The Mill 1080p WEB.mkv');
	await axe(page, 'naming with shows');
});
