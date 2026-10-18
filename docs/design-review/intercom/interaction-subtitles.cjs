const assert = require('node:assert/strict');

const titleID = 1;
const fileID = 611;
const subtitleID = 801;
const languages = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Spanish' }
];
const settings = {
	languages: [
		{ language: 'en', hearing_impaired: 'normal', forced: false },
		{ language: 'es', hearing_impaired: 'normal', forced: false }
	],
	pass_mark_movie: 70,
	pass_mark_episode: 90,
	use_built_in: true,
	ignore_picture_tracks: true,
	hi_suffix: 'hi',
	search_hours: 6,
	better: true,
	better_hours: 24,
	better_days: 21,
	batch: 4,
	adaptive: true,
	sync_from_fps: 25,
	sync_to_fps: 23.976,
	sync_step_ms: 500
};

function subtitleRow(overrides = {}) {
	return {
		ID: subtitleID,
		file_id: fileID,
		language: 'en',
		forced: false,
		hearing_impaired: false,
		place: 'beside',
		codec: 'srt',
		relative_path: 'Signal Garden (2024)/Signal.Garden.2024.1080p.en.srt',
		site: 'Preview Subtitles',
		site_key: 'preview-en-1',
		score: 94,
		score_parts: [
			{ name: 'title', points: 40, matched: true },
			{ name: 'year', points: 20, matched: true },
			{ name: 'release_group', points: 10, matched: true },
			{ name: 'resolution', points: 10, matched: true },
			{ name: 'format', points: 8, matched: true },
			{ name: 'trusted', points: 6, matched: true }
		],
		origin: 'fetched',
		state: 'present',
		saved_at: '2026-10-17T12:00:00Z',
		tries: 1,
		sync: {},
		...overrides
	};
}

function titleSubtitles({ manualChosen = false, turnedDown = false } = {}) {
	const subtitles = [];
	if (!turnedDown) subtitles.push(subtitleRow());
	if (manualChosen) subtitles.push(subtitleRow({
		ID: 803,
		language: 'es',
		relative_path: 'Signal Garden (2024)/Signal.Garden.2024.1080p.es.srt',
		site_key: 'preview-es-1',
		score: 86,
		origin: 'by_hand'
	}));
	if (!manualChosen) subtitles.push(subtitleRow({
		ID: 802,
		language: 'es',
		place: 'wish',
		codec: undefined,
		relative_path: undefined,
		site: undefined,
		site_key: undefined,
		score: undefined,
		score_parts: [],
		origin: 'fetched',
		state: 'wanted',
		saved_at: undefined,
		tries: 2,
		next_try_at: '2026-10-17T18:00:00Z'
	}));
	return {
		wishes: [{ ID: 701, title_id: titleID, languages: structuredClone(settings.languages) }],
		files: [{
			file_id: fileID,
			item_id: 1,
			version_id: 91,
			relative_path: 'Signal Garden (2024)/Signal.Garden.2024.1080p.mkv',
			wish: { languages: structuredClone(settings.languages), from: 'title' },
			subtitles
		}]
	};
}

function choices() {
	return {
		file_id: fileID,
		sites_asked: ['Preview Subtitles'],
		outcomes: [{
			want: { language: 'es', hearing_impaired: 'normal', forced: false },
			result: 'choices',
			choices: [{
				site: 'Preview Subtitles',
				key: 'preview-es-1',
				language: 'es',
				hearing_impaired: false,
				forced: false,
				release_names: ['Signal.Garden.2024.1080p.WEB-DL'],
				title: 'Signal Garden',
				year: 2024,
				uploader: 'preview-uploader',
				trusted: true,
				downloads: 42,
				format: 'srt',
				score: {
					total: 86,
					parts: [
						{ name: 'title', points: 40, matched: true },
						{ name: 'year', points: 20, matched: true },
						{ name: 'release_group', points: 0, matched: false },
						{ name: 'trusted', points: 6, matched: true }
					]
				}
			}]
		}]
	};
}

function installReads(ctx, state = {}) {
	ctx.intercept(async ({ method, path }) => {
		if (method === 'GET' && path === `/titles/${titleID}/subtitles`) return { json: titleSubtitles(state) };
		if (method === 'GET' && path === '/subtitles/languages') return { json: { languages } };
		if (method === 'GET' && path === '/settings/subtitles') return { json: settings };
		return null;
	});
}

