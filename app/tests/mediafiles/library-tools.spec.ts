import { expect, test, type APIResponse, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { libraryTool, libraryView, noSidewaysScroll, owner, signIn, titleAction } from '../helpers/helpers.ts';
import { tmp } from '../helpers/data-folder.ts';

// Library tools land on their own page (Recently deleted opens from Sort & filter).
test('Library tools open to their own page', async ({ page }) => {
	await signIn(page, owner());
	for (const tool of ['Recently deleted', 'Import files by hand']) {
		await page.goto('/library');
		await libraryTool(page, tool);
		await expect(page.getByRole('heading', { level: 1, name: tool })).toBeVisible();
	}
});

test('Library and title pages load TMDB pictures only through the artwork cache', async ({ page }) => {
	const outside: string[] = [];
	page.on('request', (r) => { if (/^https?:\/\/[^/]*tmdb\.org\//.test(r.url())) outside.push(`${new URL(page.url()).pathname}: ${r.url()}`); });
	await signIn(page, owner());
	const { titles } = (await (await page.request.get('/api/v1/titles')).json()) as { titles: { id: number; kind: string }[] };
	const paths = ['/library?kind=movie&layout=table', '/library?kind=movie&layout=grid', '/library?kind=show&layout=overview'];
	for (const t of titles.filter((x) => x.kind === 'movie' || x.kind === 'show')) paths.push(`/library/${t.id}`, `/library/${t.id}?tab=library`);
	// Every Library title adds two pages, so the time grows with the e2e Library.
	test.setTimeout(30_000 + paths.length * 3_000);
	for (const p of paths) {
		await page.goto(p);
		await page.waitForLoadState('networkidle');
	}
	expect(outside).toEqual([]);
});

test('Library tools fit a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 360, height: 780 } });
	const page = await ctx.newPage();
	await signIn(page, owner());
	for (const path of ['/library', '/library/deleted', '/library/import-files']) {
		await page.goto(path);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(page);
	}
	await ctx.close();
});

// The behaviour tests below each set up their own movie and files. The fake
// TMDB knows a fixed set of movies, and earlier projects may have added the
// same one, so a movie is first taken out of the Library (its imported files
// to the holding folder) and added again with no file.

type Call = (method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown) => Promise<APIResponse>;

/** api signs requests with the session's CSRF token, as the app does. */
async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const call: Call = (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
	// Import by hand reads only from known folders: the loose files' folder is an extra import folder.
	const byHand = join(tmp, 'by-hand');
	mkdirSync(byHand, { recursive: true });
	const now = (await (await call('GET', '/settings/import-folders')).json()) as { extra_folders?: string[] };
	const extra = now.extra_folders ?? [];
	if (!extra.includes(byHand)) {
		const put = await call('PUT', '/settings/import-folders', { extra_folders: [...extra, byHand] });
		expect(put.ok(), await put.text()).toBe(true);
	}
	return call;
}

/** freshMovie puts a fake-TMDB movie in the Library with no file and returns its id. */
async function freshMovie(call: Call, tmdb: number, name: string): Promise<number> {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	for (const t of titles.filter((x) => x.kind === 'movie' && x.name === name)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}
	const added = await call('POST', '/titles', { id: `tmdb:${tmdb}`, search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	return ((await added.json()) as { title: { id: number } }).title.id;
}

/** looseFile writes one stand-in video, named like a release, in its own folder under .tmp/by-hand. */
function looseFile(name: string, year: number, folder: string): string {
	const dir = join(tmp, 'by-hand', folder);
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	const file = join(dir, `${name.replace(/[^A-Za-z0-9]+/g, '.')}.${year}.1080p.WEB-DL.H264-LAB.mkv`);
	writeFileSync(file, 'stand-in video '.repeat(64));
	return file;
}

/** giveFile imports a loose file into a movie through the API and returns where it now lives. */
async function giveFile(call: Call, titleId: number, file: string): Promise<string> {
	const r = await call('POST', '/manual-import', { items: [{ path: file, title_id: titleId, mode: 'move' }] });
	expect(r.status(), await r.text()).toBe(200);
	const [res] = ((await r.json()) as { results: { outcome: string; target?: string; error?: string }[] }).results;
	expect(res.outcome, res.error).toBe('placed');
	expect(existsSync(res.target ?? '')).toBe(true);
	return res.target ?? '';
}

/** videosUnder lists the .mkv files anywhere under dir. */
function videosUnder(dir: string): string[] {
	if (!existsSync(dir)) return [];
	return readdirSync(dir, { recursive: true, encoding: 'utf8' }).filter((p) => p.endsWith('.mkv') && !p.includes('.holyarrs-'));
}

/** deleteWithFiles deletes a movie from its page, files too (held files come back with Undo, so no name is typed). */
async function deleteWithFiles(page: Page, name: string) {
	await titleAction(page, 'More actions', 'Delete movie');
	const sheet = page.getByRole('dialog', { name: `Delete ${name}?` });
	await sheet.getByRole('switch', { name: /Also delete the files/ }).click();
	await expect(sheet.getByTestId('delete-plan')).toContainText('holding folder');
	const del = sheet.getByRole('button', { name: 'Delete movie and files' });
	await expect(del).toBeEnabled();
	await del.click();
	await expect(sheet).toBeHidden();
}

// Import files by hand.
test('Import files by hand: the scan guesses the movie, Copy imports it and its Library tab shows the file', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call, 900007, 'Northern Kettle');
	const file = looseFile('Northern Kettle', 2018, 'northern-kettle');

	await page.goto('/library');
	await libraryTool(page, 'Import files by hand');
	await page.getByLabel('Folder or file on the server').fill(dirname(file));
	await page.getByRole('button', { name: 'Scan', exact: true }).click();
	const row = page.getByTestId('import-file').filter({ hasText: basename(file) });
	await expect(row).toContainText('Northern Kettle (2018)');
	await expect(row.getByRole('checkbox', { name: `Import ${basename(file)}` })).toBeChecked();
	// Scanning only reads: the file is still where it was.
	expect(existsSync(file)).toBe(true);

	await page.getByRole('radio', { name: /^Copy/ }).check();
	const answer = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/api/v1/manual-import'));
	await page.getByRole('button', { name: 'Import 1 file' }).click();
	const [res] = ((await (await answer).json()) as { results: { outcome: string; target?: string; title_id?: number }[] }).results;
	expect(res.outcome).toBe('placed');
	expect(res.title_id).toBe(id);
	const target = res.target ?? '';
	await expect(row.getByRole('status')).toContainText(/^(Copied|Linked \(no extra space\))/);
	await expect(page.getByText('1 file imported')).toBeVisible();
	await expect(row.getByRole('checkbox')).not.toBeChecked();
	// Copy keeps the original and puts a copy (or a link) in the Library folder.
	expect(existsSync(file)).toBe(true);
	expect(existsSync(target)).toBe(true);

	await row.getByRole('link', { name: 'Open' }).click();
	await expect(page).toHaveURL(new RegExp(`/library/${id}$`));
	await page.getByRole('tab', { name: 'Library' }).click();
	const pane = page.getByTestId('lib-pane');
	await expect(pane).toContainText('In your library');
	await expect(pane).toContainText(basename(target));
});

// Recently deleted.
test('Recently deleted: a movie deleted with its files is listed with days left, and Restore brings both back', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call, 900009, 'Velvet Tide');
	const target = await giveFile(call, id, looseFile('Velvet Tide', 2016, 'velvet-tide'));

	await page.goto(`/library/${id}`);
	await deleteWithFiles(page, 'Velvet Tide');
	await expect(page.getByText('Velvet Tide deleted')).toBeVisible();
	await expect(page.getByText(/Files kept \d+ days/)).toBeVisible();
	expect(existsSync(target)).toBe(false);

	await page.goto('/library');
	await libraryTool(page, 'Recently deleted');
	// The row names the file by its path inside the Library folder.
	const row = page.getByTestId('deleted-file').filter({ hasText: basename(target) });
	await expect(row).toContainText('Velvet Tide');
	await expect(row).toContainText('with the title');
	await expect(row.getByTestId('deleted-days-left')).toHaveText(/^\d+ days? left$/);
	await expect(page.getByTestId('restore-deleted')).toBeDisabled();
	await row.getByRole('checkbox').check();
	await page.getByTestId('restore-deleted').click();
	await expect(page.getByText('1 file restored')).toBeVisible();
	await expect(row).toHaveCount(0);
	expect(existsSync(target)).toBe(true);

	await page.goto('/library?kind=movie');
	await expect(page.getByRole('main').getByRole('link', { name: /Velvet Tide/ }).first()).toBeVisible();
});

// Undo in the delete message.
test('Undo in the delete message puts the movie and its file back', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call, 900006, 'Ember Road');
	const target = await giveFile(call, id, looseFile('Ember Road', 2019, 'ember-road'));

	await page.goto(`/library/${id}`);
	await deleteWithFiles(page, 'Ember Road');
	const toast = page.locator('.ui-toast').filter({ hasText: 'Ember Road deleted' });
	await expect(toast).toBeVisible();
	expect(existsSync(target)).toBe(false);
	await toast.getByRole('button', { name: 'Undo' }).click();
	await expect(page.getByText('Ember Road restored')).toBeVisible();
	expect(existsSync(target)).toBe(true);
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { name: string; kind: string }[] };
	expect(titles.filter((t) => t.kind === 'movie' && t.name === 'Ember Road')).toHaveLength(1);
});

