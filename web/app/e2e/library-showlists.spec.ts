import { expect, test, type APIResponse, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { owner, signIn } from './helpers.ts';
import { traktKey } from './fakes.mjs';

const tmp = join(dirname(fileURLToPath(import.meta.url)), '.tmp');

// #104 Auto-add shows from lists: an Outside list that adds shows, against
// the fake TMDB list 104 and the fake Trakt list lab/tv-picks in fakes.mjs.
// Each list keeps its own season choice; Never-add shows are skipped; a list
// with Search when added looks for the new show's episodes. The lists use
// the 4K Pick rules, so the fake Source's Releases are all turned away and
// nothing downloads. Everything made here is taken away again at the end.
test.describe.configure({ mode: 'serial' });

type Call = (method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown) => Promise<APIResponse>;

async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	return (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
}

type TitleRow = { id: number; name: string; kind: string; why_added: string; show?: { seasons: { number: number; keep_up_to_date: boolean }[] } };

async function show(call: Call, name: string): Promise<TitleRow | undefined> {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: TitleRow[] };
	const t = titles.find((x) => x.kind === 'show' && x.name === name);
	return t && ((await (await call('GET', `/titles/${t.id}`)).json()) as TitleRow);
}

/** kept says which seasons of a show are kept up to date, as "1:on 2:off". */
const kept = (t: TitleRow | undefined) => (t?.show?.seasons ?? []).map((s) => `${s.number}:${s.keep_up_to_date ? 'on' : 'off'}`).join(' ');

let page: Page;
let call: Call;

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
	}
});

test.afterAll(async () => {
	for (const name of ['Bellwether Ferry', 'Rookery Dial']) {
		const t = await show(call, name);
		if (t) await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
	}
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
	await form.getByLabel('Pick rules').selectOption({ label: '4K' });
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
	type Search = { title_id?: number; kind: string };
	await expect
		.poll(
			async () => {
				const { running, recent } = (await (await call('GET', '/searches?limit=100')).json()) as { running: Search[]; recent: Search[] };
				return [...running, ...recent].filter((s) => s.title_id === ferry?.id).length;
			},
			{ timeout: 30_000 }
		)
		.toBeGreaterThan(0);

	await page.goto('/library');
	const item = (name: string) => page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: name });
	await expect(item('Bellwether Ferry').first()).toBeVisible();
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

	for (const name of ['Lab shows', 'Trakt picks']) {
		page.once('dialog', (d) => d.accept());
		await page.getByTestId('outside-list').filter({ hasText: name }).getByRole('button', { name: 'Remove' }).click();
		await expect(page.getByTestId('outside-list').filter({ hasText: name })).toHaveCount(0);
	}
	// The shows the lists added stay in the Library.
	expect(await show(call, 'Bellwether Ferry')).toBeTruthy();

	await page.getByRole('button', { name: 'Take off Cinder Lane' }).click();
	await expect(page.getByTestId('never-add').filter({ hasText: 'Cinder Lane' })).toHaveCount(0);
});
