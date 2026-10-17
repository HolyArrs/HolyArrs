/**
 * Fixture-backed activity scenarios for the preview interaction runner.
 * Mutations are intercepted: these exercise the real controls without changing
 * preview data or credentials.
 */

const now = '2026-10-17T16:00:00Z';
const person = (id, name) => ({ id, name });
const askedBy = (id, name) => ({ id, name, asked_at: now });

const ask = (id, name, state = 'asked', mine = false) => ({
	id,
	kind: 'movie',
	tmdb_id: String(9000 + id),
	name,
	year: 2026,
	state,
	state_label: state === 'asked' ? 'Waiting' : state === 'approved' ? 'Approved' : 'Turned down',
	asked_by: person(id + 20, `person-${id}`),
	asked_at: now,
	people: [askedBy(id + 20, `person-${id}`)],
	auto_approved: false,
	mine
});

const downloadRow = {
	id: 701,
	state: 'downloading',
	label: 'Downloading',
	text: 'Waiting for the remaining pieces.',
	progress: 0.42,
	speed: 2500000,
	eta: 360,
	active: true,
	phase: 'downloading',
	name: 'Signal.Garden.2026.1080p.mkv',
	title_id: 76,
	title: 'Signal Garden',
	year: 2026,
	version: 'HD',
	downloader: 'Preview downloader',
	size: 6400000000,
	updated_at: now,
	can_act: true,
	kind: 'movie'
};

const downloadsActions = {
	name: 'Downloads details, failure and remove actions',
	route: '/downloads',
	setup: async (ctx) => {
		ctx.activity = { failed: 0, removed: 0 };
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/downloads/view') return { json: { downloads: [downloadRow] } };
			if (method === 'GET' && path === '/imports') return { json: { imports: [] } };
			if (method === 'GET' && path === '/titles') return { json: { titles: [] } };
			if (method === 'GET' && path === '/settings/downloaders') return { json: { remove_delete_files: true, remove_turn_down: false, remove_search_again: true } };
			if (method === 'POST' && path === '/downloads/701/mark-failed') {
				ctx.activity.failed += 1;
				return { delay: 120, json: {} };
			}
			if (method === 'POST' && path === '/downloads/701/remove') {
				ctx.activity.removed += 1;
				return { delay: 120, json: {} };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('details selects the download and moves keyboard focus', async () => {
			await page.getByRole('button', { name: 'Details for Signal Garden (2026)' }).click();
			const heading = page.getByRole('heading', { name: 'Signal Garden (2026)' }).last();
			await ctx.expect(await heading.evaluate((node) => document.activeElement === node), 'download detail heading receives focus');
		});
		await ctx.step('mark failed confirms once and blocks duplicate activation', async () => {
			ctx.confirmNext(true);
			const button = page.locator('.detail-actions').getByRole('button', { name: 'Mark failed' });
			await button.click();
			await ctx.expect(await button.isDisabled(), 'Mark failed disables during request');
			await page.getByText(/Marked failed/).waitFor();
			await ctx.expect(ctx.activity.failed === 1, 'Mark failed posts once');
		});
		await ctx.step('remove dialog options and submit work', async () => {
			await page.locator('.detail-actions').getByRole('button', { name: 'Remove' }).click();
			const dialog = page.getByRole('dialog', { name: 'Remove this download?' });
			await ctx.expect(await dialog.isVisible(), 'Remove dialog opens');
			const again = dialog.getByLabel('Search again');
			await ctx.expect(await again.isDisabled(), 'Search again starts disabled without Turned down');
			await dialog.getByLabel('Add its Release to Turned down').check();
			await ctx.expect(!(await again.isDisabled()), 'Turned down enables Search again');
			const submit = dialog.getByRole('button', { name: 'Remove', exact: true });
			await submit.click();
			await ctx.expect(await submit.isDisabled(), 'Remove submit disables during request');
			await dialog.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.removed === 1, 'Remove posts once');
		});
	}
};

