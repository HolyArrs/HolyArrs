/**
 * Settings interaction scenarios for interaction-runner.cjs.
 *
 * Every mutation in this file is fulfilled by the browser runner. The preview
 * backend never receives a password, key, setting change, or destructive
 * control request.
 */

const visible = async (ctx, locator, label) => {
	await ctx.expect(locator, `${label} is visible`).toBeVisible();
};

const hidden = async (ctx, locator, label) => {
	await ctx.expect(locator, `${label} is hidden`).toBeHidden();
};

const emptyEditor = ({ name, route, add, heading, save, cancel, required, clearRequiredLabel, setup }) => ({
	name,
	route,
	setup,
	run: async (ctx) => {
		const { page } = ctx;
		await ctx.step('open editor', async () => {
			await page.getByRole('button', { name: add, exact: true }).first().click();
			await visible(ctx, page.getByRole('heading', { name: heading, exact: true }), heading);
		});
		await ctx.step('empty required state', async () => {
			const submit = page.getByRole('button', { name: save, exact: true });
			if (required) {
				if (clearRequiredLabel) await page.getByLabel(clearRequiredLabel, { exact: true }).fill('');
				if (await submit.isDisabled()) await ctx.expect(true, `${save} is disabled while required fields are empty`);
				else {
					await submit.click();
					const form = page.getByRole('heading', { name: heading, exact: true }).locator('xpath=ancestor::form[1]');
					await ctx.expect((await form.locator(':invalid').count()) > 0, `${save} is blocked by native required-field validation`);
				}
			}
		});
		await ctx.step('cancel editor', async () => {
			await page.getByRole('button', { name: cancel, exact: true }).first().click();
			await hidden(ctx, page.getByRole('heading', { name: heading, exact: true }), `${heading} editor`);
		});
	}
});

const settingsSectionPicker = {
	name: 'settings section navigation',
	route: '/settings',
	run: async (ctx) => {
		const { page } = ctx;
		if (ctx.width <= 1000) {
			const picker = page.getByLabel('Settings section', { exact: true });
			await visible(ctx, picker, 'settings section picker');
			await picker.selectOption('/settings/search');
			await page.waitForURL(/\/settings\/search$/);
		} else {
			const link = page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Search', exact: true });
			await visible(ctx, link, 'desktop Search settings link');
			await link.click();
			await page.waitForURL(/\/settings\/search$/);
		}
		await ctx.expect(await page.getByRole('heading', { name: 'Search', exact: true }).isVisible(), 'selected settings section loads');
	}
};

const sourceSaveFailure = {
	name: 'Source form validation, pending state and recovery',
	route: '/settings/sources',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/sources', { sources: [] });
		ctx.intercept(async ({ method, path }) => {
			if (method === 'POST' && path.endsWith('/sources')) return { status: 422, json: { error: 'Preview rejected this Source without saving it.' }, delay: 180 };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: 'Add a Source', exact: true }).first().click();
		const save = page.getByRole('button', { name: 'Save Source', exact: true });
		await ctx.expect(await save.isDisabled(), 'Source save starts disabled');
		await page.getByLabel('Name', { exact: true }).fill('Interaction fixture');
		await page.getByLabel('Address', { exact: true }).fill('https://example.invalid/api');
		await ctx.expect(!(await save.isDisabled()), 'Source save enables with required fields');
		await save.click();
		await ctx.expect.poll(() => save.getAttribute('aria-busy')).toBe('true');
		await visible(ctx, page.getByRole('alert').filter({ hasText: 'Preview rejected this Source' }), 'Source save error');
		await ctx.expect(!(await save.isDisabled()), 'Source form recovers after failure');
	}
};

const connectionEditors = [
	emptyEditor({
		name: 'Downloader add form cancellation', route: '/settings/downloaders', add: 'Add a Downloader', heading: 'Add a Downloader', save: 'Save Downloader', cancel: 'Cancel', required: true, clearRequiredLabel: 'Name',
		setup: async (ctx) => {
			await ctx.routeJSON('GET', '/downloaders', { downloaders: [] });
			await ctx.routeJSON('GET', '/settings/downloaders', { fail_bad_files: true, auto_retry: true });
		}
	}),
	emptyEditor({
		name: 'Alert add form cancellation', route: '/settings/alerts', add: 'Add an Alert', heading: 'Add an Alert', save: 'Save Alert', cancel: 'Cancel', required: true,
		setup: async (ctx) => ctx.routeJSON('GET', '/alerts', { alerts: [] })
	}),
	emptyEditor({
		name: 'Media server add form cancellation', route: '/settings/media-servers', add: 'Add a media server', heading: 'Add a media server', save: 'Save', cancel: 'Cancel', required: true,
		setup: async (ctx) => ctx.routeJSON('GET', '/media-servers', { media_servers: [] })
	}),
	emptyEditor({
		name: 'Library folder validation and cancellation', route: '/settings/library-folders', add: 'Add a folder', heading: 'New Library folder', save: 'Add and check', cancel: 'Cancel', required: false,
		setup: async (ctx) => ctx.routeJSON('GET', '/library-folders', { library_folders: [] })
	})
];

