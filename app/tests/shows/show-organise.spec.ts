import { expect, test, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, titleAction, openSeasonNamed, episodeAction, seasonKept } from '../helpers/helpers.ts';
import { axe as scan } from '../helpers/axe.ts';

// Organise for shows (5.2): Quarry Bells sits on disk before HolyArrs knows
// it. The owner imports it from the show Library folder (nothing moves),
// renames its files from the show page, deletes one episode of a
// multi-episode file (the file stays) and then the season (its files go to
// the holding folder), reads an episode's History, finds this week's
// episode on the Calendar, picks how pack Alerts are told, and checks it all
// from a phone. Runs after show-downloads.spec.ts.
test.describe.configure({ mode: 'serial' });

let page: Page;
const showsDir = join(moviesDir, '..', 'shows');
const folder = join(showsDir, 'Quarry Bells (2021)');
const files = {
	one: join(folder, 'Season 1', 'Quarry.Bells.S01E01.1080p.WEB-DL.H264-HOLYARRS.mkv'),
	multi: join(folder, 'Season 1', 'Quarry.Bells.S01E02E03.1080p.WEB-DL.H264-HOLYARRS.mkv'),
	extra: join(folder, 'Extras', 'making of.mkv')
};

const axe = (p: Page, where: string) => scan(p, where, { waitAll: true });
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
	await lf.selectOption((await lf.locator('option', { hasText: showsDir }).first().getAttribute('value')) ?? '');
	await page.getByRole('button', { name: 'Scan', exact: true }).click();
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
	// Last to first: unchecking a box shifts the later "checked" matches, not the earlier ones.
	for (const box of (await plan.getByRole('checkbox', { checked: true }).all()).reverse()) {
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
	const sheet = page.getByRole('dialog', { name: 'Rename files' });
	await expect(sheet).toHaveAccessibleDescription(/Quarry Bells/);
	await expect(sheet.getByTestId('rename-step')).toHaveCount(2);
	await expect(sheet).toContainText('Quarry Bells - S01E01 - Echo 1080p WEB.mkv');
	await expect(sheet).toContainText('Quarry Bells - S01E02-E03 - Dust + Granite 1080p WEB.mkv');
	await axe(page, 'show rename preview');
	await sheet.getByRole('button', { name: 'Rename 2 files' }).click();
	await expect(page.getByText('2 files renamed')).toBeVisible({ timeout: 30_000 });
	// The sheet closes itself when the rename is done; opened again, nothing is left to rename.
	await expect(sheet).toBeHidden();
	await titleAction(page, 'Library', 'Rename files');
	await expect(sheet.getByTestId('rename-none')).toBeVisible();
	await sheet.getByRole('button', { name: 'Close', exact: true }).click();
	await expect(sheet).toBeHidden();
	const season1 = readdirSync(join(folder, 'Season 1')).sort();
	expect(season1).toEqual(['Quarry Bells - S01E01 - Echo 1080p WEB.mkv', 'Quarry Bells - S01E02-E03 - Dust + Granite 1080p WEB.mkv']);
	expect(existsSync(files.extra)).toBe(true);
});

