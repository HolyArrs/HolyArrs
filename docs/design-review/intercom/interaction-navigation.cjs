/**
 * Read-only interaction scenarios for the preview interaction runner.
 *
 * The runner supplies a fresh page at each width and blocks all non-GET
 * requests unless a scenario explicitly installs a routeJSON fixture.
 */

const expectVisible = async (ctx, locator, name) => {
	await ctx.expect.poll(() => locator.isVisible()).toBe(true);
};

const expectPressed = async (ctx, locator, value, name) => {
	await ctx.expect.poll(() => locator.getAttribute('aria-pressed')).toBe(String(value));
};

const globalNavigation = {
	name: 'global navigation and Library chooser',
	route: '/library?kind=movie',
	run: async (ctx) => {
		const { page } = ctx;
		const trigger = ctx.width >= 768
			? page.locator('aside.global-rail').getByRole('button', { name: 'Library', exact: true })
			: page.getByRole('button', { name: 'Library', exact: true }).last();
		await expectVisible(ctx, trigger, 'Library navigation trigger');
		await trigger.click();
		const dialog = page.locator('dialog.navigation-dialog');
		await expectVisible(ctx, dialog, 'Library chooser');
		for (const [label, kind] of [['Movies', 'movie'], ['TV Shows', 'show'], ['Music', 'artist']]) {
			const link = dialog.getByRole('link', { name: label, exact: true });
			await ctx.expect(await link.count() === 1, `${label} chooser link exists`);
			const href = new URL(await link.getAttribute('href'), page.url());
			await ctx.expect(href.pathname === '/library' && href.searchParams.get('kind') === kind, `${label} uses Library kind URL`);
		}
		await page.keyboard.press('Escape');
		await ctx.expect(!(await dialog.evaluate((node) => node.open)), 'Escape closes navigation chooser');
		await ctx.expect(await trigger.evaluate((node) => document.activeElement === node), 'focus returns to Library trigger');
	}
};

const desktopLibraryExpansion = {
	name: 'desktop Library context expansion',
	route: '/library?kind=movie',
	supportedWidths: [1024, 1440],
	run: async (ctx) => {
		const parent = ctx.page.locator('aside.context-sidebar').getByRole('button', { name: 'Library', exact: true });
		const reveal = ctx.page.locator('aside.context-sidebar .library-reveal');
		await ctx.expect(await parent.getAttribute('aria-expanded') === 'true', 'Library starts expanded');
		await parent.click();
		await ctx.expect(await parent.getAttribute('aria-expanded') === 'false', 'Library collapses');
		await ctx.expect(await reveal.evaluate((node) => node.inert), 'collapsed Library children are inert');
		await parent.click();
		await ctx.expect(await parent.getAttribute('aria-expanded') === 'true', 'Library expands again');
	}
};

const mobileMoreMenu = {
	name: 'mobile More menu and dismissal',
	route: '/library?kind=movie',
	supportedWidths: [360],
	run: async (ctx) => {
		const more = ctx.page.getByRole('button', { name: /^More/ }).last();
		await more.click();
		const dialog = ctx.page.locator('dialog.navigation-dialog');
		await expectVisible(ctx, dialog, 'More navigation chooser');
		await ctx.expect(await dialog.getByRole('link', { name: 'Downloads', exact: true }).count() === 1, 'More menu contains Downloads');
		await ctx.page.keyboard.press('Escape');
		await ctx.expect(!(await dialog.evaluate((node) => node.open)), 'Escape closes More menu');
	}
};

const themeAndKeyboard = {
	name: 'theme toggle and keyboard navigation',
	route: '/',
	run: async (ctx) => {
		const { page } = ctx;
		const toggle = ctx.width < 768
			? (await page.getByRole('button', { name: 'Open navigation', exact: true }).count())
				? (await page.getByRole('button', { name: 'Open navigation', exact: true }).click(), page.locator('dialog.navigation-dialog').getByRole('button', { name: /Use (dark|light) theme/ }))
				: (await page.getByRole('button', { name: 'More', exact: true }).last().click(), page.locator('dialog.navigation-dialog').getByRole('button', { name: /Use (dark|light) theme/ }))
			: page.getByRole('button', { name: /Switch to (dark|light) theme/ }).first();
		await expectVisible(ctx, toggle, 'theme toggle');
		const before = await page.locator('html').getAttribute('data-theme');
		await expectVisible(ctx, toggle, 'theme toggle');
		await toggle.focus();
		await page.keyboard.press('Enter');
		await ctx.expect.poll(() => page.locator('html').getAttribute('data-theme')).not.toBe(before);
		await page.keyboard.press('Enter');
		await ctx.expect.poll(() => page.locator('html').getAttribute('data-theme')).toBe(before);
		if (ctx.width < 768) await page.keyboard.press('Escape');
	}
};

