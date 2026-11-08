import { expect, test, type Browser, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ensureInLibrary, noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { fresh } from './support.ts';
import { api, type Call } from '../helpers/api.ts';

// A member follows a Library artist at phone width (Pixel 7). Its own project,
// after the music ones (playwright.config.ts): it needs a music Library folder.
test.describe.configure({ mode: 'serial' });

let own: Page;

test.beforeAll(async ({ browser }) => {
	// Run on setup alone, Quiet Orchard (added by movies) is not there yet.
	await ensureInLibrary(browser, 'imdb:tt9000030', 'Quiet Orchard');
	own = await fresh(browser);
	await signIn(own, owner());
});

const slow = { timeout: 45_000 };

/** member makes an invite link as the owner and joins with it on a phone. */
async function member(browser: Browser, name: string): Promise<Page> {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill(name);
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const p = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-${name}`;
	await p.goto(new URL(link).pathname);
	await p.getByLabel('Name', { exact: true }).fill(name);
	await p.getByLabel('Password', { exact: true }).fill(pw);
	await p.getByLabel('Confirm password').fill(pw);
	await p.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(p).toHaveURL(/\/$/);
	return p;
}

// Quartz Gulls in the fake MusicBrainz (fakes/): only the artist follow test uses it.
const gulls = 'a6666666-0000-4000-8000-000000000006';

/** musicFolder is a music Library folder's id: music.spec's when it ran first, else one of its own under .tmp. */
async function musicFolder(call: Call): Promise<number> {
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { id: number; kind: string }[] };
	const have = library_folders.find((f) => f.kind === 'music');
	if (have) return have.id;
	const path = join(dirname(fileURLToPath(import.meta.url)), '..', '.tmp', 'requests-music');
	mkdirSync(path, { recursive: true });
	const made = await call('POST', '/library-folders', { path, kind: 'music', label: 'Requests music' });
	expect(made.status(), await made.text()).toBe(201);
	return ((await made.json()) as { id: number }).id;
}

// In the full run this test is its own project after the music ones (playwright.config.ts), so
// it uses music.spec's folder and never adds a second one before music.spec counts its folders.
test('a member follows a Library artist: Following stays after a reload and nothing is asked for', async ({ browser }) => {
	test.setTimeout(120_000);
	const ownCall = await api(own);
	const { titles } = (await (await ownCall('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string; state?: string }[] };
	if (!titles.some((t) => t.kind === 'artist' && t.name === 'Quartz Gulls' && t.state !== 'removed')) {
		// No albums kept up to date: adding it searches for nothing.
		const added = await ownCall('POST', '/artists', { id: `mbid:${gulls}`, library_folder_id: await musicFolder(ownCall), albums: 'none', keep_up_to_date: false, follow: false });
		expect(added.status(), await added.text()).toBe(201);
	}
	const asked = async () => {
		const out: string[] = [];
		for (const state of ['asked', 'approved']) out.push(...((await (await ownCall('GET', `/asks?state=${state}`)).json()) as { asks: { name: string }[] }).asks.map((a) => a.name));
		return out.filter((n) => n === 'Quartz Gulls').length;
	};
	const ada = await member(browser, 'ada');
	await ada.goto(`/music/artist/${gulls}`);
	await expect(ada.getByRole('heading', { level: 1, name: 'Quartz Gulls' })).toBeVisible(slow);
	const bell = ada.getByRole('button', { name: /^Follow(ing)?$/ });
	await expect(bell).toHaveText('Follow', slow);
	await expect(bell).toHaveAttribute('aria-pressed', 'false');
	await bell.click();
	await expect(bell).toHaveText('Following');
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await ada.reload();
	await expect(ada.getByRole('heading', { level: 1, name: 'Quartz Gulls' })).toBeVisible(slow);
	await expect(bell).toHaveText('Following', slow);
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await noSidewaysScroll(ada);
	// Following asks for nothing.
	expect(await asked()).toBe(0);
	await ada.goto('/requests');
	await expect(ada.getByText('Nothing on the Requests page yet')).toBeVisible();
});