const libraryFolderValidation = {
	name: 'Library folder inline validation recovery',
	route: '/settings/library-folders',
	setup: async (ctx) => ctx.routeJSON('GET', '/library-folders', { library_folders: [] }),
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: 'Add a folder', exact: true }).first().click();
		await page.getByRole('button', { name: 'Add and check', exact: true }).click();
		await visible(ctx, page.getByRole('alert').filter({ hasText: 'Type the folder path' }), 'folder path validation');
		await page.getByLabel('Folder path', { exact: true }).fill('/preview-only');
		await ctx.expect(await page.getByLabel('Folder path', { exact: true }).inputValue() === '/preview-only', 'folder path remains editable after validation');
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	}
};

const userDialogs = {
	name: 'Users form validation and edit dialog keyboard dismissal',
	route: '/settings/users',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/users', {
			users: [{ id: 900001, name: 'Preview household member', role: 'member', state: 'active', perms: ['ask'], two_factor: false, password: true, links: [], created_at: '2026-10-01T00:00:00Z' }],
			perms: ['ask', 'auto_approve', 'approve', 'manage_library', 'settings']
		});
		await ctx.routeJSON('GET', '/users/invites', { invites: [] });
		await ctx.routeJSON('GET', '/ask-limits', { limits: [{ user_id: 900001, movies_per_week: null, shows_per_week: null, music_per_week: null, used: { movies: 0, shows: 0, music: 0 }, no_limit: false }] });
		await ctx.routeJSON('GET', '/settings/asks', { movies_per_week: 5, shows_per_week: 3, music_per_week: 8 });
	},
	run: async (ctx) => {
		const { page } = ctx;
		const days = page.getByLabel('Days the link lasts', { exact: true });
		await days.fill('31');
		await page.getByRole('button', { name: 'Make an invite link', exact: true }).click();
		await ctx.expect(await days.evaluate((node) => !node.validity.valid), 'invite lifetime uses native range validation');

		const change = page.getByRole('button', { name: 'Change Preview household member', exact: true });
		await change.click();
		const heading = page.getByRole('heading', { name: 'Change Preview household member', exact: true });
		await visible(ctx, heading, 'user editor heading');
		const dialog = page.locator('dialog[aria-labelledby="h-edit"]');
		await visible(ctx, dialog, 'user editor dialog');
		await ctx.expect(await dialog.getByLabel('Movie asks', { exact: true }).count() === 1, 'weekly ask limits are available');
		await page.keyboard.press('Escape');
		await ctx.expect(dialog, 'Escape closes user editor').toBeHidden();
		await ctx.expect(await change.evaluate((node) => document.activeElement === node), 'focus returns to Change button');
	}
};

const signInToggleRollback = {
	name: 'Sign-in switches show pending state and roll back rejected saves',
	route: '/settings/sign-in',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/settings', {
			skip_login_local_network: false,
			require_two_factor_admins: false,
			min_password_length: 12,
			plex: { on: false, join: 'nobody', allow: '' },
			jellyfin: { on: false, url: '', join: 'nobody', allow: '' },
			emby: { on: false, url: '', join: 'nobody', allow: '' }
		});
		await ctx.routeJSON('GET', '/auth/keys', { keys: [] });
		ctx.intercept(async ({ method, path }) => {
			if (method === 'PUT' && path.endsWith('/auth/settings')) return { status: 503, json: { error: 'Preview save intentionally rejected.' }, delay: 220 };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		for (const label of ['Skip sign-in on my local network', 'Admins must use two-factor sign-in']) {
			const toggle = page.getByLabel(label, { exact: true });
			await ctx.expect(toggle, `${label} is available`).toBeEnabled();
			await ctx.expect(!(await toggle.isChecked()), `${label} starts off`);
			const attemptsBefore = ctx.attempts.length;
			await toggle.focus();
			await page.keyboard.press('Space');
			await ctx.expect.poll(() => toggle.isDisabled()).toBe(true);
			await ctx.expect.poll(() => toggle.getAttribute('aria-busy')).toBe('true');
			await ctx.expect.poll(() => toggle.isChecked()).toBe(true);
			await ctx.expect.poll(() => ctx.attempts.length).toBe(attemptsBefore + 1);
			await toggle.evaluate((node) => node.dispatchEvent(new Event('change', { bubbles: true })));
			await page.waitForTimeout(40);
			await ctx.expect(ctx.attempts.length === attemptsBefore + 1, `${label} serializes programmatic duplicate changes`);
			await ctx.expect.poll(() => toggle.isDisabled()).toBe(false);
			await ctx.expect.poll(() => toggle.isChecked()).toBe(false);
		}
		await page.getByText('Preview save intentionally rejected.', { exact: true }).first().waitFor();
	}
};

