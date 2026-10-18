const assert = require('node:assert/strict');

const roleFixtures = {
	member: { id: 991001, name: 'Preview member', via: 'session', csrf: 'role-fixture', role: 'member', perms: ['ask'], local_recent: true },
	admin: { id: 991002, name: 'Preview admin', via: 'session', csrf: 'role-fixture', role: 'admin', perms: ['ask', 'auto_approve', 'approve', 'manage_library'], local_recent: true },
	settingsHelper: { id: 991003, name: 'Preview settings helper', via: 'session', csrf: 'role-fixture', role: 'member', perms: ['settings'], local_recent: true },
	settingsManager: { id: 991004, name: 'Preview settings manager', via: 'session', csrf: 'role-fixture', role: 'member', perms: ['settings', 'manage_library'], local_recent: true }
};

function roleSetup(ctx, identity) {
	ctx.intercept(async ({ method, path }) => {
		if (method === 'GET' && path === '/auth/me') return { json: identity };
		if (method === 'GET' && path === '/parts') return { json: { parts: [], screens: {}, safe_mode: false } };
		if (method === 'GET' && path === '/messages') return { json: { unread: 0, waiting: 0, messages: [] } };
		if (method === 'GET' && path === '/needs-decision') return { json: { count: 0, items: [] } };
		return null;
	});
}

async function present(expect, locator, visible, label) {
	if (visible) await expect(locator.first(), `${label} is visible`).toBeVisible();
	else await expect(locator, `${label} is absent`).toHaveCount(0);
}

async function assertShell(ctx, expected) {
	const { page, width, expect } = ctx;
	if (width < 768) {
		// Mobile gives staff one Settings destination; My account remains the
		// standalone destination only when Settings is unavailable.
		const mobileMyAccount = expected.MyAccount && !expected.Settings;
		const tabs = page.locator('nav.tabbar');
		await expect(tabs).toBeVisible();
		await present(expect, tabs.getByRole('button', { name: 'Library', exact: true }), expected.Library, 'mobile Library');
		await present(expect, tabs.getByRole('link', { name: 'Search', exact: true }), expected.Search, 'mobile Search');
		await present(expect, tabs.getByRole('link', { name: 'Wishlist', exact: true }), expected.Wishlist, 'mobile Wishlist');
		await present(expect, tabs.getByRole('link', { name: 'Downloads', exact: true }), expected.Downloads, 'mobile Downloads');
		await tabs.getByRole('button', { name: 'More', exact: true }).click();
		const menu = page.locator('dialog.navigation-dialog');
		await expect(menu).toBeVisible();
		await present(expect, menu.getByRole('link', { name: 'Home', exact: true }), expected.Home, 'More Home');
		await present(expect, menu.getByRole('link', { name: 'Search', exact: true }), expected.Search, 'More Search');
		await present(expect, menu.getByRole('link', { name: 'Wishlist', exact: true }), expected.Wishlist, 'More Wishlist');
		await present(expect, menu.getByRole('link', { name: 'Downloads', exact: true }), expected.Downloads, 'More Downloads');
		await present(expect, menu.getByRole('link', { name: 'Calendar', exact: true }), expected.Calendar, 'More Calendar');
		await present(expect, menu.getByRole('link', { name: 'Settings', exact: true }), expected.Settings, 'More Settings');
		await present(expect, menu.getByRole('link', { name: 'My account', exact: true }), mobileMyAccount, 'More My account');
		return;
	}

	const rail = page.locator('aside.global-rail');
	await expect(rail).toBeVisible();
	await present(expect, rail.getByRole('link', { name: 'Home', exact: true }), expected.Home, 'desktop Home');
	await present(expect, rail.getByRole('button', { name: 'Library', exact: true }), expected.Library, 'desktop Library');
	await present(expect, rail.getByRole('link', { name: 'Search', exact: true }), expected.Search, 'desktop Search');
	await present(expect, rail.getByRole('link', { name: 'Wishlist', exact: true }), expected.Wishlist, 'desktop Wishlist');
	await present(expect, rail.getByRole('link', { name: 'Downloads', exact: true }), expected.Downloads, 'desktop Downloads');
	await present(expect, rail.getByRole('link', { name: 'Calendar', exact: true }), expected.Calendar, 'desktop Calendar');
	await present(expect, rail.getByRole('link', { name: 'Settings', exact: true }), expected.Settings, 'desktop Settings');
	await present(expect, rail.getByRole('link', { name: 'My account', exact: true }), expected.MyAccount, 'desktop My account');
}

