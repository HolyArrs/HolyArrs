import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';
import { qbitPort } from './fakes.mjs';

// Album downloads on the Downloads screen and Needs a decision (4.14.2).
// Runs after musicsearch.spec.ts: the FLAC copy of Tin Lanterns – Harbour
// Songs (four tracks, Song 1.1 to Song 1.4) is downloading in the fake
// qBittorrent with one download.
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;
const dl = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'downloads');
const name = 'Tin Lanterns - Harbour Songs (2019) [FLAC]';
// The fake indexer's info hash for an album Release name (fakes.mjs).
const hashOf = (n: string) => [...n].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 11).toString(16).padStart(8, '0').repeat(5);

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

// The album's table row (the second line shows the tracks; the Release name is
// its tooltip), and the row's details sheet.
const row = () => page.getByTestId('dl-table-row').filter({ has: page.locator('.rel[title*="Harbour Songs (2019) [FLAC]"]') }).first();
const details = () => page.getByRole('dialog', { name: 'Download details' });
async function open() {
	await row().locator('.nm').click();
	await expect(details()).toBeVisible();
}
async function close() {
	await page.keyboard.press('Escape');
	await expect(details()).toBeHidden();
}

test('the Downloads screen names Artist – Album and the tracks the download covers', async () => {
	await page.goto('/downloads');
	const r = row();
	await expect(r.locator('.nm')).toHaveText('Tin Lanterns – Harbour Songs (2019)');
	await expect(r.locator('.rel')).toHaveText('4 tracks');
	// The tracks it covers, each with where it is, in the row's details.
	await open();
	const d = details();
	await d.getByTestId('row-episodes').locator('summary').click();
	await expect(d.getByTestId('row-episodes').locator('summary')).toHaveText('The 4 tracks it covers');
	await expect(d.getByTestId('row-episode')).toHaveCount(4);
	await expect(d.getByTestId('row-episode').first()).toContainText('Track 01');
	await expect(d.getByTestId('row-episode').first()).toContainText('Song 1.1');
	await expect(d.getByTestId('row-episode').first()).toContainText('Downloading');
	await close();
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
	await expect(r.getByTestId('row-state')).toHaveText(/^Needs a decision/, { timeout: 30_000 });
	await open();
	await expect(details().getByTestId('row-text')).toHaveText('3 of 4 tracks in the library; Track 04 needs a decision.');
	await close();

	await page.locator('main').getByRole('link', { name: /Needs a decision/ }).click();
	const card = page.getByTestId('decision').filter({ hasText: 'Harbour Songs' });
	await expect(card.getByRole('link', { name: 'Tin Lanterns – Harbour Songs Track 04 Song 1.4' })).toBeVisible();
	await expect(card.getByTestId('decision-why')).toHaveText('The import could not finish on its own.');
	for (const n of ['Search again', 'Add to Turned down', 'Stop looking']) await expect(card.getByRole('button', { name: n, exact: true })).toBeVisible();
	await expect(card.getByRole('link', { name: 'Add another Version' })).toHaveCount(0);
	await axe(page, 'needs a decision with a track');
});

test('stuck tracks on a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/downloads/decide');
	await expect(phone.getByTestId('decision').filter({ hasText: 'Harbour Songs' })).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'needs a decision with a track on a phone');
	await phone.goto('/downloads');
	await expect(phone.getByTestId('dl-table-list').locator('li').filter({ hasText: 'Tin Lanterns – Harbour Songs (2019)' }).locator('.cv')).toHaveText('4 tracks');
	await noSidewaysScroll(phone);
	await ctx.close();
});

test('Stop looking for one track settles it; the rest of the album stays in the library', async () => {
	const card = page.getByTestId('decision').filter({ hasText: 'Harbour Songs' });
	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: 'Stop looking', exact: true }).click();
	await expect(page.getByText(/Stopped looking for Tin Lanterns – Harbour Songs Track 04/)).toBeVisible();
	await expect(card).toHaveCount(0, { timeout: 30_000 });
});
