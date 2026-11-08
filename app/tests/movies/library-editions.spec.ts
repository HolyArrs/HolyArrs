import { expect, test, type APIResponse, type Page } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { noSidewaysScroll, openLibraryTab, owner, signIn, titleAction } from '../helpers/helpers.ts';
import { plexPort, plexServerToken } from '../fakes/index.mjs';
import { tmp } from '../helpers/data-folder.ts';

// Steps here wait up to 30 s each; under a full chain the default 30 s for the whole test runs out first.
test.describe.configure({ timeout: 90_000 });

const plex = `http://127.0.0.1:${plexPort}`;
const edition = "Director's Cut";
const tag = `{edition-${edition}}`;

// Movie editions: an edition checked on a movie's copy names the file
// with Plex's edition tag, and Plex is asked to scan that movie's folder.

type Call = (method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, data?: unknown) => Promise<APIResponse>;

/** api signs requests with the session's CSRF token, as the app does. */
async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const call: Call = (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
	// Import by hand reads only from known folders: the loose file's folder is an extra import folder.
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

type Copies = {
	files: { ID: number; RelativePath: string; MediaInfo?: Record<string, unknown> }[];
	versions: { id: number; library_folder_id: number }[];
};

const slash = (p: string) => p.replaceAll('\\', '/');

test("Movie editions: a Director's Cut checked on the Library tab renames the file with Plex's edition tag and Plex scans its folder", async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call, 900081, 'Zephyr Projectionist');

	// A stand-in video named like a release, imported into the movie by hand.
	const dir = join(tmp, 'by-hand', 'zephyr-projectionist');
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	const loose = join(dir, 'Zephyr.Projectionist.2009.1080p.WEB-DL.H264-LAB.mkv');
	writeFileSync(loose, 'stand-in video '.repeat(64));
	const imported = await call('POST', '/manual-import', { items: [{ path: loose, title_id: id, mode: 'move' }] });
	expect(imported.status(), await imported.text()).toBe(200);
	const [placed] = ((await imported.json()) as { results: { outcome: string; target?: string; error?: string }[] }).results;
	expect(placed.outcome, placed.error).toBe('placed');
	const before = placed.target ?? '';
	expect(existsSync(before)).toBe(true);
	expect(basename(before)).not.toContain('{edition-');
	const folder = dirname(before);

	// Plex as a media server whose movie library is this Library folder; it
	// is told about renames only, so the scan below comes from the rename.
	let copies = (await (await call('GET', `/titles/${id}/copies`)).json()) as Copies;
	expect(copies.files).toHaveLength(1);
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { id: number; path: string }[] };
	const root = library_folders.find((f) => f.id === copies.versions[0].library_folder_id)?.path ?? '';
	expect(slash(folder).startsWith(`${slash(root)}/`), `${folder} is inside ${root}`).toBe(true);
	const lab = await page.request.put(`${plex}/lab/movie-library`, { data: { path: root } });
	expect(lab.status()).toBe(204);
	const { media_servers } = (await (await call('GET', '/media-servers')).json()) as { media_servers: { id: number; name: string }[] };
	for (const m of media_servers.filter((x) => x.name === 'Lab Plex editions')) expect((await call('DELETE', `/media-servers/${m.id}`)).ok()).toBe(true);
	const made = await call('POST', '/media-servers', { name: 'Lab Plex editions', kind: 'plex', url: plex, token: plexServerToken, on_import: false, on_rename: true, on_delete: false });
	expect(made.status(), await made.text()).toBe(201);
	const serverId = ((await made.json()) as { id: number }).id;

	// The copy's edition, checked by hand. Its box is not in the app (docs/library/audio-versions.md),
	// so the check goes through the API the box used.
	const saved = await call('PUT', `/titles/${id}/files/${copies.files[0].ID}/choices`, { edition });
	expect(saved.ok(), await saved.text()).toBe(true);
	await page.goto(`/library/${id}`);
	await openLibraryTab(page);
	copies = (await (await call('GET', `/titles/${id}/copies`)).json()) as Copies;
	expect(copies.files[0].MediaInfo).toMatchObject({ edition, edition_source: 'confirmed' });
	// Saving the edition alone moves nothing.
	expect(existsSync(before)).toBe(true);

	// Rename files shows the Plex tag in the new name, then renames on disk.
	await titleAction(page, 'More actions', 'Rename files');
	const dialog = page.getByRole('dialog');
	const step = dialog.getByTestId('rename-step');
	await expect(step).toHaveCount(1);
	await expect(step.locator('.old')).toContainText(basename(before));
	await expect(step.locator('.new')).toContainText(tag);
	await dialog.getByRole('button', { name: 'Rename 1 file' }).click();
	// The rename runs as a job; wait for the old file to go (its toast can be stacked behind a newer alert).
	await expect.poll(() => existsSync(before), { timeout: 30_000 }).toBe(false);

	// On disk: one video in the movie's folder, named Title (Year) … {edition-Director's Cut}.mkv.
	const names = readdirSync(folder).filter((n) => n.endsWith('.mkv'));
	expect(names).toHaveLength(1);
	expect(names[0].startsWith('Zephyr Projectionist (2009) '), names[0]).toBe(true);
	expect(names[0].endsWith(` ${tag}.mkv`), names[0]).toBe(true);
	expect(existsSync(before)).toBe(false);
	copies = (await (await call('GET', `/titles/${id}/copies`)).json()) as Copies;
	expect(basename(copies.files[0].RelativePath)).toBe(names[0]);

	// Plex is asked to scan exactly that movie folder in its movie library (sent after a short gathering wait).
	const scans = async () => ((await (await page.request.get(`${plex}/lab/scans`)).json()) as { section: string; path: string }[]).filter((s) => slash(s.path) === slash(folder));
	await expect.poll(async () => (await scans()).map((s) => s.section), { timeout: 30_000 }).toEqual(['7']);

	expect((await call('DELETE', `/media-servers/${serverId}`)).ok()).toBe(true);

	// The Library tab with the tagged name still fits a phone.
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(`/library/${id}`);
	await openLibraryTab(page);
	await expect(page.getByTestId('version-card').first()).toContainText(tag);
	await noSidewaysScroll(page);
});
