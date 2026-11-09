import { expect, test, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { qbitPort } from '../fakes/index.mjs';
import { axe } from '../helpers/axe.ts';
import { tmp } from '../helpers/data-folder.ts';

// Album downloads on the Downloads screen and Needs a decision (4.14.2).
// Runs after music-search.spec.ts: the FLAC copy of Tin Lanterns, Harbour
// Songs (four tracks, Song 1.1 to Song 1.4) is downloading in the fake
// qBittorrent with one download.
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;
const dl = join(tmp, 'downloads');
const name = 'Tin Lanterns - Harbour Songs (2019) [FLAC]';
// The fake indexer's info hash for an album Release name (fakes/).
const hashOf = (n: string) => [...n].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 11).toString(16).padStart(8, '0').repeat(5);

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

// The album's table row (the second line shows the tracks; the Release name is
// its tooltip), and the row's details panel (it opens in place under the row).
const row = () => page.getByTestId('dl-table-row').filter({ has: page.locator('.rel[title*="Harbour Songs (2019) [FLAC]"]') }).first();
const details = () => page.getByTestId('dl-panel');
async function open() {
	await row().getByTestId('dl-line').click();
	await expect(details()).toBeVisible();
}

test('the Downloads screen names Artist – Album and the tracks the download covers', async () => {
	await page.goto('/downloads');
	const r = row();
	await expect(r.locator('.tt')).toContainText('Tin Lanterns – Harbour Songs (2019)');
	await expect(r.locator('.rel')).toContainText('4 tracks');
	await axe(page, 'downloads with an album');
});

test('a finished album is placed track by track; a track whose file cannot be placed needs a decision', async () => {
	// Three track files are on the disk with a cover; the fourth is
	// reported by the Downloader but missing, so it cannot be placed.
	mkdirSync(join(dl, name), { recursive: true });
	for (let i = 1; i <= 3; i++) writeFileSync(join(dl, name, `0${i} - Song 1.${i}.flac`), `track ${i}`);
	writeFileSync(join(dl, name, 'cover.jpg'), 'cover');
	const files = [1, 2, 3, 4].map((i) => `${name}/0${i} - Song 1.${i}.flac`).concat(`${name}/cover.jpg`).join('|');
	await fetch(`${qbit}/set?hash=${hashOf(name)}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(files)}`);
	const r = row();
	// The row says Review (what used to read Needs a decision).
	await expect(r.locator('[data-col="status"]')).toContainText('Review', { timeout: 30_000 });
	await open();
	// The choices are in the same panel; the panel stays open for the last test.
	const card = details().getByTestId('decision').filter({ hasText: 'Harbour Songs' });
	// The row above names the album, so the card in its details names only the track.
	await expect(card.getByTestId('decision-reason')).toContainText('The file for track 04 could not be placed');
	await card.getByTestId('decision-why').click();
	await expect(card.getByTestId('decision-why-box')).toContainText('The import could not finish on its own.');
	// The main choice is a link to the album's Releases; Search again and the rest are under More.
	await card.getByRole('button', { name: /^More/ }).click();
	for (const n of ['Search again', 'Add to Turned down', 'Stop looking']) await expect(page.getByRole('menuitem', { name: n, exact: true })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: 'Add another Version' })).toHaveCount(0);
	await page.keyboard.press('Escape');
	await axe(page, 'needs a decision with a track');
});

test('stuck tracks on a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/downloads?show=problems.decide');
	await phone.getByTestId('dl-list').locator('li').filter({ hasText: 'Tin Lanterns – Harbour Songs (2019)' }).getByTestId('dl-line').click();
	const sheet = phone.getByTestId('dl-panel');
	await expect(sheet.getByTestId('decision').filter({ hasText: 'Harbour Songs' })).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'needs a decision with a track on a phone');
	await phone.goto('/downloads');
	await expect(phone.getByTestId('dl-list').locator('li').filter({ hasText: 'Tin Lanterns – Harbour Songs (2019)' }).locator('.rel')).toContainText('4 tracks');
	await noSidewaysScroll(phone);
	await ctx.close();
});

test('Stop looking for one track settles it; the rest of the album stays in the library', async () => {
	const card = details().getByTestId('decision').filter({ hasText: 'Harbour Songs' });
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: /^More/ }).click();
	await page.getByRole('menuitem', { name: 'Stop looking', exact: true }).click();
	await expect(page.getByText(/Stopped looking for Tin Lanterns – Harbour Songs Track 04/)).toBeVisible();
	await expect(card).toHaveCount(0, { timeout: 30_000 });
});