// Holding set to 0 keeps deleted files until the holding folder is emptied, so no name is typed there either.
test('Delete with files and holding kept until emptied: no name to type, Delete is ready', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call, 900009, 'Velvet Tide');
	const target = await giveFile(call, id, looseFile('Velvet Tide', 2016, 'velvet-tide-off'));
	const rules = (await (await call('GET', '/settings/files')).json()) as Record<string, unknown>;

	await page.goto(`/library/${id}`);
	await titleAction(page, 'More actions', 'Delete movie');
	const sheet = page.getByRole('dialog', { name: 'Delete Velvet Tide?' });
	// Holding is set to 0 only until the plan has loaded, so other projects' deletes are not affected for long.
	const off = await call('PUT', '/settings/files', { ...rules, holding_days: 0 });
	expect(off.ok(), await off.text()).toBe(true);
	try {
		await sheet.getByRole('switch', { name: /Also delete the files/ }).click();
		await expect(sheet.getByTestId('delete-plan')).toContainText('1 file to the holding folder');
	} finally {
		const back = await call('PUT', '/settings/files', rules);
		expect(back.ok(), await back.text()).toBe(true);
	}
	await expect(sheet.getByPlaceholder('Velvet Tide')).toHaveCount(0);
	await expect(sheet.getByRole('button', { name: 'Delete movie and files' })).toBeEnabled();
	// Nothing was deleted: cancel leaves the movie and its file in place.
	await sheet.getByRole('button', { name: 'Cancel' }).click();
	await expect(sheet).toBeHidden();
	expect(existsSync(target)).toBe(true);
});

