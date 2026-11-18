import { expect, test, type Page } from '@playwright/test';
import { join } from 'node:path';
import { noSidewaysScroll, signIn, setUpFresh } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

// Plugins page: the real server's answer (no signing key here), then a fake
// list for the rows, the reach box before Install and the two Remove choices.
// Own fresh app: npx playwright test -c ../tests/setups/plugins.config.ts
test.describe.configure({ mode: 'serial' });

const tmp = dataDir('phone');
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
const owner = () => ({ name: 'luis', password: pw });

const fake = {
	key_set: true,
	blocklist_checked_at: null,
	blocklist_stale: true,
	can_run: true,
	plugins: [
		{ id: 'tailscale', version: '1.2.0', installed: false, running: false, enabled: false, name: 'Tailscale', author: 'HolyArrs', reaches_app: true, access: { hosts: ['controlplane.tailscale.com'], folders: ['tailscale'], actions: [] } },
		{ id: 'mcp-server', version: '0.3.0', installed: true, installed_version: '0.2.0', running: true, enabled: true, access: null },
		{ id: 'a-plugin-with-a-really-quite-long-name-to-wrap', version: '10.20.30-beta.1', installed: false, running: false, enabled: false, author: 'Someone With A Long Name', description: 'Does one small thing and explains it in a sentence that runs long enough to need two lines or be cut.', access: { hosts: ['example.org'], folders: [], actions: [] } },
		{ id: 'old-thing', version: '0.1.0', installed: true, running: false, enabled: false, disabled_reason: 'on the blocklist', access: null }
	]
};

test.beforeAll(async ({ browser }) => {
	test.setTimeout(240_000);
	const page = await browser.newPage();
	await setUpFresh(page, owner(), join(tmp, 'movies'));
	await page.context().close();
});

test('plugins: the real server answer shows its notices', async ({ page }) => {
	await signIn(page, owner());
	await page.goto('/plugins');
	await expect(page.getByRole('heading', { name: 'Plugins', level: 1 })).toBeVisible();
	// The signing keys are pinned now; off Linux the page says plugins can't run here.
	if (process.platform !== 'linux') await expect(page.getByTestId('plugins-notice').first()).toContainText('Plugins run only on Linux.');
	await expect(page.getByRole('navigation').getByRole('link', { name: 'Plugins' }).first()).toBeVisible();
});

async function withFake(page: Page, calls: string[]) {
	await page.route('**/api/v1/plugins**', async (route) => {
		const r = route.request();
		if (r.method() === 'GET') return route.fulfill({ json: fake });
		calls.push(`${r.method()} ${new URL(r.url()).pathname}${new URL(r.url()).search}`);
		return route.fulfill({ json: { ok: true } });
	});
}