const searchAndChoose = {
	name: 'Title and file subtitle search with manual candidate choice',
	route: `/library/${titleID}`,
	setup(ctx) {
		ctx.activity = { titleSearches: 0, choiceSearches: 0, choices: 0, titleBody: null, chooseBody: null };
		const state = { manualChosen: false };
		installReads(ctx, state);
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'POST' && path === '/subtitles/search') {
				ctx.activity.titleSearches += 1;
				ctx.activity.titleBody = body;
				return { delay: 180, status: 202, json: { queued: 1 } };
			}
			if (method === 'POST' && path === `/files/${fileID}/subtitles/choices`) {
				ctx.activity.choiceSearches += 1;
				return { delay: 180, json: choices() };
			}
			if (method === 'POST' && path === `/files/${fileID}/subtitles/choose`) {
				ctx.activity.choices += 1;
				ctx.activity.chooseBody = body;
				state.manualChosen = true;
				return { delay: 180, json: { path: '/preview/Signal Garden (2024)/Signal.Garden.2024.1080p.es.srt' } };
			}
			return null;
		});
	},
	async run(ctx) {
		const panel = ctx.page.getByTestId('subtitles').first();
		await ctx.step('title search exposes a delayed busy state and ignores a duplicate activation', async () => {
			const search = panel.getByTestId('subtitle-search');
			await search.click();
			await ctx.expect(search).toBeDisabled();
			await search.evaluate((node) => node.click());
			await ctx.page.getByText('Looking for subtitles now.', { exact: true }).waitFor();
			assert.equal(ctx.activity.titleSearches, 1, 'title subtitle search posts once');
			assert.deepEqual(ctx.activity.titleBody, { title_id: titleID }, 'title subtitle search uses the title scope');
		});
		await ctx.step('file search loads schema-complete choices and saves one candidate once', async () => {
			const open = panel.getByTestId('subtitle-choose');
			await open.click();
			await ctx.expect(open).toHaveAttribute('aria-expanded', 'true');
			await panel.getByText('Asking the Subtitle sites…', { exact: true }).waitFor();
			const choice = panel.getByRole('button', { name: 'Choose Signal.Garden.2024.1080p.WEB-DL from Preview Subtitles' });
			await choice.waitFor();
			assert.equal(ctx.activity.choiceSearches, 1, 'manual candidate search posts once');
			await choice.click();
			await ctx.expect(choice).toBeDisabled();
			await choice.evaluate((node) => node.click());
			await ctx.page.getByText('Saved Signal.Garden.2024.1080p.es.srt.', { exact: true }).waitFor();
			assert.equal(ctx.activity.choices, 1, 'manual candidate save posts once');
			assert.deepEqual(ctx.activity.chooseBody, { site: 'Preview Subtitles', key: 'preview-es-1' }, 'manual candidate save identifies the selected site result');
			const spanish = panel.getByTestId('subtitle-row')
				.filter({ hasText: 'Spanish' })
				.filter({ hasText: 'Chosen by hand from Preview Subtitles' });
			await ctx.expect(spanish).toHaveCount(1);
			await ctx.expect(spanish).toContainText('Signal.Garden.2024.1080p.es.srt');
			await ctx.expect(panel.getByTestId('subtitle-missing').filter({ hasText: 'Spanish' })).toHaveCount(0);
		});
		await ctx.capture('subtitle-search-and-choice');
	}
};