const decisionsActions = {
	name: 'Needs a decision actions and confirmation',
	route: '/downloads/decide',
	setup: async (ctx) => {
		ctx.activity = { decisions: [] };
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'GET' && path === '/needs-decision') return { json: { items: [{
				slot_id: 801, title_id: 76, title: 'Signal Garden', year: 2026, version: 'HD', versions: 1,
				why: 'ran_out', reason: 'No acceptable release remains.', release_id: 55,
				release_name: 'Signal.Garden.2026.1080p', has_copy: false, since: now, kind: 'movie'
			}], count: 1 } };
			if (method === 'GET' && path === '/titles') return { json: { titles: [] } };
			if (method === 'POST' && path === '/needs-decision/801/decide') {
				ctx.activity.decisions.push(body?.action);
				return { delay: 120, json: {} };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('turn down can be cancelled without a request', async () => {
			ctx.confirmNext(false);
			await page.getByRole('button', { name: 'Add to Turned down' }).click();
			await ctx.expect(ctx.activity.decisions.length === 0, 'cancelled decision sends no request');
		});
		await ctx.step('search again submits once and exposes busy state', async () => {
			const button = page.getByRole('button', { name: 'Search again', exact: true });
			await button.click();
			await ctx.expect(await button.isDisabled(), 'decision action disables during request');
			await page.getByText(/Searching again for Signal Garden/).waitFor();
			await ctx.expect(ctx.activity.decisions.join(',') === 'search', 'decision action posts search once');
		});
	}
};