for (const width of [1280, 375]) {
	test(`plugins: rows, reach box and remove at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		const calls: string[] = [];
		await withFake(page, calls);
		await page.goto('/plugins');
		const rows = page.getByTestId('plugin-row');
		const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
		await expect(rows).toHaveCount(4);
		// Sorted by name by default; Newest is the signed list's order, last added first.
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'a-plugin-with-a-really-quite-long-name-to-wrap');
		await expect(row('tailscale')).toContainText('Tailscale');
		await expect(row('tailscale')).toContainText('By HolyArrs');
		await expect(row('tailscale')).toContainText('Signed');
		await expect(row('tailscale')).toContainText('without opening ports');
		await expect(page.getByRole('heading', { name: 'Plugins', level: 2 })).toHaveCount(0);
		await expect(row('tailscale')).toContainText('Not installed');
		await expect(row('mcp-server')).toContainText('Update available');
		await expect(row('mcp-server').getByRole('button', { name: 'Update MCP Server' })).toBeVisible();
		await expect(row('old-thing')).toContainText('Turned off');
		await expect(row('old-thing').getByTestId('plugin-hint')).toHaveText('on the blocklist');
		await expect(row('old-thing').getByRole('button', { name: 'Start Old thing' })).toHaveCount(0);
		await expect(page.getByTestId('plugins-notice')).toHaveText(["Couldn't check for blocked plugins for over a week."]);
		await expect(row('tailscale').getByTestId('plugin-access')).toContainText('Can access');
		await expect(row('tailscale').getByTestId('plugin-access')).toContainText("Connect to Tailscale's servers");
		await expect(row('tailscale').getByTestId('plugin-access')).toContainText('Let devices on your tailnet open HolyArrs');
		await expect(row('a-plugin-with-a-really-quite-long-name-to-wrap').getByTestId('plugin-access')).toContainText('Connect to example.org');
		await expect(row('mcp-server').getByTestId('plugin-access')).toContainText('Nothing outside its own folder');
		await expect(page.locator('#plugin-limits')).toHaveCount(0);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-${width}.png`), fullPage: true });

		// Search by name or description, an empty state, then sort and the list view.
		const find = page.getByTestId('plugins-find-input');
		await find.fill('two lines');
		await expect(rows).toHaveCount(1);
		await find.fill('nothing like this');
		await expect(rows).toHaveCount(0);
		await expect(page.getByRole('heading', { name: 'No plugins match' })).toBeVisible();
		await page.getByRole('button', { name: 'Clear search' }).last().click();
		await expect(rows).toHaveCount(4);
		await page.getByTestId('plugins-sort').getByRole('button', { name: 'Sort by' }).click();
		await page.getByRole('menuitemradio', { name: 'Newest' }).click();
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'old-thing');
		await page.getByRole('radio', { name: 'List' }).click();
		await expect(page.getByTestId('plugins-list')).toBeVisible();
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-list-${width}.png`), fullPage: true });
		await page.getByRole('radio', { name: 'Cards' }).click();
		await expect(page.getByTestId('plugins-grid')).toBeVisible();
		// Both picks are kept on the account: set Name back for the next width.
		await page.getByTestId('plugins-sort').getByRole('button', { name: 'Sort by' }).click();
		await page.getByRole('menuitemradio', { name: 'Name' }).click();
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'a-plugin-with-a-really-quite-long-name-to-wrap');

		await row('tailscale').getByRole('button', { name: 'Install' }).click();
		const reach = page.getByRole('dialog', { name: 'Install Tailscale?' });
		await expect(reach).toContainText('without opening ports');
		await expect(reach.getByTestId('plugin-reach')).toContainText("Connect to Tailscale's servers");
		await expect(reach.getByTestId('plugin-reach')).toContainText('Let devices on your tailnet open HolyArrs');
		await expect(reach.getByTestId('plugin-reach')).toContainText('Keep files in its own folder');
		await page.screenshot({ path: test.info().outputPath(`plugins-install-${width}.png`) });
		await reach.getByRole('button', { name: 'Cancel' }).click();
		await expect(reach).toBeHidden();
		expect(calls).toEqual([]);
		await row('tailscale').getByRole('button', { name: 'Install' }).click();
		await reach.getByRole('button', { name: 'Install' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/install');

		await row('mcp-server').getByRole('button', { name: 'Remove' }).click();
		const rm = page.getByRole('dialog', { name: 'Remove MCP Server?' });
		await rm.getByRole('button', { name: 'Keep for later' }).click();
		await expect.poll(() => calls).toContain('DELETE /api/v1/plugins/mcp-server?keep=1');
		await row('mcp-server').getByRole('button', { name: 'Remove' }).click();
		await rm.getByRole('button', { name: 'Sign out and delete' }).click();
		await expect.poll(() => calls).toContain('DELETE /api/v1/plugins/mcp-server');
		await row('mcp-server').getByRole('button', { name: 'Stop' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/stop');
		await page.context().close();
	});
}

// Settings > Remote access without Tailscale: one line and a button to Plugins.
for (const width of [1280, 375]) {
	test(`plugins: remote access page without Tailscale at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		await withFake(page, []);
		await page.goto('/settings/remote-access');
		await expect(page.getByRole('heading', { name: 'Remote access', level: 1 })).toBeVisible();
		const box = page.getByTestId('remote-needs-plugin');
		await expect(box).toContainText('Remote access uses the Tailscale plugin.');
		await expect(page.getByTestId('remote-access')).toHaveCount(0);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`remote-none-${width}.png`), fullPage: true });
		await box.getByRole('link', { name: 'Go to Plugins' }).click();
		await expect(page).toHaveURL(/\/plugins$/);
		await expect(page.getByTestId('plugin-row').first()).toBeVisible();
		await expect(page.getByRole('link', { name: 'Remote access settings' })).toHaveCount(0);
		await page.context().close();
	});
}

