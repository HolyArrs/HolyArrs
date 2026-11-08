import { expect, test, type APIResponse, type Locator, type Page } from '@playwright/test';
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { owner, signIn, inLibrary } from '../helpers/helpers.ts';
import { tmp } from '../helpers/data-folder.ts';

// Steps here wait up to 30 s each; under a full chain the default 30 s for the whole test runs out first.
test.describe.configure({ timeout: 90_000 });

// Change many titles at once: the Library's select bar. Each test ticks
// the same two movies and one show (fakes/ keeps them for this file only),
// first taken out of the Library and added again so every test starts clean.

type Call = (method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown) => Promise<APIResponse>;
type Picked = { id: number; name: string };
type TitleState = { id: number; name: string; kind: string; keep_up_to_date: boolean; pick_rules_id?: number; pick_rules_name?: string };

const movies = [
	{ tmdb: 900451, name: 'Bramble Foundry', year: 2016 },
	{ tmdb: 900452, name: 'Lichen Postcard', year: 2013 }
];
const show = { tmdb: 800451, name: 'Sorrel Junction' };

/** api signs requests with the session's CSRF token, as the app does, and lets Import by hand read .tmp/by-hand. */
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

/** dropTitles takes every title of this kind and name out of the Library, its files too. */
async function dropTitles(call: Call, kind: string, name: string) {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: TitleState[] };
	for (const t of titles.filter((x) => x.kind === kind && x.name === name)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}
}

/** showFolder makes sure a Library folder for shows exists (the shows project adds this same one in a full run). */
async function showFolder(call: Call) {
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { kind: string }[] };
	if (library_folders.some((f) => f.kind === 'show')) return;
	const dir = join(tmp, 'shows');
	mkdirSync(dir, { recursive: true });
	const made = await call('POST', '/library-folders', { path: dir, kind: 'show', label: 'Shows' });
	expect(made.status(), await made.text()).toBe(201);
}

