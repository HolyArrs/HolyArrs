import { expect, test } from '@playwright/test';
import { existsSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { episodeAction, episodeState, openFromSearch, openSeasonNamed, owner, signIn } from '../helpers/helpers.ts';
import { animeReleaseHash, qbitPort } from '../fakes/index.mjs';
import { api, type Call } from '../helpers/api.ts';
import { tmp } from '../helpers/data-folder.ts';

// Steps here wait up to 30 s each; under a full chain the default 30 s for the whole test runs out first.
test.describe.configure({ timeout: 90_000 });

// Anime episode matching. Hoshikage Relay (fakes/) has 12 episodes
// in season 1 and 2 in season 2; the fake Source names its Releases by
// running number only ("[Lab Fansub] Hoshikage Relay - 13 [1080p]"). Added
// with the running-number episode order, running number 13 must be matched
// to S02E01, the Release for 14 to S02E02, and the file of 13 must land on
// S02E01 in the Library.

const qbit = `http://127.0.0.1:${qbitPort}`;
const show = 'Hoshikage Relay';
const want = `[Lab Fansub] ${show} - 13 [1080p]`;
// The fake qBittorrent takes any key.
const fakeClientKey = ['stand', 'in', 'qbit'].join('-');

type Episode = { id: number; season: number; episode: number; absolute?: number; slots: { state: string; file_id?: number; file?: { RelativePath?: string; Quality?: { resolution?: number; source?: string } } }[] };
type Release = { name: string; episode?: string; passed: boolean; reasons: { effect: string; text: string }[] };

/** showFolder returns the Library folder for shows, adding one under .tmp when the earlier projects made none. */
async function showFolder(call: Call): Promise<string> {
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { path: string; kind: string }[] };
	const have = library_folders.find((f) => f.kind === 'show');
	if (have) return have.path;
	const path = join(tmp, 'shows');
	mkdirSync(path, { recursive: true });
	const made = await call('POST', '/library-folders', { path, kind: 'show', label: 'Shows' });
	expect(made.status(), await made.text()).toBe(201);
	return path;
}

/** fakeQbit makes sure the fake qBittorrent is a Downloader (downloaders.spec adds it in a full run). */
async function fakeQbit(call: Call) {
	const { downloaders } = (await (await call('GET', '/downloaders')).json()) as { downloaders: { kind: string; url: string }[] };
	if (downloaders.some((d) => d.kind === 'qbittorrent' && d.url.startsWith(qbit))) return;
	const made = await call('POST', '/downloaders', { name: 'Lab qBittorrent', kind: 'qbittorrent', url: qbit, secret: fakeClientKey, allow_local_network: true });
	expect(made.status(), await made.text()).toBe(201);
}

async function episodes(call: Call, id: number): Promise<Episode[]> {
	const r = await call('GET', `/titles/${id}/episodes?files=1`);
	expect(r.ok(), await r.text()).toBe(true);
	return ((await r.json()) as { episodes: Episode[] }).episodes;
}
const one = (eps: Episode[], season: number, episode: number) => eps.find((e) => e.season === season && e.episode === episode);

/** videosUnder lists the .mkv files anywhere under dir, with forward slashes. */
function videosUnder(dir: string): string[] {
	if (!existsSync(dir)) return [];
	return readdirSync(dir, { recursive: true, encoding: 'utf8' }).filter((p) => p.endsWith('.mkv') && !p.includes('.holyarrs-')).map((p) => p.replaceAll('\\', '/'));
}

