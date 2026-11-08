import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page, type Route, type TestInfo } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

type ViewportCase = { name: string; width: number; height: number };

const viewports: ViewportCase[] = [
	{ name: 'desktop', width: 1280, height: 900 },
	{ name: 'phone', width: 390, height: 844 }
];

const person = {
	id: '202',
	name: 'Alex Kim',
	department: 'Directing',
	known_for: [{ name: 'North Star', year: 2022, overview: '', ids: { tmdb: '601' } }]
};

// People pages show only credits with a picture that loads, so each film has a poster (served by mockCommunity).
const films = [
	{ name: 'North Star', year: 2022, overview: '', ids: { tmdb: '601' }, roles: ['Director'], title_id: 77, poster: '/e2e-art/601.png' },
	{ name: 'Second Light', year: 2024, overview: '', ids: { tmdb: '602' }, roles: ['Director'], poster: '/e2e-art/602.png' },
	{ name: 'Third Light', year: 2025, overview: '', ids: { tmdb: '603' }, roles: ['Director'], poster: '/e2e-art/603.png' }
];
// A 1x1 PNG for the fixture posters.
const tinyPng = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

const version = {
	id: 701,
	label: '1080p',
	main: true,
	pick_rules_id: 1,
	pick_rules_name: 'Any',
	library_folder_id: 1,
	folder_path: 'Movies/North Star (2022)',
	keep_up_to_date: true,
	edition: '',
	audio_languages: []
};

const title = {
	id: 77,
	kind: 'movie',
	name: 'North Star',
	year: 2022,
	overview: 'A mocked movie used to verify credit navigation.',
	original_language: 'en',
	ids: { tmdb: '601' },
	keep_up_to_date: true,
	why_added: 'Added for this browser fixture.',
	release_gate: 'released',
	gate_delay_days: 0,
	stage: 'released',
	gate_open: true,
	gate_reason: 'Released',
	artwork: [],
	versions: [version],
	slots: [{ id: 801, version: '1080p', version_id: 701, state: 'missing', state_label: 'Missing' }],
	asks: []
};

const collectionTitles = [
	{ id: 77, name: 'North Star', year: 2022, state: 'owned', version_ids: [701], files: [{ id: 901, version_id: 701, path: 'North Star.mkv', origin: 'managed', state: 'present', protected: false }], protected: false },
	{ id: 78, name: 'North Star II', year: 2024, state: 'owned', version_ids: [702], files: [{ id: 902, version_id: 702, path: 'North Star II.mkv', origin: 'managed', state: 'present', protected: true }], protected: true },
	{ id: 79, name: 'North Star III', year: 2026, state: 'owned', version_ids: [703], files: [], protected: false }
];

function copy(file_id: number, version_id: number, title_id: number, name: string, codec: 'h264' | 'hevc', protectedCopy = false) {
	return {
		file_id,
		version_id,
		title_id,
		title: name,
		year: 2024,
		version: '1080p',
		edition: '',
		version_audio_languages: [],
		relative_path: `${name}.mkv`,
		protected: protectedCopy,
		codec,
		inspection_status: 'inspected',
		container: 'matroska',
		audio_languages: ['en'],
		audio_tracks: 1,
		audio_unknown: false,
		added_at: '2026-10-15T12:00:00Z'
	};
}

async function json(route: Route, body: unknown, status = 200) {
	await route.fulfill({ status, json: body });
}