const syncValidationAndSave = {
	name: 'Subtitle sync validation and delayed save guard',
	route: `/library/${titleID}`,
	setup(ctx) {
		ctx.activity = { syncs: 0, body: null };
		installReads(ctx);
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'POST' && path === `/subtitles/${subtitleID}/sync`) {
				ctx.activity.syncs += 1;
				ctx.activity.body = body;
				return {
					delay: 180,
					status: 200,
					json: subtitleRow({
						ID: 804,
						sync: { offset_ms: 500, from_fps: 0, to_fps: 0, from_subtitle_id: subtitleID }
					})
				};
			}
			return null;
		});
	},
	async run(ctx) {
		const panel = ctx.page.getByTestId('subtitles').first();
		await ctx.step('empty Sync fix stays local and explains the required change', async () => {
			await panel.getByTestId('subtitle-sync').click();
			const form = panel.getByRole('form', { name: 'Sync fix' });
			await form.getByRole('button', { name: 'Fix the timing' }).click();
			await ctx.expect(form.getByRole('alert')).toHaveText('Shift the timing or change the frame rate first.');
			assert.equal(ctx.activity.syncs, 0, 'invalid no-op sync does not post');
		});
		await ctx.step('valid offset saves once while the delayed request is busy', async () => {
			const form = panel.getByRole('form', { name: 'Sync fix' });
			await form.getByRole('button', { name: '500 ms later' }).click();
			await ctx.expect(form.getByLabel('Shift (milliseconds; below zero is earlier)')).toHaveValue('500');
			const save = form.getByRole('button', { name: 'Fix the timing' });
			await save.click();
			await ctx.expect(save).toBeDisabled();
			await save.evaluate((node) => node.click());
			await ctx.page.getByText('Timing fixed. The old file is in the holding folder.', { exact: true }).waitFor();
			assert.equal(ctx.activity.syncs, 1, 'valid sync posts once');
			assert.deepEqual(ctx.activity.body, { offset_ms: 500 }, 'sync sends only the selected offset');
			await ctx.expect(form).toHaveCount(0);
		});
		await ctx.capture('subtitle-sync-saved');
	}
};

const turnDownRecovery = {
	name: 'Subtitle turn-down cancel and rejected retry recovery',
	route: `/library/${titleID}`,
	setup(ctx) {
		ctx.activity = { turnDowns: 0, bodies: [] };
		const state = { turnedDown: false };
		installReads(ctx, state);
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'POST' && path === `/subtitles/${subtitleID}/turn-down`) {
				ctx.activity.turnDowns += 1;
				ctx.activity.bodies.push(body);
				if (ctx.activity.turnDowns === 1) return { delay: 180, status: 503, json: { error: 'Preview subtitle turn-down rejected' } };
				state.turnedDown = true;
				return { delay: 180, status: 204 };
			}
			return null;
		});
	},
	async run(ctx) {
		const panel = ctx.page.getByTestId('subtitles').first();
		await ctx.step('Turn down can be cancelled without a request and reopens cleanly', async () => {
			await panel.getByTestId('subtitle-turn-down').click();
			let form = panel.getByRole('form', { name: 'Turn down' });
			await form.getByLabel('Why (optional)').fill('This should be discarded.');
			await form.getByRole('button', { name: 'Cancel' }).click();
			await ctx.expect(form).toHaveCount(0);
			assert.equal(ctx.activity.turnDowns, 0, 'cancel does not post a turn-down');
			await panel.getByTestId('subtitle-turn-down').click();
			form = panel.getByRole('form', { name: 'Turn down' });
			await ctx.expect(form.getByLabel('Why (optional)')).toHaveValue('');
		});
		await ctx.step('rejected Turn down keeps its reason and permits one successful retry', async () => {
			const form = panel.getByRole('form', { name: 'Turn down' });
			const reason = form.getByLabel('Why (optional)');
			await reason.fill('Wrong film and out of sync.');
			const submit = form.getByTestId('subtitle-turn-down-yes');
			await submit.click();
			await ctx.expect(submit).toBeDisabled();
			await submit.evaluate((node) => node.click());
			await ctx.expect(form.getByRole('alert')).toHaveText('Preview subtitle turn-down rejected');
			assert.equal(ctx.activity.turnDowns, 1, 'delayed rejected turn-down posts once');
			await ctx.expect(reason).toHaveValue('Wrong film and out of sync.');
			await ctx.expect(submit).toBeEnabled();
			await submit.click();
			await ctx.expect(submit).toBeDisabled();
			await ctx.page.getByText('Subtitle turned down; HolyArrs looks for another one.', { exact: true }).waitFor();
			assert.equal(ctx.activity.turnDowns, 2, 'retry adds one successful turn-down request');
			assert.deepEqual(ctx.activity.bodies, [
				{ reason: 'Wrong film and out of sync.' },
				{ reason: 'Wrong film and out of sync.' }
			], 'rejected retry preserves the reason');
			await ctx.expect(form).toHaveCount(0);
		});
		await ctx.capture('subtitle-turn-down-recovered');
	}
};

module.exports = [searchAndChoose, syncValidationAndSave, turnDownRecovery];
