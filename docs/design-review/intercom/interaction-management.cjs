function copyFixture(title = 'Afterlight') {
	return {
		file_id: 501,
		version_id: 601,
		title_id: 76,
		title,
		year: 2025,
		version: 'Main',
		edition: '',
		version_audio_languages: ['en'],
		relative_path: `${title}/movie.mkv`,
		protected: false,
		codec: 'hevc',
		inspection_status: 'inspected',
		container: 'mkv',
		audio_languages: ['English'],
		audio_tracks: 1,
		audio_unknown: false,
		added_at: '2026-10-17T12:00:00Z'
	};
}

function listedRelease() {
	return {
		id: 7001,
		name: 'Afterlight.2025.1080p.WEB-DL',
		size: 3_200_000_000,
		seeders: 18,
		peers: 2,
		protocol: 'torrent',
		state: 'held',
		found_by: 'Preview Indexer',
		source_id: 1,
		source_name: 'Preview Indexer',
		slot_id: 41,
		version: 'Main',
		title_id: 76,
		title_name: 'Afterlight',
		published_at: '2026-10-17T10:00:00Z',
		hold_until: '2026-10-18T10:00:00Z',
		outcome: 'held',
		passed: true,
		rank: 1,
		points: 100,
		step: 'wait',
		good_enough: true,
		needs_force: false,
		gate_skipped: false,
		reasons: [{ code: 'wait', effect: 'hold', text: 'Waiting for the preferred release window.' }],
		created_at: '2026-10-17T10:05:00Z'
	};
}

function missingSubtitle(kind = 'episode') {
	return {
		file_id: kind === 'movie' ? 8102 : 8101,
		item_id: kind === 'movie' ? 9102 : 9101,
		title_id: kind === 'movie' ? 76 : 1,
		title: kind === 'movie' ? 'Afterlight' : 'Severance',
		kind,
		season: kind === 'episode' ? 2 : 0,
		episode: kind === 'episode' ? 3 : 0,
		language: 'en',
		hearing_impaired: false,
		forced: false,
		state: 'wanted',
		tries: 0
	};
}

