import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { openLibraryTab, openSeasonNamed, finishFirstRun } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

test.describe.configure({ mode: 'serial' });

const tmp = dataDir('acceptance-library');
const moviesDir = join(tmp, 'movies');
const showsDir = join(tmp, 'shows');
const musicDir = join(tmp, 'music');
const sourceAlbum = join(musicDir, 'Copper Wren', 'Low Orbit (2018)');
const renamedAlbum = join(musicDir, 'Copper Wren', '2018 - Low Orbit');
const musicFiles = {
	one: join(sourceAlbum, '01 - Song 1.1.mp3'),
	two: join(sourceAlbum, '02 - Song 1.2.mp3'),
	single: join(musicDir, 'Copper Wren', 'Wire (2019)', '01 - Song 2.1.mp3'),
	loose: join(musicDir, 'Copper Wren', 'intro.mp3')
};
const episode = join(showsDir, 'Fennel Clock (2020)', 'Season 1', 'Fennel.Clock.S01E01.1080p.WEB-DL.H264-LAB.mkv');
const episodeSubtitle = join(dirname(episode), 'Fennel.Clock.S01E01.1080p.WEB-DL.H264-LAB.en.srt');
const owner = { name: 'acceptance-owner', password: ['acceptance', 'library', 'owner', 'pw'].join('-') };

let context: BrowserContext;
let page: Page;

async function addLibraryFolder(kind: 'music' | 'show', path: string, name: string) {
	await page.goto('/settings/library-folders');
	await page.getByRole('button', { name: 'Add a folder' }).first().click();
	await page.getByLabel('Holds').selectOption(kind);
	await page.getByLabel('Folder path').fill(path);
	await page.getByLabel(/^Name/).fill(name);
	await page.getByRole('button', { name: 'Add', exact: true }).click();
	await expect(page.getByTestId('folder-kind').filter({ hasText: kind === 'music' ? 'For music' : 'For shows' })).toHaveCount(1);
}

async function adopt(folderText: string, name: string, button: string, doneText: string) {
	await page.goto('/library/import');
	const folders = page.getByLabel('Library folder');
	const option = folders.locator('option', { hasText: folderText }).first();
	await folders.selectOption((await option.getAttribute('value')) ?? '');
	await page.getByRole('button', { name: 'Scan', exact: true }).click();
	const plan = page.getByTestId('adopt-plan');
	await expect(plan).toBeVisible({ timeout: 30_000 });
	await expect(plan.getByRole('checkbox', { name: `Adopt ${name}` })).toBeChecked();
	await page.getByRole('button', { name: button }).click();
	await expect(page.getByTestId('adopt-results')).toContainText(doneText, { timeout: 30_000 });
}

