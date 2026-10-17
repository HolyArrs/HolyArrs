/**
 * Preview-only account boundary scenarios. These use synthetic GET identity
 * responses and exercise validation, cancellation and keyboard-safe controls;
 * no credential or account mutation is allowed through this module.
 */

const signedOut = {
	name: '',
	via: '',
	csrf: '',
	role: 'member',
	perms: [],
	two_factor: false,
	local_recent: false,
	auth_method: '',
	safe_mode: false
};

const owner = {
	name: 'Preview Owner',
	via: 'session',
	csrf: 'preview-csrf',
	role: 'owner',
	perms: ['ask', 'auto_approve', 'approve', 'manage_library', 'settings'],
	two_factor: false,
	local_recent: true,
	auth_method: 'local_password',
	safe_mode: false
};

const authState = { needs_setup: false, ways: ['password'], safe_mode: false, min_password_length: 8 };

const loginValidation = {
	name: 'signed-out login controls and recovery disclosure',
	route: '/login?next=%2Flibrary',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', { error: 'Sign in required.' }, 401);
		await ctx.routeJSON('GET', '/auth/state', authState);
	},
	run: async (ctx) => {
		const { page } = ctx;
		const name = page.locator('#name');
		const password = page.locator('#password');
		const submit = page.getByRole('button', { name: 'Sign in', exact: true });
		await ctx.expect(await name.count() === 1 && await password.count() === 1, 'login fields render');
		await ctx.expect(await submit.count() === 1, 'login submit renders');
		// This is a native disclosure (`summary`), not a button role in Chromium.
		const recovery = page.locator('summary').filter({ hasText: 'Forgot the password?' });
		await ctx.expect(await recovery.count() === 1, 'password recovery disclosure renders');
		await recovery.click();
		await ctx.expect(await page.getByTestId('reset-owner-help').isVisible(), 'password help disclosure opens');
		await recovery.press('Enter');
		await ctx.expect(!(await page.getByTestId('reset-owner-help').isVisible()), 'password help disclosure closes');
		await name.fill('Preview user');
		await password.fill('preview-password');
		await ctx.expect(!(await submit.isDisabled()), 'login submit enables with credentials');
	}
};

const inviteValidation = {
	name: 'invite acceptance validation and password recovery',
	route: '/invite/preview-token',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/state', authState);
		await ctx.routeJSON('GET', '/auth/invites/preview-token', { role: 'member' });
	},
	run: async (ctx) => {
		const { page } = ctx;
		const name = page.locator('#name');
		const password = page.locator('#password');
		const again = page.locator('#pwAgain');
		const submit = page.getByRole('button', { name: 'Join and sign in', exact: true });
		await ctx.expect(await submit.count() === 1, 'invite submit renders');
		await name.fill('Preview member');
		await password.fill('one-long-preview-password');
		await again.fill('different-preview-password');
		await submit.click();
		await ctx.expect(await page.getByRole('alert').filter({ hasText: 'not the same' }).isVisible(), 'invite mismatch is announced');
		await again.fill('one-long-preview-password');
		await password.fill('short');
		await submit.click();
		await ctx.expect(await page.getByRole('alert').filter({ hasText: 'at least' }).isVisible(), 'invite short password is announced');
	}
};

const loginRecovery = {
	name: 'login rejected save clears password and recovers',
	route: '/login',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', { error: 'Sign in required.' }, 401);
		await ctx.routeJSON('GET', '/auth/state', authState);
		ctx.intercept(({ method, path }) => method === 'POST' && path === '/auth/login'
			? { delay: 300, status: 503, json: { error: 'Preview sign-in rejected.' } } : null);
	},
	run: async (ctx) => {
		const { page } = ctx;
		const name = page.locator('#name');
		const password = page.locator('#password');
		const submit = page.getByRole('button', { name: 'Sign in', exact: true });
		await name.fill('preview-user');
		await password.fill('preview-password');
		await submit.click();
		await ctx.expect(submit).toBeDisabled();
		await ctx.expect(submit).toHaveAttribute('aria-busy', 'true');
		await ctx.expect.poll(() => submit.getAttribute('aria-busy')).toBe('false');
		await ctx.expect.poll(() => page.getByRole('alert').filter({ hasText: 'rejected' }).count()).toBe(1);
		await ctx.expect.poll(() => password.inputValue()).toBe('');
	}
};

const loginTwoFactor = {
	name: 'login two-factor ticket restart clears code flow',
	route: '/login',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', { error: 'Sign in required.' }, 401);
		await ctx.routeJSON('GET', '/auth/state', authState);
		await ctx.routeJSON('POST', '/auth/login', { two_factor: true, ticket: 'preview-ticket' });
	},
	run: async (ctx) => {
		const { page } = ctx;
		await page.locator('#name').fill('preview-user');
		await page.locator('#password').fill('preview-password');
		await page.getByRole('button', { name: 'Sign in', exact: true }).click();
		const code = page.locator('#otp');
		await ctx.expect.poll(() => code.count()).toBe(1);
		await code.fill('123456');
		await page.getByRole('button', { name: 'Start again', exact: true }).click();
		await ctx.expect.poll(() => page.locator('#password').count()).toBe(1);
		await ctx.expect.poll(() => code.count()).toBe(0);
	}
};

