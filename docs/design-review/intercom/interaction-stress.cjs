const previewOrigin = 'http://127.0.0.1:18746';

const fixtureOwner = {
	id: 990001,
	name: 'Preview stress owner',
	via: 'session',
	csrf: 'preview-stress-only',
	role: 'owner',
	perms: ['ask', 'approve', 'manage_library', 'settings'],
	local_recent: true
};

function fixtureSlot(index) {
	const missing = index % 2 === 0;
	return {
		id: 8_000_000 + index,
		version: 'Main',
		version_id: 8_100_000 + index,
		state: missing ? 'wanted' : 'in_library',
		state_label: missing ? 'Missing' : 'In library',
		reason: missing ? 'Waiting for a matching release.' : 'Ready in the disposable preview.'
	};
}

function searchMatches(title, value) {
	const words = value.toLowerCase().trim().split(/\s+/).filter(Boolean).slice(0, 16);
	const name = title.name.toLowerCase();
	return words.every((word) => name.includes(word) || String(title.year ?? '') === word);
}

const scenarios = [
	{
		name: 'Library service failure is distinct from empty and recovers',
		route: '/library',
		async setup({ intercept }) {
			let titleCalls = 0;
			intercept(async ({ method, path, url }) => {
				if (method === 'GET' && path === '/auth/me') return { json: fixtureOwner };
				if (method !== 'GET' || path !== '/titles' || url.search) return null;
				titleCalls += 1;
				if (titleCalls === 1) return { status: 503, json: { error: 'Preview Library service unavailable.' } };
				return { json: { titles: [] } };
			});
		},
		async run({ page, expect, step }) {
			await step('show a real load failure instead of the empty state', async () => {
				const alert = page.getByRole('alert');
				await expect(alert).toContainText('Preview Library service unavailable.');
				await expect(page.getByRole('heading', { name: 'Your Library is empty' })).toHaveCount(0);
				await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
			});
			await step('retry into the recovered empty Library', async () => {
				await page.getByRole('button', { name: 'Try again' }).click();
				await expect(page.getByRole('heading', { name: 'Your Library is empty' })).toBeVisible();
				await expect(page.getByRole('alert')).toHaveCount(0);
				await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
			});
		}
	},
	{
		name: 'Library thousand-title stress preserves controls and fit',
		route: '/library?kind=movie',
		async setup({ page, intercept }) {
			const response = await fetch(`${previewOrigin}/api/v1/titles`, { headers: { accept: 'application/json' } });
			if (!response.ok) throw new Error(`Could not read disposable preview titles: ${response.status}`);
			const payload = await response.json();
			const base = payload.titles?.find((title) => title.kind === 'movie');
			if (!base) throw new Error('Disposable preview has no movie TitleView to clone for the stress fixture');

			const longWord = 'UnbrokenMetadataWord'.repeat(8);
			const titles = Array.from({ length: 1000 }, (_, index) => ({
				...base,
				id: 7_000_000 + index,
				name: index === 0
					? 'Newest Fixture Title'
					: index === 999
						? `The ${longWord} Library Stress Title With A Long Secondary Name`
						: `Fixture Movie ${String(index).padStart(4, '0')}`,
				year: index === 0 ? 2099 : 1980 + (index % 45),
				overview: index % 7 === 0 ? undefined : `Disposable stress metadata ${index}`,
				ids: index % 11 === 0 ? null : { preview: String(7_000_000 + index) },
				artwork: index % 3 === 0 ? [] : base.artwork.slice(0, 1),
				versions: [],
				slots: [fixtureSlot(index)],
				asks: index % 5 === 0 ? [{
					id: 7_500_000 + index,
					kind: 'movie',
					tmdb_id: String(7_000_000 + index),
					name: `Fixture Movie ${String(index).padStart(4, '0')}`,
					year: 1980 + (index % 45),
					state: 'approved',
					state_label: 'Approved',
					title_id: 7_000_000 + index,
					asked_by: { id: 990002, name: 'Preview household member' },
					asked_at: '2026-10-17T12:00:00Z',
					people: []
				}] : []
			}));

			await page.addInitScript(() => localStorage.setItem('holyarrs.library-view', 'list'));
			intercept(async ({ method, path, url }) => {
				if (method === 'GET' && path === '/auth/me') return { json: fixtureOwner };
				if (method !== 'GET' || path !== '/titles') return null;
				const query = url.searchParams.get('q') ?? '';
				return { json: { titles: query ? titles.filter((title) => searchMatches(title, query)) : titles } };
			});
		},
		async run({ page, expect, step }) {
			const rows = page.getByTestId('library-row');
			const longNeedle = 'UnbrokenMetadataWord'.repeat(8);

			await step('render and count one thousand partial-metadata titles', async () => {
				await expect(page.getByRole('button', { name: 'List' })).toHaveAttribute('aria-pressed', 'true');
				await expect(rows).toHaveCount(1000);
				await expect(page.getByText('1000 titles in this view')).toBeVisible();
				await expect(page.getByRole('button', { name: 'Remove type filter' })).toContainText('Movies');
				await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
			});

			await step('sort by newest year without losing the full result count', async () => {
				await page.getByLabel('Sort titles').selectOption('year');
				await expect(rows.first().locator('.row-link b')).toHaveText('Newest Fixture Title');
				await expect(rows).toHaveCount(1000);
			});

			await step('search a long unbroken value and expose removable chips', async () => {
				await page.getByLabel('Find in the Library').fill(longNeedle);
				await expect(rows).toHaveCount(1);
				await expect(rows.first()).toContainText(longNeedle);
				await expect(page.getByText('1 title in this view')).toBeVisible();
				await expect(page.getByRole('button', { name: 'Remove search filter' })).toContainText(`Search: ${longNeedle}`);
				await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
			});

			await step('remove one chip, apply status, then clear every active filter', async () => {
				await page.getByRole('button', { name: 'Remove search filter' }).click();
				await expect(rows).toHaveCount(1000);
				const statusMenu = page.locator('details.status-filter').first();
				await statusMenu.locator('summary').click();
				await expect(statusMenu).toHaveAttribute('open', '');
				await expect(page.locator('#status-value')).toBeVisible();
				await page.locator('#status-value').selectOption('missing');
				await expect(rows).toHaveCount(500);
				await expect(page.getByRole('button', { name: 'Remove status filter' })).toContainText('Missing');
				await page.getByRole('button', { name: 'Clear filters' }).click();
				await expect(rows).toHaveCount(1000);
				await expect(page.getByRole('group', { name: 'Active filters' })).toHaveCount(0);
				await expect(page.getByLabel('Find in the Library')).toHaveValue('');
				await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
				await expect(page).toHaveURL(/\/library(?:\?sort=year)?$/);
			});
		}
	}
];

module.exports = { scenarios };
