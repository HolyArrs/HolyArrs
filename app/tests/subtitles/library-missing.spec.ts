import { expect, test } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { libraryStatus, owner, signIn } from '../helpers/helpers.ts';
import { api, type Call } from '../helpers/api.ts';
import { tmp } from '../helpers/data-folder.ts';

// Missing list: Search all missing (top of the Library's Missing list,
// after a confirm) searches every missing title of the kind shown at once:
// first movies only, then everything. The fake Source answers
// the searches; the titles here use the 4K Pick rules, so every Release it
// finds is turned away and nothing is downloaded.

type TitleRow = {
	id: number;
	name: string;
	kind: string;
	versions: { id: number; keep_up_to_date: boolean }[];
	slots: { version_id: number; state: string; active_download_id?: number }[];
	show?: { missing: number };
	artist?: { missing: number };
};

async function titles(call: Call): Promise<TitleRow[]> {
	return ((await (await call('GET', '/titles')).json()) as { titles: TitleRow[] }).titles;
}

/** removeNamed takes earlier copies of a title out of the Library. */
async function removeNamed(call: Call, kind: string, name: string) {
	for (const t of (await titles(call)).filter((x) => x.kind === kind && x.name === name)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}
}

/**
 * pauseOthers stops keeping every other Version that still has something
 * missing up to date, so the bulk search covers only this test's titles and
 * grabs nothing for titles other tests own. Each Version it turned off goes
 * into paused as soon as it is off, so the finally block can turn it back on
 * even when a later one fails; failures are collected, not thrown.
 */
async function pauseOthers(call: Call, mine: number[], paused: [number, number][], failed: string[]) {
	for (const t of await titles(call)) {
		if (mine.includes(t.id)) continue;
		const wanted = new Set(t.slots.filter((s) => s.state === 'wanted' && !s.active_download_id).map((s) => s.version_id));
		const many = (t.show?.missing ?? 0) > 0 || (t.artist?.missing ?? 0) > 0;
		for (const v of t.versions.filter((x) => x.keep_up_to_date && (many || wanted.has(x.id)))) {
			try {
				const r = await call('PUT', `/titles/${t.id}/versions/${v.id}`, { keep_up_to_date: false });
				if (r.ok()) paused.push([t.id, v.id]);
				else failed.push(`pause ${t.name} version ${v.id}: ${r.status()} ${await r.text()}`);
			} catch (e) {
				failed.push(`pause ${t.name} version ${v.id}: ${e}`);
			}
		}
	}
}

/** restore turns every paused Version back on; it never stops at the first failure. */
async function restore(call: Call, paused: [number, number][], failed: string[]) {
	for (const [t, v] of paused) {
		try {
			const r = await call('PUT', `/titles/${t}/versions/${v}`, { keep_up_to_date: true });
			if (!r.ok()) failed.push(`restore title ${t} version ${v}: ${r.status()} ${await r.text()}`);
		} catch (e) {
			failed.push(`restore title ${t} version ${v}: ${e}`);
		}
	}
}

type Search = { job_id: number; kind: string; state: string; title_id?: number; episode?: string; by_hand: boolean; found: number; refused: number; outcome?: string };