/** freshSet puts the two movies and the show in the Library with no files, kept up to date or not. */
async function freshSet(call: Call, keep: boolean): Promise<Picked[]> {
	const out: Picked[] = [];
	for (const m of movies) {
		await dropTitles(call, 'movie', m.name);
		const added = await call('POST', '/titles', { id: `tmdb:${m.tmdb}`, search_on_add: false, keep_up_to_date: keep, follow: false });
		expect(added.status(), await added.text()).toBe(201);
		out.push({ id: ((await added.json()) as { title: { id: number } }).title.id, name: m.name });
	}
	await showFolder(call);
	await dropTitles(call, 'show', show.name);
	const added = await call('POST', '/shows', { id: `tmdb:${show.tmdb}`, seasons: 'all', search_on_add: false, keep_up_to_date: keep, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	const id = ((await added.json()) as { title: { id: number } }).title.id;
	// The show's episodes are filled in by its first refresh.
	await expect.poll(async () => ((await (await call('GET', `/titles/${id}/episodes`)).json()) as { episodes?: unknown[] }).episodes?.length ?? 0).toBe(2);
	out.push({ id, name: show.name });
	return out;
}

async function titleOf(call: Call, id: number): Promise<TitleState> {
	const r = await call('GET', `/titles/${id}`);
	expect(r.status(), await r.text()).toBe(200);
	return (await r.json()) as TitleState;
}

/** looseFile writes one stand-in video in its own folder under .tmp/by-hand. */
function looseFile(folder: string, name: string): string {
	const dir = join(tmp, 'by-hand', folder);
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	const file = join(dir, name);
	writeFileSync(file, 'stand-in video '.repeat(64));
	return file;
}

/** giveFile imports a loose file into a title (an episode for a show) and returns where it now lives. */
async function giveFile(call: Call, item: Record<string, unknown>): Promise<string> {
	const r = await call('POST', '/manual-import', { items: [{ ...item, mode: 'move' }] });
	expect(r.status(), await r.text()).toBe(200);
	const [res] = ((await r.json()) as { results: { outcome: string; target?: string; error?: string }[] }).results;
	expect(res.outcome, res.error).toBe('placed');
	expect(existsSync(res.target ?? '')).toBe(true);
	return res.target ?? '';
}

/** selectTitles opens the whole Library newest first, turns on Select and ticks each title; it returns the select bar. */
async function selectTitles(page: Page, titles: Picked[]): Promise<Locator> {
	await page.goto('/library?kind=movie&sort=added');
	await page.getByTestId('library-select').click();
	for (const t of titles) await (await inLibrary(page, '', `[data-testid="library-pick"][aria-label="Select ${t.name}"]`)).click();
	const bar = page.getByTestId('select-bar');
	await expect(bar).toContainText(`${titles.length} selected`);
	return bar;
}

test('Keep up to date and Stop keeping: two ticked movies change together', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const all = await freshSet(call, false);
	// The Library shows one kind at a time, so the two movies are ticked together; the show stays as it was.
	const titles = all.slice(0, 2);
	for (const t of titles) expect((await titleOf(call, t.id)).keep_up_to_date, t.name).toBe(false);

	const bar = await selectTitles(page, titles);
	await bar.getByRole('button', { name: 'Keep up to date', exact: true }).click();
	await expect(page.getByRole('status').filter({ hasText: '2 titles kept up to date' })).toBeVisible();
	for (const t of titles) expect((await titleOf(call, t.id)).keep_up_to_date, t.name).toBe(true);

	// The ticks stay, so the same three can be switched back at once.
	await expect(bar).toContainText('2 selected');
	await bar.getByRole('button', { name: 'Stop keeping', exact: true }).click();
	await expect(page.getByRole('status').filter({ hasText: '2 titles not kept up to date' })).toBeVisible();
	for (const t of titles) expect((await titleOf(call, t.id)).keep_up_to_date, t.name).toBe(false);
});

test('Pick rules: two ticked movies move to the chosen rules', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const titles = (await freshSet(call, false)).slice(0, 2);
	// Rules of our own for movies and shows, copied from the ones the first movie got.
	const name = 'Bulk Shelf Rules';
	const { pick_rules } = (await (await call('GET', '/pick-rules')).json()) as { pick_rules: { id: number; name: string }[] };
	let ruleId = pick_rules.find((r) => r.name === name)?.id;
	if (!ruleId) {
		const from = (await titleOf(call, titles[0].id)).pick_rules_id;
		const src = (await (await call('GET', `/pick-rules/${from}`)).json()) as { body: unknown };
		const made = await call('POST', '/pick-rules', { name, kinds: ['movie', 'show'], body: src.body });
		expect(made.status(), await made.text()).toBe(201);
		ruleId = ((await made.json()) as { id: number }).id;
	}
	for (const t of titles) expect((await titleOf(call, t.id)).pick_rules_id, t.name).not.toBe(ruleId);

	const bar = await selectTitles(page, titles);
	await bar.getByRole('button', { name: 'Quality', exact: true }).click();
	const menu = page.getByRole('menu', { name: 'Quality for the selected titles' });
	// Music rules cannot apply to movies or shows, so they are not offered.
	await expect(menu.getByRole('menuitem', { name: 'Standard', exact: true })).toBeVisible();
	await expect(menu.getByRole('menuitem', { name: 'Any music', exact: true })).toHaveCount(0);
	await menu.getByRole('menuitem', { name, exact: true }).click();
	await expect(page.getByRole('status').filter({ hasText: `2 titles set to ${name}` })).toBeVisible();
	for (const t of titles) {
		const now = await titleOf(call, t.id);
		expect(now.pick_rules_id, t.name).toBe(ruleId);
		expect(now.pick_rules_name, t.name).toBe(name);
	}
});