const historyRecovery = {
	name: 'History error recovery, filters, disclosure and paging',
	route: '/downloads/history',
	setup: async (ctx) => {
		ctx.activity = { eventReads: 0 };
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/titles') return { json: { titles: [{ id: 76, name: 'Signal Garden', year: 2026, kind: 'movie', artwork: [] }] } };
			if (method === 'GET' && path === '/events') {
				ctx.activity.eventReads += 1;
				if (ctx.activity.eventReads === 1) return { status: 500, json: { error: 'preview history unavailable' } };
				return { json: { events: [{ ID: 991, At: now, Kind: 'download_failed', Summary: 'Download failed', TitleID: 76, title_name: 'Signal Garden', Data: { error: 'Preview failure', path: '/preview/file.mkv' } }], total: 75, limit: 50, offset: 0 } };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('load error retries into the event list', async () => {
			await page.getByRole('button', { name: 'Try again' }).click();
			await page.getByTestId('history-row').waitFor();
			await ctx.expect(ctx.activity.eventReads >= 2, 'history retry reads again');
		});
		await ctx.step('why disclosure is keyboard operable', async () => {
			const why = page.getByTestId('history-row').locator('button.more');
			await why.focus();
			await page.keyboard.press('Enter');
			await ctx.expect(await why.getAttribute('aria-expanded') === 'true', 'history reason expands');
			await ctx.expect(await page.getByText('Preview failure').isVisible(), 'history reason is shown');
		});
		await ctx.step('invalid date range recovers when corrected', async () => {
			await page.locator('#h-from').fill('2026-10-17');
			await page.locator('#h-from').blur();
			await page.locator('#h-to').fill('2026-10-01');
			await page.locator('#h-to').blur();
			await page.locator('#history-date-error').waitFor();
			await page.locator('#h-to').fill('2026-10-31');
			await page.locator('#h-to').blur();
			await page.locator('#history-date-error').waitFor({ state: 'hidden' });
		});
	}
};

const wishlistActions = {
	name: 'Wishlist messages, approval, denial and take back',
	route: '/wishlist',
	setup: async (ctx) => {
		const initial = [ask(11, 'Approve Me'), ask(12, 'Turn Me Down'), ask(13, 'Take Me Back', 'asked', true)];
		ctx.activity = { read: 0, approve: 0, deny: 0, takeBack: 0 };
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'GET' && path === '/asks') return { json: { asks: initial } };
			if (method === 'GET' && path === '/messages') return { json: { unread: 1, waiting: 3, messages: [{ id: 1, kind: 'approved', text: 'One request changed.', created_at: now }] } };
			if (method === 'GET' && path === '/library-folders') return { json: { library_folders: [{ id: 1, path: '/preview/movies', kind: 'movie', label: 'Movies', default_pick_rules_id: 1, free_space: 1, health: 'ok', health_reason: '' }] } };
			if (method === 'GET' && path === '/pick-rules') return { json: { pick_rules: [{ id: 1, name: 'Balanced', kinds: ['movie'], preset: true }] } };
			if (method === 'POST' && path === '/messages/read') {
				ctx.activity.read += 1;
				if (ctx.activity.read === 1) return { delay: 120, status: 500, json: { error: 'Preview read failure' } };
				return { delay: 120, json: {} };
			}
			if (method === 'POST' && path === '/asks/11/approve') { ctx.activity.approve += 1; return { delay: 120, json: { ask: { ...initial[0], state: 'approved', state_label: 'Approved', decided_by: person(1, 'owner'), decided_at: now, note: body?.note ?? '' } } }; }
			if (method === 'POST' && path === '/asks/12/turn-down') { ctx.activity.deny += 1; return { delay: 120, json: { ask: { ...initial[1], state: 'turned_down', state_label: 'Turned down', decided_by: person(1, 'owner'), decided_at: now, note: body?.note ?? '' } } }; }
			if (method === 'DELETE' && path === '/asks/13') { ctx.activity.takeBack += 1; return { delay: 120, json: {} }; }
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('mark all read reports failure and retries once', async () => {
			const read = page.getByRole('button', { name: 'Mark all read' });
			await read.click();
			await ctx.expect(await read.isDisabled(), 'Mark all read disables during request');
			await page.getByText('Preview read failure').waitFor();
			await ctx.expect(!(await read.isDisabled()), 'Mark all read re-enables after failure');
			await read.click();
			await ctx.expect(await read.isDisabled(), 'Mark all read disables during retry');
			await read.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.read === 2, 'messages read retries only after the explicit second click');
		});
		await ctx.step('approve dialog submits its form once', async () => {
			await page.getByTestId('ask').filter({ hasText: 'Approve Me' }).getByRole('button', { name: 'Approve', exact: true }).click();
			const dialog = page.getByRole('dialog', { name: /Approve Approve Me/ });
			await dialog.getByLabel('Note (optional)').fill('Looks good');
			const submit = dialog.getByRole('button', { name: 'Approve and add' });
			await submit.click();
			await ctx.expect(await submit.isDisabled(), 'Approve submit disables during request');
			await dialog.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.approve === 1, 'approval posts once');
		});
		await ctx.step('turn down dialog submits its reason once', async () => {
			await page.getByTestId('ask').filter({ hasText: 'Turn Me Down' }).getByRole('button', { name: 'Turn down' }).click();
			const dialog = page.getByRole('dialog', { name: /Turn down Turn Me Down/ });
			await dialog.getByLabel('Why (sent to the people who asked)').fill('Not a fit');
			await dialog.getByRole('button', { name: 'Turn down', exact: true }).click();
			await dialog.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.deny === 1, 'denial posts once');
		});
		await ctx.step('request review keyboard behavior matches its responsive layout', async () => {
			const review = page.getByTestId('ask').filter({ hasText: 'Take Me Back' }).getByRole('button', { name: 'Review Take Me Back' });
			await review.focus();
			await page.keyboard.press('Enter');
			const heading = page.locator('#wishlist-review').getByRole('heading', { name: /Take Me Back/ });
			await ctx.expect.poll(() => heading.evaluate((node) => document.activeElement === node)).toBe(true);
			if (ctx.width <= 600) {
				const close = page.getByRole('button', { name: 'Close review' });
				await ctx.expect(await close.isVisible(), 'compact review exposes its close control');
				await close.click();
				await ctx.expect.poll(() => review.evaluate((node) => document.activeElement === node)).toBe(true);
			} else {
				await ctx.expect(await review.getAttribute('aria-pressed') === 'true', 'inline review marks the selected request');
			}
		});
		await ctx.step('take back confirms and blocks a duplicate request', async () => {
			ctx.confirmNext(true);
			const button = page.getByTestId('ask').filter({ hasText: 'Take Me Back' }).getByRole('button', { name: 'Take back' });
			await button.click();
			await ctx.expect(await button.isDisabled(), 'Take back disables during request');
			await page.getByText(/Took back the ask/).waitFor();
			await ctx.expect(ctx.activity.takeBack === 1, 'take back deletes once');
		});
	}
};