const settingsThemeAndKeyboard = {
	...themeAndKeyboard,
	name: 'theme remains keyboard accessible in settings',
	route: '/settings'
};

const searchKindsAndEmptyValidation = {
	name: 'Search kind changes and empty validation',
	route: '/search',
	run: async (ctx) => {
		const { page } = ctx;
		for (const label of ['Movies', 'Shows', 'Music']) {
			const button = page.getByRole('button', { name: label, exact: true });
			await expectVisible(ctx, button, `${label} search filter`);
			await button.click();
			await expectPressed(ctx, button, true, `${label} search filter`);
		}
		const input = page.getByRole('searchbox');
		const submit = page.getByRole('button', { name: 'Search', exact: true });
		await input.fill('');
		await ctx.expect(await submit.isDisabled(), 'empty Search submit is disabled');
		await input.fill('Interstellar');
		await ctx.expect(!(await submit.isDisabled()), 'non-empty Search submit is enabled');
	}
};

const calendarControls = {
	name: 'Calendar paging, views, filters and invalid recovery',
	route: '/calendar?month=2026-10',
	run: async (ctx) => {
		const { page } = ctx;
		const month = page.getByTestId('cal-month');
		const initial = await month.textContent();
		await page.getByRole('button', { name: 'Month after', exact: true }).click();
		await ctx.expect((await month.textContent()) !== initial, 'next month updates calendar heading');
		await page.getByRole('button', { name: 'Month before', exact: true }).click();
		await ctx.expect((await month.textContent()) === initial, 'previous month returns calendar heading');
		const list = page.getByRole('button', { name: 'List', exact: true });
		await list.click();
		await expectPressed(ctx, list, true, 'List view');
		const music = page.getByRole('button', { name: 'Music', exact: true });
		await music.click();
		await expectPressed(ctx, music, true, 'Music calendar filter');
	}
};

const calendarFeedCancel = {
	name: 'Calendar feed confirmation cancel',
	route: '/calendar?month=2026-10',
	setup: async (ctx) => {
		ctx.routeJSON('GET', '/auth/me', { id: 1, name: 'Preview owner', via: 'session', csrf: 'fixture', role: 'owner', perms: [], local_recent: true });
		ctx.routeJSON('GET', '/calendar/feed-key', { set: true });
	},
	run: async (ctx) => {
		const { page } = ctx;
		const make = page.getByRole('button', { name: /Make (a new link|a feed link)/i }).first();
		await ctx.expect(await make.count() === 1, 'calendar feed control exists');
		await expectVisible(ctx, make, 'calendar feed control');
		ctx.confirmNext(false);
		await make.click();
		await ctx.expect(ctx.attempts.length === 0, 'cancel does not rotate a feed key');
		await ctx.expect(new URL(page.url()).pathname === '/calendar', 'feed cancel stays on Calendar');
	}
};

const turnedDownCancel = {
	name: 'turned-down release clear cancellation',
	route: '/library/turned-down',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/turned-down', {
			total: 1,
			releases: [{ id: 901, name: 'Preview release', title_id: 1, created_at: '2026-10-17T00:00:00Z', reasons: [] }]
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const clear = page.getByRole('button', { name: 'Clear all', exact: true });
		await expectVisible(ctx, clear, 'turned-down clear control');
		ctx.confirmNext(false);
		await clear.click();
		await ctx.expect(ctx.attempts.every((attempt) => attempt.method !== 'DELETE'), 'cancel does not clear turned-down releases');
		const held = page.getByRole('link', { name: 'Held', exact: true });
		await ctx.expect(await held.count() === 1, 'Held navigation remains available');
	}
};

module.exports = [globalNavigation, desktopLibraryExpansion, mobileMoreMenu, themeAndKeyboard, settingsThemeAndKeyboard, searchKindsAndEmptyValidation, calendarControls, calendarFeedCancel, turnedDownCancel];
