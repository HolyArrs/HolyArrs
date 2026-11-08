/**
 * Additional Settings interaction coverage for interaction-runner.cjs.
 *
 * GET fixtures avoid reading stored credentials or machine configuration. All
 * mutations are fulfilled in the runner and never reach the preview backend.
 */

const searchSettings = {
	feed_check_minutes: 15,
	missing_search_count: 20,
	missing_search_better: false,
	try_next_downloader: true,
	missing_search_shows: true,
	season_pack_upgrade: 'all',
	season_pack_prefer: 'rank',
	multi_season_packs: false,
	missing_search_music: true,
	music_discography_packs: false,
	album_track_tolerance: 0,
	refuse_early_releases: true,
	early_release_grace_hours: 24,
	refuse_tiny_releases: true,
	import_min_video_mb: 0,
	import_min_track_kb: 0,
	import_size_floors: true,
	hold_unknown_releases: false,
	episode_default_minutes: 20
};

const rejected = (ctx, method, endpoint, note) => {
	ctx.intercept(async ({ method: actual, path }) => {
		if (actual === method && path === endpoint) return { status: 503, json: { error: note }, delay: 180 };
		return null;
	});
};

const searchValidation = {
	name: 'Search settings validation and rejected save recovery',
	route: '/settings/search',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/search', searchSettings);
		rejected(ctx, 'PUT', '/settings/search', 'Preview search save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const minutes = page.getByLabel('Check Source feeds every (minutes)', { exact: true });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await minutes.fill('5');
		await save.click();
		await ctx.expect(page.getByRole('alert').filter({ hasText: 'Check feeds every 10 to 120 minutes' }), 'custom feed interval validation appears').toBeVisible();
		await ctx.expect(ctx.attempts.filter((x) => x.method === 'PUT').length === 0, 'local validation sends no request');
		await minutes.fill('15');
		await save.click();
		await ctx.expect.poll(() => save.getAttribute('aria-busy')).toBe('true');
		await page.getByRole('alert').filter({ hasText: 'Preview search save intentionally rejected.' }).waitFor();
		await ctx.expect(!(await save.isDisabled()), 'search form recovers after a rejected save');
	}
};

const musicSaveFailure = {
	name: 'Music settings successful save preserves edits',
	route: '/settings/music',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/music', {
			album_choice: 'all', new_albums: 'all', album_types: ['album'], secondary_types: ['studio'], release_statuses: ['official'],
			pressing_rule: 'common', musicbrainz_url: '', cache_hours: 24
		});
		await ctx.routeJSON('PUT', '/settings/music', {
			album_choice: 'all', new_albums: 'all', album_types: ['album'], secondary_types: ['studio'], release_statuses: ['official'],
			pressing_rule: 'common', musicbrainz_url: 'https://music.example.invalid/', cache_hours: 24
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const address = page.getByLabel(/MusicBrainz address/);
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await address.fill('https://music.example.invalid/');
		await save.click();
		await page.getByText('Music settings saved.', { exact: true }).waitFor();
		await ctx.expect(await address.inputValue() === 'https://music.example.invalid/', 'successful save preserves the normalized MusicBrainz address');
	}
};

const musicRejectedSave = {
	name: 'Music settings rejected save recovers without losing form state',
	route: '/settings/music',
	setup: async (ctx) => {
		await musicSaveFailure.setup(ctx);
		rejected(ctx, 'PUT', '/settings/music', 'Preview music save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const address = page.getByLabel(/MusicBrainz address/);
		await address.fill('https://music.example.invalid/');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview music save intentionally rejected.' }).waitFor();
		await ctx.expect(await address.inputValue() === 'https://music.example.invalid/', 'rejected save preserves MusicBrainz address');
	}
};

const wishlistValidation = {
	name: 'Requests native validation and successful save',
	route: '/settings/requests',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/asks', {
			movies_per_week: 10, shows_per_week: 10, music_per_week: 10,
			library_folder_id: 0, pick_rules_id: 0, search_on_approve: true,
			show_library_folder_id: 0, show_pick_rules_id: 0, music_library_folder_id: 0, music_pick_rules_id: 0
		});
		await ctx.routeJSON('GET', '/library-folders', { library_folders: [] });
		await ctx.routeJSON('GET', '/pick-rules', { pick_rules: [] });
		await ctx.routeJSON('PUT', '/settings/asks', {
			movies_per_week: 12, shows_per_week: 10, music_per_week: 10,
			library_folder_id: 0, pick_rules_id: 0, search_on_approve: true,
			show_library_folder_id: 0, show_pick_rules_id: 0, music_library_folder_id: 0, music_pick_rules_id: 0
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const movies = page.getByLabel('Movie asks', { exact: true });
		const save = page.getByRole('button', { name: 'Save', exact: true });
		await movies.fill('1001');
		await save.click();
		await ctx.expect(await movies.evaluate((node) => !node.validity.valid), 'weekly ask limit enforces its native maximum');
		await ctx.expect(ctx.attempts.filter((x) => x.method === 'PUT').length === 0, 'invalid weekly limit sends no request');
		await movies.fill('12');
		await save.click();
		await page.getByText('Request settings saved.', { exact: true }).waitFor();
		await ctx.expect(await movies.inputValue() === '12', 'successful save preserves the weekly limit');
	}
};