const searchRace = {
	name: 'Search kind race and address',
	route: '/search',
	setup: async (ctx) => {
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/titles/lookup') return { delay: 300, json: { movies: [{ name: 'Stale Movie Result', year: 2026, overview: 'old', original_language: 'en', ids: { tmdb: '9001' }, never_add: false }], attribution: 'Preview fixture' } };
			if (method === 'GET' && path === '/shows/lookup') return { delay: 30, json: { shows: [{ source: 'tmdb', name: 'Current Show Result', year: 2026, overview: 'new', status: 'running', ids: { tmdb: '9002' }, seasons: [{ number: 1 }] }], attribution: 'Preview fixture' } };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('newer kind search wins after the slower response completes', async () => {
			const input = page.getByRole('searchbox');
			await input.fill('preview query');
			await page.getByRole('button', { name: 'Search', exact: true }).click();
			await page.getByRole('button', { name: 'Shows', exact: true }).click();
			await page.getByText('Current Show Result').waitFor();
			await page.waitForTimeout(350);
			await ctx.expect(await page.getByText('Current Show Result').isVisible(), 'newer show result remains after old response');
			await ctx.expect(await page.getByText('Stale Movie Result').count() === 0, 'stale movie response is ignored');
			const current = new URL(page.url());
			await ctx.expect(current.searchParams.get('kind') === 'show' && current.searchParams.get('q') === 'preview query', 'Search URL keeps current kind and query');
		});
	}
};

const clearedSearchIgnoresPendingFailure = {
	name: 'Clearing query before changing Search kind ignores pending errors',
	route: '/search?kind=music',
	setup: async (ctx) => {
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/artists/lookup') return { delay: 300, status: 500, json: { error: 'Preview delayed MusicBrainz failure' } };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('clearing the query and changing kind invalidates the pending request', async () => {
			const input = page.getByRole('searchbox');
			const submit = page.getByRole('button', { name: 'Search', exact: true });
			await input.fill('pending artist');
			await submit.click();
			await input.fill('');
			await page.getByRole('button', { name: 'Movies', exact: true }).click();
			await page.waitForTimeout(350);
			const current = new URL(page.url());
			await ctx.expect(!current.searchParams.has('kind') && !current.searchParams.has('q'), 'Search URL keeps the empty Movies view');
			await ctx.expect(await page.getByRole('heading', { name: 'Look for a movie' }).isVisible(), 'empty Movies view remains visible');
			await ctx.expect(await page.getByText('Preview delayed MusicBrainz failure').count() === 0, 'pending music failure does not surface in the Movies view');
			await ctx.expect(await submit.isDisabled(), 'Search remains disabled for an empty query');
			await ctx.expect(await submit.getAttribute('aria-busy') === 'false', 'Search is no longer busy after changing kind');
		});
	}
};

