import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, titleAction, openSeasonNamed, episodeAction } from './helpers.ts';

// Organise for shows (5.2): Quarry Bells sits on disk before HolyArrs knows
// it. The owner imports it from the show Library folder (nothing moves),
// renames its files from the show page, deletes one episode of a
// multi-episode file (the file stays) and then the season (its files go to
// the holding folder), reads an episode's History, finds this week's
// episode on the Calendar, picks how pack Alerts are told, and checks it all
// from a phone. Runs after showdownloads.spec.ts.
test.describe.configure({ mode: 'serial' });

let page: Page;
const showsDir = join(moviesDir, '..', 'shows');
const folder = join(showsDir, 'Quarry Bells (2021)');
const files = {
	one: join(folder, 'Season 1', 'Quarry.Bells.S01E01.1080p.WEB-DL.H264-HOLYARRS.mkv'),
	multi: join(folder, 'Season 1', 'Quarry.Bells.S01E02E03.1080p.WEB-DL.H264-HOLYARRS.mkv'),
	extra: join(folder, 'Extras', 'making of.mkv')
};

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

test.beforeAll(async ({ browser }) => {
	for (const f of Object.values(files)) {
		mkdirSync(dirname(f), { recursive: true });
		writeFileSync(f, `video ${f}`);
	}
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

test('Import a library scans the show folder, lists each file with its episodes, and adopts with nothing moved', async () => {
	await page.goto('/library/import');
	const lf = page.getByLabel('Library folder');
	await lf.selectOption((await lf.locator('option', { hasText: 'Shows: ' }).first().getAttribute('value')) ?? '');
	await page.getByRole('button', { name: 'Scan (nothing is moved)' }).click();
	const plan = page.getByTestId('adopt-plan');
	await expect(plan).toBeVisible({ timeout: 30_000 });
	const quarry = plan.getByTestId('group-matched').locator('li.folder').filter({ hasText: 'Quarry Bells (2021)' });
	await expect(quarry).toContainText('Quarry Bells (2021)');
	await expect(quarry.getByRole('checkbox', { name: 'Adopt Quarry Bells (2021)' })).toBeChecked();
	const list = quarry.getByTestId('show-files');
	await expect(list.locator('summary')).toHaveText('3 episodes in 2 files; 1 file with no episode, left alone');
	await list.locator('summary').click();
	await expect(list).toContainText('S01E02, S01E03');
	await expect(list).toContainText('Left alone');
	await axe(page, 'import a show library');
	// Shows already in the Library are left as they are.
	await expect(plan.getByTestId('group-in_library')).toContainText('Harbour Lights');
	for (const box of await plan.getByRole('checkbox', { checked: true }).all()) {
		if (!(await box.getAttribute('aria-label'))?.includes('Quarry Bells')) await box.uncheck();
	}
	await page.getByRole('button', { name: 'Adopt 1 show' }).click();
	const done = page.getByTestId('adopt-results');
	await expect(done).toContainText('1 show is now in the Library.', { timeout: 30_000 });
	await expect(done).toContainText('3 episodes, 1 file left alone');
	for (const f of Object.values(files)) expect(existsSync(f), f).toBe(true);
	await done.getByRole('link', { name: 'Open the show' }).click();
	await expect(page.getByRole('heading', { name: /Quarry Bells/ })).toBeVisible();
});

test('Rename files on the show page renames the adopted episodes to the episode pattern', async () => {
	await titleAction(page, 'Library', 'Rename files');
	const dialog = page.getByRole('dialog', { name: /Rename the files of Quarry Bells/ });
	await expect(dialog.getByTestId('rename-step')).toHaveCount(2);
	await expect(dialog).toContainText('Quarry Bells - S01E01 - Echo 1080p WEB.mkv');
	await expect(dialog).toContainText('Quarry Bells - S01E02-E03 - Dust + Granite 1080p WEB.mkv');
	await axe(page, 'show rename preview');
	await dialog.getByRole('button', { name: 'Rename 2 files' }).click();
	await expect(page.getByText('2 files renamed.')).toBeVisible({ timeout: 30_000 });
	await expect(dialog.getByTestId('rename-none')).toBeVisible();
	await dialog.getByRole('button', { name: 'Close' }).click();
	const season1 = readdirSync(join(folder, 'Season 1')).sort();
	expect(season1).toEqual(['Quarry Bells - S01E01 - Echo 1080p WEB.mkv', 'Quarry Bells - S01E02-E03 - Dust + Granite 1080p WEB.mkv']);
	expect(existsSync(files.extra)).toBe(true);
});

test('deleting one episode of a multi-episode file keeps the file; deleting the season holds its files', async () => {
	const s1 = await openSeasonNamed(page, 'Season 1');
	await expect(s1.getByTestId('episode').filter({ hasText: 'S01E02' })).toBeVisible();
	await episodeAction(page, 'S01E02', 'Delete S01E02');
	const dialog = page.getByTestId('show-delete');
	await expect(dialog.getByRole('heading', { level: 2 })).toHaveText('Delete Quarry Bells S01E02?');
	await expect(dialog.getByRole('checkbox', { name: /Never-add/ })).toHaveCount(0);
	await dialog.getByRole('checkbox', { name: /Also delete their files/ }).check();
	await expect(dialog.getByTestId('delete-plan')).toContainText('it also holds S01E03, which stays');
	await axe(page, 'delete an episode');
	await dialog.getByRole('button', { name: 'Cancel' }).click();

	await s1.getByRole('button', { name: 'Season actions', exact: true }).click();
	await page.getByRole('menuitem', { name: 'Delete Season 1' }).click();
	await expect(dialog.getByRole('heading', { level: 2 })).toHaveText('Delete Quarry Bells Season 1?');
	await expect(dialog.getByTestId('delete-plan')).toContainText('Season 1 (3 episodes): HolyArrs stops keeping them up to date.');
	await dialog.getByRole('checkbox', { name: /Also delete their files/ }).check();
	await expect(dialog.getByTestId('delete-plan')).toContainText('2 files move to the holding folder');
	await dialog.getByRole('button', { name: 'Delete Season 1' }).click();
	await expect(page.getByText(/Quarry Bells Season 1 is deleted\. Their files are in the holding folder/)).toBeVisible();
	expect(existsSync(join(folder, 'Season 1', 'Quarry Bells - S01E01 - Echo 1080p WEB.mkv'))).toBe(false);
	// The season folder it emptied goes; the person's Extras folder stays.
	expect(existsSync(join(folder, 'Season 1'))).toBe(false);
	const held = join(showsDir, '.holyarrs-replaced');
	expect(existsSync(held)).toBe(true);
	expect(existsSync(files.extra)).toBe(true);
	// The show stays, with Season 1 no longer kept up to date.
	await expect(page.getByRole('heading', { name: /Quarry Bells/ })).toBeVisible();
	await expect(s1.getByRole('checkbox', { name: 'Keep Season 1 up to date' })).not.toBeChecked();
});

test('an episode History link lists only that episode, named on each row', async () => {
	const s1 = await openSeasonNamed(page, 'Season 1');
	await expect(s1.getByTestId('episode').filter({ hasText: 'S01E02' })).toBeVisible();
	await episodeAction(page, 'S01E02', 'History');
	await expect(page).toHaveURL(/episode=\d+/);
	await expect(page.getByTestId('episode-filter')).toContainText('Only S01E02 Dust of Quarry Bells');
	const rows = page.getByTestId('history-row');
	await expect(rows.first()).toBeVisible();
	await expect(page.getByTestId('history-row').filter({ hasText: 'Deleted Quarry Bells season 1 from the Library' })).toBeVisible();
	await expect(rows.first().getByTestId('history-episode')).toBeVisible();
	await axe(page, 'history of an episode');
	await page.getByRole('button', { name: 'All of the show' }).click();
	await expect(page.getByTestId('episode-filter')).toHaveCount(0);
	await expect(page.getByTestId('history-row').filter({ hasText: 'Adopted Quarry Bells' })).toBeVisible();
});

test("the Calendar shows this week's episode, and the movies and shows filter", async () => {
	await page.goto('/calendar');
	await page.getByRole('radio', { name: 'List', exact: true }).click();
	const item = page.getByTestId('cal-item').filter({ hasText: 'Quarry Bells S02E01 New Bell' });
	// The episode airs tomorrow (UTC); when that is next month, step forward.
	const now = new Date();
	const thisMonth = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
	if (new Date(Date.now() + 86400000).toISOString().slice(0, 7) !== thisMonth) await page.getByRole('button', { name: 'Month after' }).click();
	await expect(item).toBeVisible();
	await expect(item).toContainText('Episode');
	await expect(item).toContainText('on Lab Three');
	await page.getByTestId('cal-titles').getByRole('radio', { name: 'Movies' }).click();
	await expect(item).toHaveCount(0);
	await page.getByTestId('cal-titles').getByRole('radio', { name: 'Shows' }).click();
	await expect(item).toBeVisible();
	await expect(page.getByRole('checkbox', { name: 'In cinemas' })).toHaveCount(0);
	await page.getByRole('radio', { name: 'Month', exact: true }).click();
	await expect(page.getByTestId('cal-entry').filter({ hasText: 'Quarry Bells S02E01' })).toBeVisible();
	await axe(page, 'calendar with episodes');
});

test('Settings > Alerts: one Alert per pack, or one per episode', async () => {
	await page.goto('/settings/alerts');
	const card = page.getByTestId('pack-alerts');
	await expect(card.getByRole('radio', { name: /One Alert for the pack/ })).toBeChecked();
	await card.getByRole('radio', { name: /One Alert per episode/ }).check();
	await expect(page.getByText('A pack now sends one Alert per episode.')).toBeVisible();
	await page.reload();
	await expect(card.getByRole('radio', { name: /One Alert per episode/ })).toBeChecked();
	await card.getByRole('radio', { name: /One Alert for the pack/ }).check();
	await expect(page.getByText('A pack now sends one Alert listing its episodes.')).toBeVisible();
	await axe(page, 'alerts with packs');
});

test('the show page, import, history and calendar work at phone width', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/library');
	await phone.getByRole('link', { name: /Quarry Bells/ }).first().click();
	await expect(phone.getByRole('button', { name: 'Library', exact: true })).toBeVisible();
	await noSidewaysScroll(phone);
	await titleAction(phone, 'Library', 'Delete show');
	await expect(phone.getByTestId('show-delete').getByRole('checkbox', { name: /Never-add/ })).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'delete a show on a phone');
	await phone.getByTestId('show-delete').getByRole('button', { name: 'Cancel' }).click();
	for (const path of ['/library/import', '/downloads/history', '/calendar']) {
		await phone.goto(path);
		await expect(phone.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(phone);
	}
	await ctx.close();
});

test('deleting the whole show with Never-add keeps its files and leaves the Library', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Quarry Bells/ }).first().click();
	await titleAction(page, 'Library', 'Delete show');
	const dialog = page.getByTestId('show-delete');
	await dialog.getByRole('checkbox', { name: /Never-add/ }).check();
	await expect(dialog.getByTestId('delete-plan')).toContainText('The show leaves the Library');
	await expect(dialog.getByTestId('delete-plan')).toContainText('It goes on the Never-add list');
	await dialog.getByRole('button', { name: 'Delete show' }).click();
	await expect(page).toHaveURL(/\/library$/);
	await expect(page.getByText('Quarry Bells is deleted. Files stay on disk.')).toBeVisible();
	expect(existsSync(files.extra)).toBe(true);
});