const scenarios = [
	{
		name: 'member shell and direct staff-route guard',
		route: '/downloads',
		setup(ctx) { roleSetup(ctx, roleFixtures.member); },
		async run(ctx) {
			await ctx.step('redirect a member away from Downloads', async () => {
				await ctx.expect(ctx.page).toHaveURL(/\/library$/);
			});
			await ctx.step('show only member destinations', async () => {
				await assertShell(ctx, { Home: false, Library: true, Search: true, Wishlist: true, Downloads: false, Calendar: false, Settings: false, MyAccount: true });
			});
		}
	},
	{
		name: 'default admin gets user administration but not Settings permission pages',
		route: '/settings/music',
		setup(ctx) { roleSetup(ctx, roleFixtures.admin); },
		async run(ctx) {
			const { page, expect } = ctx;
			await ctx.step('redirect a media-managing admin without Settings permission to Home', async () => {
				await expect(page).toHaveURL(/\/$/);
			});
			await ctx.step('show the default admin workspace destinations', async () => {
				await assertShell(ctx, { Home: true, Library: true, Search: true, Wishlist: true, Downloads: true, Calendar: true, Settings: true, MyAccount: true });
			});
			await ctx.step('limit the Settings overview to user-administration sections', async () => {
				await page.goto(new URL('/settings', page.url()).href, { waitUntil: 'networkidle' });
				const overview = page.locator('main .settings-overview');
				for (const label of ['Move from other apps', 'Parts and controls', 'Users', 'Logs', 'My account']) {
					const link = overview.getByRole('link', { name: new RegExp(`^${label}`) });
					await expect(link).toHaveCount(1);
					await expect(link).toBeVisible();
				}
				await expect(overview.getByRole('link', { name: /^Music/ })).toHaveCount(0);
				await expect(overview.getByRole('link', { name: /^Sources/ })).toHaveCount(0);
			});
		}
	},
	{
		name: 'settings-only shell presentation (frontend only; backend permission enforcement not certified)',
		route: '/settings/users',
		setup(ctx) { roleSetup(ctx, roleFixtures.settingsHelper); },
		async run(ctx) {
			const { page, expect } = ctx;
			await ctx.step('observe the frontend Users redirect without treating it as backend authorization evidence', async () => {
				await expect(page).toHaveURL(/\/library$/);
			});
			await ctx.step('show Settings without media-management destinations', async () => {
				await assertShell(ctx, { Home: false, Library: true, Search: true, Wishlist: false, Downloads: false, Calendar: false, Settings: true, MyAccount: true });
			});
			await ctx.step('show configuration sections without owner and admin controls', async () => {
				await page.goto(new URL('/settings', page.url()).href, { waitUntil: 'networkidle' });
				const overview = page.locator('main .settings-overview');
				for (const label of ['Music', 'Sources', 'My account']) {
					const link = overview.getByRole('link', { name: new RegExp(`^${label}`) });
					await expect(link).toHaveCount(1);
					await expect(link).toBeVisible();
				}
				await expect(overview.getByRole('link', { name: /^Users/ })).toHaveCount(0);
				await expect(overview.getByRole('link', { name: /^Logs/ })).toHaveCount(0);
				await expect(overview.getByRole('link', { name: /^Parts and controls/ })).toHaveCount(0);
			});
		}
	},
	{
		name: 'interrupted Music settings save returns authoritative values',
		route: '/settings/music',
		setup(ctx) {
			const defaults = {
				album_choice: 'all',
				new_albums: 'all',
				album_types: ['album', 'ep'],
				secondary_types: ['studio'],
				release_statuses: ['official'],
				pressing_rule: 'common',
				musicbrainz_url: '',
				cache_hours: 24
			};
			let releaseCommit;
			const commitGate = new Promise((resolve) => { releaseCommit = resolve; });
			ctx.musicSettings = {
				authoritative: defaults,
				puts: 0,
				putBody: null,
				committed: false,
				searchFixtureLoaded: false,
				releaseCommit
			};
			roleSetup(ctx, roleFixtures.settingsManager);
			ctx.intercept(async ({ method, path, body }) => {
				if (method === 'GET' && path === '/library-folders') {
					ctx.musicSettings.searchFixtureLoaded = true;
					return { json: { library_folders: [] } };
				}
				if (method === 'GET' && path === '/pick-rules') return { json: { pick_rules: [] } };
				if (method === 'GET' && path === '/settings/release-gate') return { json: { release_gate: 'released' } };
				if (method === 'GET' && path === '/settings/shows') return { json: {} };
				if (path !== '/settings/music') return null;
				if (method === 'GET') return { json: ctx.musicSettings.authoritative };
				if (method === 'PUT') {
					ctx.musicSettings.puts += 1;
					ctx.musicSettings.putBody = body;
					const saved = { ...ctx.musicSettings.authoritative, ...body, cache_hours: Number(body.cache_hours) };
					await commitGate;
					ctx.musicSettings.authoritative = saved;
					ctx.musicSettings.committed = true;
					return { json: saved };
				}
				return null;
			});
		},
		async run(ctx) {
			const { page, expect } = ctx;
			await ctx.step('hold one exact save request while a deterministic Search fixture loads', async () => {
				const hours = page.getByLabel('Keep answers for (hours)');
				await expect(hours).toHaveValue('24');
				await hours.fill('72');
				const save = page.getByRole('button', { name: 'Save', exact: true });
				await save.click();
				try {
					await expect.poll(() => ctx.musicSettings.puts).toBe(1);
					await expect(save).toBeDisabled();
					await expect(save).toHaveAttribute('aria-busy', 'true');
					assert.deepEqual(ctx.musicSettings.putBody, {
						album_choice: 'all',
						new_albums: 'all',
						album_types: ['album', 'ep'],
						secondary_types: ['studio'],
						release_statuses: ['official'],
						pressing_rule: 'common',
						musicbrainz_url: '',
						cache_hours: 72
					});
					assert.equal(ctx.musicSettings.committed, false);
					ctx.musicSettings.searchFixtureLoaded = false;
					await page.goto(new URL('/search', page.url()).href, { waitUntil: 'networkidle' });
					await expect(page).toHaveURL(/\/search$/);
					await expect.poll(() => ctx.musicSettings.searchFixtureLoaded).toBe(true);
					assert.equal(ctx.musicSettings.committed, false);
				} finally {
					ctx.musicSettings.releaseCommit();
				}
				await expect.poll(() => ctx.musicSettings.committed).toBe(true);
			});
			await ctx.step('return and reload the authoritative saved value', async () => {
				await page.goto(new URL('/settings/music', page.url()).href, { waitUntil: 'networkidle' });
				await expect(page.getByLabel('Keep answers for (hours)')).toHaveValue('72');
				await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled();
				await expect(page.getByRole('alert')).toHaveCount(0);
				await page.reload({ waitUntil: 'networkidle' });
				await expect(page.getByLabel('Keep answers for (hours)')).toHaveValue('72');
				await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled();
				await expect(page.getByRole('alert')).toHaveCount(0);
				await expect.poll(() => ctx.musicSettings.puts).toBe(1);
			});
		}
	}
];

module.exports = { scenarios };