const ownerAddDialog = {
	name: 'Owner Search Add dialog',
	route: '/search',
	setup: async (ctx) => {
		ctx.activity = { added: 0, addedBody: null };
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'GET' && path === '/auth/me') return { json: { id: 1, name: 'preview-owner', via: 'session', csrf: 'preview-only', role: 'owner', perms: ['ask', 'approve', 'manage_library', 'settings'] } };
			if (method === 'GET' && path === '/titles/lookup') return { json: { movies: [{ name: 'Addable Preview Movie', year: 2026, overview: 'A fixture for the real Add form.', original_language: 'en', ids: { tmdb: '9200' }, never_add: false }], attribution: 'Preview fixture' } };
			if (method === 'GET' && path === '/library-folders') return { json: { library_folders: [{ id: 1, path: '/preview/movies', kind: 'movie', label: 'Movies', default_pick_rules_id: 1, free_space: 1, health: 'ok', health_reason: '' }] } };
			if (method === 'GET' && path === '/pick-rules') return { json: { pick_rules: [{ id: 1, name: 'Balanced', kinds: ['movie'], preset: true }] } };
			if (method === 'GET' && path === '/settings/release-gate') return { json: { release_gate: 'released', gate_delay_days: 0, bulk_search_respects_gate: true } };
			if (method === 'POST' && path === '/titles') {
				ctx.activity.added += 1;
				ctx.activity.addedBody = body;
				return { delay: 120, json: { title: { id: 920, name: 'Addable Preview Movie', year: 2026 } } };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('owner configures and submits Add dialog once', async () => {
			await page.getByRole('searchbox').fill('Addable Preview Movie');
			await page.getByRole('button', { name: 'Search', exact: true }).click();
			const hit = page.locator('li.hit').filter({ hasText: 'Addable Preview Movie' });
			await hit.getByRole('button', { name: 'Add', exact: true }).click();
			const dialog = page.getByRole('dialog', { name: 'Add Addable Preview Movie' });
			await ctx.expect(await dialog.getByLabel('Library folder').inputValue() === '1', 'Add dialog selects a usable folder');
			await ctx.expect(await dialog.getByLabel('Pick rules', { exact: true }).inputValue() === '1', 'Add dialog selects compatible Pick rules');
			await dialog.getByLabel('Search now').uncheck();
			const submit = dialog.getByRole('button', { name: 'Add to Library' });
			await submit.click();
			await ctx.expect(await submit.isDisabled(), 'Add submit disables during request');
			await dialog.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.added === 1, 'Add posts once');
			await ctx.expect(ctx.activity.addedBody?.search_on_add === false, 'Add form sends the selected Search now value');
			await ctx.expect(await hit.getByRole('link', { name: 'In your Library' }).isVisible(), 'Added result changes to a Library link');
		});
	}
};

const memberAskDialog = {
	name: 'Member Search Ask dialog',
	route: '/search?kind=show',
	setup: async (ctx) => {
		ctx.activity = { asked: 0 };
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'GET' && path === '/auth/me') return { json: { id: 42, name: 'preview-member', via: 'session', csrf: 'preview-only', role: 'member', perms: ['ask'] } };
			if (method === 'GET' && path === '/parts') return { json: { parts: [], screens: {}, safe_mode: false } };
			if (method === 'GET' && path === '/messages') return { json: { unread: 0, waiting: 0, messages: [] } };
			if (method === 'GET' && path === '/asks/limit') return { json: { no_limit: true, movies: { per_week: 0, used: 0, left: 0, no_limit: true }, shows: { per_week: 0, used: 0, left: 0, no_limit: true }, music: { per_week: 0, used: 0, left: 0, no_limit: true } } };
			if (method === 'GET' && path === '/shows/lookup') return { json: { shows: [{ source: 'tmdb', name: 'Askable Preview Show', year: 2026, status: 'running', ids: { tmdb: '9100' }, seasons: [{ number: 1 }, { number: 2 }] }] } };
			if (method === 'POST' && path === '/asks') {
				ctx.activity.asked += 1;
				return { delay: 120, json: { ask: { ...ask(41, 'Askable Preview Show', 'asked', true), kind: 'show', ref: 'tmdb:9100', seasons: body?.seasons ?? [] }, joined: false, already: false } };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('member chooses seasons and submits Ask dialog once', async () => {
			await page.getByRole('searchbox').fill('Askable Preview Show');
			await page.getByRole('button', { name: 'Search', exact: true }).click();
			await page.getByRole('button', { name: 'Ask', exact: true }).click();
			const dialog = page.getByRole('dialog', { name: 'Ask for Askable Preview Show' });
			await dialog.getByLabel('Only some seasons').check();
			await dialog.getByLabel('Season 2').check();
			const submit = dialog.getByRole('button', { name: 'Ask for season 2' });
			await submit.click();
			await ctx.expect(await submit.isDisabled(), 'Ask submit disables during request');
			await dialog.waitFor({ state: 'hidden' });
			await ctx.expect(ctx.activity.asked === 1, 'Ask posts once');
		});
	}
};