test('deleting one episode of a multi-episode file keeps the file; deleting the season holds its files', async () => {
	// One episode: from its sheet on the Library tab.
	const s1 = await openSeasonNamed(page, 'Season 1');
	await s1.getByRole('button', { name: /^Episode 2:/ }).click();
	const sheet = page.getByRole('dialog', { name: 'Season 1 · Episode 2' });
	await sheet.getByRole('button', { name: /^Delete episode/ }).click();
	const dialog = page.getByTestId('show-delete');
	await expect(page.getByRole('dialog', { name: 'Delete Quarry Bells S01E02?' })).toBeVisible();
	await expect(dialog.getByRole('switch', { name: /Never add again/ })).toHaveCount(0);
	await dialog.getByRole('switch', { name: /Also delete their files/ }).click();
	await expect(dialog.getByTestId('delete-plan')).toContainText('it also holds S01E03, which stays');
	await axe(page, 'delete an episode');
	await dialog.getByRole('button', { name: 'Cancel' }).click();

	// The whole season: from its Files, rename and history sheet.
	await page.getByRole('button', { name: 'Season 1 tools' }).click();
	await page.getByTestId('season-tools').getByRole('button', { name: 'Delete Season 1' }).click();
	await expect(page.getByRole('dialog', { name: 'Delete Quarry Bells Season 1?' })).toBeVisible();
	await expect(dialog.getByTestId('delete-plan')).toContainText('Season 1: 3 episodes stop being kept up to date');
	await dialog.getByRole('switch', { name: /Also delete their files/ }).click();
	await expect(dialog.getByTestId('delete-plan')).toContainText('2 files to the holding folder');
	await dialog.getByRole('button', { name: 'Delete Season 1' }).click();
	await expect(page.getByText("Quarry Bells Season 1 deleted")).toBeVisible();
	await expect(page.getByText(/Files kept/)).toBeVisible();
	expect(existsSync(join(folder, 'Season 1', 'Quarry Bells - S01E01 - Echo 1080p WEB.mkv'))).toBe(false);
	// The season folder it emptied goes; the person's Extras folder stays.
	expect(existsSync(join(folder, 'Season 1'))).toBe(false);
	const held = join(showsDir, '.holyarrs-replaced');
	expect(existsSync(held)).toBe(true);
	expect(existsSync(files.extra)).toBe(true);

	// The show stays, with Season 1 no longer kept up to date and no files left.
	await page.reload();
	await expect(page.getByRole('heading', { name: /Quarry Bells/ })).toBeVisible();
	expect(await seasonKept(page, 'Season 1')).toBe(false);
	await page.getByRole('button', { name: 'Season 1 tools' }).click();
	const tools = page.getByTestId('season-tools');
	await expect(tools).toContainText('0 of 3 episodes on disk');
	await tools.getByRole('tab', { name: 'History' }).click();
	await expect(tools).toContainText('Deleted Quarry Bells season 1 from the Library');
	await page.getByRole('dialog', { name: 'Season 1' }).getByRole('button', { name: 'Close', exact: true }).click();
	await expect(tools).toBeHidden();
});

test("an episode's History lists only that episode", async () => {
	await episodeAction(page, 'S01E02', 'History');
	const sheet = page.getByRole('dialog', { name: 'Season 1 · Episode 2' });
	await expect(sheet.getByRole('heading', { name: 'Dust' })).toBeVisible();
	const events = sheet.getByTestId('episode-sheet').locator('ol.es-tl');
	await expect(events).toContainText('Deleted Quarry Bells season 1 from the Library');
	// Adopting was the whole show, so it is not in the episode's History.
	await expect(events).not.toContainText('Adopted Quarry Bells');
	await axe(page, 'history of an episode');
	await sheet.getByRole('button', { name: 'Close', exact: true }).click();
	await expect(sheet).toBeHidden();
	// All history (the whole show) lists the adopt too.
	await titleAction(page, 'More actions', 'All history');
	await expect(page).toHaveURL(/\/downloads\/history\?title=\d+/);
	// History rows name the title, the episodes and a kind word; the kind is on data-kind.
	await expect(page.locator('[data-testid="history-row"][data-kind="adopted"]').filter({ hasText: 'Quarry Bells' })).toBeVisible();
	await expect(page.locator('[data-testid="history-row"][data-kind="episodes_removed"]').filter({ hasText: /3 episodes/ })).toBeVisible();
});