const wishlistRejectedSave = {
	name: 'Requests rejected save recovers without losing form state',
	route: '/settings/requests',
	setup: async (ctx) => {
		await wishlistValidation.setup(ctx);
		rejected(ctx, 'PUT', '/settings/asks', 'Preview wishlist save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const movies = page.getByLabel('Movie asks', { exact: true });
		await movies.fill('12');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview wishlist save intentionally rejected.' }).waitFor();
		await ctx.expect(await movies.inputValue() === '12', 'rejected save preserves weekly ask limit');
	}
};

const downloaderRejectedSave = {
	name: 'Downloader settings rejected save recovers without losing form state',
	route: '/settings/downloaders',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/downloaders', { downloaders: [] });
		await ctx.routeJSON('GET', '/settings/downloaders', {
			stalled_after_hours: 6, stalled_act_hours: 24, remove_failed: true, fail_dangerous: true,
			replace_vanished: true, grab_spacing_seconds: 2, try_next_downloader: true,
			remove_delete_files: true, remove_turn_down: false, remove_search_again: true
		});
		rejected(ctx, 'PUT', '/settings/downloaders', 'Preview downloader settings save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const stalled = page.getByLabel('Stalled after (hours without progress)', { exact: true });
		await stalled.fill('8');
		await page.getByRole('button', { name: 'Save settings', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview downloader settings save intentionally rejected.' }).waitFor();
		await ctx.expect(await stalled.inputValue() === '8', 'rejected save preserves stalled threshold');
	}
};

const namingValidation = {
	name: 'Naming local validation blocks unsafe save',
	route: '/settings/naming',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/naming', {
			naming: {
				file_pattern: '{Title} ({Year})', folder_pattern: '{Title} ({Year})', colon: 'smart', illegal: 'replace', rename_on_import: true,
				adopt_scan_limit: 5000, music_adopt_scan_limit: 200, show_folder_pattern: '{Series Title}', season_folders: true,
				season_folder_pattern: 'Season {Season Number}', specials_folder: 'Specials', episode_pattern: '{Series Title} - S{season:00}E{episode:00}',
				daily_pattern: '{Series Title} - {Air Date}', anime_pattern: '{Series Title} - {absolute:000}', multi_episode_style: 'range', air_date_style: 'dash', absolute_digits: 3,
				artist_folder_pattern: '{Artist Name}', album_folder_pattern: '{Album Title}', track_pattern: '{track:00} - {Track Title}',
				multi_disc_track_pattern: '{medium:00}-{track:00} - {Track Title}', disc_folders: false, disc_folder_pattern: 'Disc {medium:00}'
			},
			words: {}, example: 'Example (2026).mkv', example_folder: 'Example (2026)', example_episodes: {}, example_music: {}
		});
		await ctx.routeJSON('GET', '/settings/files', { holding_days: 7, min_free_mb: 100, skip_free_space_check: false });
		ctx.intercept(async ({ method, path }) => {
			if (method === 'POST' && path === '/settings/naming/example') return { status: 200, json: { example: 'Example (2026).mkv', example_folder: 'Example (2026)', example_episodes: {}, example_music: {} } };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByLabel('Keep deleted files for (days)', { exact: true }).fill('-1');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await ctx.expect(await page.getByLabel('Keep deleted files for (days)', { exact: true }).evaluate((node) => !node.validity.valid), 'holding days enforces its native minimum');
		await ctx.expect(ctx.attempts.every((x) => x.path === '/settings/naming/example'), 'invalid naming form sends only intercepted live-preview requests');
	}
};

const namingRejectedFileSave = {
	name: 'Naming file settings rejected second save preserves form state',
	route: '/settings/naming',
	setup: async (ctx) => {
		await namingValidation.setup(ctx);
		ctx.intercept(async ({ method, path, body }) => {
			if (method === 'PUT' && path === '/settings/naming') {
				return { status: 200, json: { naming: body, words: {}, example: 'Example (2026).mkv', example_folder: 'Example (2026)', example_episodes: {}, example_music: {} } };
			}
			if (method === 'PUT' && path === '/settings/files') return { status: 503, json: { error: 'Preview file settings save intentionally rejected.' }, delay: 180 };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const days = page.getByLabel('Keep deleted files for (days)', { exact: true });
		await days.fill('9');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview file settings save intentionally rejected.' }).waitFor();
		await ctx.expect(await days.inputValue() === '9', 'rejected file-settings save preserves holding days');
	}
};

const subtitleEditor = {
	name: 'Subtitle site required state and cancellation',
	route: '/settings/subtitles',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/subtitles', {
			languages: [{ language: 'en', hearing_impaired: 'either', forced: false }], pass_mark_movie: 80, pass_mark_episode: 80,
			use_built_in: true, ignore_picture_tracks: true, hi_suffix: 'hi', search_hours: 12, better: false,
			better_hours: 24, better_days: 30, batch: 20, adaptive: true, sync_from_fps: 23.976, sync_to_fps: 24, sync_step_ms: 100
		});
		await ctx.routeJSON('GET', '/subtitles/languages', { languages: [{ code: 'en', name: 'English' }] });
		await ctx.routeJSON('GET', '/subtitle-sites/kinds', { kinds: [{ kind: 'opensubtitles', name: 'OpenSubtitles', default_url: 'https://www.opensubtitles.com', movies: true, shows: true, needs_account: false }] });
		await ctx.routeJSON('GET', '/subtitle-sites', { sites: [] });
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: 'Add a Subtitle site', exact: true }).click();
		const save = page.getByRole('button', { name: 'Save Subtitle site', exact: true });
		await ctx.expect(!(await save.isDisabled()), 'default site name and address produce a ready form');
		await page.getByLabel('Name', { exact: true }).fill('');
		await ctx.expect(await save.isDisabled(), 'required site name disables save');
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await ctx.expect(await page.getByRole('heading', { name: 'Add a Subtitle site', exact: true }).count() === 0, 'cancel closes subtitle editor');
	}
};

