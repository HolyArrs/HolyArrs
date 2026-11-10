import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { existsSync, readdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { makeOldLibrary } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn, titleAction, libraryTool, openAdvanced } from '../helpers/helpers.ts';
import { tmp } from '../helpers/data-folder.ts';

// Organise: import an existing library from the screen, rename
// files after seeing the preview, delete a movie after seeing the plan, and
// Settings > Naming and files. Runs last.
test.describe.configure({ mode: 'serial' });

const oldLib = join(tmp, 'old-library');
let page: Page;

async function fresh(browser: Browser, width = 1280) {
	const ctx = await browser.newContext({ viewport: { width, height: 900 } });
	return ctx.newPage();
}

test.beforeAll(async ({ browser }) => {
	rmSync(oldLib, { recursive: true, force: true });
	makeOldLibrary(oldLib);
	page = await fresh(browser);
	await signIn(page, owner());
	await page.goto('/settings/library-folders');
	await page.getByRole('button', { name: 'Add a folder' }).click();
	await page.getByLabel('Folder path').fill(oldLib);
	await page.getByLabel('Name').fill('Old library');
	await page.getByRole('button', { name: 'Add', exact: true }).click();
	// the row shows its name and a short path (full path on hover)
	await expect(page.locator('#main .folder').filter({ hasText: 'Old library' })).toBeVisible();
});

test('import: scan shows matched, unsure and unmatched; fix the unsure one; adopt without moving anything', async () => {
	await page.goto('/library');
	await libraryTool(page, 'Import a library');
	await expect(page.getByRole('heading', { level: 1, name: 'Import a library' })).toBeVisible();
	await expect(page.getByTestId('nothing-moved')).toContainText('Files stay where they are');
	await page.getByLabel('Library folder').selectOption({ label: `Old library (${oldLib})` });
	await page.getByRole('button', { name: 'Scan', exact: true }).click();

	const plan = page.getByTestId('adopt-plan');
	await expect(plan).toBeVisible({ timeout: 30_000 });
	const matched = page.getByTestId('group-matched');
	await expect(matched).toContainText('Marble Comet (2014)');
	await expect(matched).toContainText('Willow Signal {tmdb-900013}');
	await expect(matched.getByRole('checkbox', { name: 'Adopt Marble Comet (2014)' })).toBeChecked();
	const unsure = page.getByTestId('group-unsure');
	await expect(unsure).toContainText('Cinder (2012)');
	await expect(unsure.getByRole('checkbox')).not.toBeChecked();
	await expect(page.getByTestId('group-unmatched')).toContainText('Home Videos');
	await expect(page.getByTestId('group-unmatched')).toContainText('Notes');
	await expect(page.getByRole('checkbox', { name: 'Adopt Notes' })).toBeDisabled();
	// Nothing has changed on disk.
	expect(existsSync(join(oldLib, 'Marble Comet (2014)', 'Marble.Comet.2014.1080p.WEB-DL.H264-HOLYARRS.mkv'))).toBe(true);

	// Fix the unsure one with a lookup.
	await unsure.getByRole('button', { name: 'Look up' }).click();
	await unsure.getByLabel('Look up a movie for Cinder (2012)').fill('Cinder Waltz');
	await unsure.getByRole('button', { name: 'Look up', exact: true }).click();
	await unsure.locator('.found li', { hasText: 'Cinder Waltz (2012)' }).getByRole('button', { name: 'Use this' }).click();
	await expect(unsure.getByRole('checkbox', { name: 'Adopt Cinder (2012)' })).toBeChecked();
	await expect(unsure).toContainText('Cinder Waltz (2012)');

	await page.getByRole('button', { name: 'Adopt 3 movies' }).click();
	const done = page.getByTestId('adopt-results');
	await expect(done).toContainText('3 movies are now in the Library', { timeout: 30_000 });
	// Every file is still where it was.
	expect(readdirSync(join(oldLib, 'Cinder (2012)'))).toEqual(['Cinder.2012.720p.BluRay.x264-OLD.mkv']);
	expect(readdirSync(join(oldLib, 'Willow Signal {tmdb-900013}'))).toEqual(['willow signal.mkv']);
	await page.goto('/library');
	await expect(page.getByRole('link', { name: /Marble Comet/ }).first()).toBeVisible();
});

