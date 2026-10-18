import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, titleAction } from './helpers.ts';

// Organise for music (4.15.4): runs after musicdownloads.spec.ts, so Tin
// Lanterns – Harbour Songs (2019) has three tracks in the music Library
// folder. Copper Wren sits on disk before HolyArrs knows it. The owner
// renames Harbour Songs from its album page after changing the album
// folder pattern, imports Copper Wren from the music Library folder (nothing
// moves; its single is left alone), deletes one track with its file (to the
// holding folder) and reads the album's History, finds Harbour Songs on the
// Calendar under Music, checks it from a phone, and deletes Copper Wren with
// Never-add.
test.describe.configure({ mode: 'serial' });

let page: Page;
const musicDir = join(moviesDir, '..', 'music');
const wren = join(musicDir, 'Copper Wren');
const wrenFiles = {
	one: join(wren, 'Low Orbit (2018)', '01 - Song 1.1.mp3'),
	two: join(wren, 'Low Orbit (2018)', '02 - Song 1.2.mp3'),
	single: join(wren, 'Wire (2019)', '01 - Song 2.1.mp3'),
	loose: join(wren, 'intro.mp3')
};

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

async function openAlbum(p: Page, artist: string, album: string) {
	await p.goto('/library');
	await p.getByRole('link', { name: new RegExp(artist) }).first().click();
	await p.getByRole('link', { name: album }).first().click();
	await expect(p.getByRole('heading', { level: 1, name: album })).toBeVisible();
}

