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
	limits: { Cgroup: true, Pids: true, Memory: true, CPU: true, GroupKill: true },
	plugins: [
		{ id: 'tailscale', version: '1.2.0', installed: false, running: false, enabled: false, access: { hosts: ['controlplane.tailscale.com'], folders: ['/config/plugins/tailscale'], actions: [] } },
		{ id: 'mcp-server', version: '0.3.0', installed: true, installed_version: '0.3.0', running: true, enabled: true, access: null },
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
	// This test build has no signing key; off Linux it cannot run plugins either.
	await expect(page.getByTestId('plugins-notice').first()).toContainText("Plugins can't be installed yet");
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
		await expect(rows).toHaveCount(3);
		await expect(rows.nth(0)).toContainText('Tailscale');
		await expect(rows.nth(0)).toContainText('By HolyArrs · signed');
		await expect(rows.nth(0)).toContainText('Not installed');
		await expect(rows.nth(1)).toContainText('Running');
		await expect(rows.nth(2)).toContainText('Turned off');
		await expect(rows.nth(2).getByTestId('plugin-hint')).toHaveText('on the blocklist');
		await expect(rows.nth(2).getByRole('button', { name: 'Start Old thing' })).toHaveCount(0);
		await expect(page.getByTestId('plugins-notice')).toHaveText(["Couldn't check for blocked plugins for over a week."]);
		await expect(page.getByText('At most 256 processes')).toBeVisible();
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-${width}.png`), fullPage: true });

		await rows.nth(0).getByRole('button', { name: 'Install' }).click();
		const reach = page.getByRole('dialog', { name: 'Install Tailscale?' });
		await expect(reach.getByTestId('plugin-reach')).toContainText('controlplane.tailscale.com');
		await expect(reach.getByTestId('plugin-reach')).toContainText('/config/plugins/tailscale');
		await page.screenshot({ path: test.info().outputPath(`plugins-install-${width}.png`) });
		await reach.getByRole('button', { name: 'Cancel' }).click();
		await expect(reach).toBeHidden();
		expect(calls).toEqual([]);
		await rows.nth(0).getByRole('button', { name: 'Install' }).click();
		await reach.getByRole('button', { name: 'Install' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/install');

		await rows.nth(1).getByRole('button', { name: 'Remove' }).click();
		const rm = page.getByRole('dialog', { name: 'Remove MCP Server?' });
		await rm.getByRole('button', { name: 'Keep for later' }).click();
		await expect.poll(() => calls).toContain('DELETE /api/v1/plugins/mcp-server?keep=1');
		await rows.nth(1).getByRole('button', { name: 'Remove' }).click();
		await rm.getByRole('button', { name: 'Sign out and delete' }).click();
		await expect.poll(() => calls).toContain('DELETE /api/v1/plugins/mcp-server');
		await rows.nth(1).getByRole('button', { name: 'Stop' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/stop');
		await page.context().close();
	});
}

// Remote access card: a mocked Tailscale status, the sign-in link and QR,
// the 5 s refresh to Connected with its facts, Disconnect and Sign out.
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
			return route.fulfill({ json: { ok: true } });
		});
		await page.goto('/plugins');
		const card = page.getByTestId('remote-access');
		await expect(page.getByRole('heading', { name: 'Remote access' })).toBeVisible();
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
		await page.context().close();
	});
}
