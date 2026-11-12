import { expect, test, type APIResponse, type Page } from '@playwright/test';
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, sep } from 'node:path';
import { episodeState, libraryTool, owner, signIn } from '../helpers/helpers.ts';
import { tmp } from '../helpers/data-folder.ts';

// Import files by hand: the parts a person does in the page itself.
// Picking another Library title than the scan's guess, and bringing in
// loose episodes of a show, with the season and episode picked where the
// name does not say.

type Call = (method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown) => Promise<APIResponse>;

/** api signs requests with the session's CSRF token and makes .tmp/by-hand an extra import folder. */
async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const call: Call = (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
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

/** dropTitles takes every Library title of this kind and name out, files too. */
async function dropTitles(call: Call, kind: 'movie' | 'show', name: string) {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	for (const t of titles.filter((x) => x.kind === kind && x.name === name)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}
}

/** freshMovie puts a fake-TMDB movie in the Library with no file and returns its id. */
async function freshMovie(call: Call, tmdb: number, name: string): Promise<number> {
	await dropTitles(call, 'movie', name);
	const added = await call('POST', '/titles', { id: `tmdb:${tmdb}`, search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	return ((await added.json()) as { title: { id: number } }).title.id;
}

/** freshShow puts a fake show in a show Library folder (made when there is none) with no files and returns its id. */
async function freshShow(call: Call, tmdb: number, name: string): Promise<number> {
	await dropTitles(call, 'show', name);
	type Folder = { id: number; path: string; kind: string };
	const list = async () => ((await (await call('GET', '/library-folders')).json()) as { library_folders: Folder[] }).library_folders;
	let folder = (await list()).find((f) => f.kind === 'show');
	if (!folder) {
		const path = join(tmp, 'shows');
		mkdirSync(path, { recursive: true });
		const made = await call('POST', '/library-folders', { path, kind: 'show', label: 'Shows' });
		expect(made.status(), await made.text()).toBe(201);
		folder = (await list()).find((f) => f.kind === 'show');
	}
	const added = await call('POST', '/shows', { id: `tmdb:${tmdb}`, library_folder_id: folder?.id, keep_up_to_date: false, search_on_add: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	return ((await added.json()) as { title: { id: number } }).title.id;
}

/** looseFiles writes stand-in videos with these names in their own folder under .tmp/by-hand. */
function looseFiles(folder: string, names: string[]): string[] {
	const dir = join(tmp, 'by-hand', folder);
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	return names.map((n) => {
		const file = join(dir, n);
		writeFileSync(file, 'stand-in video '.repeat(64));
		return file;
	});
}

type Result = { path: string; outcome: string; placed_by?: string; target?: string; title_id?: number; error?: string };

/** scanFolder opens Import files by hand from the Library's Add menu and scans one folder. */
async function scanFolder(page: Page, folder: string) {
	await page.goto('/library');
	await libraryTool(page, 'Import files by hand');
	await page.getByLabel('Folder or file on the server').fill(folder);
	await page.getByRole('button', { name: 'Scan', exact: true }).click();
	await expect(page.getByTestId('import-files')).toBeVisible();
}

/** importTicked presses the Import button and returns the server's results. */
async function importTicked(page: Page, label: string): Promise<Result[]> {
	const answer = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/api/v1/manual-import'));
	await page.getByRole('button', { name: label, exact: true }).click();
	return ((await (await answer).json()) as { results: Result[] }).results;
}

test('Import files by hand: Change matches a wrongly named file to another Library movie and imports it there without asking Copy or Move', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const wrong = await freshMovie(call, 900040, 'Tamarind Quay');
	const right = await freshMovie(call, 900041, 'Ochre Bandstand');
	// Named like Tamarind Quay, but it is Ochre Bandstand.
	const [file] = looseFiles('tamarind-quay', ['Tamarind.Quay.2013.1080p.WEB-DL.H264-LAB.mkv']);

	await scanFolder(page, dirname(file));
	const row = page.getByTestId('import-file').filter({ hasText: basename(file) });
	await expect(row.locator('.match')).toContainText('Tamarind Quay (2013)');
	await expect(row.getByRole('checkbox', { name: `Import ${basename(file)}` })).toBeChecked();

	// Change: look the right movie up in the Library and pick it.
	await row.getByRole('button', { name: 'Change', exact: true }).click();
	const look = row.getByPlaceholder('Movie or show in the Library');
	await expect(look).toHaveValue('Tamarind Quay');
	await look.fill('Ochre Bandstand');
	await row.getByRole('button', { name: 'Look up', exact: true }).click();
	const hit = row.locator('.found li').filter({ hasText: 'Ochre Bandstand (2012)' });
	await expect(hit).toContainText('Movie');
	await hit.getByRole('button', { name: 'Pick', exact: true }).click();
	await expect(hit.getByRole('button', { name: 'Picked', exact: true })).toBeVisible();
	await row.getByRole('button', { name: 'Use this', exact: true }).click();
	await expect(look).toBeHidden();
	await expect(row.locator('.match')).toContainText('Ochre Bandstand (2012)');
	await expect(row.locator('.match')).not.toContainText('Tamarind Quay');
	await expect(row.getByRole('checkbox', { name: `Import ${basename(file)}` })).toBeChecked();

	// No Copy/Move choice: placed like automatic imports.
	await expect(page.getByRole('radio')).toHaveCount(0);
	const [res] = await importTicked(page, 'Import 1 file');
	expect(res.outcome, res.error).toBe('placed');
	expect(res.title_id).toBe(right);
	expect(['hardlink', 'copy']).toContain(res.placed_by);
	const target = res.target ?? '';
	await expect(row.getByRole('status')).toHaveText(/^(Added to the Library \(no extra space used\)|Copied to the Library \(uses extra space\))/);
	await expect(page.getByText('1 file imported')).toBeVisible();
	// The loose file stays and the movie's file is in Ochre Bandstand's folder.
	expect(existsSync(file)).toBe(true);
	expect(existsSync(target)).toBe(true);
	const { versions } = (await (await call('GET', `/titles/${right}`)).json()) as { versions: { folder_path: string }[] };
	expect(target.startsWith(versions[0].folder_path + sep), `${target} is in ${versions[0].folder_path}`).toBe(true);
	expect(basename(versions[0].folder_path)).toMatch(/^Ochre Bandstand/);
	// The guessed movie got nothing.
	const was = (await (await call('GET', `/titles/${wrong}`)).json()) as { slots: { file?: unknown }[] };
	expect(was.slots.filter((s) => s.file)).toEqual([]);

	await row.getByRole('link', { name: 'Open' }).click();
	await expect(page).toHaveURL(new RegExp(`/library/${right}$`));
	await page.getByRole('tab', { name: 'Library' }).click();
	const pane = page.getByTestId('lib-pane');
	await expect(pane).toContainText('In your library');
	await expect(pane).toContainText(basename(target));
});

test('Import files by hand: two loose episodes of a Library show, one season and episode picked by hand, land in the show folder', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const show = await freshShow(call, 800041, 'Gristmill Hollow');
	// One named with its episode; one named by a day no episode aired (it is S02E01).
	const [named, dated] = looseFiles('gristmill-hollow', ['Gristmill.Hollow.S01E01.1080p.WEB-DL.H264-LAB.mkv', 'Gristmill.Hollow.2019.04.07.1080p.WEB-DL.H264-LAB.mkv']);

	await scanFolder(page, dirname(named));
	const first = page.getByTestId('import-file').filter({ hasText: basename(named) });
	const second = page.getByTestId('import-file').filter({ hasText: basename(dated) });
	await expect(first.locator('.match')).toContainText('Gristmill Hollow (2018) · S01E01');
	await expect(first.getByRole('checkbox', { name: `Import ${basename(named)}` })).toBeChecked();
	await expect(second.locator('.match')).toContainText('Gristmill Hollow (2018) · pick the episode');
	await expect(second).toContainText('Pick the season and episode.');
	await expect(second.getByRole('checkbox', { name: `Import ${basename(dated)}` })).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Import 1 file', exact: true })).toBeEnabled();

	// Change: the show is already picked; pick Season 2, episode 1.
	await second.getByRole('button', { name: 'Change', exact: true }).click();
	const use = second.getByRole('button', { name: 'Use this', exact: true });
	await expect(use).toBeDisabled();
	await second.getByLabel('Season', { exact: true }).selectOption({ label: 'Season 2' });
	await second.getByLabel('Episode', { exact: true }).selectOption({ label: 'S02E01 · Flood Gate' });
	await use.click();
	await expect(use).toBeHidden();
	await expect(second.locator('.match')).toContainText('Gristmill Hollow (2018) · S02E01');
	await expect(second).not.toContainText('Pick the season and episode.');
	await expect(second.getByRole('checkbox', { name: `Import ${basename(dated)}` })).toBeChecked();

	// Importing keeps the loose files.
	const results = await importTicked(page, 'Import 2 files');
	const byName = (f: string) => results.find((r) => basename(r.path) === basename(f));
	for (const f of [named, dated]) {
		const r = byName(f);
		expect(r?.outcome, `${basename(f)}: ${r?.error}`).toBe('placed');
		expect(r?.title_id).toBe(show);
	}
	expect(results).toHaveLength(2);
	await expect(page.getByText('2 files imported')).toBeVisible();
	for (const row of [first, second]) await expect(row.getByRole('status')).toHaveText(/^(Copied|Linked \(no extra space\))/);
	expect(existsSync(named)).toBe(true);
	expect(existsSync(dated)).toBe(true);

	// Both files are in the show's folder, named for their episodes.
	const { versions } = (await (await call('GET', `/titles/${show}`)).json()) as { versions: { folder_path: string }[] };
	const folder = versions[0].folder_path;
	expect(basename(folder)).toMatch(/^Gristmill Hollow/);
	for (const [t, code] of [[byName(named)?.target ?? '', 'S01E01'], [byName(dated)?.target ?? '', 'S02E01']]) {
		expect(t.startsWith(folder + sep), `${t} is in ${folder}`).toBe(true);
		expect(basename(t)).toContain(code);
		expect(existsSync(t), t).toBe(true);
	}

	// The two episodes are in the Library; the others still have no file.
	const { episodes } = (await (await call('GET', `/titles/${show}/episodes`)).json()) as { episodes: { season: number; episode: number; slots: { state: string; file_id?: number }[] }[] };
	const got = episodes.filter((e) => e.slots[0]?.file_id).map((e) => [e.season, e.episode, e.slots[0].state]).sort((a, b) => String(a).localeCompare(String(b)));
	expect(got).toEqual([[1, 1, 'in_library'], [2, 1, 'in_library']]);

	await page.goto(`/library/${show}`);
	await expect(await episodeState(page, 'S01E01')).toHaveText(/^1080p/);
	await expect(await episodeState(page, 'S02E01')).toHaveText(/^1080p/);
	await expect(await episodeState(page, 'S01E02')).not.toHaveText(/^1080p/);
});