const subtitleRejectedSettings = {
	name: 'Subtitle settings rejected save recovers without losing form state',
	route: '/settings/subtitles',
	setup: async (ctx) => {
		await subtitleEditor.setup(ctx);
		rejected(ctx, 'PUT', '/settings/subtitles', 'Preview subtitle settings save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const movies = page.getByLabel('Movies', { exact: true });
		await movies.fill('85');
		await page.getByRole('button', { name: 'Save settings', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview subtitle settings save intentionally rejected.' }).waitFor();
		await ctx.expect(await movies.inputValue() === '85', 'rejected subtitle save preserves pass mark');
	}
};

const thisAppRejectedSettings = {
	name: 'Health settings rejected save recovers without losing form state',
	route: '/settings/this-app',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/health', { ok: true, version: 'preview', schema: 1, uptime_s: 120, write_wait_p95_ms: 1, write_wait_samples: 5, writes: 10, sources: [], downloaders: [], live_clients: 1, url_base: '' });
		await ctx.routeJSON('GET', '/health/checks', { checks: [], worst: 'ok' });
		await ctx.routeJSON('GET', '/health/states', { safe_mode: false, states: [] });
		await ctx.routeJSON('GET', '/settings/health', { every_minutes: 5, flap_guard: 2, low_space_mb: 1024, update_check_url: '', update_check: true, update_check_local: false, update_prerelease: false, update_check_default: 'https://example.invalid/releases' });
		await ctx.routeJSON('GET', '/app/update', { on: true, feed: 'https://example.invalid/releases', newer: false, version: 'preview' });
		await ctx.routeJSON('GET', '/app/storage', { app: [], library_folders: [], save_path: '' });
		rejected(ctx, 'PUT', '/settings/health', 'Preview health settings save intentionally rejected.');
	},
	run: async (ctx) => {
		const { page } = ctx;
		const interval = page.getByLabel('Check every (minutes)', { exact: true });
		await ctx.expect(interval, 'health settings interval is visible').toBeVisible();
		await interval.fill('10');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Preview health settings save intentionally rejected.' }).waitFor();
		await ctx.expect(await interval.inputValue() === '10', 'rejected health save preserves interval');
	}
};

const outsideLists = {
	name: 'Outside list and Never-add validation without credential writes',
	route: '/settings/outside-lists',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/outside-lists', { outside_lists: [] });
		await ctx.routeJSON('GET', '/library-folders', { library_folders: [{ id: 400001, kind: 'movie', label: 'Preview movies', path: '/preview/movies' }] });
		await ctx.routeJSON('GET', '/pick-rules', { pick_rules: [] });
		await ctx.routeJSON('GET', '/never-add', { never_add: [] });
		await ctx.routeJSON('GET', '/settings/trakt-key', { key: '' });
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: 'Add a list', exact: true }).first().click();
		const save = page.getByRole('button', { name: 'Save list', exact: true });
		await ctx.expect(!(await save.isDisabled()), 'folder availability enables list save');
		await save.click();
		const listForm = page.getByRole('heading', { name: 'New Outside list', exact: true }).locator('xpath=ancestor::form[1]');
		await ctx.expect(await listForm.getByLabel('Name', { exact: true }).evaluate((node) => !node.validity.valid), 'outside list uses required-field validation');
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await page.getByRole('button', { name: 'Add to the Never-add list', exact: true }).click();
		await page.getByRole('alert').filter({ hasText: 'Give the TMDB number and the name.' }).waitFor();
		await ctx.expect(ctx.attempts.length === 0, 'validation and cancel send no requests');
	}
};

