import { expect, test, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { openLibraryTab, owner, signIn } from '../helpers/helpers.ts';
import { api, type Call } from '../helpers/api.ts';
import { tmp } from '../helpers/data-folder.ts';

// Library folders: a show and an artist move to a second Library folder
// of their kind later on, from Edit on their Library page, files and all
// (movies already move; library-tools.spec.ts). Each test sets up its own
// folders under .tmp and adopts a fake title that only these tests use.

type Folder = { id: number; path: string; kind: string; label: string };

async function folders(call: Call): Promise<Folder[]> {
	return ((await (await call('GET', '/library-folders')).json()) as { library_folders: Folder[] }).library_folders;
}

/** firstFolder is the Library folder at path, added through the API when it is not there yet. */
async function firstFolder(call: Call, path: string, kind: 'show' | 'music', label: string): Promise<number> {
	mkdirSync(path, { recursive: true });
	const had = (await folders(call)).find((f) => f.path === path);
	if (had) return had.id;
	const made = await call('POST', '/library-folders', { path, kind, label });
	expect(made.status(), await made.text()).toBe(201);
	return ((await made.json()) as Folder).id;
}

/** addFolderInSettings adds a second Library folder from Settings > Library folders, as the owner does. */
async function addFolderInSettings(page: Page, call: Call, path: string, kind: 'show' | 'music', label: string): Promise<number> {
	mkdirSync(path, { recursive: true });
	await page.goto('/settings/library-folders');
	await page.getByRole('button', { name: 'Add a folder' }).first().click();
	await page.getByLabel('Holds').selectOption(kind);
	await page.getByLabel('Folder path').fill(path);
	await page.getByLabel(/^Name/).fill(label);
	await page.getByRole('button', { name: 'Add', exact: true }).click();
	await expect(page.getByRole('listitem').filter({ has: page.locator('.name', { hasText: label }) }).getByTestId('folder-kind')).toHaveText(kind === 'show' ? 'For shows' : 'For music');
	const f = (await folders(call)).find((x) => x.path === path);
	expect(f?.kind).toBe(kind);
	return f?.id ?? 0;
}

/** adopt takes a folder already in a Library folder into the Library as the given show or artist and returns its Title id. */
async function adopt(call: Call, folderId: number, folder: string, id: string): Promise<number> {
	const r = await call('POST', `/library-folders/${folderId}/adopt`, { folders: [{ folder, id }], follow: false });
	expect(r.status(), await r.text()).toBe(202);
	const job = ((await r.json()) as { job_id: number }).job_id;
	type Job = { State: string; Result?: { results?: { title_id?: number; error?: string }[] }; Error?: unknown };
	let j: Job = { State: 'queued' };
	await expect.poll(async () => (j = (await (await call('GET', `/jobs/${job}`)).json()) as Job).State, { timeout: 20_000 }).toMatch(/^(done|failed)$/);
	const res = j.Result?.results?.[0];
	expect(j.State, JSON.stringify(j.Error)).toBe('done');
	expect(res?.error ?? '').toBe('');
	return res?.title_id ?? 0;
}

/** filesUnder lists the video and audio files anywhere under dir, with forward slashes, sorted. */
function filesUnder(dir: string): string[] {
	if (!existsSync(dir)) return [];
	return readdirSync(dir, { recursive: true, encoding: 'utf8' })
		.filter((p) => /\.(mkv|mp3)$/.test(p) && !p.includes('.holyarrs-'))
		.map((p) => p.replaceAll('\\', '/'))
		.sort();
}

/** writeFiles writes stand-in media files at the given paths under dir. */
function writeFiles(dir: string, rels: string[]) {
	rmSync(dir, { recursive: true, force: true });
	for (const rel of rels) {
		const p = join(dir, rel);
		mkdirSync(dirname(p), { recursive: true });
		writeFileSync(p, `stand-in ${rel} `.repeat(32));
	}
}

test('Move a show with its episode files to a second show folder from the Folder row on its Library tab', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const one = join(tmp, 'lf-shows-one');
	const two = join(tmp, 'lf-shows-two');
	const name = 'Saltmarsh Signal (2020)';
	const episodes = [
		'Season 1/Saltmarsh.Signal.S01E01.1080p.WEB-DL.H264-HOLYARRS.mkv',
		'Season 1/Saltmarsh.Signal.S01E02.1080p.WEB-DL.H264-HOLYARRS.mkv',
		'Season 2/Saltmarsh.Signal.S02E01.1080p.WEB-DL.H264-HOLYARRS.mkv'
	];
	const fromId = await firstFolder(call, one, 'show', 'Show shelf one');
	writeFiles(join(one, name), episodes);
	const id = await adopt(call, fromId, name, 'tmdb:800008');
	const toId = await addFolderInSettings(page, call, two, 'show', 'Show shelf two');

	await page.goto(`/library/${id}`);
	await openLibraryTab(page);
	// The Folder row shows the folder's name (only when there is another folder to move to).
	const folderRow = page.getByTestId('folder-row');
	await expect(folderRow).toContainText('Show shelf one');
	await folderRow.getByRole('button').click();
	await page.getByRole('menuitemradio', { name: 'Show shelf two' }).or(page.getByRole('menuitem', { name: 'Show shelf two' })).click();
	const confirm = page.getByRole('dialog', { name: 'Move to Show shelf two?' });
	await expect(confirm).toContainText(two);
	await confirm.getByRole('button', { name: 'Move files' }).click();
	await expect(page.getByText('Moving files to Show shelf two')).toBeVisible();

	// The move runs as a Job: every episode file lands in the second folder
	// in its season folder, and the old show folder is gone.
	await expect.poll(() => filesUnder(join(two, name)), { timeout: 20_000 }).toEqual(episodes);
	expect(existsSync(join(one, name)), 'the old show folder is gone').toBe(false);
	await expect.poll(async () => ((await (await call('GET', `/titles/${id}`)).json()) as { library_folder_id: number }).library_folder_id).toBe(toId);
	const t = (await (await call('GET', `/titles/${id}`)).json()) as { folder_path: string };
	expect(t.folder_path).toBe(join(two, name));

	// The show's Library tab and the Library both show the new folder.
	await page.reload();
	await openLibraryTab(page);
	await expect(page.getByTestId('folder-row')).toContainText('Show shelf two');
});