// Remote access card on Settings > Remote access: a mocked Tailscale status, the sign-in
// link and QR, the 5 s refresh to Connected with its facts, Disconnect and Sign out.
for (const width of [1280, 375]) {
	test(`plugins: remote access card at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		const ov = { ...fake, blocklist_stale: false, plugins: [{ ...fake.plugins[0], installed: true, installed_version: '1.2.0', running: true, enabled: true }] };
		let status: Record<string, unknown> = {
			state: 'waiting_for_sign_in',
			login_url: 'https://login.tailscale.com/a/e2e',
			login_qr: ['1110111', '1010101', '1110111', '0000000', '1110111', '1010101', '1110111'],
			last_updated: new Date().toISOString()
		};
		const calls: string[] = [];
		await page.route('**/api/v1/plugins**', async (route) => {
			const r = route.request();
			const path = new URL(r.url()).pathname;
			if (r.method() === 'GET') return route.fulfill({ json: path.endsWith('/tailscale/status') ? status : ov });
			calls.push(`${r.method()} ${path}`);
			return route.fulfill({ json: path.endsWith('/tailscale/signout') ? { signed_out: true, removed_remotely: false } : { ok: true } });
		});
		await page.goto('/plugins');
		await expect(page.getByTestId('remote-access')).toHaveCount(0);
		await page.getByTestId('plugin-row').first().getByRole('link', { name: 'Open Tailscale' }).click();
		await expect(page).toHaveURL(/\/settings\/remote-access$/);
		const card = page.getByTestId('remote-access');
		await expect(page.getByRole('heading', { name: 'Remote access', level: 1 })).toBeVisible();
		await expect(card).toContainText('Waiting for sign-in');
		await expect(card.getByRole('button', { name: 'Open Tailscale sign-in' })).toBeVisible();
		await expect(card.getByRole('img', { name: 'QR code for the Tailscale sign-in link' })).toBeVisible();
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`remote-signin-${width}.png`), fullPage: true });

		status = {
			state: 'connected',
			tsnet_name: 'holyarrs.tail1234.ts.net',
			tailscale_version: '1.90.0',
			last_updated: new Date().toISOString(),
			login_expiry: new Date(Date.now() + 90 * 864e5).toISOString(),
			connection: 'direct'
		};
		await expect(card).toContainText('Connected', { timeout: 8000 });
		await expect(card).toContainText('holyarrs.tail1234.ts.net');
		await expect(card).toContainText('1.90.0');
		await expect(card).toContainText('Direct');
		await expect(card.getByRole('button', { name: 'Open Tailscale sign-in' })).toHaveCount(0);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`remote-connected-${width}.png`), fullPage: true });

		await card.getByRole('button', { name: 'Disconnect' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/stop');
		await card.getByRole('button', { name: 'Sign out' }).click();
		const ask = page.getByRole('dialog', { name: 'Sign out of Tailscale?' });
		await ask.getByRole('button', { name: 'Cancel' }).click();
		await expect(ask).toBeHidden();
		expect(calls).not.toContain('POST /api/v1/plugins/tailscale/signout');
		await card.getByRole('button', { name: 'Sign out' }).click();
		await ask.getByRole('button', { name: 'Sign out' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/signout');
		await expect(card.getByTestId('remote-notice')).toHaveText("Signed out here. Remove the device in Tailscale's admin page too.");
		await page.context().close();
	});
}

// Settings > AI apps: an AI key's tick boxes (mocked keys API). The first four start ticked,
// mcp_groups goes with the new key, and the list shows each key's boxes in short form.
for (const width of [1280, 375]) {
	test(`ai key: tick boxes and list at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		const now = new Date().toISOString();
		const keys = [{ id: 1, label: 'Old app', hint: 'ab12', created_at: now, scopes: ['mcp'], mcp_groups: ['see', 'add', 'control', 'settings', 'delete', 'admin'] }];
		const sent: Record<string, unknown>[] = [];
		// AI apps are held back in this build; let the page show.
		await page.route('**/api/v1/auth/me', async (route) => {
			const me = await (await route.fetch()).json();
			return route.fulfill({ json: { ...me, held: (me.held ?? []).filter((f: string) => f !== 'ai_apps') } });
		});
		await page.route('**/api/v1/settings/ai', (route) => route.fulfill({ json: { enabled: true } }));
		const edits: unknown[] = [];
		await page.route('**/api/v1/auth/keys/*', async (route) => {
			const body = route.request().postDataJSON();
			edits.push(body);
			return route.fulfill({ json: { id: 1, label: 'Old app', mcp_groups: body.mcp_groups } });
		});
		await page.route('**/api/v1/auth/keys', async (route) => {
			const r = route.request();
			if (r.method() === 'GET') return route.fulfill({ json: { keys } });
			const body = r.postDataJSON();
			sent.push(body);
			keys.push({ id: 2, label: body.label, hint: 'cd34', created_at: now, scopes: body.scopes, mcp_groups: body.mcp_groups });
			return route.fulfill({ json: { id: 2, key: 'hak_e2e_fake_key_value', label: body.label } });
		});
		await page.goto('/settings/ai');
		await expect(page.getByTestId('ai-key-groups')).toHaveText(['Can: See · Add · Downloads · Settings · Delete · Admin']);
		await page.getByText('Make a key by hand').click();
		const form = page.getByTestId('ai-key-groups-form');
		for (const name of ['See library and activity', 'Add and request', 'Control downloads', 'Change settings']) await expect(form.getByRole('checkbox', { name })).toBeChecked();
		await expect(form.getByRole('checkbox', { name: 'Delete' })).not.toBeChecked();
		await expect(form.getByRole('checkbox', { name: /^Admin/ })).not.toBeChecked();
		await expect(form).toContainText('Lets the AI manage users, keys and backups.');
		await expect(form.getByRole('checkbox', { name: /^Approve requests/ })).not.toBeChecked();
		await expect(form).toContainText('You can change these later from the key list.');
		await form.getByRole('checkbox', { name: 'Change settings' }).uncheck();
		await form.getByRole('checkbox', { name: 'Delete' }).check();
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`ai-key-groups-${width}.png`), fullPage: true });
		await page.getByLabel('Key name').fill('Claude Desktop');
		await page.getByRole('button', { name: 'Make an AI key' }).click();
		await expect.poll(() => sent).toEqual([{ label: 'Claude Desktop', scopes: ['mcp'], mcp_groups: ['see', 'add', 'control', 'delete'] }]);
		await expect(page.getByTestId('ai-key-groups')).toHaveText(['Can: See · Add · Downloads · Settings · Delete · Admin', 'Can: See · Add · Downloads · Delete']);
		// The boxes go back to the defaults for the next key.
		await expect(form.getByRole('checkbox', { name: 'Change settings' })).toBeChecked();
		await expect(form.getByRole('checkbox', { name: 'Delete' })).not.toBeChecked();
		// Change an existing key's boxes: drop Admin, add Approve requests.
		await page.getByRole('button', { name: 'Change what Old app may do' }).click();
		const edit = page.getByTestId('ai-key-edit-form');
		await expect(edit.getByRole('checkbox', { name: /^Admin/ })).toBeChecked();
		await edit.getByRole('checkbox', { name: /^Admin/ }).uncheck();
		await edit.getByRole('checkbox', { name: /^Approve requests/ }).check();
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`ai-key-edit-${width}.png`), fullPage: true });
		await page.getByRole('button', { name: 'Save' }).click();
		await expect.poll(() => edits).toEqual([{ mcp_groups: ['see', 'add', 'control', 'settings', 'delete', 'approve'] }]);
		await expect(page.getByTestId('ai-key-groups').first()).toHaveText('Can: See · Add · Downloads · Settings · Delete · Approve');
		await page.context().close();
	});
}