const scenarios = [
	{
		name: 'library codec filters and review cancellation',
		route: '/library/copies',
		async setup({ intercept }) {
			await intercept(async ({ method, path, url }) => {
				if (method === 'GET' && path === '/library/copies') {
					const title = url.searchParams.get('q') === 'Atlas' ? 'Atlas' : 'Afterlight';
					const copy = copyFixture(title);
					return { status: 200, json: { copies: [copy], total: 1, limit: 50, offset: 0, codecs: { hevc: 1 } } };
				}
				if (method === 'POST' && path === '/library/copies/search/preview') {
					return { status: 200, json: { copies: [copyFixture('Atlas')], slot_count: 1, review_token: '<preview-only>' } };
				}
				return null;
			});
		},
		async run({ page, expect, step }) {
			await step('apply the title filter', async () => {
				await page.getByLabel('Find by title').fill('Atlas');
				await page.getByRole('button', { name: 'Apply' }).click();
				await expect(page.getByRole('link', { name: /Atlas/ })).toBeVisible();
				await expect(page.getByRole('link', { name: /Afterlight/ })).toHaveCount(0);
			});
			await step('review and cancel a selected codec search', async () => {
				await page.getByRole('checkbox', { name: 'Select Atlas, Main' }).check();
				await page.getByRole('button', { name: 'Review searches' }).click();
				await expect(page.getByRole('heading', { name: 'Review 1 copy' })).toBeVisible();
				await page.getByRole('button', { name: 'Cancel' }).click();
				await expect(page.getByRole('heading', { name: 'Review 1 copy' })).toHaveCount(0);
			});
		}
	},
	{
		name: 'held release removal refreshes the list',
		route: '/library/held',
		async setup({ intercept }) {
			let removed = false;
			await intercept(async ({ method, path }) => {
				if (method === 'GET' && path === '/held') {
					return { status: 200, json: { releases: removed ? [] : [listedRelease()], total: removed ? 0 : 1, limit: 25, offset: 0 } };
				}
				if (method === 'DELETE' && path === '/held/7001') {
					removed = true;
					return { status: 200, json: {} };
				}
				return null;
			});
		},
		async run({ page, expect, step }) {
			await step('remove a held release', async () => {
				await expect(page.getByText('Afterlight.2025.1080p.WEB-DL')).toBeVisible();
				await page.getByRole('button', { name: 'Stop holding Afterlight.2025.1080p.WEB-DL' }).click();
				await expect(page.getByRole('heading', { name: 'Nothing is held' })).toBeVisible();
			});
		}
	},
	{
		name: 'rename failure preserves the preview for recovery',
		route: '/library/rename',
		async setup({ routeJSON }) {
			await routeJSON('GET', '/rename', {
				steps: [{ file_id: 91, title_id: 76, title_name: 'Afterlight', origin: 'managed', from: 'Afterlight/old-name.mkv', to: 'Afterlight/Afterlight (2025).mkv' }],
				plan_hash: 'rename-plan'
			});
			await routeJSON('POST', '/rename', { error: 'Rename service unavailable' }, 503);
		},
		async run({ page, expect, step }) {
			await step('report a refused rename without losing the plan', async () => {
				await expect(page.getByTestId('rename-step')).toHaveCount(1);
				await page.getByRole('button', { name: 'Rename 1 file' }).click();
				await expect(page.getByRole('alert')).toContainText('Rename service unavailable');
				await expect(page.getByTestId('rename-step')).toHaveCount(1);
				await expect(page.getByRole('button', { name: 'Rename 1 file' })).toBeEnabled();
			});
		}
	},
	{
		name: 'movie collection list opens the selected collection',
		route: '/library/collections',
		async setup({ routeJSON }) {
			await routeJSON('GET', '/collections', { collections: [{ id: 900001, source_id: 'preview:900001', name: 'The Vale Collection', owned_titles: 1 }] });
			await routeJSON('GET', '/collections/900001', {
				id: 900001,
				source_id: 'preview:900001',
				name: 'The Vale Collection',
				owned_titles: 1,
				titles: [{ id: 76, name: 'Afterlight', year: 2025, state: 'in_library', version_ids: [1], files: [], protected: false }]
			});
		},
		async run({ page, expect, step }) {
			await step('open a collection from the list', async () => {
				await page.getByRole('link', { name: /The Vale Collection/ }).click();
				await expect(page).toHaveURL(/\/library\/collections\/900001$/);
				await expect(page.getByRole('heading', { name: 'The Vale Collection' })).toBeVisible();
				await expect(page.getByRole('checkbox', { name: /Afterlight/ })).toBeChecked();
			});
		}
	},
	{
		name: 'missing subtitle filters and row search remain operable',
		route: '/library/missing-subtitles',
		async setup({ intercept, routeJSON }) {
			await routeJSON('GET', '/subtitles/languages', { languages: [{ code: 'en', name: 'English' }] });
			await routeJSON('POST', '/subtitles/search', { job_ids: [9901] });
			await intercept(async ({ method, path, url }) => {
				if (method !== 'GET' || path !== '/subtitles/missing') return null;
				const kind = url.searchParams.get('kind') === 'movie' ? 'movie' : 'episode';
				return {
					status: 200,
					json: {
						missing: [missingSubtitle(kind)],
						total: 1,
						titles: [{ id: kind === 'movie' ? 76 : 1, name: kind === 'movie' ? 'Afterlight' : 'Severance', count: 1 }]
					}
				};
			});
		},
		async run({ page, expect, step }) {
			await step('filter missing subtitles to movies', async () => {
				await page.getByLabel('Kind').selectOption('movie');
				await expect(page).toHaveURL(/kind=movie/);
				await expect(page.getByRole('link', { name: /Afterlight/ })).toBeVisible();
			});
			await step('start a search for the filtered row', async () => {
				await page.getByRole('button', { name: 'Search English subtitles for Afterlight' }).click();
				await expect(page.getByTestId('search-all-progress')).toBeVisible();
				await expect(page.getByRole('progressbar', { name: 'Subtitle searches done' })).toHaveAttribute('max', '1');
			});
		}
	}
];

module.exports = { scenarios };