// Move ticked titles to another folder.
test('Move: two ticked movies go to a second movie folder together', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const shelf = join(tmp, 'movies-shelf-two');
	const label = 'Shelf two';
	mkdirSync(shelf, { recursive: true });
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { path: string }[] };
	if (!library_folders.some((f) => f.path === shelf)) {
		const made = await call('POST', '/library-folders', { path: shelf, kind: 'movie', label });
		expect(made.status(), await made.text()).toBe(201);
	}
	const movies = [
		{ tmdb: 900008, name: 'Copper Lighthouse', year: 2017 },
		{ tmdb: 900001, name: 'The Lantern Keepers', year: 2031 }
	];
	const targets: string[] = [];
	for (const m of movies) {
		const id = await freshMovie(call, m.tmdb, m.name);
		targets.push(await giveFile(call, id, looseFile(m.name, m.year, `move-${m.tmdb}`)));
	}
	expect(videosUnder(shelf)).toEqual([]);

	await page.goto('/library?kind=movie');
	await page.getByTestId('library-select').click();
	for (const m of movies) await page.locator(`[data-testid="library-pick"][aria-label="Select ${m.name}"]`).click();
	const bar = page.getByTestId('select-bar');
	await expect(bar).toContainText('2 selected');
	await bar.getByRole('button', { name: 'Move', exact: true }).click();
	await page.getByRole('menu', { name: 'Move the selected titles to another folder' }).getByRole('menuitem', { name: label, exact: true }).click();
	await expect(page.getByText('2 titles being moved')).toBeVisible();
	await expect(bar).toBeHidden();

	// The move runs as a Job: both files land in the second folder and leave the first.
	await expect.poll(() => videosUnder(shelf).map((p) => basename(p)).sort(), { timeout: 20_000 }).toEqual(targets.map((t) => basename(t)).sort());
	for (const t of targets) expect(existsSync(t)).toBe(false);
});