const moveValidation = {
	name: 'Move tool required fields block credential submission',
	route: '/settings/move',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/migrate/snapshots', { snapshots: [] });
		await ctx.routeJSON('GET', '/migrate/moves', { moves: [] });
		await ctx.routeJSON('GET', '/migrate/saved-keys', { saved: [] });
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: /Read Radarr/ }).click();
		await ctx.expect(await page.getByLabel('Address', { exact: true }).evaluate((node) => !node.validity.valid), 'address is required');
		await ctx.expect(await page.getByLabel('API key', { exact: true }).evaluate((node) => !node.validity.valid), 'API key is required');
		await ctx.expect(ctx.attempts.length === 0, 'missing credentials never submit');
	}
};

const apiDisclosure = {
	name: 'API route filter and disclosure keyboard behavior',
	route: '/settings/api',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/openapi.json', {
			'x-holyarrs-scopes': { read: 'Read preview state.' },
			paths: {
				'/titles': { get: { summary: 'List titles', description: 'Returns Library titles.', tags: ['Library'], 'x-holyarrs-scope': 'read', 'x-holyarrs-permission': 'ask' } },
				'/downloads/view': { get: { summary: 'List downloads', description: 'Returns current downloads.', tags: ['Downloads'], 'x-holyarrs-scope': 'read', 'x-holyarrs-permission': 'manage_library' } }
			}
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const search = page.getByLabel('Find a route', { exact: true });
		await search.fill('downloads');
		await page.getByText('1 of 2 routes', { exact: true }).waitFor();
		const route = page.getByRole('button', { name: /GET \/downloads\/view/ });
		await route.focus();
		await page.keyboard.press('Enter');
		await ctx.expect(await route.getAttribute('aria-expanded') === 'true', 'Enter expands an API route');
		await page.keyboard.press('Enter');
		await ctx.expect(await route.getAttribute('aria-expanded') === 'false', 'Enter collapses an API route');
	}
};

const logsFilter = {
	name: 'Logs filters submit and clear through isolated fixtures',
	route: '/settings/logs',
	setup: async (ctx) => {
		ctx.intercept(async ({ method, path }) => {
			if (method === 'GET' && path === '/logs') return { status: 200, json: { lines: [], total: 0, offset: 0, limit: 100 } };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const query = page.getByLabel('Text', { exact: true });
		await query.fill('preview-only');
		await page.getByRole('button', { name: 'Show', exact: true }).click();
		await ctx.expect.poll(() => page.getByRole('button', { name: 'Show', exact: true }).getAttribute('aria-busy')).toBe('false');
		await page.getByRole('button', { name: 'Clear', exact: true }).click();
		await ctx.expect(await query.inputValue() === '', 'Clear resets the log text filter');
		await ctx.expect(ctx.attempts.length === 0, 'log filtering performs GET requests only');
	}
};

const tmdbBoundary = {
	name: 'TMDB credential form validates locally or shows proof boundary',
	route: '/settings/tmdb',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/settings/tmdb-token', { token: '', from: 'stored' });
	},
	run: async (ctx) => {
		const { page } = ctx;
		const key = page.getByLabel('Paste the key', { exact: true });
		if (await key.count()) {
			await page.getByRole('button', { name: 'Save the key', exact: true }).click();
			await page.getByRole('alert').filter({ hasText: 'Paste the TMDB read access token.' }).waitFor();
			await ctx.expect(ctx.attempts.length === 0, 'empty credential form sends no request');
		} else {
			await ctx.expect(await page.getByRole('heading', { name: 'Confirm your password to manage this key', exact: true }).isVisible(), 'protected credential state explains proof requirement');
			await ctx.expect(await page.getByRole('link', { name: 'Open My account', exact: true }).isVisible(), 'proof state offers account recovery path');
		}
	}
};

module.exports = [
	searchValidation,
	musicSaveFailure,
	musicRejectedSave,
	wishlistValidation,
	wishlistRejectedSave,
	downloaderRejectedSave,
	namingValidation,
	namingRejectedFileSave,
	subtitleEditor,
	subtitleRejectedSettings,
	thisAppRejectedSettings,
	outsideLists,
	moveValidation,
	apiDisclosure,
	logsFilter,
	tmdbBoundary
];