async function mockCommunity(page: Page) {
	const added: string[] = [];
	const copyQueries: URL[] = [];
	let staleOnce = true;

	await page.route('**/e2e-art/**', (route) => route.fulfill({ status: 200, contentType: 'image/png', body: tinyPng }));
	await page.route('**/api/v1/**', async (route) => {
		const request = route.request();
		const url = new URL(request.url());
		const path = url.pathname.replace(/^\/api\/v1/, '');
		const method = request.method();

		if (path === '/auth/me') return json(route, { id: 1, name: 'owner', via: 'session', csrf: 'fixture', role: 'owner', perms: ['ask', 'approve', 'manage_library', 'settings'] });
		if (path === '/parts') return json(route, { parts: [], screens: {}, safe_mode: false });
		if (path === '/messages') return json(route, { messages: [], unread: 0, waiting: 0 });
		if (path === '/needs-decision') return json(route, { items: [], count: 0 });

		if (path === '/people/search') {
			return json(route, {
				people: [
					{ id: '101', name: 'Alex Kim', department: 'Acting', known_for: [{ name: 'City Lines', year: 2018, overview: '', ids: { tmdb: '501' } }] },
					person
				],
				page: 1,
				total_pages: 1,
				total_results: 2,
				attribution: 'Fixture provider'
			});
		}
		if (path === '/people/202' && method === 'GET') return json(route, { person: { ...person, biography: 'Director biography.' }, movies: films, page: 1, page_size: 24, total_pages: 1, total_results: 3, complete: false, provider_pages: 1, attribution: 'Fixture provider' });
		if (path === '/people/202/movies/review' && method === 'POST') {
			const body = request.postDataJSON() as { tmdb_ids: string[] };
			return json(route, { movies: films.filter((movie) => body.tmdb_ids.includes(movie.ids.tmdb)), automatic_future_credits: false });
		}
		if (path === '/titles' && method === 'POST') {
			const body = request.postDataJSON() as { id: string };
			added.push(body.id);
			if (body.id === 'tmdb:603') return json(route, { error: 'The weekly fixture limit was reached.' }, 429);
			return json(route, { title: { id: 88 } }, 201);
		}

		if (path === '/titles/77' && method === 'GET') return json(route, title);
		// The title page reads the film's facts from the lookup and the metadata answer.
		if (path === '/titles/lookup') return json(route, { movies: [{ ...films[0], title_id: 77 }] });
		if (path === '/metadata/movie/601') return json(route, { name: 'North Star', year: 2022, overview: '', poster: films[0].poster, genres: [], cast: [{ id: '202', name: 'Alex Kim', role: 'Director' }], similar: [] });
		if (path === '/titles/77/people') return json(route, { people: [{ person, roles: ['Director'], characters: [] }] });
		if (path === '/titles/77/copies') return json(route, { files: [], versions: [version] });
		if (path === '/titles/77/subtitles') return json(route, { files: [], wishes: [] });
		if (path === '/subtitles/languages') return json(route, { languages: [] });
		if (path === '/settings/subtitles') return json(route, { languages: [], pass_mark_movie: 80, pass_mark_episode: 80, sync_from_fps: 25, sync_to_fps: 23.976, sync_step_ms: 500 });
		if (path === '/events') return json(route, { events: [], total: 0 });
		if (path === '/titles/77/releases') return json(route, { releases: [], total: 0, running_job_ids: [] });

		if (path === '/collections/9' && method === 'GET') return json(route, { id: 9, source_id: 'tmdb:900', name: 'North Star series', owned_titles: 3, titles: collectionTitles });
		if (path === '/collections/9/remove/preview' && method === 'POST') {
			const body = request.postDataJSON() as { title_ids: number[]; delete_files: boolean; never_add: boolean; acknowledge_protection: boolean };
			return json(route, {
				collection: { id: 9, source_id: 'tmdb:900', name: 'North Star series', owned_titles: 3 },
				titles: collectionTitles.map((item) => ({ ...item, selected: body.title_ids.includes(item.id), ...(item.protected ? { skip: 'Protected copy requires the explicit acknowledgement recorded in this review.' } : {}) })),
				choice: body,
				review_token: staleOnce ? 'TEST_STALE_REVIEW' : 'TEST_FRESH_REVIEW',
				applied: false
			});
		}
		if (path === '/collections/9/remove' && method === 'POST') {
			if (staleOnce) {
				staleOnce = false;
				return json(route, { error: 'This review is stale. Review the current files again.' }, 409);
			}
			return json(route, {
				collection: { id: 9, source_id: 'tmdb:900', name: 'North Star series', owned_titles: 3 },
				titles: collectionTitles.map((item) => ({ ...item, selected: true })),
				choice: { delete_files: true, never_add: true, acknowledge_protection: true },
				review_token: 'TEST_FRESH_REVIEW',
				applied: true,
				results: [
					{ title_id: 77, name: 'North Star', status: 'removed' },
					{ title_id: 78, name: 'North Star II', status: 'skipped', error: 'the protected file changed after review' },
					{ title_id: 79, name: 'North Star III', status: 'failed', error: 'the Library row changed after review' }
				]
			});
		}

		if (path === '/library/copies' && method === 'GET') {
			copyQueries.push(url);
			const offset = Number(url.searchParams.get('offset') ?? 0);
			const copies = offset === 0
				? [copy(1001, 701, 77, 'Alpha', 'h264'), copy(1002, 702, 78, 'Protected Beta', 'h264', true)]
				: [copy(1051, 751, 127, 'Gamma', 'h264')];
			return json(route, { copies, total: 51, limit: 50, offset, codecs: { h264: 51, hevc: 4 } });
		}
		if (path === '/library/copies/search/preview' && method === 'POST') {
			const body = request.postDataJSON() as { selections: { file_id: number; version_id: number }[] };
			const copies = body.selections.map((selection) => selection.file_id === 1001 ? copy(1001, 701, 77, 'Alpha', 'h264') : copy(1051, 751, 127, 'Gamma', 'h264'));
			return json(route, { copies, slot_count: copies.length, review_token: 'TEST_COPY_REVIEW' });
		}

		return json(route, { error: `No fixture for ${method} ${path}` }, 404);
	});

	return { added, copyQueries };
}

async function signedInCommunity(page: Page) {
	await signIn(page, owner());
	return mockCommunity(page);
}

async function verifyPage(page: Page, testInfo: TestInfo, name: string) {
	await noSidewaysScroll(page);
	// Let fades finish (a button turning back on fades in), so contrast is read on the settled page.
	await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity));
	const result = await new AxeBuilder({ page }).include('main').withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(result.violations.map((violation) => `${violation.id}: ${violation.nodes.map((node) => node.target.join(' ')).join(', ')}`)).toEqual([]);
	const artifactDir = join(process.cwd(), '..', 'tests', '.tmp', 'community-artifacts');
	mkdirSync(artifactDir, { recursive: true });
	const artifact = join(artifactDir, `${name}.png`);
	await page.screenshot({ path: artifact, fullPage: true });
	await testInfo.attach(name, { path: artifact, contentType: 'image/png' });
}