// Remove a movie series: the fake TMDB's one series, The Brumwick Films.
test('Remove a movie series: grouped by series, Open series, review both movies, remove them and hold the file', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const first = await freshMovie(call, 900021, 'Brumwick Gate');
	const second = await freshMovie(call, 900022, 'Brumwick Gate Again');
	const target = await giveFile(call, first, looseFile('Brumwick Gate', 2008, 'brumwick-gate'));

	// Movies grouped by series (Banners view) show the series with both movies.
	await page.goto('/library?kind=movie');
	// Group by series is a row in the Sort and filter panel.
	await page.getByRole('main').getByRole('button', { name: /^Sort( &|)/ }).first().click();
	await page.getByRole('region', { name: 'Sort' }).getByRole('button', { name: 'Group by collection' }).click();
	await page.keyboard.press('Escape');
	await expect(page).toHaveURL(/group=series/);
	await libraryView(page, 'Banners');
	const head = page.locator('.series-head').filter({ has: page.getByRole('heading', { name: 'The Brumwick Films', exact: true }) });
	await expect(head).toContainText('2 in the Library');
	await head.getByRole('link', { name: 'Open series' }).click();
	await expect(page).toHaveURL(/\/library\/collections\/\d+$/);
	await expect(page.getByRole('heading', { level: 1, name: 'The Brumwick Films' })).toBeVisible();
	const titles = page.locator('ul.titles');
	await expect(titles.getByRole('checkbox', { name: /^Brumwick Gate ?2008 / })).toBeChecked();
	await expect(titles.getByRole('checkbox', { name: /^Brumwick Gate Again ?2012 / })).toBeChecked();

	// The review names both movies and their files; nothing has changed yet.
	await page.getByLabel('Move HolyArrs-managed files to the holding folder').check();
	await page.getByRole('button', { name: 'Review 2 selected' }).click();
	const confirm = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Confirm this exact selection' }) });
	await expect(confirm.locator('.confirm-row').filter({ has: page.getByText('Brumwick Gate', { exact: true }) })).toContainText('1 File');
	await expect(confirm.locator('.confirm-row').filter({ has: page.getByText('Brumwick Gate Again', { exact: true }) })).toContainText('0 Files');
	expect(existsSync(target)).toBe(true);

	await confirm.getByRole('button', { name: 'Remove reviewed movies' }).click();
	const done = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Collection removed' }) });
	await expect(done).toContainText('Brumwick Gate was removed from the Library.');
	await expect(done).toContainText('Brumwick Gate Again was removed from the Library.');

	// Both are gone from the Library; the file went to the holding folder.
	const { titles: left } = (await (await call('GET', '/titles')).json()) as { titles: { id: number }[] };
	expect(left.filter((t) => t.id === first || t.id === second)).toEqual([]);
	expect(existsSync(target)).toBe(false);
	// Holding paths are given inside their Library folder, with forward slashes.
	const { files } = (await (await call('GET', '/holding')).json()) as { files: { path: string; held_path: string; title_id: number }[] };
	const held = files.find((f) => f.title_id === first);
	expect(held, 'the movie file is in the holding folder').toBeTruthy();
	const was = target.replaceAll('\\', '/');
	expect(was.endsWith(`/${held?.path}`), `${was} ends with ${held?.path}`).toBe(true);
	const heldOnDisk = join(was.slice(0, was.length - (held?.path.length ?? 0)), held?.held_path ?? '');
	expect(existsSync(heldOnDisk), heldOnDisk).toBe(true);
	await page.goto('/library?kind=movie');
	await expect(page.getByRole('main').getByRole('link', { name: /Brumwick Gate/ })).toHaveCount(0);
});