const signoutRecovery = {
	name: 'sign out rejection returns to login without an unhandled error',
	route: '/',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', owner);
		await ctx.routeJSON('GET', '/auth/state', authState);
		await ctx.routeJSON('GET', '/my/alerts', { email: '', discord_id: '' });
		await ctx.routeJSON('POST', '/auth/logout', { error: 'Preview logout rejected.' }, 503);
	},
	run: async (ctx) => {
		const { page } = ctx;
		let signout;
		if (ctx.width >= 1024) {
			signout = page.locator('aside.context-sidebar').getByRole('button', { name: 'Sign out', exact: true });
		} else if (ctx.width >= 768) {
			await page.locator('aside.global-rail').getByRole('button', { name: 'Library', exact: true }).click();
			signout = page.locator('dialog.navigation-dialog').getByRole('button', { name: 'Sign out', exact: true });
		} else {
			const open = page.getByRole('button', { name: 'Open navigation', exact: true });
			if (await open.count()) await open.click();
			else await page.getByRole('button', { name: /^More/ }).last().click();
			signout = page.locator('dialog.navigation-dialog').getByRole('button', { name: 'Sign out', exact: true });
		}
		await ctx.expect.poll(() => signout.count()).toBe(1);
		await signout.click();
		await ctx.expect.poll(() => page.locator('#password').count()).toBe(1);
		await ctx.expect(ctx.attempts.filter((attempt) => attempt.path === '/auth/logout').length === 1, 'logout request is intercepted once');
		await ctx.expect(page.getByText('Could not confirm sign-out with the server.', { exact: true })).toBeVisible();
	}
};

const accountValidation = {
	name: 'account password validation and two-factor setup cancellation',
	route: '/account',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', owner);
		await ctx.routeJSON('GET', '/auth/state', authState);
		await ctx.routeJSON('GET', '/my/alerts', { email: '', discord_id: '' });
		await ctx.routeJSON('POST', '/auth/two-factor/setup', { secret: '<preview-only>', uri: 'otpauth://totp/HolyArrs:Preview' });
	},
	run: async (ctx) => {
		const { page } = ctx;
		const newPassword = page.locator('#pw-new');
		const repeat = page.locator('#pw-again');
		const change = page.getByRole('button', { name: 'Change password', exact: true });
		await ctx.expect(await newPassword.count() === 1 && await repeat.count() === 1, 'account password fields render');
		await newPassword.fill('preview-password-one');
		await repeat.fill('preview-password-two');
		await change.click();
		await ctx.expect(await page.getByRole('alert').filter({ hasText: 'not the same' }).isVisible(), 'account mismatch is announced');
		const setup = page.getByRole('button', { name: 'Set up two-factor sign-in', exact: true });
		await setup.click();
		const cancel = page.getByRole('button', { name: 'Cancel', exact: true });
		await ctx.expect.poll(() => cancel.count()).toBe(1);
		await cancel.click();
		await ctx.expect.poll(() => setup.isVisible()).toBe(true);
		await ctx.expect(ctx.attempts.every((attempt) => attempt.path !== '/auth/two-factor/enable'), 'two-factor cancel sends no enable request');
	}
};

const welcomeValidation = {
	name: 'welcome parts guard prevents empty configuration',
	route: '/welcome?first=1',
	setup: async (ctx) => {
		await ctx.routeJSON('GET', '/auth/me', owner);
		await ctx.routeJSON('GET', '/auth/state', authState);
	},
	run: async (ctx) => {
		const { page } = ctx;
		const parts = ['Movies', 'Shows', 'Music', 'Subtitles'];
		for (const label of parts) {
			const box = page.getByRole('checkbox', { name: label, exact: true });
			await ctx.expect(await box.count() === 1, `${label} setup control renders`);
			if (await box.isChecked()) await box.uncheck();
		}
		const continueButton = page.getByRole('button', { name: 'Continue', exact: true });
		await ctx.expect(await continueButton.isDisabled(), 'empty parts configuration cannot continue');
		await ctx.expect(await page.getByRole('alert').isVisible(), 'empty parts guard explains required choice');
	}
};

const accountSettingsAlias = { ...accountValidation, name: 'settings account password validation and two-factor setup cancellation', route: '/settings/account' };

module.exports = [loginValidation, loginRecovery, loginTwoFactor, signoutRecovery, inviteValidation, accountValidation, accountSettingsAlias, welcomeValidation];