test('Move an artist with its album files to a second music folder from the Folder row on its Library page', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const one = join(tmp, 'lf-music-one');
	const two = join(tmp, 'lf-music-two');
	const name = 'Slate Herons';
	const tracks = ['Glass Estuary (2017)/01 - Song 1.1.mp3', 'Glass Estuary (2017)/02 - Song 1.2.mp3', 'Kelp Radio (2020)/01 - Song 2.1.mp3'];
	const fromId = await firstFolder(call, one, 'music', 'Music shelf one');
	writeFiles(join(one, name), tracks);
	const id = await adopt(call, fromId, name, 'mbid:a5555555-0000-4000-8000-000000000005');
	const toId = await addFolderInSettings(page, call, two, 'music', 'Music shelf two');

	const libraryTab = page.getByTestId('artist-tabs').getByRole('tab', { name: 'Library' });
	await page.goto(`/library/${id}`);
	await libraryTab.click();
	const folderRow = page.getByTestId('folder-row');
	await expect(folderRow).toContainText('Music shelf one');
	await folderRow.getByRole('button').click();
	await page.getByRole('menuitemradio', { name: 'Music shelf two' }).or(page.getByRole('menuitem', { name: 'Music shelf two' })).click();
	const confirm = page.getByRole('dialog', { name: 'Move to Music shelf two?' });
	await expect(confirm).toContainText(two);
	await confirm.getByRole('button', { name: 'Move files' }).click();
	await expect(page.getByText('Moving files to Music shelf two')).toBeVisible();

	// Every track lands in the second folder in its album folder, and the old artist folder is gone.
	await expect.poll(() => filesUnder(join(two, name)), { timeout: 20_000 }).toEqual(tracks);
	expect(existsSync(join(one, name)), 'the old artist folder is gone').toBe(false);
	await expect.poll(async () => ((await (await call('GET', `/titles/${id}`)).json()) as { library_folder_id: number }).library_folder_id).toBe(toId);
	const t = (await (await call('GET', `/titles/${id}`)).json()) as { folder_path: string };
	expect(t.folder_path).toBe(join(two, name));

	// The artist's Folder row and the Library both show the new folder.
	await page.reload();
	await libraryTab.click();
	await expect(page.getByTestId('folder-row')).toContainText('Music shelf two');
});