test('rename: the Movie page shows the old and new name and renames after Apply', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Marble Comet/ }).first().click();
	await titleAction(page, 'Library', 'Rename files');
	const dialog = page.getByRole('dialog');
	const step = dialog.getByTestId('rename-step');
	await expect(step).toHaveCount(1);
	await expect(step).toContainText('Marble.Comet.2014.1080p.WEB-DL.H264-HOLYARRS.mkv');
	await expect(step.locator('.new')).toContainText('Marble Comet (2014)');
	const to = (await step.locator('.new').textContent())!.trim();
	await dialog.getByRole('button', { name: 'Rename 1 file' }).click();
	await expect(page.getByText('1 file renamed')).toBeVisible({ timeout: 30_000 });
	// Done, the sheet closes; opened again it has nothing left to rename.
	await expect(dialog).toBeHidden();
	await titleAction(page, 'Library', 'Rename files');
	await expect(dialog.getByTestId('rename-none')).toContainText('Every name matches.');
	expect(readdirSync(join(oldLib, 'Marble Comet (2014)'))).toEqual([to]);
	await dialog.getByRole('button', { name: 'Close', exact: true }).click();
	await expect(dialog).toBeHidden();
});

test('rename: the whole Library preview lists each movie, and skipped files say why', async () => {
	await page.goto('/library/rename');
	await expect(page.getByRole('heading', { level: 1, name: 'Rename files' })).toBeVisible();
	const steps = page.getByTestId('rename-step');
	await expect(steps.filter({ hasText: 'Cinder Waltz' })).toContainText('Cinder.2012.720p.BluRay.x264-OLD.mkv');
	await expect(steps.filter({ hasText: 'Marble Comet' })).toHaveCount(0);
	await noSidewaysScroll(page);
});

test('rename: a plan that changed since the preview renames nothing and says to look again', async () => {
	// The preview this page shows is made stale, as if a file arrived after it.
	await page.route('**/api/v1/rename', async (route) => {
		if (route.request().method() !== 'GET') return route.fallback();
		const res = await route.fetch();
		const body = await res.json();
		await route.fulfill({ response: res, json: { ...body, plan_hash: '0'.repeat(64) } });
	});
	await page.goto('/library/rename');
	const steps = page.getByTestId('rename-step');
	await expect(steps.filter({ hasText: 'Cinder Waltz' })).toBeVisible();
	await page.unroute('**/api/v1/rename');
	const n = await steps.count();
	await page.getByRole('button', { name: n === 1 ? 'Rename 1 file' : `Rename ${n} files` }).click();
	await expect(page.getByRole('alert')).toContainText('Files changed, so nothing was renamed', { timeout: 30_000 });
	expect(readdirSync(join(oldLib, 'Cinder (2012)'))).toEqual(['Cinder.2012.720p.BluRay.x264-OLD.mkv']);
	expect(readdirSync(join(oldLib, 'Willow Signal {tmdb-900013}'))).toEqual(['willow signal.mkv']);
});