test.beforeAll(async ({ browser }) => {
	for (const f of Object.values(wrenFiles)) {
		mkdirSync(dirname(f), { recursive: true });
		writeFileSync(f, `audio ${f}`);
	}
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

test('Settings > Naming has music patterns with a live example', async () => {
	await page.goto('/settings/naming');
	const card = page.getByTestId('music-naming');
	await expect(card.getByRole('heading', { name: 'Music names' })).toBeVisible();
	const ex = card.getByTestId('music-examples');
	await expect(ex).toContainText('The Lab Lanterns/Small Hours (2025)/03 - Harbour Light.flac');
	await expect(ex).toContainText('Long Night (2025)/2-04 - Second Wind.flac');
	await card.getByRole('textbox', { name: 'Artist folder pattern', exact: true }).fill('{artist_sort}');
	await expect(ex).toContainText('Lab Lanterns, The/Small Hours (2025)/03 - Harbour Light.flac');
	await card.getByRole('textbox', { name: 'Artist folder pattern', exact: true }).fill('{artist}');
	await card.getByRole('textbox', { name: 'Album folder pattern', exact: true }).fill('{year} - {album}');
	await expect(ex).toContainText('The Lab Lanterns/2025 - Small Hours/03 - Harbour Light.flac');
	await card.getByRole('checkbox', { name: /A folder for each disc/ }).check();
	await expect(ex).toContainText('2025 - Long Night/Disc 2/04 - Second Wind.flac');
	await card.getByRole('checkbox', { name: /A folder for each disc/ }).uncheck();
	await card.getByRole('textbox', { name: 'Track file name pattern', exact: true }).fill('{track_title}');
	await expect(page.getByTestId('music-examples').getByRole('alert')).toContainText('must hold {track}');
	await card.getByRole('textbox', { name: 'Track file name pattern', exact: true }).fill('{track} - {track_title}');
	await axe(page, 'naming with music');
	await page.getByRole('button', { name: /^Save/ }).click();
	await expect(page.getByText('Naming and file settings saved.')).toBeVisible();
});

test('Rename files on the album page renames its tracks into the new album folder', async () => {
	await openAlbum(page, 'Tin Lanterns', 'Harbour Songs');
	await page.getByTestId('rename-album').click();
	const dialog = page.getByRole('dialog', { name: 'Rename the files of Harbour Songs' });
	await expect(dialog.getByTestId('rename-step')).toHaveCount(3);
	await expect(dialog.getByTestId('rename-step').first()).toContainText('Tin Lanterns/2019 - Harbour Songs/01 - Song 1.1.flac');
	await axe(page, 'album rename preview');
	await dialog.getByRole('button', { name: 'Rename 3 files' }).click();
	await expect(page.getByText('3 files renamed.')).toBeVisible({ timeout: 30_000 });
	await expect(dialog.getByTestId('rename-none')).toBeVisible();
	await dialog.getByRole('button', { name: 'Close' }).click();
	const folder = join(musicDir, 'Tin Lanterns', '2019 - Harbour Songs');
	expect(readdirSync(folder).sort()).toEqual(['01 - Song 1.1.flac', '02 - Song 1.2.flac', '03 - Song 1.3.flac']);
	// The artist page has the whole artist's rename: nothing left.
	await page.getByRole('link', { name: 'Tin Lanterns' }).first().click();
	await titleAction(page, 'Library', 'Rename files');
	await expect(page.getByRole('dialog', { name: 'Rename the files of Tin Lanterns' }).getByTestId('rename-none')).toBeVisible();
	await page.getByRole('dialog', { name: 'Rename the files of Tin Lanterns' }).getByRole('button', { name: 'Close' }).click();
	// Back to the default album folder pattern for the tests after this one.
	await page.goto('/settings/naming');
	await page.getByTestId('music-naming').getByRole('textbox', { name: 'Album folder pattern', exact: true }).fill('{album} ({year})');
	await page.getByRole('button', { name: /^Save/ }).click();
	await expect(page.getByText('Naming and file settings saved.')).toBeVisible();
});

test('Import a library scans the music folder, matches albums and tracks, and adopts with nothing moved', async () => {
	await page.goto('/library/import');
	const lf = page.getByLabel('Library folder');
	await lf.selectOption((await lf.locator('option', { hasText: 'Music: ' }).first().getAttribute('value')) ?? '');
	await page.getByRole('button', { name: 'Scan (nothing is moved)' }).click();
	const plan = page.getByTestId('adopt-plan');
	await expect(plan).toBeVisible({ timeout: 30_000 });
	await expect(plan.getByTestId('group-in_library')).toContainText('Tin Lanterns');
	const choir = plan.getByTestId('group-matched').locator('li.folder').filter({ hasText: 'Copper Wren' });
	await expect(choir.getByRole('checkbox', { name: 'Adopt Copper Wren' })).toBeChecked();
	const albums = choir.getByTestId('music-albums');
	await expect(albums.locator('summary')).toHaveText('2 tracks in 2 albums; 2 files left alone');
	await albums.locator('summary').click();
	const slow = albums.getByTestId('music-album').filter({ hasText: 'Low Orbit (2018)' });
	await expect(slow).toContainText('2 of 2 files hold a track of');
	await expect(slow).toContainText('track 02 Song 1.2');
	await expect(albums.getByTestId('music-album').filter({ hasText: 'Wire' })).toContainText('left out by the album kinds');
	await expect(albums).toContainText('not in an album folder');
	await axe(page, 'import a music library');
	await page.getByRole('button', { name: 'Adopt 1 artist' }).click();
	const done = page.getByTestId('adopt-results');
	await expect(done).toContainText('1 artist is now in the Library.', { timeout: 30_000 });
	await expect(done).toContainText('2 tracks, 2 files left alone');
	for (const f of Object.values(wrenFiles)) expect(existsSync(f), f).toBe(true);
	await done.getByRole('link', { name: 'Open the artist' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Copper Wren' })).toBeVisible();
});

test('deleting one track with its file moves it to the holding folder; the album page shows its History', async () => {
	await openAlbum(page, 'Copper Wren', 'Low Orbit');
	await expect(page.getByTestId('track-state').first()).toContainText(/In the Library|Better copy sought/);
	await page.getByRole('button', { name: 'Delete track 02 Song 1.2' }).click();
	const dialog = page.getByTestId('show-delete');
	await expect(dialog.getByRole('heading', { level: 2 })).toHaveText('Delete Copper Wren – track 02 Song 1.2?');
	await expect(dialog.getByRole('checkbox', { name: /Never-add/ })).toHaveCount(0);
	await dialog.getByRole('checkbox', { name: /Also delete their files/ }).check();
	await expect(dialog.getByTestId('delete-plan')).toContainText('1 file moves to the holding folder');
	await expect(dialog.getByTestId('delete-plan')).toContainText('Low Orbit (2018)/02 - Song 1.2.mp3');
	await axe(page, 'delete a track');
	await dialog.getByRole('button', { name: 'Delete track 02 Song 1.2' }).click();
	await expect(page.getByText(/is deleted\. Its file is in the holding folder/)).toBeVisible();
	expect(existsSync(wrenFiles.two)).toBe(false);
	expect(existsSync(wrenFiles.one)).toBe(true);
	const hist = page.getByTestId('album-history');
	await expect(hist.getByTestId('album-event').filter({ hasText: 'Deleted Copper Wren Low Orbit: track 02 Song 1.2 from the Library' })).toBeVisible();
	await expect(hist.getByTestId('album-event').filter({ hasText: 'Adopted Copper Wren' })).toHaveCount(0);
	await axe(page, 'album with history');
});

test('the Calendar lists album release days under Music', async () => {
	await page.goto('/calendar?titles=music&month=2019-04');
	const entry = page.getByTestId('cal-entry').filter({ hasText: 'Tin Lanterns – Harbour Songs' });
	await expect(entry).toBeVisible();
	await expect(entry).toHaveAttribute('href', /\/library\/\d+\/albums\/\d+$/);
	await page.getByRole('radio', { name: 'List', exact: true }).click();
	await expect(page.getByTestId('cal-item').filter({ hasText: 'Harbour Songs' })).toContainText('Album release');
	await page.getByTestId('cal-titles').getByRole('radio', { name: 'Movies' }).click();
	await expect(page.getByTestId('cal-item').filter({ hasText: 'Harbour Songs' })).toHaveCount(0);
	await axe(page, 'calendar with music');
});

test('the album page, import and delete work at phone width', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await openAlbum(phone, 'Copper Wren', 'Low Orbit');
	await expect(phone.getByTestId('delete-album')).toBeVisible();
	await noSidewaysScroll(phone);
	await phone.getByTestId('delete-album').click();
	await expect(phone.getByTestId('show-delete').getByTestId('delete-plan')).toContainText('Low Orbit (2 tracks)');
	await noSidewaysScroll(phone);
	await axe(phone, 'delete an album on a phone');
	await phone.getByTestId('show-delete').getByRole('button', { name: 'Cancel' }).click();
	await phone.getByTestId('rename-album').click();
	await expect(phone.getByRole('dialog', { name: 'Rename the files of Low Orbit' })).toBeVisible();
	await noSidewaysScroll(phone);
	await phone.getByRole('dialog', { name: 'Rename the files of Low Orbit' }).getByRole('button', { name: 'Close' }).click();
	for (const path of ['/library/import', '/settings/naming', '/calendar?titles=music']) {
		await phone.goto(path);
		await expect(phone.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(phone);
	}
	await ctx.close();
});

test('deleting the whole artist with Never-add keeps files it only found and leaves the Library', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Copper Wren/ }).first().click();
	await titleAction(page, 'Library', 'Delete artist');
	const dialog = page.getByTestId('show-delete');
	await dialog.getByRole('checkbox', { name: /Never-add/ }).check();
	await expect(dialog.getByTestId('delete-plan')).toContainText('The artist leaves the Library');
	await expect(dialog.getByTestId('delete-plan')).toContainText('It goes on the Never-add list');
	await dialog.getByRole('button', { name: 'Delete artist' }).click();
	await expect(page).toHaveURL(/\/library$/);
	for (const f of [wrenFiles.one, wrenFiles.single, wrenFiles.loose]) expect(existsSync(f), f).toBe(true);
});