for (const viewport of viewports) {
	test.describe(`${viewport.name} community browsing`, () => {
		test.use({ viewport: { width: viewport.width, height: viewport.height } });

		test('disambiguates a person by stable identity, shows the card filmography, and follows title credits', async ({ page }, testInfo) => {
			await signedInCommunity(page);
			// The old People list now opens Search's People tab; a person's own page is unchanged.
			await page.goto('/people?q=Alex');
			await expect(page).toHaveURL(/\/search\?kind=people&q=Alex$/);
			await page.goto('/people/202');
			await expect(page.getByRole('heading', { level: 1, name: 'Alex Kim' })).toBeVisible();
			// The filmography is rows of cards by decade, under the Filmography heading and filters: the owned film opens its Library page, the others open their title page.
			const filmography = page.getByRole('group', { name: '2020s' });
			await expect(filmography.locator('a[href$="/library/77"]')).toHaveAttribute('aria-label', /^North Star/);
			await expect(filmography.locator('a[href$="/title/movie/602"]')).toHaveAttribute('aria-label', /^Second Light/);
			await expect(filmography.locator('a[href$="/title/movie/603"]')).toHaveAttribute('aria-label', /^Third Light/);
			// Selecting and reviewing movies on this page was removed on purpose.
			await expect(page.getByRole('button', { name: /^Review \d+ selected$/ })).toHaveCount(0);
			await expect(page.getByLabel('Select Second Light')).toHaveCount(0);

			await page.goto('/library/77');
			await expect(page.getByRole('heading', { level: 1, name: 'North Star' })).toBeVisible();
			await expect(page.getByText('2022', { exact: false }).locator('visible=true').first()).toBeVisible();
			// Cast and crew loads by itself on the new title page.
			// The side facts and the Cast row both link the director.
			await expect(page.getByRole('link', { name: /^Alex Kim/ }).first()).toHaveAttribute('href', '/people/202');
			await verifyPage(page, testInfo, `${viewport.name}-people-and-credits`);
		});

		test('re-reviews stale movie-series removal and shows protected per-title results', async ({ page }, testInfo) => {
			await signedInCommunity(page);
			await page.goto('/library/collections/9');
			await expect(page.getByText('Protected copy')).toBeVisible();
			await page.getByLabel('Move HolyArrs-managed files to the holding folder').check();
			await page.getByLabel('Put removed movies on Never-add').check();
			await page.getByLabel('I understand protected copies in the selected titles will also be removed').check();
			await page.getByRole('button', { name: 'Review 3 selected' }).click();
			await expect(page.getByText('Protected copy requires the explicit acknowledgement recorded in this review.')).toBeVisible();
			await page.getByRole('button', { name: 'Remove reviewed movies' }).click();
			await expect(page.getByRole('alert')).toContainText('This review is stale.');

			await page.getByRole('button', { name: 'Review 3 selected' }).click();
			await page.getByRole('button', { name: 'Remove reviewed movies' }).click();
			await expect(page.getByRole('heading', { name: 'Collection removed' })).toBeVisible();
			await expect(page.getByText('North Star was removed from the Library.')).toBeVisible();
			await expect(page.getByText('North Star II stayed in the Library: the protected file changed after review.')).toBeVisible();
			await expect(page.getByText('North Star III could not be removed: the Library row changed after review.')).toBeVisible();
			await verifyPage(page, testInfo, `${viewport.name}-series-results`);
		});

		test('combines codec filters and keeps selections across pages for an exact preview', async ({ page }, testInfo) => {
			const mocked = await signedInCommunity(page);
			await page.goto('/library/copies');
			await expect(page.getByText('1–2 of 51')).toBeVisible();
			await page.getByTestId('copies-find-input').fill('star');
			await page.getByLabel('Codec', { exact: true }).selectOption('h264');
			await page.getByLabel('Sort', { exact: true }).selectOption('codec');
			await expect.poll(() => mocked.copyQueries.some((url) => url.searchParams.get('q') === 'star' && url.searchParams.get('codec') === 'h264' && url.searchParams.get('sort') === 'codec')).toBe(true);
			await page.getByLabel('Select Alpha, 1080p').check();
			await expect(page.getByLabel('Select Protected Beta, 1080p')).toBeDisabled();
			await page.getByRole('button', { name: 'Next' }).click();
			await expect(page.getByText('51–51 of 51')).toBeVisible();
			await page.getByLabel('Select Gamma, 1080p').check();
			await expect(page.getByText('2 selected')).toBeVisible();
			await page.getByRole('button', { name: 'Review', exact: true }).click();
			const review = page.getByRole('region', { name: 'Search 2 copies again' });
			await expect(review).toContainText('Alpha');
			await expect(review).toContainText('Gamma');
			await expect(review).toContainText('Nothing downloads on its own.');
			await verifyPage(page, testInfo, `${viewport.name}-codec-preview`);
		});
	});
}