const missingSubtitlesSearchAll = {
	name: 'Missing Subtitles Search all duplicate guard',
	route: '/library/missing-subtitles',
	setup: async (ctx) => {
		ctx.activity = { searchAll: 0 };
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/subtitles/missing') return { json: {
				missing: [{ file_id: 611, item_id: 612, title_id: 76, title: 'Signal Garden', kind: 'movie', language: 'es', forced: false, hearing_impaired: false, state: 'wanted', tries: 0 }],
				total: 1,
				titles: [{ id: 76, name: 'Signal Garden', kind: 'movie', count: 1 }]
			} };
			if (method === 'GET' && path === '/subtitles/languages') return { json: { languages: [{ code: 'es', name: 'Spanish' }] } };
			if (method === 'POST' && path === '/subtitles/missing/search') {
				ctx.activity.searchAll += 1;
				return { delay: 200, json: { queued: 1, job_ids: [1601] } };
			}
			if (method === 'GET' && path === '/jobs') return { json: { jobs: [{ ID: 1601, State: 'running' }] } };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('Search all becomes busy immediately and ignores a second activation', async () => {
			const button = page.getByTestId('search-all-subtitles');
			await button.click();
			await page.waitForFunction(() => document.querySelector('[data-testid="search-all-subtitles"]')?.matches(':disabled'));
			await ctx.expect(await button.isDisabled(), 'Search all disables while its request is pending');
			await button.evaluate((node) => node.click());
			await page.waitForTimeout(250);
			await ctx.expect(ctx.activity.searchAll === 1, 'Search all posts once during a delayed response');
			await page.getByTestId('search-all-progress').waitFor();
		});
	}
};

const subtitleWishRejectedSave = {
	name: 'Title subtitle languages rejected save and cancel',
	route: '/library/1',
	setup: async (ctx) => {
		ctx.activity = { saves: 0 };
		ctx.intercept(async ({ method, path }) => {
			if (method === 'PUT' && path === '/titles/1/subtitle-wish') {
				ctx.activity.saves += 1;
				return { delay: 120, status: 503, json: { error: 'Preview subtitle save rejected' } };
			}
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('subtitle language editor keeps rejected changes recoverable and cancels cleanly', async () => {
			const panel = page.getByTestId('subtitles').first();
			await panel.getByTestId('subtitle-wish-change').click();
			const form = panel.getByRole('form', { name: 'Subtitle languages' });
			await form.getByLabel(/Its own languages/).check();
			if ((await form.getByTestId('wish-row').count()) === 0) await form.getByRole('button', { name: 'Add a language' }).click();
			await form.getByLabel('Hearing-impaired').first().selectOption('either');
			const submit = form.getByRole('button', { name: 'Save languages' });
			await submit.click();
			await ctx.expect(await submit.isDisabled(), 'subtitle language save exposes its pending state');
			await form.getByText('Preview subtitle save rejected').waitFor();
			await ctx.expect(ctx.activity.saves === 1, 'rejected subtitle language save submits once');
			await ctx.expect(await form.getByLabel('Hearing-impaired').first().inputValue() === 'either', 'rejected save keeps the edited value available');
			await form.getByRole('button', { name: 'Cancel' }).click();
			await form.waitFor({ state: 'hidden' });
			await ctx.expect(await panel.getByTestId('subtitle-wish-change').isVisible(), 'cancel returns to the subtitle summary');
		});
	}
};

module.exports = [downloadsActions, decisionsActions, historyRecovery, wishlistActions, searchRace, clearedSearchIgnoresPendingFailure, ownerAddDialog, memberAskDialog, missingSubtitlesSearchAll, subtitleWishRejectedSave];
