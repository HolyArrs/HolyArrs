import { expect, test, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { owner, signIn, inLibrary } from '../helpers/helpers.ts';
import { traktKey } from '../fakes/index.mjs';
import { api, type Call } from '../helpers/api.ts';
import { tmp } from '../helpers/data-folder.ts';

// Steps here wait up to 30 s each; under a full chain the default 30 s for the whole test runs out first.
test.describe.configure({ timeout: 90_000 });

// Auto-add shows from lists: an Outside list that adds shows, against
// the fake TMDB list 104 and the fake Trakt list lab/tv-picks in fakes/.
// Each list keeps its own season choice; Never-add shows are skipped; a list
// with Search when added looks for the new show's episodes. The lists use
// the 4K Quality rules, so the fake Source's Releases are all turned away and
// nothing downloads. Everything made here (lists, shows, the Never-add
// entry, a show folder it had to make) is taken away in afterAll through the
// API, so a failed test leaves nothing behind for the specs after it.
test.describe.configure({ mode: 'serial' });

type TitleRow = { id: number; name: string; kind: string; why_added: string; show?: { seasons: { number: number; keep_up_to_date: boolean }[] } };

async function show(call: Call, name: string): Promise<TitleRow | undefined> {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: TitleRow[] };
	const t = titles.find((x) => x.kind === 'show' && x.name === name);
	return t && ((await (await call('GET', `/titles/${t.id}`)).json()) as TitleRow);
}

/** searchesFor counts the searches (queued, running or done) for one title. */
async function searchesFor(call: Call, titleId: number | undefined): Promise<number> {
	type Search = { title_id?: number };
	const { running, recent } = (await (await call('GET', '/searches?limit=100')).json()) as { running: Search[]; recent: Search[] };
	return [...running, ...recent].filter((s) => s.title_id === titleId).length;
}

/** noSearchFor checks for a few seconds that no search starts for a title. */
async function noSearchFor(titleId: number | undefined) {
	for (let i = 0; i < 5; i++) {
		expect(await searchesFor(call, titleId), 'no search without Search when added').toBe(0);
		await page.waitForTimeout(1000);
	}
}

/** removeShow takes a show out of the Library, trying again while its scan or search still runs. */
async function removeShow(call: Call, name: string) {
	const t = await show(call, name);
	if (!t) return;
	await expect
		.poll(async () => (await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true })).ok(), { timeout: 30_000 })
		.toBe(true);
}

/** kept says which seasons of a show are kept up to date, as "1:on 2:off". */
const kept = (t: TitleRow | undefined) => (t?.show?.seasons ?? []).map((s) => `${s.number}:${s.keep_up_to_date ? 'on' : 'off'}`).join(' ');

let page: Page;
let call: Call;
// A show folder this spec made, taken away again at the end.
let madeFolder = 0;
const listNames = ['Lab shows', 'Trakt picks', 'Nothing kept'];

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
	call = await api(page);
	// The show tests may not have run: make a show folder when there is none.
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { kind: string }[] };
	if (!library_folders.some((f) => f.kind === 'show')) {
		mkdirSync(join(tmp, 'shows'), { recursive: true });
		const made = await call('POST', '/library-folders', { path: join(tmp, 'shows'), kind: 'show', label: 'Shows' });
		expect(made.status(), await made.text()).toBe(201);
		madeFolder = ((await made.json()) as { id: number }).id;
	}
});

test.afterAll(async () => {
	const { outside_lists } = (await (await call('GET', '/outside-lists')).json()) as { outside_lists: { id: number; name: string }[] };
	for (const l of outside_lists.filter((x) => listNames.includes(x.name))) await call('DELETE', `/outside-lists/${l.id}`);
	const { never_add } = (await (await call('GET', '/never-add')).json()) as { never_add: { id: number; name: string }[] };
	for (const n of never_add.filter((x) => x.name === 'Cinder Lane')) await call('DELETE', `/never-add/${n.id}`);
	for (const name of ['Bellwether Ferry', 'Rookery Dial']) await removeShow(call, name).catch(() => {});
	if (madeFolder) await call('DELETE', `/library-folders/${madeFolder}`);
	await page.close();
});

/** addList fills the Add a list form for a show list and saves it. */
async function addList(name: string, kind: 'tmdb_list' | 'trakt_list', ref: string, seasons: string, searchOnAdd: boolean) {
	await page.getByRole('button', { name: 'Add a list' }).first().click();
	const form = page.locator('form.sc-form');
	await form.getByLabel('Name', { exact: true }).fill(name);
	await form.getByLabel('Kind').selectOption(kind);
	await form.getByLabel('Adds').selectOption('show');
	// Show lists offer show folders and the season choices; movie lists do not.
	await expect(form.getByLabel('Seasons to keep up to date')).toBeVisible();
	await form.getByLabel('Which list').fill(ref);
	await form.getByLabel('Quality').selectOption({ label: '4K' });
	await form.getByLabel('Seasons to keep up to date').selectOption(seasons);
	if (searchOnAdd) await form.getByRole('checkbox', { name: /^Search when added/ }).check();
	await form.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(form).toBeHidden();
}