test.beforeAll(async ({ browser }) => {
	for (const file of [...Object.values(musicFiles), episode]) {
		mkdirSync(dirname(file), { recursive: true });
		writeFileSync(file, `synthetic fixture ${file}`);
	}
	mkdirSync(moviesDir, { recursive: true });

	context = await browser.newContext();
	page = await context.newPage();
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible();
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(owner.name);
	await page.getByLabel('Password', { exact: true }).fill(owner.password);
	await page.getByLabel('Password again').fill(owner.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await finishFirstRun(page, moviesDir, { music: true });
	await expect(page).toHaveURL(/\/$/);

	await addLibraryFolder('music', musicDir, 'Music');
	await addLibraryFolder('show', showsDir, 'Shows');
});

test.afterAll(async () => {
	await context.close();
});

test('music adoption and rename stay on one fixture and prove the exact disk result', async () => {
	await page.goto('/settings/naming');
	await page.getByTestId('music-naming').getByRole('textbox', { name: 'Album folder pattern', exact: true }).fill('{year} - {album}');
	await page.getByRole('button', { name: /^Save/ }).click();
	await expect(page.getByText('Naming settings saved')).toBeVisible();

	await adopt('Music: ', 'Copper Wren', 'Adopt 1 artist', '1 artist is now in the Library.');
	for (const file of Object.values(musicFiles)) expect(existsSync(file), `adoption moved ${file}`).toBe(true);

	await page.getByTestId('adopt-results').getByRole('link', { name: 'Open the artist' }).click();
	// The album page opens from the artist's Library tab (its album rows).
	await page.getByTestId('artist-tabs').getByRole('tab', { name: 'Library' }).click();
	const row = page.getByTestId('artist-library').getByTestId('album').filter({ hasText: 'Low Orbit' });
	await row.getByRole('button', { expanded: false }).click();
	await row.getByRole('link', { name: 'Open the album page of Low Orbit' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Low Orbit' })).toBeVisible();
	// Rename files is in the album's More menu.
	await page.getByRole('button', { name: 'More actions' }).first().click();
	await page.getByRole('menuitem', { name: 'Rename files' }).click();
	const dialog = page.getByRole('dialog', { name: 'Rename files' }).filter({ hasText: 'Low Orbit' });
	const steps = dialog.getByTestId('rename-step');
	await expect(steps).toHaveCount(2);
	await expect(steps.nth(0)).toContainText('Copper Wren/Low Orbit (2018)/01 - Song 1.1.mp3');
	await expect(steps.nth(0)).toContainText('Copper Wren/2018 - Low Orbit/01 - Song 1.1.mp3');
	await expect(steps.nth(1)).toContainText('Copper Wren/Low Orbit (2018)/02 - Song 1.2.mp3');
	await expect(steps.nth(1)).toContainText('Copper Wren/2018 - Low Orbit/02 - Song 1.2.mp3');
	await dialog.getByRole('button', { name: 'Rename 2 files' }).click();
	await expect(page.getByText('2 files renamed')).toBeVisible({ timeout: 30_000 });
	await expect(dialog.getByTestId('rename-none')).toBeVisible();

	expect(existsSync(sourceAlbum), 'old album folder remains').toBe(false);
	expect(readdirSync(renamedAlbum).sort()).toEqual(['01 - Song 1.1.mp3', '02 - Song 1.2.mp3']);
	expect(existsSync(musicFiles.single), 'single excluded from album adoption moved').toBe(true);
	expect(existsSync(musicFiles.loose), 'loose file moved').toBe(true);
});

test('episode subtitle dialog chooses from the fake site and saves the exact file', async () => {
	await page.goto('/settings/subtitles');
	await page.getByRole('button', { name: 'Add a language' }).click();
	await page.locator('#def-lang-0').selectOption('en');
	await page.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Subtitle settings saved')).toBeVisible();

	await page.getByRole('button', { name: 'Add a Subtitle site' }).click();
	await page.getByRole('radio', { name: /^Lab subtitle site \(/ }).check();
	await page.getByRole('textbox', { name: 'Address' }).fill('http://127.0.0.1:18742');
	await page.getByText('Limits').click();
	await page.getByLabel('Wait between calls (ms)').fill('10');
	await page.getByLabel('Calls per window, at most').fill('5000');
	await page.getByLabel(/Reach the local network/).check();
	await page.locator('form[aria-labelledby="h-site-form"]').getByRole('button', { name: 'Save', exact: true }).click();
	const site = page.getByTestId('subtitle-site').filter({ hasText: 'Lab subtitle site' }).first();
	await expect(site.getByTestId('subtitle-site-test')).toHaveText(/· \d+ subtitles?$/, { timeout: 15_000 });

	await adopt('Shows: ', 'Fennel Clock', 'Adopt 1 show', '1 show is now in the Library');
	await page.goto('/library');
	await page.getByRole('link', { name: /Fennel Clock/ }).first().click();
	// An episode's subtitles live in its sheet, opened from its row on the Library tab.
	await openLibraryTab(page);
	await openSeasonNamed(page, 'Season 1');
	await page.getByRole('button', { name: /^Episode 1:/ }).first().click();
	await page.getByTestId('episode-subtitles-row').getByRole('button', { name: 'Subtitle options' }).click();
	await page.getByRole('menuitem', { name: 'Choose languages…' }).click();
	const dialog = page.getByTestId('episode-subtitles');
	await expect(dialog.getByRole('heading', { name: 'Subtitles of S01E01' })).toBeVisible();
	await expect(dialog.getByTestId('subtitle-missing')).toContainText('English');
	await dialog.getByTestId('subtitle-choose').click();
	const choices = dialog.getByTestId('subtitle-choices');
	await expect(choices.getByRole('heading', { name: 'English' })).toBeVisible({ timeout: 30_000 });
	await expect(choices.getByTestId('subtitle-choice').first()).toContainText('Lab subtitle site');
	await choices.locator('ul[data-lang="en"]').getByRole('button', { name: /^Choose / }).first().click();
	await expect(page.getByRole('status').filter({ hasText: 'Subtitle saved' })).toContainText('Fennel.Clock.S01E01.1080p.WEB-DL.H264-LAB.en.srt', { timeout: 30_000 });
	await expect(dialog.getByTestId('subtitle-row').filter({ hasText: 'English' })).toContainText('Chosen by hand from Lab subtitle site');

	expect(existsSync(episodeSubtitle), episodeSubtitle).toBe(true);
	expect(readFileSync(episodeSubtitle, 'utf8')).toContain('00:00:01,000 --> 00:00:02,500');
});