test('delete: the plan shows first, changes with each choice, and files go to the holding folder', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Willow Signal/ }).first().click();
	await titleAction(page, 'Library', 'Delete movie');
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByRole('heading', { name: 'Delete Willow Signal?' })).toBeVisible();
	const plan = dialog.getByTestId('delete-plan');
	await expect(plan).toContainText('1 file stays on disk');
	await expect(plan).not.toContainText('holding folder');
	await dialog.getByRole('switch', { name: /^Also delete the files/ }).click();
	await expect(plan).toContainText('1 file to the holding folder for 7 days');
	await dialog.getByRole('switch', { name: /^Never add again/ }).click();
	await expect(plan).toContainText('Lists never add it again');
	// Still nothing has happened; held files come back with Undo, so no name is asked for.
	expect(readdirSync(join(oldLib, 'Willow Signal {tmdb-900013}'))).toEqual(['willow signal.mkv']);
	const go = dialog.getByRole('button', { name: 'Delete movie and files' });
	await expect(go).toBeEnabled();
	await expect(dialog.getByRole('textbox')).toHaveCount(0);
	await go.click();
	await expect(page.getByText("Willow Signal deleted")).toBeVisible();
	await expect(page.getByText("Files kept 7 days")).toBeVisible();
	// The title page stays, no longer in the Library: no Library tab or status.
	await expect(dialog).toBeHidden();
	await expect(page.getByTestId('own-tabs')).toHaveCount(0);
	await expect(page.getByTestId('hero-library')).toHaveCount(0);
	expect(existsSync(join(oldLib, 'Willow Signal {tmdb-900013}', 'willow signal.mkv'))).toBe(false);
	expect(existsSync(join(oldLib, '.holyarrs-replaced'))).toBe(true);
});

test('settings: patterns show a live example, a bad word is caught, and the choices save', async () => {
	await page.goto('/settings');
	await page.getByTestId('settings-group').filter({ hasText: 'Library' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Naming and files', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Naming and files' })).toBeVisible();
	const ex = page.getByTestId('naming-example');
	await expect(ex).toContainText('Dune - Part Two (2024)/Dune - Part Two (2024) 1080p BluRay.mkv');
	await page.getByLabel('File name pattern', { exact: true }).fill('{title} [{year}]');
	await expect(ex).toContainText('Dune - Part Two [2024].mkv');
	await openAdvanced(page);
	await page.getByLabel('A colon in a name becomes').selectOption('remove');
	await expect(ex).toContainText('Dune Part Two [2024].mkv');
	await page.getByLabel('File name pattern', { exact: true }).fill('{title} {nope}');
	await expect(ex.getByRole('alert')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save' })).toBeDisabled();
	await page.getByLabel('File name pattern', { exact: true }).fill('{title} ({year}) {quality}');
	await page.getByLabel('A colon in a name becomes').selectOption('smart');
	await page.getByLabel('Keep deleted files for (days)').fill('14');
	await page.getByLabel('Always leave this much free space (MB)').fill('250');
	await expect(page.getByRole('button', { name: 'Save' })).toBeEnabled();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Naming settings saved')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Keep deleted files for (days)')).toHaveValue('14');
	await expect(page.getByLabel('Always leave this much free space (MB)')).toHaveValue('250');
	// 5.1.7: the folders one Import a library scan reads.
	await expect(page.getByLabel('Folders read per library scan')).toHaveValue('5000');
	await page.getByLabel('Folders read per library scan').fill('5');
	await page.getByRole('button', { name: 'Save' }).click();
	// The browser holds the form back: under 10 is not a valid number here.
	expect(await page.getByLabel('Folders read per library scan').evaluate((e) => (e as HTMLInputElement).validity.valid)).toBe(false);
	// Nothing was saved; put the change back so leaving does not ask first.
	await page.getByTestId('savebar').getByRole('button', { name: 'Discard' }).click();
	await page.reload();
	await expect(page.getByLabel('Folders read per library scan')).toHaveValue('5000');
	await page.getByRole('button', { name: 'Clear now' }).click();
	await expect(page.getByText('Removing held files')).toBeVisible();
});