test('Search now: a search starts for each ticked movie, and History shows what it found', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const titles = (await freshSet(call, true)).slice(0, 2);
	const searchKinds = 'search_done,search_failed,release_chosen,release_held';
	for (const t of titles) {
		const { events } = (await (await call('GET', `/events?title=${t.id}&kind=${searchKinds}`)).json()) as { events: unknown[] };
		expect(events, `${t.name} has no search yet`).toEqual([]);
	}

	const bar = await selectTitles(page, titles);
	await bar.getByRole('button', { name: 'Search now', exact: true }).click();
	await expect(page.getByRole('status').filter({ hasText: '2 titles being searched for' })).toBeVisible();

	// Each title got a search by hand that asked the Sources and found Releases.
	type Search = { title_id?: number; by_hand: boolean; state: string; found: number; outcome?: string };
	const searchesOf = async (id: number) => {
		const r = (await (await call('GET', '/searches?limit=50')).json()) as { running: Search[]; recent: Search[] };
		return [...r.running, ...r.recent].filter((s) => s.title_id === id && s.by_hand);
	};
	for (const t of titles) {
		await expect.poll(async () => (await searchesOf(t.id)).filter((s) => s.state === 'done' && s.found > 0).length, { message: t.name, timeout: 30_000 }).toBeGreaterThan(0);
		expect((await searchesOf(t.id)).filter((s) => s.state === 'failed'), t.name).toEqual([]);
	}
	// History names the outcome for every title: a Release picked, held, or none good enough.
	for (const t of titles) {
		const { events } = (await (await call('GET', `/events?title=${t.id}&kind=${searchKinds}`)).json()) as { events: { Kind: string }[] };
		expect(events.length, t.name).toBeGreaterThan(0);
		expect(events.map((e) => e.Kind), t.name).not.toContain('search_failed');
	}
});

test('Remove: two ticked movies leave the Library and their files stay on disk', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const titles = await freshSet(call, false);
	const files: string[] = [];
	for (const [i, m] of movies.entries()) {
		const loose = looseFile(`bulk-${m.tmdb}`, `${m.name.replace(/[^A-Za-z0-9]+/g, '.')}.${m.year}.1080p.WEB-DL.H264-LAB.mkv`);
		files.push(await giveFile(call, { path: loose, title_id: titles[i].id }));
	}
	const episode = looseFile(`bulk-${show.tmdb}`, 'Sorrel.Junction.S01E01.1080p.WEB-DL.H264-LAB.mkv');
	files.push(await giveFile(call, { path: episode, title_id: titles[2].id, season: 1, episode: 1 }));

	// The Library shows one kind at a time: the two movies are removed together, the show stays.
	const gone = titles.slice(0, 2);
	const bar = await selectTitles(page, gone);
	await bar.getByTestId('select-delete').click();
	const dialog = page.getByRole('dialog', { name: 'Remove 2 titles?' });
	await expect(dialog).toContainText('All 2 leave the Library. Files stay on disk.');
	// Nothing changes until the remove is confirmed.
	for (const t of titles) await titleOf(call, t.id);
	await dialog.getByTestId('bulk-delete-confirm').click();
	await expect(page.getByRole('status').filter({ hasText: '2 titles removed' })).toContainText('Files stay on disk');
	await expect(dialog).toBeHidden();
	await expect(bar).toBeHidden();

	// The titles are gone from the Library and the page; every file is still where it was.
	const { titles: left } = (await (await call('GET', '/titles')).json()) as { titles: { id: number }[] };
	expect(left.filter((t) => gone.some((x) => x.id === t.id))).toEqual([]);
	expect(left.some((t) => t.id === titles[2].id), 'the show stays').toBe(true);
	for (const t of gone) await expect(page.locator(`[data-testid="library-row"]`).filter({ hasText: t.name })).toHaveCount(0);
	for (const f of files) expect(existsSync(f), f).toBe(true);
	// Nothing went to the holding folder either.
	const { files: held } = (await (await call('GET', '/holding')).json()) as { files: { title_id: number }[] };
	expect(held.filter((h) => gone.some((t) => t.id === h.title_id))).toEqual([]);

	// Clean up the kept files so the next run starts with empty folders.
	for (const f of files) rmSync(dirname(f), { recursive: true, force: true });
});
