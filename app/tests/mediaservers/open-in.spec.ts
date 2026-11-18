import { expect, test, type APIResponse, type Page } from '@playwright/test';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { jellyKey, jellyfinPort } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { tmp } from '../helpers/data-folder.ts';

// Open in Jellyfin: the owner gives the fake Jellyfin a browser
// address; the Library page of a movie that server has shows an Open in
// Jellyfin button that opens the server's details page in a new tab, at
// the browser address and never at HolyArrs' own address for the server.
// The movie gets a stand-in file first: a missing title shows no button.
// Without a browser address there is no button. Checked at 390 wide.
// Cleans up the server and the movie. Title only this spec uses (fakes/):
// Saffron Lighthouse.

type Call = (method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, data?: unknown) => Promise<APIResponse>;

async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	return (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
}

const name = 'Lab Jellyfin open in';
const jelly = `http://127.0.0.1:${jellyfinPort}`;

test('Open in Jellyfin: a Library movie the server has opens on its details page at the browser address', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const { media_servers } = (await (await call('GET', '/media-servers')).json()) as { media_servers: { id: number; name: string }[] };
	for (const m of media_servers.filter((x) => x.name === name)) expect((await call('DELETE', `/media-servers/${m.id}`)).ok()).toBe(true);
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	for (const t of titles.filter((x) => x.kind === 'movie' && x.name === 'Saffron Lighthouse')) {
		expect((await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true })).ok()).toBe(true);
	}
	const added = await call('POST', '/titles', { id: 'tmdb:900415', search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	const id = ((await added.json()) as { title: { id: number } }).title.id;

	// A stand-in video, imported into the movie by hand from an extra import folder.
	const byHand = join(tmp, 'by-hand');
	const now = (await (await call('GET', '/settings/import-folders')).json()) as { extra_folders?: string[] };
	if (!(now.extra_folders ?? []).includes(byHand)) {
		expect((await call('PUT', '/settings/import-folders', { extra_folders: [...(now.extra_folders ?? []), byHand] })).ok()).toBe(true);
	}
	const dir = join(byHand, 'saffron-lighthouse');
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	const loose = join(dir, 'Saffron.Lighthouse.2016.1080p.WEB-DL.H264-LAB.mkv');
	writeFileSync(loose, 'stand-in video '.repeat(64));
	const imported = await call('POST', '/manual-import', { items: [{ path: loose, title_id: id, mode: 'move' }] });
	expect(imported.status(), await imported.text()).toBe(200);
	const [placed] = ((await imported.json()) as { results: { outcome: string; error?: string }[] }).results;
	expect(placed.outcome, placed.error).toBe('placed');

	const base = { name, kind: 'jellyfin', url: jelly, token: jellyKey, on_import: false, on_rename: false, on_delete: false };
	const made = await call('POST', '/media-servers', base);
	expect(made.status(), await made.text()).toBe(201);
	const serverId = ((await made.json()) as { id: number }).id;

	// No browser address yet: no button.
	await page.goto(`/library/${id}`);
	await expect(page.getByRole('heading', { name: 'Saffron Lighthouse' }).first()).toBeVisible();
	await expect(page.getByTestId('watch-link')).toHaveCount(0);

	// A javascript: address is refused.
	expect((await call('PUT', `/media-servers/${serverId}`, { ...base, token: '', web_url: 'javascript:alert(1)' })).status()).toBe(400);
	const put = await call('PUT', `/media-servers/${serverId}`, { ...base, token: '', web_url: 'http://jellyfin.home.example/' });
	expect(put.ok(), await put.text()).toBe(true);

	await page.goto(`/library/${id}`);
	const link = page.getByTestId('watch-link');
	await expect(link).toHaveCount(1);
	await expect(link).toHaveAccessibleName('Open in Jellyfin');
	await expect(link).toHaveAttribute('href', 'http://jellyfin.home.example/web/#/details?id=jf-saffron&serverId=fake-jellyfin');
	await expect(link).toHaveAttribute('target', '_blank');
	await expect(link).toHaveAttribute('rel', 'noopener noreferrer');

	await page.setViewportSize({ width: 390, height: 844 });
	await expect(link).toBeVisible();
	await noSidewaysScroll(page);

	expect((await call('DELETE', `/media-servers/${serverId}`)).ok()).toBe(true);
	expect((await call('POST', `/titles/${id}/delete`, { delete_files: true, never_add: false, apply: true })).ok()).toBe(true);
});