test('Search all missing searches each missing movie, then each episode, at the Source', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const ids: number[] = [];
	const paused: [number, number][] = [];
	const pauseFailed: string[] = [];
	const restoreFailed: string[] = [];
	try {
		// Two movies and a show, all missing, kept up to date with the 4K Pick rules.
		const { pick_rules } = (await (await call('GET', '/pick-rules')).json()) as { pick_rules: { id: number; name: string }[] };
		const fourK = pick_rules.find((p) => p.name === '4K')?.id;
		expect(fourK, 'the 4K Pick rules').toBeTruthy();
		const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { id: number; path: string; kind: string }[] };
		let showFolder = library_folders.find((f) => f.kind === 'show')?.id;
		if (!showFolder) {
			mkdirSync(join(tmp, 'shows'), { recursive: true });
			const made = await call('POST', '/library-folders', { path: join(tmp, 'shows'), kind: 'show', label: 'Shows' });
			expect(made.status(), await made.text()).toBe(201);
			showFolder = ((await made.json()) as { id: number }).id;
		}
		const movies = [
			{ tmdb: 900271, name: 'Hollow Compass', dotted: 'Hollow.Compass.2015' },
			{ tmdb: 900272, name: 'Brass Meadow', dotted: 'Brass.Meadow.2014' }
		];
		for (const m of movies) {
			await removeNamed(call, 'movie', m.name);
			const added = await call('POST', '/titles', { id: `tmdb:${m.tmdb}`, pick_rules_id: fourK, keep_up_to_date: true, search_on_add: false, follow: false });
			expect(added.status(), await added.text()).toBe(201);
			ids.push(((await added.json()) as { title: { id: number } }).title.id);
		}
		await removeNamed(call, 'show', 'Gilded Bracken');
		const addedShow = await call('POST', '/shows', { id: 'tmdb:800271', library_folder_id: showFolder, pick_rules_id: fourK, keep_up_to_date: true, seasons: 'all', search_on_add: false, follow: false });
		expect(addedShow.status(), await addedShow.text()).toBe(201);
		const showId = ((await addedShow.json()) as { title: { id: number } }).title.id;
		ids.push(showId);
		// Three episodes are out and missing; S02E02 is not out yet.
		await expect.poll(async () => ((await (await call('GET', `/titles/${showId}`)).json()) as TitleRow).show?.missing, { timeout: 15_000 }).toBe(3);
		for (const id of ids) expect(((await (await call('GET', `/titles/${id}/releases`)).json()) as { releases: unknown[] }).releases).toEqual([]);

		await pauseOthers(call, ids, paused, pauseFailed);
		expect(pauseFailed, 'every other missing Version was paused').toEqual([]);

		// The Library's Missing list holds all three: the movies on Movies, the show on TV shows.
		const item = (name: string) => page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: name }).first();
		const allThreeMissing = async () => {
			await page.goto('/library?kind=movie');
			await libraryStatus(page, 'missing');
			await expect(page).toHaveURL(/show=missing/);
			for (const name of ['Hollow Compass', 'Brass Meadow']) await expect(item(name)).toBeVisible();
			await page.goto('/library?kind=show&show=missing');
			await expect(item('Gilded Bracken')).toBeVisible();
		};
		await allThreeMissing();

		// mineDone lists this test's searches once none of them is still running.
		const mineDone = async () => {
			const { running, recent } = (await (await call('GET', '/searches?limit=100')).json()) as { running: Search[]; recent: Search[] };
			if (running.some((s) => s.title_id && ids.includes(s.title_id))) return [];
			return recent.filter((s) => s.title_id && ids.includes(s.title_id));
		};
		type Job = { State: string; Params?: { which?: string; kind?: string }; Result?: { slots: number; queued: number; seasons: number } };
		let posts = 0;
		page.on('request', (r) => {
			if (r.method() === 'POST' && r.url().endsWith('/api/v1/search/missing')) posts++;
		});
		/** searchAll presses Search all missing, checks the confirm's count, cancels once, then searches. */
		const searchAll = async (label: string, said: RegExp): Promise<number> => {
			const button = page.getByTestId('search-all-missing');
			await expect(button).toHaveText(label);
			const n = Number((await page.getByText(/^\d+ of \d+ (movies?|TV shows?|artists?)$/).textContent())?.split(' ')[0]);
			expect(n).toBeGreaterThanOrEqual(1);
			const dialog = page.getByRole('dialog', { name: `Search for ${n} missing ${n === 1 ? 'title' : 'titles'}?` });
			await button.click();
			await expect(dialog).toBeVisible();
			await dialog.getByRole('button', { name: 'Cancel' }).click();
			await expect(dialog).toBeHidden();
			expect(posts, 'Cancel starts nothing').toBe(0);
			await button.click();
			const answer = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/api/v1/search/missing'));
			await dialog.getByRole('button', { name: 'Search' }).click();
			const res = await answer;
			expect(res.status()).toBe(202);
			posts = 0;
			await expect(page.getByText(said)).toBeVisible();
			await expect(dialog).toBeHidden();
			const [jobId] = ((await res.json()) as { job_ids: number[] }).job_ids;
			await expect.poll(async () => ((await (await call('GET', `/jobs/${jobId}`)).json()) as Job).State, { timeout: 15_000 }).toBe('done');
			return jobId;
		};

		// Movies only: the button names them, and only the two movies are searched.
		await page.goto('/library?show=missing&kind=movie');
		const movieJob = (await (await call('GET', `/jobs/${await searchAll('Search all missing movies', /^Searching for \d+ missing titles?$/)}`)).json()) as Job;
		expect(movieJob.Params).toMatchObject({ which: 'missing', kind: 'movie' });
		expect(movieJob.Result?.slots ?? 0).toBeGreaterThanOrEqual(2);
		expect(movieJob.Result?.seasons ?? 0).toBe(0);
		await expect.poll(async () => (await mineDone()).filter((s) => !s.by_hand && s.state === 'done').length, { timeout: 45_000 }).toBeGreaterThanOrEqual(2);
		const movieDone = await mineDone();
		for (const id of ids.slice(0, 2)) {
			expect(movieDone.find((s) => s.title_id === id && s.kind === 'search_slot'), `movie ${id} was searched`).toMatchObject({ state: 'done', by_hand: false, found: 3, refused: 3 });
		}
		expect(movieDone.filter((s) => s.title_id === showId), 'the show got no search from the movie press').toEqual([]);

		// Shows only: the show's episodes are searched, the movies are not searched again.
		await page.goto('/library?show=missing&kind=show');
		const jobId = await searchAll('Search all missing shows', /^Searching for \d+ missing titles?$/);

		// The bulk job finishes having queued at least this test's searches:
		// season 1 as one season search and S02E01 on its own.
		// Other titles may still be missing, so totals are only lower bounds;
		// the per-title checks below are the real evidence.
		const job = (await (await call('GET', `/jobs/${jobId}`)).json()) as Job;
		expect(job.Params).toMatchObject({ which: 'missing', kind: 'show' });
		expect(job.Result?.queued ?? 0).toBeGreaterThanOrEqual(2);
		expect(job.Result?.seasons ?? 0).toBeGreaterThanOrEqual(1);

		// Each search ran against the Source and found its Releases there.
		// Four searches from this press: season 1, S02E01, and then S01E01 and
		// S01E02 one by one, since the 4K rules turned the whole season down;
		// the movie press before it made two.
		await expect.poll(async () => (await mineDone()).filter((s) => !s.by_hand && s.state === 'done').length, { timeout: 45_000 }).toBeGreaterThanOrEqual(6);
		const done = await mineDone();
		for (const id of ids.slice(0, 2)) {
			expect(done.find((s) => s.title_id === id && s.kind === 'search_slot'), `movie ${id} was searched`).toMatchObject({ state: 'done', by_hand: false, found: 3, refused: 3 });
		}
		expect(done.find((s) => s.title_id === showId && s.kind === 'search_season'), 'season 1 was searched as a season').toMatchObject({ state: 'done', by_hand: false, episode: 'S01', found: 3, refused: 3 });
		expect(done.find((s) => s.title_id === showId && s.kind === 'search_slot' && s.episode?.startsWith('S02E01')), 'S02E01 was searched').toMatchObject({ state: 'done', by_hand: false });
		for (const code of ['S01E01', 'S01E02']) {
			expect(done.find((s) => s.title_id === showId && s.kind === 'search_slot' && s.episode?.startsWith(code)), `${code} was searched on its own after the season`).toMatchObject({ state: 'done', by_hand: false });
		}

		const names = async (id: number) => ((await (await call('GET', `/titles/${id}/releases`)).json()) as { releases: { name: string; passed: boolean }[] }).releases;
		for (const [i, m] of movies.entries()) {
			const rels = await names(ids[i]);
			expect(rels.map((r) => r.name).sort()).toEqual([`${m.dotted}.1080p.WEB-DL.H264-HOLYARRS`, `${m.dotted}.720p.WEB.H264-HOLYARRS`, `${m.dotted}.HDCAM.x264-NOPE`].sort());
			// The 4K rules turn every one away: nothing is downloaded.
			expect(rels.filter((r) => r.passed)).toEqual([]);
		}
		const showRels = (await names(showId)).map((r) => r.name);
		for (const code of ['S01E01', 'S01E02', 'S02E01']) expect(showRels).toContain(`Gilded.Bracken.${code}.1080p.WEB-DL.H264-HOLYARRS`);
		expect(showRels.some((n) => n.includes('S02E02'))).toBe(false);

		// The movie page lists what the search found.
		await page.goto(`/library/${ids[0]}`);
		await page.getByRole('button', { name: 'More actions' }).click();
		await page.getByRole('menuitem', { name: 'Choose a release' }).click();
		const sheet = page.getByRole('dialog', { name: 'Choose a release' });
		await sheet.getByRole('tab', { name: /^All/ }).click();
		await expect(sheet.getByTestId('release')).toHaveCount(3);
		await expect(sheet.getByTestId('release').filter({ hasText: 'Hollow.Compass.2015.1080p.WEB-DL' })).toBeVisible();
		await page.keyboard.press('Escape');

		// Nothing passed, so all three stay on the Missing lists.
		await allThreeMissing();
	} finally {
		await restore(call, paused, restoreFailed);
		for (const id of ids) {
			try {
				const gone = await call('POST', `/titles/${id}/delete`, { delete_files: true, never_add: false, apply: true });
				if (!gone.ok()) restoreFailed.push(`delete title ${id}: ${gone.status()} ${await gone.text()}`);
			} catch (e) {
				restoreFailed.push(`delete title ${id}: ${e}`);
			}
		}
	}
	expect(restoreFailed, 'other titles turned back on and the test titles removed').toEqual([]);
});