test('the new screens fit a phone and pass the accessibility scan', async ({ browser }) => {
	const ph = await fresh(browser, 390);
	await signIn(ph, owner());
	for (const url of ['/library/import', '/library/rename', '/settings/naming']) {
		await ph.goto(url);
		await expect(ph.getByRole('heading', { level: 1 })).toBeVisible();
		await ph.waitForLoadState('networkidle');
		await noSidewaysScroll(ph);
		await ph.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
		const r = await new AxeBuilder({ page: ph }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
		expect(r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id), url).toEqual([]);
	}
	// The scan plan and both dialogs at phone width.
	await ph.goto('/library/import');
	await ph.getByLabel('Library folder').selectOption({ label: `Old library (${oldLib})` });
	await ph.getByRole('button', { name: 'Scan', exact: true }).click();
	await expect(ph.getByTestId('adopt-plan')).toBeVisible({ timeout: 30_000 });
	await noSidewaysScroll(ph);
	await expect(ph.getByRole('button', { name: 'Scan', exact: true })).toBeEnabled();
	await ph.evaluate(() => Promise.all(document.getAnimations().map((x) => x.finished.catch(() => undefined))));
	const a = await new AxeBuilder({ page: ph }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = a.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ') + ' ' + n.failureSummary).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), 'scan plan').toEqual([]);
	await ph.goto('/library');
	await ph.getByRole('link', { name: /Cinder Waltz/ }).first().click();
	for (const [button, heading] of [
		['Rename files', 'Rename files'],
		['Delete movie', 'Delete Cinder Waltz?']
	]) {
		await titleAction(ph, 'Library', button);
		const d = ph.getByRole('dialog');
		await expect(d.getByRole('heading', { name: heading })).toBeVisible();
		if (button === 'Delete movie') await expect(d.getByTestId('delete-plan')).toContainText('stays on disk');
		else await expect(d.getByRole('button', { name: 'Rename 1 file' })).toBeVisible();
		// Measure and scan the settled dialog, not the menu closing and the dialog opening.
		await ph.evaluate(() => Promise.all(document.getAnimations().map((x) => x.finished.catch(() => undefined))));
		await noSidewaysScroll(ph);
		const r = await new AxeBuilder({ page: ph }).include('dialog[open]').withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
		expect(r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id), heading).toEqual([]);
		await ph.keyboard.press('Escape');
	}
	await ph.context().close();
});

test('members see none of it', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	await page.getByLabel('Note for yourself').fill('Sam');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const sam = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
	await sam.goto(new URL(link).pathname);
	await sam.getByLabel('Name').fill('sam');
	await sam.getByLabel('Password', { exact: true }).fill(pw);
	await sam.getByLabel('Confirm password').fill(pw);
	await sam.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(sam).toHaveURL(/\/$/);
	await sam.goto('/library');
	await expect(sam.getByRole('link', { name: 'Import a library' })).toHaveCount(0);
	await expect(sam.getByRole('link', { name: 'Rename files' })).toHaveCount(0);
	await sam.getByRole('link', { name: /Cinder Waltz/ }).first().click();
	await expect(sam.getByRole('heading', { level: 1 })).toContainText('Cinder Waltz');
	await expect(sam.getByRole('button', { name: 'Delete' })).toHaveCount(0);
	await expect(sam.getByRole('button', { name: 'Rename files' })).toHaveCount(0);
	// Members get More with only Report a problem: none of the Library tools.
	await sam.getByRole('button', { name: 'More actions', exact: true }).click();
	await expect(sam.getByRole('menuitem')).toHaveText(['Report a problem']);
	await sam.keyboard.press('Escape');
	for (const url of ['/library/import', '/library/rename']) {
		await sam.goto(url);
		await expect(sam).not.toHaveURL(new RegExp(url.replace(/\//g, '\\/') + '$'));
	}
	// A Settings page they may not open shows a note in place of the page.
	await sam.goto('/settings/naming');
	await expect(sam.getByRole('heading', { name: "You can't change this" })).toBeVisible();
	const res = await sam.request.get('/api/v1/rename');
	expect(res.status()).toBe(403);
	await sam.context().close();
});