test('a TMDB show list adds its shows with the first season kept, skips the Never-add list and looks for episodes', async () => {
	await page.goto('/settings/outside-lists');
	await expect(page.getByRole('heading', { level: 1, name: 'Outside lists' })).toBeVisible();

	// Cinder Lane must never be added.
	await page.getByLabel('Movie or show').selectOption('show');
	await page.getByLabel('TMDB number').fill('800105');
	await page.getByLabel('Name', { exact: true }).fill('Cinder Lane');
	await page.getByRole('button', { name: 'Add to the Never-add list' }).click();
	await expect(page.getByTestId('never-add').filter({ hasText: 'Cinder Lane' })).toContainText('show · tmdb:800105');

	await addList('Lab shows', 'tmdb_list', 'https://www.themoviedb.org/list/104', 'first', true);
	const row = page.getByTestId('outside-list').filter({ hasText: 'Lab shows' });
	await expect(row).toContainText('Shows');
	// Two shows on the list (its movie is left out), one added, one on the Never-add list.
	await expect(row.getByTestId('list-result')).toContainText('2 on the list, 1 added', { timeout: 20_000 });
	await expect(row.getByTestId('list-result')).toContainText('1 on the Never-add list');

	const ferry = await show(call, 'Bellwether Ferry');
	expect(ferry?.why_added).toBe('list');
	expect(kept(ferry)).toBe('1:on 2:off 3:off');
	expect(await show(call, 'Cinder Lane')).toBeUndefined();

	// Search when added: once its folder is scanned, season 1 is looked for at the Source.
	await expect.poll(async () => searchesFor(call, ferry?.id), { timeout: 30_000 }).toBeGreaterThan(0);

	await page.goto('/library?kind=show');
	const item = (name: string) => page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: name });
	await expect(await inLibrary(page, 'Bellwether Ferry')).toBeVisible();
	await expect(item('Cinder Lane')).toHaveCount(0);
});

test('a Trakt show list adds a show by its TVDB id with the latest season kept, and skips one already there', async () => {
	// The Trakt key is set by the Outside lists tests; set it here when they have not run.
	const { key } = (await (await call('GET', '/settings/trakt-key')).json()) as { key: string };
	if (!key) expect((await call('PUT', '/settings/trakt-key', { key: traktKey })).status()).toBe(200);

	await page.goto('/settings/outside-lists');
	await addList('Trakt picks', 'trakt_list', 'https://trakt.tv/users/lab/lists/tv-picks', 'latest', false);
	const row = page.getByTestId('outside-list').filter({ hasText: 'Trakt picks' });
	await expect(row.getByTestId('list-result')).toContainText('2 on the list, 1 added, 1 already in the Library', { timeout: 20_000 });

	const rookery = await show(call, 'Rookery Dial');
	expect(rookery?.why_added).toBe('list');
	expect(kept(rookery)).toBe('1:off 2:on');
	// Search when added is off: the new show's folder is scanned, nothing is looked for.
	await noSearchFor(rookery?.id);
});

test('a show list with no seasons kept adds the show with every season off', async () => {
	// Take the show the first list added out again, so this list adds it anew.
	await removeShow(call, 'Bellwether Ferry');
	await page.goto('/settings/outside-lists');
	await addList('Nothing kept', 'tmdb_list', '104', 'none', false);
	const row = page.getByTestId('outside-list').filter({ hasText: 'Nothing kept' });
	await expect(row.getByTestId('list-result')).toContainText('2 on the list, 1 added', { timeout: 20_000 });
	await expect(row.getByTestId('list-result')).toContainText('1 on the Never-add list');
	const ferry = await show(call, 'Bellwether Ferry');
	expect(kept(ferry)).toBe('1:off 2:off 3:off');
	await noSearchFor(ferry?.id);
});

test('change a show list, then remove both lists and take the show off the Never-add list', async () => {
	await page.goto('/settings/outside-lists');
	const row = page.getByTestId('outside-list').filter({ hasText: 'Lab shows' });
	await row.getByRole('button', { name: 'Change' }).click();
	const form = page.locator('form.sc-form');
	// The form opens with the list's own choices.
	await expect(form.getByLabel('Adds')).toHaveValue('show');
	await expect(form.getByLabel('Seasons to keep up to date')).toHaveValue('first');
	await form.getByLabel('Seasons to keep up to date').selectOption('future');
	await expect(form.getByLabel('New seasons')).toBeDisabled();
	await form.getByRole('checkbox', { name: /^Read this list/ }).uncheck();
	await form.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(row).toContainText('Off');
	const { outside_lists } = (await (await call('GET', '/outside-lists')).json()) as { outside_lists: { name: string; seasons: string; new_seasons: string }[] };
	expect(outside_lists.find((l) => l.name === 'Lab shows')).toMatchObject({ seasons: 'future', new_seasons: 'all' });

	for (const name of listNames) {
		page.once('dialog', (d) => d.accept());
		await page.getByTestId('outside-list').filter({ hasText: name }).getByRole('button', { name: 'Remove' }).click();
		await expect(page.getByTestId('outside-list').filter({ hasText: name })).toHaveCount(0);
	}
	// The shows the lists added stay in the Library.
	expect(await show(call, 'Bellwether Ferry')).toBeTruthy();

	await page.getByRole('button', { name: 'Take off Cinder Lane' }).click();
	await expect(page.getByTestId('never-add').filter({ hasText: 'Cinder Lane' })).toHaveCount(0);
});