test('Anime: a Release named by running number 13 is matched to S02E01, picked, and its file imported into S02E01', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const shows = await showFolder(call);
	await fakeQbit(call);
	// A rerun starts from no show.
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	for (const t of titles.filter((x) => x.kind === 'show' && x.name === show)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}

	// Add it from Search with every season kept and the running-number order.
	await openFromSearch(page, 'hoshikage', show, 'show');
	await page.getByRole('button', { name: `Add ${show}` }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Seasons to keep up to date').selectOption('all');
	await dialog.getByLabel('Episode order').selectOption('absolute');
	await expect(dialog).toContainText('For anime: episode 27, counted from the first.');
	const searchOnAdd = dialog.getByRole('checkbox', { name: /^Search when added/ });
	if (await searchOnAdd.isChecked()) await searchOnAdd.uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(dialog).toBeHidden();
	await expect(page.getByRole('status')).toContainText(`${show} added`);
	await expect(page).toHaveURL(/\/library\/\d+/);
	const id = Number(/\/library\/(\d+)/.exec(page.url())![1]);

	// Running numbers are counted over both seasons: S01E12 is 12, S02E01 is 13.
	let eps = await episodes(call, id);
	expect(one(eps, 1, 12)?.absolute).toBe(12);
	expect(one(eps, 2, 1)?.absolute).toBe(13);
	expect(one(eps, 2, 2)?.absolute).toBe(14);
	const s2e1 = one(eps, 2, 1)!;

	// Choose a release for S02E01: the Source answers by running number only.
	await episodeAction(page, 'S02E01', 'Choose a release');
	const sheet = page.getByRole('dialog', { name: 'Choose a release' });
	await expect(sheet.getByTestId('release-sheet')).toBeVisible();
	await expect(sheet).toContainText('S2 E1');
	await sheet.getByRole('button', { name: 'Search now' }).click();
	await sheet.getByRole('tab', { name: /^All/ }).click();
	await expect(sheet.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	const row = (name: string) => sheet.getByTestId('release').filter({ has: page.locator('.rs-name', { hasText: name }) });
	// 13 in 1080p is the one to take; 14 is S02E02, which was not asked for.
	await expect(row(want).locator('.rs-rec')).toHaveText('Recommended');
	await expect(row(`${show} - 14 [1080p]`).locator('.rs-why.fail')).toContainText('holds S02E02, not what was searched for');

	// The stored verdicts say the same: 13 and its 720p copy are for S02E01.
	const listed = await call('GET', `/titles/${id}/releases?item_id=${s2e1.id}&limit=60&offset=0`);
	const { releases } = (await listed.json()) as { releases: Release[] };
	const byName = (n: string) => releases.find((r) => r.name === n);
	for (const n of [want, `[Lab Fansub] ${show} - 13 [720p]`]) {
		expect(byName(n)?.passed, n).toBe(true);
		expect(byName(n)?.episode, n).toContain('S02E01');
		expect(byName(n)?.reasons.map((r) => r.text), n).toContain('holds S02E01');
	}
	expect(byName(`[Lab Fansub] ${show} - 14 [1080p]`)?.passed).toBe(false);

	// Pick it: the fake qBittorrent gets exactly that Release.
	await row(want).getByRole('button', { name: 'Pick', exact: true }).click();
	await expect(sheet).toBeHidden();
	await expect(page.getByRole('status')).toContainText("Sent to Downloader");
	const hash = animeReleaseHash(want);
	await expect.poll(async () => ((await (await fetch(`${qbit}/state`)).json()) as { added: string[] }).added, { timeout: 20_000 }).toContain(hash);

	// The download finishes with the fansub file in its folder; HolyArrs imports it into S02E01.
	const dl = join(tmp, 'downloads');
	rmSync(join(dl, want), { recursive: true, force: true });
	mkdirSync(join(dl, want), { recursive: true });
	writeFileSync(join(dl, want, `${want}.mkv`), 'stand-in episode '.repeat(64));
	const finished = await fetch(`${qbit}/set?hash=${hash}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(`${want}/${want}.mkv`)}`);
	expect(finished.ok).toBe(true);
	await expect.poll(async () => one(await episodes(call, id), 2, 1)?.slots[0]?.file_id ?? 0, { timeout: 30_000 }).toBeGreaterThan(0);
	eps = await episodes(call, id);
	const file = one(eps, 2, 1)!.slots[0].file!;
	expect(file.Quality?.resolution).toBe(1080);
	// Nothing landed on the neighbours.
	expect(one(eps, 2, 2)?.slots[0]?.file_id ?? 0).toBe(0);
	expect(one(eps, 1, 12)?.slots[0]?.file_id ?? 0).toBe(0);

	// On disk: one file for the show, named for S02E01 and running number 013.
	const placed = videosUnder(shows).filter((p) => p.includes(show));
	expect(placed).toHaveLength(1);
	expect(placed[0]).toMatch(/S02E01 - 013\b/);
	expect(placed[0].endsWith(file.RelativePath ?? '?')).toBe(true);

	// The Library tab shows S02E01 in the Library, S02E02 still missing.
	await page.reload();
	await expect(await episodeState(page, 'S02E01')).toHaveText('1080p WEB');
	await expect(await episodeState(page, 'S02E02')).toHaveText('Missing');
	await expect((await openSeasonNamed(page, 'Season 2')).locator('.lib-count')).toHaveText('1 / 2');
});