test("the Calendar shows this week's episode, and the movies and shows filter", async () => {
	await page.goto('/calendar');
	await page.getByRole('radio', { name: 'List', exact: true }).click();
	const item = page.getByTestId('cal-item').filter({ hasText: 'Quarry Bells' }).filter({ hasText: 'Season 2, Episode 1 · New Bell' });
	// The owner follows nothing here (the show was adopted, not asked for), so
	// the Calendar opens on My follows with a way to the Whole Library.
	const whole = page.getByRole('button', { name: 'Show the Whole Library' });
	await expect(whole.or(item).first()).toBeVisible();
	if (await whole.isVisible()) await whole.click();
	// The episode airs tomorrow (UTC); the Calendar opens on the weeks from today.
	await expect(item).toBeVisible();
	// A list row names the episode as Season N, Episode N; a channel the browser's
	// country does not know (Lab Three) is not named on it.
	await expect(item.getByRole('link', { name: /^Quarry Bells, .*Season 2, Episode 1 · New Bell$/ })).toBeVisible();
	await page.getByTestId('cal-titles').getByRole('radio', { name: 'Movies' }).click();
	await expect(item).toHaveCount(0);
	await page.getByTestId('cal-titles').getByRole('radio', { name: 'TV shows', exact: true }).click();
	await expect(item).toBeVisible();
	await expect(page.getByRole('checkbox', { name: 'In cinemas' })).toHaveCount(0);
	await page.getByRole('radio', { name: 'Cards', exact: true }).click();
	await expect(page.getByTestId('cal-cards').getByRole('link', { name: 'Quarry Bells' }).first()).toBeVisible();
	await axe(page, 'calendar with episodes');
});

test('Settings > Alerts: one Alert per pack, or one per episode', async () => {
	await page.goto('/settings/alerts');
	const card = page.getByTestId('pack-alerts');
	await expect(card.getByRole('radio', { name: /One Alert for the pack/ })).toBeChecked();
	await card.getByRole('radio', { name: /One Alert per episode/ }).check();
	await expect(page.getByText('One Alert per episode')).toBeVisible();
	await page.reload();
	await expect(card.getByRole('radio', { name: /One Alert per episode/ })).toBeChecked();
	await card.getByRole('radio', { name: /One Alert for the pack/ }).check();
	await expect(page.getByText('One Alert per pack')).toBeVisible();
	await axe(page, 'alerts with packs');
});

test('the show page, import, history and calendar work at phone width', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await phone.goto('/library?kind=show');
	await phone.getByRole('link', { name: /Quarry Bells/ }).first().click();
	await expect(phone.getByRole('button', { name: 'More actions', exact: true })).toBeVisible();
	await noSidewaysScroll(phone);
	await titleAction(phone, 'Library', 'Delete show');
	const del = phone.getByTestId('delete-sheet');
	await expect(del.getByRole('switch', { name: /Never add again/ })).toBeVisible();
	await noSidewaysScroll(phone);
	await axe(phone, 'delete a show on a phone');
	await del.getByRole('button', { name: 'Cancel' }).click();
	await expect(del).toBeHidden();
	for (const path of ['/library/import', '/downloads/history', '/calendar']) {
		await phone.goto(path);
		await expect(phone.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(phone);
	}
	await ctx.close();
});

test('deleting the whole show with Never-add keeps its files and leaves the Library', async () => {
	await page.goto('/library?kind=show');
	await page.getByRole('link', { name: /Quarry Bells/ }).first().click();
	await titleAction(page, 'Library', 'Delete show');
	const del = page.getByTestId('delete-sheet');
	const never = del.getByRole('switch', { name: /Never add again/ });
	await never.click();
	await expect(never).toHaveAttribute('aria-checked', 'true');
	await expect(del.getByTestId('delete-plan')).toContainText('Leaves your Library');
	await expect(del.getByTestId('delete-plan')).toContainText('Lists never add it again');
	await del.getByRole('button', { name: 'Delete show', exact: true }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Quarry Bells deleted' })).toBeVisible();
	await expect(page.getByRole('status').filter({ hasText: 'Quarry Bells deleted' })).toContainText('Files stay on disk');
	// The title page stays open, now without its Library tab; the Library no longer lists the show.
	await expect(page.getByTestId('own-tabs')).toHaveCount(0);
	expect(existsSync(files.extra)).toBe(true);
	await page.goto('/library?kind=show');
	await expect(page.getByRole('link', { name: /Harbour Lights/ }).first()).toBeVisible();
	await expect(page.getByRole('link', { name: /Quarry Bells/ })).toHaveCount(0);
});
