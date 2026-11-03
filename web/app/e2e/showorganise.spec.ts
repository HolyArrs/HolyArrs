import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, titleAction, openSeasonNamed, episodeAction, seasonKept } from './helpers.ts';

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
	await expect(page.getByText('2 files renamed.')).toBeVisible({ timeout: 30_000 });
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

interface Plan {
	scope?: string;
	season?: number;
	episodes?: string[];
	applied: boolean;
	held: { path: string }[];
	kept: { path: string; why?: string }[];
}

test('deleting one episode of a multi-episode file keeps the file; deleting the season holds its files', async () => {
	// The title page deletes only the whole show (its Delete sheet); one episode
	// or one season has no delete there, so both go through the same delete
	// call the older show page used: the plan first, then the plan applied.
	const id = Number(/\/library\/(\d+)/.exec(new URL(page.url()).pathname)?.[1]);
	expect(id).toBeGreaterThan(0);
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const del = async (body: Record<string, unknown>) => {
		const r = await page.request.post(`/api/v1/titles/${id}/delete`, { data: body, headers: { 'X-CSRF-Token': me.csrf } });
		expect(r.ok(), await r.text()).toBe(true);
		return (await r.json()) as Plan;
	};
	const s1 = await openSeasonNamed(page, 'Season 1');
	await expect(s1.getByRole('button', { name: /^Episode 2:/ })).toBeVisible();
	const eps = ((await (await page.request.get(`/api/v1/titles/${id}/episodes?season=1`)).json()) as { episodes: { id: number; episode: number }[] }).episodes;
	const e2 = eps.find((e) => e.episode === 2)?.id;
	expect(e2).toBeTruthy();

	const one = await del({ episode_ids: [e2], delete_files: true, apply: false });
	expect(one.scope).toBe('episodes');
	expect(one.episodes).toEqual(['S01E02']);
	expect(one.held).toHaveLength(0);
	expect(one.kept.map((f) => f.why).join(' ')).toContain('it also holds S01E03, which stays');

	const keep = await del({ season: 1, delete_files: false, apply: false });
	expect(keep.scope).toBe('season');
	expect(keep.episodes).toHaveLength(3);
	expect(keep.held).toHaveLength(0);
	const plan = await del({ season: 1, delete_files: true, apply: false });
	expect(plan.held).toHaveLength(2);
	const done = await del({ season: 1, delete_files: true, apply: true });
	expect(done.applied).toBe(true);
	expect(done.held).toHaveLength(2);
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
	await page.getByRole('button', { name: 'Files, rename and history of season 1' }).click();
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
	await expect(page.locator('[data-testid="history-row"][data-kind="episodes_removed"]').filter({ hasText: 'S01E01-E03, 3 episodes' })).toBeVisible();
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
	await expect(phone.getByRole('button', { name: 'More actions', exact: true })).toBeVisible();
	await noSidewaysScroll(phone);
	await titleAction(phone, 'Library', 'Delete show');
	const del = phone.getByTestId('delete-sheet');
	await expect(del.getByRole('switch', { name: /Never add it again/ })).toBeVisible();
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
	await page.goto('/library');
	await page.getByRole('link', { name: /Quarry Bells/ }).first().click();
	await titleAction(page, 'Library', 'Delete show');
	const del = page.getByTestId('delete-sheet');
	const never = del.getByRole('switch', { name: /Never add it again/ });
	await never.click();
	await expect(never).toHaveAttribute('aria-checked', 'true');
	await expect(del.getByTestId('delete-plan')).toContainText('The show leaves the Library');
	await expect(del.getByTestId('delete-plan')).toContainText('It goes on the Never-add list');
	await del.getByRole('button', { name: 'Delete show', exact: true }).click();
	await expect(page.getByText('Quarry Bells is deleted. Its files stay on disk.')).toBeVisible();
	// The title page stays open, now without its Library tab; the Library no longer lists the show.
	await expect(page.getByTestId('own-tabs')).toHaveCount(0);
	expect(existsSync(files.extra)).toBe(true);
	await page.goto('/library');
	await expect(page.getByRole('link', { name: /Harbour Lights/ }).first()).toBeVisible();
	await expect(page.getByRole('link', { name: /Quarry Bells/ })).toHaveCount(0);
});
