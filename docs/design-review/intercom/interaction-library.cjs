function personPage(role, movie) {
	return {
		person: {
			id: '900001',
			name: 'Jordan Vale',
			department: role === 'director' ? 'Directing' : 'Acting',
			profile: '',
			known_for: []
		},
		movies: [
			{
				name: movie,
				year: 2025,
				overview: '',
				ids: { tmdb: role === 'director' ? '900002' : '900001' },
				roles: [role === 'director' ? 'Director' : 'Actor']
			}
		],
		page: 1,
		page_size: 24,
		total_pages: 1,
		total_results: 1,
		complete: true,
		provider_pages: 1,
		attribution: 'Preview interaction fixture'
	};
}

const scenarios = [
	{
		name: 'library filters, title return and sidebar resume preserve context',
		route: '/library?kind=movie',
		async run({ page, expect, step, width }) {
			let query = '';
			let scrollY = 0;
			await step('set a searchable list view and open a title', async () => {
				await page.getByRole('button', { name: 'List' }).click();
				await expect(page.getByRole('button', { name: 'List' })).toHaveAttribute('aria-pressed', 'true');
				await expect(page.getByRole('region', { name: 'Library titles' })).toBeVisible();
				const firstTitle = page.locator('[data-testid="library-row"] .row-link').first();
				query = (await firstTitle.locator('b').innerText()).slice(0, 1);
				await page.getByLabel('Find in the Library').fill(query);
				await expect(page.getByRole('button', { name: 'Remove type filter' })).toContainText('Movies');
				await expect(page.getByRole('button', { name: 'Remove search filter' })).toContainText(`Search: ${query}`);
				const rowLinks = page.locator('[data-testid="library-row"] .row-link');
				await expect.poll(() => rowLinks.count()).toBeGreaterThan(0);
				await page.evaluate(() => document.fonts.ready);
				await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
				await page.evaluate(() => {
					window.scrollTo(0, Math.min(700, document.documentElement.scrollHeight - window.innerHeight));
				});
				await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
				let index = await rowLinks.evaluateAll((links) => links.findIndex((link) => {
					const bounds = link.getBoundingClientRect();
					return bounds.top >= 90 && bounds.bottom <= window.innerHeight - 100;
				}));
				if (index < 0) {
					await page.evaluate(() => window.scrollTo(0, 0));
					await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
					index = 0;
				}
				const selected = rowLinks.nth(index);
				await expect(selected).toBeInViewport();
				scrollY = await page.evaluate(() => window.scrollY);
				await selected.click();
				await expect(page).toHaveURL(/\/library\/\d+/);
			});
			await step('return to the same filtered position and title', async () => {
				await page.goBack();
				await expect(page.getByRole('button', { name: 'List' })).toHaveAttribute('aria-pressed', 'true');
				await expect(page.getByLabel('Find in the Library')).toHaveValue(query);
				await expect(page.getByTestId('last-opened')).toBeVisible();
				await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(scrollY);
			});
			await step('resume the same view through sidebar navigation', async () => {
				await page.locator('a[href="/"]:visible').first().click();
				await expect(page).toHaveURL(/\/$/);
				const trigger = width >= 768
					? page.locator('aside.global-rail').getByRole('button', { name: 'Library', exact: true })
					: page.getByRole('button', { name: 'Library', exact: true }).last();
				await trigger.click();
				const dialog = page.locator('dialog.navigation-dialog');
				await expect(dialog).toBeVisible();
				await dialog.getByRole('link', { name: 'Movies', exact: true }).click();
				await expect(page.getByLabel('Find in the Library')).toHaveValue(query);
				await expect(page.getByRole('button', { name: 'List' })).toHaveAttribute('aria-pressed', 'true');
				await expect(page.getByTestId('last-opened')).toBeVisible();
			});
		}
	},
	{
		name: 'desktop library preview returns focus to its trigger',
		route: '/library?kind=movie',
		minWidth: 1200,
		async run({ page, expect, step }) {
			await step('open and close a title preview with focus restored', async () => {
				const trigger = page.getByRole('button', { name: /^Preview / }).first();
				await expect(trigger).toBeVisible();
				await trigger.click();
				await expect(page.locator('#preview-title')).toBeFocused();
				await page.getByRole('button', { name: 'Close' }).click();
				await expect(trigger).toBeFocused();
			});
		}
	},
	{
		name: 'people search submits the current text after a pending query settles',
		route: '/people',
		async setup({ intercept }) {
			await intercept(async ({ method, path, url }) => {
				if (method !== 'GET' || path !== '/people/search') return null;
				const q = url.searchParams.get('q') ?? '';
				return {
					delay: q === 'Alpha' ? 180 : 0,
					status: 200,
					json: {
						people: [{ id: '900001', name: `${q} result`, department: 'Acting', profile: '', known_for: [] }],
						page: 1,
						total_pages: 1,
						total_results: 1,
						attribution: 'Preview interaction fixture'
					}
				};
			});
		},
		async run({ page, expect, step }) {
			await step('replace a pending query', async () => {
				const input = page.getByLabel('Person name');
				const alphaResponse = page.waitForResponse((response) => {
					const url = new URL(response.url());
					return url.pathname.endsWith('/api/v1/people/search') && url.searchParams.get('q') === 'Alpha';
				});
				await input.fill('Alpha');
				await page.getByRole('button', { name: 'Search' }).click();
				await input.fill('Beta');
				await alphaResponse;
				await expect(page.getByRole('heading', { name: 'Alpha result' })).toBeVisible();
				await input.press('Enter');
				await expect(input).toHaveValue('Beta');
				await expect(page.getByRole('heading', { name: 'Beta result' })).toBeVisible();
			});
		}
	},
	{
		name: 'person role changes keep the newest response',
		route: '/people/900001',
		async setup({ intercept }) {
			await intercept(async ({ method, path, url }) => {
				if (method !== 'GET' || path !== '/people/900001') return null;
				const role = url.searchParams.get('role') ?? 'all';
				if (role === 'actor') {
					return { delay: 220, status: 200, json: personPage('actor', 'Older actor response') };
				}
				if (role === 'director') return { delay: 20, status: 200, json: personPage('director', 'Newest director response') };
				return { status: 200, json: personPage('all', 'Initial filmography') };
			});
		},
		async run({ page, expect, step }) {
			await step('change roles before the first response finishes', async () => {
				await expect(page.getByRole('heading', { name: 'Initial filmography' })).toBeVisible();
				const actorResponse = page.waitForResponse((response) => {
					const url = new URL(response.url());
					return url.pathname.endsWith('/api/v1/people/900001') && url.searchParams.get('role') === 'actor';
				});
				await page.getByRole('button', { name: 'Actor' }).click();
				await page.getByRole('button', { name: 'Director' }).click();
				await expect(page.getByRole('button', { name: 'Director' })).toHaveAttribute('aria-pressed', 'true');
				await expect(page.getByRole('heading', { name: 'Newest director response' })).toBeVisible();
				await actorResponse;
				await page.waitForTimeout(50);
				await expect(page.getByRole('heading', { name: 'Older actor response' })).toHaveCount(0, { timeout: 1_000 });
				await expect(page.getByRole('heading', { name: 'Newest director response' })).toBeVisible();
			});
		}
	},
	{
		name: 'collection load failure can be retried without a stale error',
		route: '/library/collections/900001',
		async setup({ intercept }) {
			let calls = 0;
			await intercept(async ({ method, path }) => {
				if (method !== 'GET' || path !== '/collections/900001') return null;
				calls += 1;
				if (calls === 1) return { status: 500, json: { error: 'Temporary collection failure' } };
				return {
					status: 200,
					json: {
						id: 900001,
						source_id: 'preview:900001',
						name: 'The Vale Collection',
						owned_titles: 1,
						titles: [{ id: 76, name: 'Afterlight', year: 2025, state: 'in_library', version_ids: [1], files: [], protected: false }]
					}
				};
			});
		},
		async run({ page, expect, step }) {
			await step('recover from the failed initial load', async () => {
				await expect(page.getByRole('alert')).toContainText('Temporary collection failure');
				await page.getByRole('button', { name: 'Try again' }).click();
				await expect(page.getByRole('heading', { name: 'The Vale Collection' })).toBeVisible();
				await expect(page.getByRole('alert')).toHaveCount(0);
			});
		}
	},
	{
		name: 'movie edit and release dialogs cancel and report refused actions',
		route: '/library/1',
		async setup({ routeJSON }) {
			await routeJSON('GET', '/titles/1/releases', {
				releases: [{
					id: 9101,
					name: 'Quiet.Orchard.2025.1080p.WEB-DL',
					size: 3_000_000_000,
					seeders: 12,
					peers: 1,
					protocol: 'torrent',
					state: 'seen',
					found_by: 'Preview Indexer',
					source_id: 1,
					source_name: 'Preview Indexer',
					slot_id: 1,
					version: 'Main',
					title_id: 1,
					title_name: 'Quiet Orchard',
					outcome: 'passed_over',
					passed: true,
					rank: 1,
					points: 100,
					good_enough: true,
					needs_force: false,
					gate_skipped: false,
					reasons: [{ code: 'quality', effect: 'pass', text: 'Matches the selected quality.' }],
					created_at: '2026-10-17T12:00:00Z'
				}],
				total: 1,
				limit: 25,
				offset: 0,
				running_job_ids: []
			});
		},
		async run({ page, expect, step }) {
			await step('cancel movie edit', async () => {
				await page.getByRole('button', { name: 'Edit', exact: true }).click();
				const dialog = page.getByRole('dialog', { name: /^Edit / });
				await expect(dialog).toBeVisible();
				await dialog.getByRole('button', { name: 'Cancel' }).click();
				await expect(dialog).not.toBeVisible();
			});
			await step('keep a rejected release choice open with an error', async () => {
				await page.getByRole('button', { name: 'Choose Quiet.Orchard.2025.1080p.WEB-DL' }).click();
				const dialog = page.getByRole('dialog', { name: 'Choose this Release?' });
				await expect(dialog).toBeVisible();
				await dialog.getByRole('button', { name: 'Choose', exact: true }).click();
				await expect(dialog.getByRole('alert')).toBeVisible();
				await dialog.getByRole('button', { name: 'Cancel' }).click();
				await expect(dialog).not.toBeVisible();
			});
		}
	},
	{
		name: 'show season and edit controls remain operable',
		route: '/library/76',
		async run({ page, expect, step }) {
			await step('expand a season with the keyboard', async () => {
				const summary = page.getByTestId('season').first().locator('summary');
				await summary.focus();
				await summary.press('Enter');
				await expect(page.getByTestId('season').first().locator('details')).toHaveAttribute('open', '');
			});
			await step('cancel show edit', async () => {
				await page.getByRole('button', { name: 'Edit', exact: true }).click();
				const dialog = page.getByRole('dialog', { name: /^Edit / });
				await expect(dialog).toBeVisible();
				await dialog.getByRole('button', { name: 'Cancel' }).click();
				await expect(dialog).not.toBeVisible();
			});
		}
	},
	{
		name: 'artist and album edit dialogs cancel cleanly',
		route: '/library/141',
		async run({ page, expect, step }) {
			await step('cancel artist edit', async () => {
				await page.getByRole('button', { name: 'Edit album kinds' }).click();
				const dialog = page.getByRole('dialog', { name: /^Edit / });
				await expect(dialog).toBeVisible();
				await dialog.getByRole('button', { name: 'Cancel' }).click();
				await expect(dialog).not.toBeVisible();
			});
			await step('open an album from the artist page', async () => {
				const album = page.locator('a.album-name').first();
				await expect(album).toBeVisible();
				await album.click();
				await expect(page).toHaveURL(/\/library\/141\/albums\/\d+$/);
				await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
			});
		}
	}
];

module.exports = { scenarios };