const providerFormFailure = {
	name: 'External sign-in conditional fields and intercepted save error',
	route: '/settings/sign-in',
	setup: signInToggleRollback.setup,
	run: async (ctx) => {
		const { page } = ctx;
		const form = page.getByRole('heading', { name: 'Sign in with Jellyfin', exact: true }).locator('..');
		const enabled = form.getByLabel('Show the Jellyfin button on the sign-in page', { exact: true });
		await enabled.focus();
		await page.keyboard.press('Space');
		await form.getByLabel('Who may join the first time', { exact: true }).selectOption('list');
		const allow = form.getByLabel('Names or emails, one per line', { exact: true });
		await visible(ctx, allow, 'Jellyfin allow list');
		await allow.fill('preview@example.invalid');
		await form.getByRole('button', { name: 'Save', exact: true }).click();
		await page.getByText('Preview save intentionally rejected.', { exact: true }).waitFor();
		await ctx.expect(await allow.inputValue() === 'preview@example.invalid', 'failed provider save preserves entered values for recovery');
	}
};

const scheduleEditor = {
	name: 'Schedule inline editor validation and cancel',
	route: '/settings/schedules',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/schedules', {
			schedules: [{ kind: 'feed_check', label: 'Feed check', about: 'Reads Source feeds.', every_s: 900, min_s: 600, max_s: 7200, step_s: 300, enabled: true, state: 'waiting', part: 'search' }],
			loops: [], safe_mode: false
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const change = page.getByRole('button', { name: /Change how often .* runs/ }).first();
		await visible(ctx, change, 'schedule Change control');
		await change.click();
		const input = page.locator('form.edit input[type="number"]').first();
		await visible(ctx, input, 'schedule interval input');
		const min = Number(await input.getAttribute('min'));
		await input.fill(String(min - 1));
		await page.locator('form.edit').getByRole('button', { name: 'Save', exact: true }).click();
		await ctx.expect(await input.evaluate((node) => !node.validity.valid), 'schedule interval uses native minimum validation');
		await page.locator('form.edit').getByRole('button', { name: 'Cancel', exact: true }).click();
		await hidden(ctx, input, 'schedule editor');
	}
};

const backupPresetRollback = {
	name: 'Backups preset request serialization',
	route: '/settings/backups',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/backups', { backups: [], preset: 'balanced', restore_waiting: false, key_note: '' });
		ctx.intercept(async ({ method, path }) => {
			if (method === 'PUT' && path.endsWith('/backups/settings')) return { status: 503, json: { error: 'Preview preset save intentionally rejected.' }, delay: 220 };
			return null;
		});
	},
	run: async (ctx) => {
		const { page } = ctx;
		const balanced = page.getByLabel(/Balanced/);
		const light = page.getByLabel(/Light/);
		await ctx.expect(await balanced.isChecked(), 'Balanced preset starts selected');
		await light.click();
		await ctx.expect.poll(() => light.isChecked()).toBe(true);
		await ctx.expect.poll(async () => await balanced.isDisabled() && await light.isDisabled()).toBe(true);
		await page.getByText('Preview preset save intentionally rejected.', { exact: true }).waitFor();
		await ctx.expect(await balanced.isChecked(), 'failed preset save rolls back to Balanced');
		await ctx.expect(!(await light.isDisabled()), 'preset radios recover after failure');
	}
};

const partsConfirmation = {
	name: 'Parts destructive confirmation can be cancelled without a second request',
	route: '/settings/parts',
	setup: async (ctx) => {
		let calls = 0;
		ctx.intercept(async ({ method, path }) => {
			if (method === 'POST' && path.endsWith('/app/shutdown')) {
				calls += 1;
				return { status: 200, json: { confirm: 'preview-confirm', text: 'Shut down the preview?' }, delay: 100 };
			}
			return null;
		});
		ctx._shutdownCalls = () => calls;
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.getByRole('button', { name: 'Shut down', exact: true }).click();
		const prompt = page.getByRole('alertdialog');
		await visible(ctx, prompt, 'shutdown confirmation');
		await ctx.expect(await prompt.getByRole('button', { name: /^Yes, shut down$/ }).evaluate((node) => document.activeElement === node), 'confirmation receives focus');
		await prompt.getByRole('button', { name: 'Cancel', exact: true }).click();
		await hidden(ctx, prompt, 'shutdown confirmation');
		await ctx.expect(ctx._shutdownCalls() === 1, 'cancel does not send the confirmed shutdown request');
	}
};

module.exports = [
	settingsSectionPicker,
	sourceSaveFailure,
	...connectionEditors,
	libraryFolderValidation,
	userDialogs,
	signInToggleRollback,
	providerFormFailure,
	scheduleEditor,
	backupPresetRollback,
	partsConfirmation
];
