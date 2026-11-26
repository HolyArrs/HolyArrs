import { expect, test, type Page } from '@playwright/test';
import { join } from 'node:path';
import { noSidewaysScroll, signIn, setUpFresh } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

// Plugins page: the real server's answer (no signing key here), then a fake
// list for the tiles, tabs, the detail panel with Install, Remove and Install again.
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
		{ id: 'tailscale', version: '1.2.0', installed: false, running: false, enabled: false, name: 'Tailscale', author: 'HolyArrs', reaches_app: true, repo: 'https://github.com/HolyArrs/HolyArrs-Plugins', support_email: 'support@holyarrs.com', access: { hosts: ['controlplane.tailscale.com'], folders: ['tailscale'], actions: [] } },
		{ id: 'mcp-server', version: '0.3.0', installed: true, installed_version: '0.2.0', running: true, enabled: true, access: { hosts: ['api.example.net'], folders: [], actions: [] }, installed_access: { hosts: [], folders: [], actions: [] }, settings: [{ key: 'port', label: 'Listen port', type: 'number', default: '8765' }, { key: 'allow_requests', label: 'Allow making requests', type: 'toggle', default: 'false' }, { key: 'token', label: 'Access token', type: 'secret' }] },
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
	await expect(page.getByRole('heading', { name: 'Plugins', level: 1 })).toBeAttached();
	// The signing keys are pinned now; off Linux the page says plugins can't run here.
	if (process.platform !== 'linux') await expect(page.getByTestId('plugins-notice').first()).toContainText('Plugins run only on Linux.');
	await expect(page.getByRole('navigation').getByRole('link', { name: 'Plugins' }).first()).toBeVisible();
});

async function pick(page: Page, item: string) {
	await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
	await page.getByRole('menuitemradio', { name: item, exact: true }).click();
}

async function withFake(page: Page, calls: string[]) {
	// Remove and Install change the answer, as the real server does.
	const ov = structuredClone(fake);
	await page.route('**/api/v1/plugins**', async (route) => {
		const r = route.request();
		const url = new URL(r.url());
		if (url.pathname.endsWith('/settings')) {
			if (r.method() === 'PUT') calls.push(`PUT ${url.pathname} ${r.postData()}`);
			return route.fulfill({ json: { fields: ov.plugins[1].settings, values: { port: '8765', allow_requests: 'false' }, secrets_set: { token: r.method() === 'PUT' } } });
		}
		if (url.pathname.endsWith('/activity')) return route.fulfill({ json: { calls: [], refused: 0, limited: 0 } });
		if (r.method() === 'GET') return route.fulfill({ json: ov });
		calls.push(`${r.method()} ${url.pathname}${url.search}`);
		const id = url.pathname.split('/')[4];
		const p = ov.plugins.find((x) => x.id === id);
		if (p && r.method() === 'DELETE') Object.assign(p, { installed: false, installed_version: undefined, running: false, enabled: false });
		if (p && url.pathname.endsWith('/install')) Object.assign(p, { installed: true, installed_version: p.version, running: true, enabled: true });
		if (p && url.pathname.endsWith('/stop')) Object.assign(p, { running: false, enabled: false, disabled_reason: 'stopped by the owner' });
		if (p && url.pathname.endsWith('/start')) Object.assign(p, { running: true, enabled: true, disabled_reason: undefined });
		return route.fulfill({ json: { ok: true } });
	});
}

for (const width of [1366, 375]) {
	test(`plugins: tiles, tabs, detail panel and remove at ${width}`, async ({ browser }) => {
		test.setTimeout(120_000);
		const page = await (await browser.newContext({ viewport: { width, height: width > 400 ? 768 : 844 } })).newPage();
		await signIn(page, owner());
		const calls: string[] = [];
		await withFake(page, calls);
		await page.goto('/plugins');
		const rows = page.getByTestId('plugin-row');
		const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
		await expect(rows).toHaveCount(4);
		// Your plugins first, then the rest, each sorted by name; the badge shows only on installed plugins.
		await expect(page.getByTestId('plugins-section')).toHaveText(['Your plugins', 'More plugins']);
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'mcp-server');
		await expect(rows.nth(2)).toHaveAttribute('data-id', 'a-plugin-with-a-really-quite-long-name-to-wrap');
		await expect(row('mcp-server').getByTestId('plugin-update-dot')).toBeVisible();
		await expect(page.getByTestId('plugins-intro')).toHaveCount(0);
		await expect(row('tailscale')).toContainText('Tailscale');
		await expect(row('tailscale')).toContainText('without opening ports');
		await expect(row('tailscale')).not.toContainText('Not installed');
		await expect(row('mcp-server').getByTestId('plugin-card-update')).toBeVisible();
		await expect(row('old-thing').getByTestId('plugin-card-start')).toBeDisabled();
		await expect(page.getByRole('heading', { name: 'Plugins', level: 2, exact: true })).toHaveCount(0);
		await expect(page.getByTestId('plugins-notice')).toHaveText(["Couldn't check for blocked plugins for over a week."]);
		await expect(page.locator('#plugin-limits')).toHaveCount(0);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-${width}.png`), fullPage: true });

		// Tabs: All, then the categories present; Installed lives in Filter > Show.
		const tabs = page.getByTestId('plugins-tabs');
		await expect(tabs.getByRole('button', { name: 'Installed', exact: true })).toHaveCount(0);
		await tabs.getByRole('button', { name: 'All', exact: true }).click();
		await expect(rows).toHaveCount(4);
		await pick(page, 'Installed');
		await expect(rows).toHaveCount(2);
		await pick(page, 'All');
		await expect(rows).toHaveCount(4);

		// Search by name or description, an empty state, then sort.
		// From 768px the app's top search box searches plugins (Ctrl K focuses it); phones keep the page's own box.
		const find = width >= 768 ? page.getByTestId('search-box') : page.getByTestId('plugins-find-input');
		if (width >= 768) {
			await expect(page.getByTestId('plugins-find-input')).toBeHidden();
			await expect(find).toHaveAttribute('placeholder', 'Search plugins');
			await page.keyboard.press('Control+k');
			await expect(find).toBeFocused();
		}
		await find.fill('two lines');
		await expect(rows).toHaveCount(1);
		await find.fill('nothing like this');
		await expect(rows).toHaveCount(0);
		await expect(page.getByRole('heading', { name: 'No plugins match' })).toBeVisible();
		await page.getByRole('button', { name: 'Reset filters' }).last().click();
		await expect(rows).toHaveCount(4);
		await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
		await page.getByRole('menuitemradio', { name: 'Newest' }).click();
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'old-thing');
		// The pick is kept on the account: set Name back for the next width.
		await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
		await page.getByRole('menuitemradio', { name: 'Name' }).click();
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'mcp-server');
		// Show narrows the list; Add explains where plugins come from.
		await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
		await page.getByRole('menuitemradio', { name: 'Updates available (1)' }).click();
		await expect(rows).toHaveCount(1);
		await expect(rows.nth(0)).toHaveAttribute('data-id', 'mcp-server');
		await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
		await page.getByRole('menuitemradio', { name: 'All', exact: true }).click();
		await expect(rows).toHaveCount(4);
		await page.getByTestId('plugins-add').getByRole('button', { name: 'Add a plugin' }).click();
		const addSheet = page.getByTestId('plugins-add-sheet');
		await expect(addSheet).toContainText('Plugins come from the signed HolyArrs list.');
		// Two choices. "Build with AI" shows the one-line prompt and copies it at once.
		await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
		await addSheet.getByTestId('plugins-add-ai').click();
		await expect(addSheet.getByTestId('plugins-add-ai-panel').getByRole('status')).toHaveText('Prompt copied');
		const prompt = 'Read https://holyarrs.com/ai/plugin.md and help me make a HolyArrs plugin. My idea: <describe it here>';
		await expect(addSheet.getByTestId('plugins-add-prompt')).toHaveText(prompt);
		expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(prompt);
		await expect(addSheet.getByTestId('plugins-add-copy-prompt')).toBeVisible();
		await expect(addSheet.getByRole('link')).toHaveCount(0);
		await noSidewaysScroll(page);
		// "Build with code": two links, the guide and the template.
		await addSheet.getByTestId('plugins-add-dev').click();
		await expect(addSheet.getByRole('link')).toHaveCount(2);
		await expect(addSheet.getByTestId('plugins-add-guide')).toHaveAttribute('href', 'https://github.com/HolyArrs/HolyArrs-Plugins/blob/main/docs/making-a-plugin.md');
		await expect(addSheet.getByTestId('plugins-add-template')).toHaveAttribute('href', 'https://github.com/HolyArrs/HolyArrs-Plugins/tree/main/docs/template');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`add-sheet-${width}.png`) });
		await page.keyboard.press('Escape');
		await expect(addSheet).toBeHidden();
		// The view lives in Filter: List shows rows, Cards and Large the grid; the pick is kept in this browser.
		await pick(page, 'List');
		await expect(page.getByTestId('plugins-list')).toBeVisible();
		await expect(rows).toHaveCount(4);
		// Large: two wide tiles per row on a desktop, one on a phone, with the full description and the Install button.
		await pick(page, 'Large');
		await expect(page.getByTestId('plugins-grid').first()).toHaveClass(/large/);
		// The Language packs tile is the first tile of the first grid, so it and the first plugin share a row.
		const tops = await page.locator('[data-testid="language-packs-card"], [data-testid="plugin-row"]').evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().top)));
		expect(tops[0] === tops[1]).toBe(width > 400);
		await expect(row('tailscale').getByRole('button', { name: 'Install Tailscale' })).toBeVisible();
		await expect(row('tailscale')).toContainText('Can access');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-large-${width}.png`), fullPage: true });
		await pick(page, 'Cards');
		await page.getByTestId('plugins-filter').getByRole('button', { name: 'Filter' }).click();
		await expect(page.getByRole('menuitemradio', { name: 'Cards' })).toHaveAttribute('aria-checked', 'true');
		await page.keyboard.press('Escape');

		// Arrow keys move between the tiles.
		await row('mcp-server').getByRole('button', { name: 'Open MCP Server' }).focus();
		await page.keyboard.press('ArrowRight');
		await expect(row('old-thing').getByRole('button', { name: 'Open Old thing' })).toBeFocused();

		// Install from the card: the button shows the steps, then Running.
		await row('a-plugin-with-a-really-quite-long-name-to-wrap').getByTestId('plugin-card-install').click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/a-plugin-with-a-really-quite-long-name-to-wrap/install');
		await expect(row('a-plugin-with-a-really-quite-long-name-to-wrap').getByTestId('plugin-step')).toHaveText('Running');

		// Opening a tile shows what the plugin can reach and that it is signed, with Install; the address names it.
		await row('tailscale').getByRole('button', { name: 'Open Tailscale' }).click();
		const panel = page.getByTestId('plugin-detail');
		await expect(panel).toBeVisible();
		await expect(page).toHaveURL(/#tailscale$/);
		await expect(panel.getByTestId('plugin-signed')).toHaveText('Signed');
		await expect(panel).toContainText('without opening ports');
		await expect(panel.getByTestId('plugin-reach')).toContainText('Can access');
		await expect(panel.getByRole('link', { name: 'Source: HolyArrs/HolyArrs-Plugins' })).toHaveAttribute('href', 'https://github.com/HolyArrs/HolyArrs-Plugins');
		await expect(panel.getByTestId('plugin-contact').getByRole('button', { name: 'Contact: support@holyarrs.com' })).toHaveAttribute('title', 'support@holyarrs.com');
		await expect(panel.getByRole('link', { name: '1.2.0' })).toHaveAttribute('href', 'https://github.com/HolyArrs/HolyArrs-Plugins/releases/tag/tailscale-v1.2.0');
		await expect(panel.getByTestId('plugin-reach')).toContainText("Connect to Tailscale's servers");
		await expect(panel.getByTestId('plugin-reach')).toContainText('Let devices on your tailnet open HolyArrs');
		await expect(panel.getByTestId('plugin-reach')).toContainText('Use the folders you choose: tailscale (read only)');
		await page.screenshot({ path: test.info().outputPath(`plugins-panel-${width}.png`) });
		expect(calls.filter((c) => c.includes('tailscale'))).toEqual([]);
		await panel.getByRole('button', { name: 'Install Tailscale' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/install');
		await panel.getByTestId('plugin-detail-close').click();
		await expect(panel).toBeHidden();
		await expect(page).not.toHaveURL(/#/);

		// An off plugin says why; its Start stays in place but can't be pressed.
		await row('old-thing').getByRole('button', { name: 'Open Old thing' }).click();
		await expect(panel.getByTestId('plugin-hint')).toHaveText('on the blocklist');
		await expect(panel.getByRole('button', { name: 'Start Old thing' })).toBeDisabled();
		await panel.getByTestId('plugin-detail-close').click();

		await row('mcp-server').getByRole('button', { name: 'Open MCP Server' }).click();
		await expect(panel.getByRole('button', { name: 'Update MCP Server' })).toBeVisible();
		// The update asks for more than the installed version: a confirm lists what it adds.
		await panel.getByRole('button', { name: 'Update MCP Server' }).click();
		const more = page.getByRole('dialog', { name: 'This update can also:' });
		await expect(more.getByTestId('plugin-more-access')).toContainText('api.example.net');
		await more.getByRole('button', { name: 'Cancel' }).click();
		await expect(more).toBeHidden();
		expect(calls).not.toContain('POST /api/v1/plugins/mcp-server/install');
		// Settings open inside the dialog: the declared fields, a secret never shown back, Save and Back.
		await panel.getByRole('button', { name: 'MCP Server settings' }).click();
		const form = page.getByTestId('plugin-settings-form');
		await expect(form.getByLabel('Listen port')).toHaveValue('8765');
		await expect(form.getByLabel('Access token')).toHaveValue('');
		await form.getByLabel('Listen port').fill('9000');
		await form.getByLabel('Allow making requests').check();
		await form.getByLabel('Access token').fill('abc');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-settings-${width}.png`) });
		await form.getByRole('button', { name: 'Save' }).click();
		await expect.poll(() => calls).toContain('PUT /api/v1/plugins/mcp-server/settings {"values":{"port":"9000","allow_requests":"true","token":"abc"},"clear":[]}');
		await expect(form.getByLabel('Access token')).toHaveAttribute('placeholder', 'Saved');
		await form.getByRole('button', { name: 'Back' }).click();
		await expect(panel.getByRole('button', { name: 'Update MCP Server' })).toBeVisible();
		await expect(panel.getByTestId('plugin-reach')).toContainText('Keep files in its own folder');
		await panel.getByRole('button', { name: 'Remove MCP Server' }).click();
		const rm = page.getByRole('dialog', { name: 'Remove MCP Server?' });
		// One plain question: no settings box, Cancel and Remove on one row.
		await expect(rm).toContainText('This deletes the plugin and its settings.');
		await expect(rm.getByRole('checkbox')).toHaveCount(0);
		// Measured once the sheet has settled (on phones it slides up).
		await expect.poll(async () => {
			const cancel = await rm.getByRole('button', { name: 'Cancel' }).boundingBox();
			const confirm = await rm.getByRole('button', { name: 'Remove', exact: true }).boundingBox();
			return Math.abs((cancel?.y ?? 0) - (confirm?.y ?? 99));
		}).toBeLessThan(4);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`plugins-remove-${width}.png`) });
		await rm.getByRole('button', { name: 'Cancel' }).click();
		await expect(rm).toBeHidden();
		await panel.getByRole('button', { name: 'Stop MCP Server' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/stop');
		// Remove deletes it with its settings; the panel then offers Install again.
		await panel.getByRole('button', { name: 'Remove MCP Server' }).click();
		await rm.getByRole('button', { name: 'Remove', exact: true }).click();
		await expect.poll(() => calls).toContain('DELETE /api/v1/plugins/mcp-server');
		expect(calls.filter((c) => c.includes('keep'))).toEqual([]);
		await expect(panel.getByRole('button', { name: 'Install MCP Server' })).toBeVisible();
		await expect(panel.getByRole('button', { name: 'Remove MCP Server' })).toHaveCount(0);
		await panel.getByRole('button', { name: 'Install MCP Server' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/install');
		await expect(panel.getByRole('button', { name: 'Remove MCP Server' })).toBeVisible();
		await page.context().close();
	});
}

// One action pill per card, always in the same place: Install -> steps -> Stop -> Start -> Update, in every view and the dialog.
for (const [width, height] of [[1366, 768], [1280, 800], [375, 844]]) {
	test(`plugins: one action button walks install, stop, start, update at ${width}`, async ({ browser }) => {
		test.setTimeout(120_000);
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		const calls: string[] = [];
		await withFake(page, calls);
		await page.goto('/plugins');
		for (const v of ['List', 'Cards', 'Large']) {
			await pick(page, v);
			const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
			const st = (id: string) => row(id).locator('.st');
			await expect(st('mcp-server').getByTestId('plugin-card-update')).toBeVisible();
			await expect(st('tailscale').getByTestId('plugin-card-install')).toBeVisible();
			// The action sits at the card's bottom right, every state the same pill size.
			for (const id of ['mcp-server', 'tailscale']) {
				const b = (await st(id).locator('button').boundingBox())!;
				const c = (await row(id).boundingBox())!;
				expect(b.height).toBe(32);
				expect(c.x + c.width - (b.x + b.width)).toBeLessThan(40);
			}
			await noSidewaysScroll(page);
			await page.screenshot({ path: test.info().outputPath(`actions-${v}-${width}.png`), fullPage: true });
		}
		const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
		const id = 'tailscale';
		await row(id).getByTestId('plugin-card-install').click();
		await expect.poll(() => calls).toContain(`POST /api/v1/plugins/${id}/install`);
		await expect(page.getByTestId('plugin-detail')).toHaveCount(0);
		await row(id).getByTestId('plugin-card-stop').click();
		await expect.poll(() => calls).toContain(`POST /api/v1/plugins/${id}/stop`);
		await expect(page.getByTestId('plugin-detail')).toHaveCount(0);
		await row(id).getByTestId('plugin-card-start').click();
		await expect.poll(() => calls).toContain(`POST /api/v1/plugins/${id}/start`);
		await expect(row(id).getByTestId('plugin-card-stop')).toBeVisible();
		// Update keeps the plugin running; the dialog's main button follows the same rule.
		await row('mcp-server').getByTestId('plugin-card-update').click();
		await page.getByRole('dialog', { name: 'This update can also:' }).getByRole('button', { name: 'Update' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/install');
		await expect(row('mcp-server').getByTestId('plugin-card-stop')).toBeVisible();
		await row('mcp-server').getByRole('button', { name: 'Open MCP Server' }).click();
		const panel = page.getByTestId('plugin-detail');
		await panel.getByTestId('plugin-main-stop').click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/stop');
		await expect(panel.getByTestId('plugin-main-start')).toBeVisible();
		await page.context().close();
	});
}

// Public numbers: downloads on cards and in the dialog, New and Trending tags, a Trending sort,
// stars by the source chip; missing numbers just hide. The toolbar is one row: Add, search, Filter, Update N.
for (const [width, height] of [[1366, 768], [1280, 800], [375, 844]]) {
	test(`plugins: download numbers, tags and one-row toolbar at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		const day = (n: number) => new Date(Date.now() - n * 86_400_000).toISOString().slice(0, 10);
		const ov = structuredClone(fake) as typeof fake & { plugins: Record<string, unknown>[] };
		Object.assign(ov.plugins[0], { stats: { downloads: 1234, week: 50, stars: 12, listed: day(3) + 'T05:00:00Z' }, updated: day(2) });
		Object.assign(ov.plugins[1], { stats: { downloads: 61000, week: 5, listed: '2026-01-01T00:00:00Z' }, updated: '2026-01-01' });
		await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: ov }));
		await page.goto('/plugins');
		const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
		await expect(row('tailscale').getByTestId('plugin-downloads')).toContainText('1.2K downloads');
		await expect(row('tailscale').getByTestId('plugin-new')).toHaveText('New');
		await expect(row('tailscale').getByTestId('plugin-trending')).toHaveText('Trending');
		await expect(row('mcp-server').getByTestId('plugin-new')).toHaveCount(0);
		await expect(row('old-thing').getByTestId('plugin-downloads')).toHaveCount(0);
		await expect(row('old-thing').getByTestId('plugin-trending')).toHaveCount(0);
		// One toolbar row: Add first, then the search box, Filter and Update 1.
		const parts = width >= 768
			? [page.getByTestId('plugins-add'), page.getByTestId('search-box'), page.getByTestId('plugins-filter'), page.getByTestId('plugins-update-all')]
			: [page.getByTestId('plugins-add'), page.locator('.ptools .dlfind'), page.getByTestId('plugins-filter'), page.getByTestId('plugins-update-all')];
		const boxes = [];
		for (const x of parts) boxes.push((await x.boundingBox())!);
		for (let i = 1; i < boxes.length; i++) {
			expect(boxes[i].x).toBeGreaterThan(boxes[i - 1].x);
			expect(Math.abs(boxes[i].y + boxes[i].height / 2 - (boxes[0].y + boxes[0].height / 2))).toBeLessThan(8);
			// Every tool is as tall as the search box.
			expect(Math.abs(boxes[i].height - boxes[1].height)).toBeLessThanOrEqual(1);
		}
		await expect(page.getByTestId('plugins-view')).toHaveCount(0);
		await pick(page, 'Trending');
		// Inside More plugins (Your plugins stay first), the most downloaded this week leads.
		await expect(page.getByTestId('plugin-row').nth(2)).toHaveAttribute('data-id', 'tailscale');
		await pick(page, 'Name');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`stats-${width}.png`) });
		await row('tailscale').getByRole('button', { name: 'Open Tailscale' }).click();
		const panel = page.getByTestId('plugin-detail');
		await expect(panel.getByTestId('plugin-downloads')).toContainText('1.2K downloads');
		await expect(panel.getByTestId('plugin-updated')).toHaveClass(/fresh/);
		await expect(panel.getByTestId('plugin-stars')).toHaveText('12');
		await page.context().close();
	});
}

// Every state a plugin can be in, in Cards, Large and List and in its dialog: the one right action,
// with nothing hidden and no "Downloadingâ€¦" on Stop or Start. The list answer is dressed per state.
for (const [width, height] of [[1366, 768], [1280, 800], [375, 844]]) {
	test(`plugins: every state shows the right action and why at ${width}`, async ({ browser }) => {
		test.setTimeout(180_000);
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		const ov = structuredClone(fake) as typeof fake & { plugins: Record<string, unknown>[] };
		const base = { access: { hosts: [], folders: [], actions: [] } };
		ov.plugins.push(
			{ ...base, id: 'checking', version: '1.0.0', installed: false, running: false, enabled: false, install_step: 'checking' },
			{ ...base, id: 'failed-install', version: '1.0.0', installed: false, running: false, enabled: false, install_error: 'could not reach the plugin list' },
			{ ...base, id: 'failed-update', version: '2.0.0', installed: true, installed_version: '1.0.0', running: true, enabled: true, install_error: 'checked file changed' },
			{ ...base, id: 'needs-newer', version: '2.0.0', installed: true, installed_version: '1.0.0', running: true, enabled: true, needs_app: '9.9.0' },
			{ ...base, id: 'not-yet', version: '1.0.0', installed: false, running: false, enabled: false, needs_app: '9.9.0' },
			{ ...base, id: 'failed-start', version: '1.0.0', installed: true, installed_version: '1.0.0', running: false, enabled: false, disabled_reason: 'stopped on its own (exit status 1)' },
			{ ...base, id: 'stopped', version: '1.0.0', installed: true, installed_version: '1.0.0', running: false, enabled: false, disabled_reason: 'stopped by the owner' },
			{ id: 'needs-folder', version: '1.0.0', installed: true, installed_version: '1.0.0', running: false, enabled: false, access: { hosts: [], actions: [], folders: [{ name: 'media', mode: 'read', required: true, why: 'reads your films' }, { name: 'cache', mode: 'read_write' }] } }
		);
		let hold: (() => void) | null = null;
		await page.route('**/api/v1/plugins**', async (route) => {
			const r = route.request();
			if (r.method() === 'GET') return route.fulfill({ json: ov });
			const id = new URL(r.url()).pathname.split('/')[4];
			const p = ov.plugins.find((x) => x.id === id)!;
			// The action waits until the test lets it go, so the card can be read while it runs.
			await new Promise<void>((done) => (hold = done));
			if (r.url().endsWith('/stop')) Object.assign(p, { running: false, enabled: false, disabled_reason: 'stopped by the owner' });
			if (r.url().endsWith('/start')) Object.assign(p, { running: true, enabled: true, disabled_reason: undefined });
			return route.fulfill({ json: { ok: true } });
		});
		await page.goto('/plugins');
		const row = (id: string) => page.locator(`[data-testid="plugin-row"][data-id="${id}"]`);
		for (const v of ['Cards', 'Large', 'List']) {
			await pick(page, v);
			await expect(row('checking').getByTestId('plugin-step')).toHaveText('Checkingâ€¦');
			await expect(row('failed-install').getByTestId('plugin-card-retry')).toBeEnabled();
			await expect(row('needs-newer').getByTestId('plugin-card-update')).toHaveCount(0);
			await expect(row('needs-newer').getByTestId('plugin-card-stop')).toBeVisible();
			await expect(row('not-yet')).toContainText('Needs HolyArrs 9.9.0 or newer');
			await expect(row('failed-start').getByTestId('plugin-card-start')).toBeDisabled();
			await expect(row('failed-start').getByTestId('plugin-card-start')).toHaveAttribute('title', /stopped on its own/);
			await expect(row('stopped').getByTestId('plugin-card-start')).toBeEnabled();
			// Just installed, its required folder not chosen: the server refuses to start it, so Start says why.
			await expect(row('needs-folder').getByTestId('plugin-card-start')).toBeDisabled();
			await expect(row('needs-folder').getByTestId('plugin-card-start')).toHaveAttribute('title', 'Choose all folders first');
			await expect(row('old-thing').getByTestId('plugin-card-start')).toBeDisabled();
			// A phone's List hides the version column with its dot; the Update button says it there.
			if (v !== 'List' || width >= 768) await expect(row('mcp-server').getByRole('img', { name: 'Update available' })).toBeAttached();
			await noSidewaysScroll(page);
		}
		// List: the version and download numbers are never cut.
		Object.assign(ov.plugins[1], { stats: { downloads: 12900 } });
		await page.reload();
		if (width >= 768) {
			const ver = row('mcp-server').locator('.ver');
			await expect(ver).toBeVisible();
			expect(await ver.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
		}
		await page.screenshot({ path: test.info().outputPath(`states-list-${width}.png`), fullPage: true });

		// Cards: a failed update and an update that needs a newer HolyArrs say so on the card.
		await pick(page, 'Cards');
		await expect(row('failed-update').getByTestId('plugin-install-error')).toContainText('checked file changed');
		await expect(row('needs-newer').getByTestId('plugin-update-needs-app')).toHaveText('Needs HolyArrs 9.9.0 or newer');

		// Start on a card shows a spinner on Start, never "Downloadingâ€¦".
		await row('stopped').getByTestId('plugin-card-start').click();
		await expect.poll(() => !!hold).toBe(true);
		await expect(row('stopped').getByTestId('plugin-card-start')).toBeDisabled();
		await expect(row('stopped').getByTestId('plugin-step')).toHaveCount(0);
		await expect(page.getByText('Downloadingâ€¦')).toHaveCount(0);
		hold!();
		await expect(row('stopped').getByTestId('plugin-card-stop')).toBeVisible();

		// The dialog: first focus on its heading, not the developer's link; a server-side step locks Install.
		const panel = page.getByTestId('plugin-detail');
		await row('checking').getByRole('button', { name: 'Open Checking' }).click();
		await expect(panel.getByRole('heading', { level: 2 })).toBeFocused();
		await expect(panel.getByTestId('plugin-main-install')).toBeDisabled();
		await page.keyboard.press('Escape');
		await expect(panel).toBeHidden();
		// Changing the #id while on the page opens that plugin.
		await page.evaluate(() => (location.hash = 'needs-newer'));
		await expect(panel).toHaveAttribute('data-id', 'needs-newer');
		await expect(panel.getByTestId('plugin-needs-app')).toHaveText('Needs HolyArrs 9.9.0 or newer');
		await expect(panel.getByTestId('plugin-main-update')).toHaveCount(0);
		await expect(panel.getByTestId('plugin-main-stop')).toBeVisible();
		await page.evaluate(() => (location.hash = 'failed-update'));
		await expect(panel).toHaveAttribute('data-id', 'failed-update');
		await expect(panel.getByTestId('plugin-install-error')).toContainText('checked file changed');
		await expect(panel.getByTestId('plugin-main-update')).toBeEnabled();
		await page.evaluate(() => (location.hash = 'failed-start'));
		await expect(panel).toHaveAttribute('data-id', 'failed-start');
		await expect(panel.getByTestId('plugin-main-start')).toBeDisabled();
		await expect(panel.getByTestId('plugin-hint')).toContainText('stopped on its own');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`states-dialog-${width}.png`) });
		// Folders: Start waits for the required one, each box hints its own path, and the why sits on its own line.
		await page.evaluate(() => (location.hash = 'needs-folder'));
		await expect(panel).toHaveAttribute('data-id', 'needs-folder');
		await expect(panel.getByTestId('plugin-main-start')).toBeDisabled();
		await expect(panel.getByTestId('plugin-folder-media')).toHaveAttribute('placeholder', '/data/media');
		await expect(panel.getByTestId('plugin-folder-cache')).toHaveAttribute('placeholder', '/data/cache');
		const lab = panel.locator('.approve .lab').first(), why = panel.locator('.approve .why').first();
		expect((await why.boundingBox())!.y).toBeGreaterThanOrEqual((await lab.boundingBox())!.y + (await lab.boundingBox())!.height - 1);
		await noSidewaysScroll(page);
		await panel.getByTestId('plugin-approve').scrollIntoViewIfNeeded();
		await page.screenshot({ path: test.info().outputPath(`folders-dialog-${width}.png`) });
		await page.context().close();
	});
}

// Settings: a toggle with no default reads off and saves; a save whose restart fails says the plugin
// did not start, not that the save failed. A plugin with its own settings opens them in its dialog.
test('plugins: settings save, restart failure and own fields', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1366, height: 768 } })).newPage();
	await signIn(page, owner());
	const ov = structuredClone(fake) as typeof fake & { plugins: Record<string, unknown>[] };
	Object.assign(ov.plugins[0], { installed: true, installed_version: '1.2.0', running: true, enabled: true, settings: [{ key: 'exit', label: 'Exit node', type: 'toggle' }] });
	const puts: string[] = [];
	await page.route('**/api/v1/plugins**', async (route) => {
		const r = route.request();
		if (r.url().endsWith('/settings')) {
			if (r.method() === 'PUT') puts.push(r.postData() ?? '');
			return route.fulfill({ json: { fields: [{ key: 'exit', label: 'Exit node', type: 'toggle' }], values: { exit: 'false' }, secrets_set: {}, ...(r.method() === 'PUT' ? { start_error: 'sandbox refused' } : {}) } });
		}
		return route.fulfill({ json: ov });
	});
	await page.goto('/plugins#tailscale');
	const panel = page.getByTestId('plugin-detail');
	await panel.getByTestId('plugin-settings').click();
	const form = page.getByTestId('plugin-settings-form');
	await expect(form.getByLabel('Exit node')).not.toBeChecked();
	await form.getByTestId('plugin-settings-save').click();
	await expect.poll(() => puts.length).toBe(1);
	expect(JSON.parse(puts[0]).values).toEqual({ exit: 'false' });
	await expect(page.getByText('Tailscale settings saved')).toBeVisible();
	await expect(page.getByText('Couldnâ€™t start Tailscale').or(page.getByText("Couldn't start Tailscale"))).toBeVisible();
	await expect(page.getByText("Couldn't save the settings")).toHaveCount(0);
	await page.context().close();
});

// A link to /plugins#<id> opens that plugin; Esc closes it, clears the address and puts focus back on its tile.
// Update all updates every plugin with an update, after the same access question.
test('plugins: logos and developer pictures never load from github.com', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1366, height: 768 } })).newPage();
	await signIn(page, owner());
	const ov = structuredClone(fake) as typeof fake & { plugins: Record<string, unknown>[] };
	Object.assign(ov.plugins[0], { icon_url: 'images/tailscale/icon-256.webp', developers: ['CoderLuii', 'Gone-Person'] });
	await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: ov }));
	const webp = Buffer.from('UklGRjYAAABXRUJQVlA4ICoAAACQAQCdASoQABAAAUAmJaACdLoAA5gA/vDEC5FIsZW/+jD/8MP/ww/vCAA=', 'base64');
	const pictures: string[] = [];
	await page.route('**/api/v1/plugin-images/**', (route) => {
		const path = new URL(route.request().url()).pathname;
		pictures.push(path);
		// One developer has no copy yet: the page shows a letter instead.
		return path.endsWith('/Gone-Person-64.webp') ? route.fulfill({ status: 404 }) : route.fulfill({ body: webp, contentType: 'image/webp' });
	});
	const outside: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (u.hostname !== '127.0.0.1' && u.hostname !== 'localhost' && u.protocol !== 'data:') outside.push(u.href);
	});
	await page.goto('/plugins#tailscale');
	const panel = page.getByTestId('plugin-detail');
	await expect(panel.locator('img.pic')).toHaveAttribute('src', /\/api\/v1\/plugin-images\/images\/tailscale\/icon-128\.webp$/);
	await expect.poll(() => pictures).toContain('/api/v1/plugin-images/images/avatars/CoderLuii-64.webp');
	await expect.poll(() => pictures).toContain('/api/v1/plugin-images/images/avatars/Gone-Person-64.webp');
	await expect(panel.getByTestId('plugin-devs').locator('img')).toHaveCount(1);
	await expect(panel.getByRole('link', { name: 'Gone-Person' })).toBeVisible();
	expect(outside.filter((u) => /github\.com|githubusercontent/.test(u))).toEqual([]);
	await page.context().close();
});

test('plugins: deep link, Esc and Update all', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1366, height: 768 } })).newPage();
	await signIn(page, owner());
	const calls: string[] = [];
	await withFake(page, calls);
	await page.goto('/plugins#old-thing');
	const panel = page.getByTestId('plugin-detail');
	await expect(panel).toHaveAttribute('data-id', 'old-thing');
	await page.keyboard.press('Escape');
	await expect(panel).toBeHidden();
	await expect(page).not.toHaveURL(/#/);
	await expect(page.locator('[data-testid="plugin-row"][data-id="old-thing"] .open')).toBeFocused();
	await page.getByTestId('plugins-update-all').getByRole('button', { name: 'Update 1' }).click();
	const more = page.getByRole('dialog', { name: 'This update can also:' });
	await expect(more).toContainText('MCP Server asks for more than the version you have.');
	await more.getByRole('button', { name: 'Update' }).click();
	await expect.poll(() => calls).toContain('POST /api/v1/plugins/mcp-server/install');
	await expect(page.getByTestId('plugins-update-all')).toHaveCount(0);
	await page.context().close();
});

// No file lock on this system: one card asks once for the whole install; after Allow
// every plugin carries the tag, and Change brings the question back.
test('plugins: one file lock choice for the install', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 844 } })).newPage();
	await signIn(page, owner());
	let ov = { ...fake, blocklist_stale: false, no_file_lock: true, file_lock_reason: 'blocked', allow_no_file_lock: false, no_file_lock_asked: false };
	const calls: string[] = [];
	await page.route('**/api/v1/plugins**', async (route) => {
		const r = route.request();
		if (r.method() === 'GET') return route.fulfill({ json: ov });
		calls.push(`${r.method()} ${new URL(r.url()).pathname} ${r.postData() ?? ''}`);
		ov = { ...ov, allow_no_file_lock: JSON.parse(r.postData() ?? '{}').allow_without, no_file_lock_asked: true };
		return route.fulfill({ json: { ok: true } });
	});
	await page.goto('/plugins');
	const card = page.getByTestId('plugins-nolock');
	await expect(card).toContainText("container's security profile (seccomp)");
	await expect(card).toContainText('Let signed plugins run with the other safety limits?');
	await expect(page.getByTestId('plugin-nolock')).toHaveCount(0);
	await card.getByRole('button', { name: 'Allow' }).click();
	await expect.poll(() => calls).toContain('PUT /api/v1/plugins/file-lock {"allow_without":true}');
	await expect(card).toContainText('Signed plugins run here without the file lock.');
	await expect(page.getByTestId('plugin-nolock')).toHaveCount(4);
	// The tag explains itself (tooltip on the tile) and opens in the plugin's window with the reason and a docs link.
	await expect(page.getByTestId('plugin-nolock').first()).toHaveAttribute('title', /seccomp/);
	await page.getByTestId('plugin-row').first().locator('.open').click();
	const tag = page.getByTestId('plugin-detail').getByTestId('plugin-nolock');
	await tag.locator('summary').click();
	await expect(tag).toContainText("container's security profile (seccomp)");
	await expect(tag.getByTestId('plugin-nolock-docs')).toHaveAttribute('href', /docs\.holyarrs\.com/);
	await page.screenshot({ path: test.info().outputPath('nolock-window.png') });
	await page.keyboard.press('Escape');
	await card.getByRole('button', { name: 'Change' }).click();
	await card.getByRole('button', { name: 'Keep off' }).click();
	await expect.poll(() => calls).toContain('PUT /api/v1/plugins/file-lock {"allow_without":false}');
	await expect(card).toContainText('Plugins stay stopped until this system has the file lock.');
	await expect(page.getByTestId('plugin-nolock')).toHaveCount(0);
	await page.context().close();
});

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
		// Remote access is for a plugin granted network.forward.
		const ov = { ...fake, blocklist_stale: false, plugins: [{ ...fake.plugins[0], installed: true, installed_version: '1.2.0', running: true, enabled: true, access: { hosts: ['controlplane.tailscale.com'], folders: ['tailscale'], actions: ['network.forward'] } }] };
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
			if (r.method() === 'GET') return route.fulfill({ json: path.endsWith('/tailscale/remote/status') ? status : ov });
			calls.push(`${r.method()} ${path}`);
			return route.fulfill({ json: path.endsWith('/tailscale/remote/signout') ? { signed_out: true, removed_remotely: false } : { ok: true } });
		});
		await page.goto('/plugins');
		await expect(page.getByTestId('remote-access')).toHaveCount(0);
		await page.getByTestId('plugin-row').first().getByRole('button', { name: 'Open Tailscale' }).click();
		await page.getByTestId('plugin-detail').getByRole('link', { name: 'Tailscale settings' }).click();
		await expect(page).toHaveURL(/\/settings\/remote-access(\?plugin=tailscale)?$/);
		const card = page.getByTestId('remote-access');
		await expect(page.getByRole('heading', { name: 'Remote access', level: 1 })).toBeVisible();
		await expect(card).toContainText('Waiting for sign-in');
		await expect(card.getByRole('button', { name: 'Connect to Tailscale' })).toBeVisible();
		await expect(card.getByRole('button', { name: 'Disconnect' })).toHaveCount(0);
		await expect(card.getByRole('button', { name: 'Sign out' })).toHaveCount(0);
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
		await expect(card.getByRole('button', { name: 'Connect to Tailscale' })).toHaveCount(0);
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`remote-connected-${width}.png`), fullPage: true });

		await card.getByRole('button', { name: 'Disconnect' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/stop');
		await card.getByRole('button', { name: 'Sign out' }).click();
		const ask = page.getByRole('dialog', { name: 'Sign out of Tailscale?' });
		await ask.getByRole('button', { name: 'Cancel' }).click();
		await expect(ask).toBeHidden();
		expect(calls).not.toContain('POST /api/v1/plugins/tailscale/remote/signout');
		await card.getByRole('button', { name: 'Sign out' }).click();
		await ask.getByRole('button', { name: 'Sign out' }).click();
		await expect.poll(() => calls).toContain('POST /api/v1/plugins/tailscale/remote/signout');
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
		await expect(page.getByTestId('ai-key-groups')).toHaveText(['Can: See Â· Add Â· Downloads Â· Settings Â· Delete Â· Admin']);
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
		await expect(page.getByTestId('ai-key-groups')).toHaveText(['Can: See Â· Add Â· Downloads Â· Settings Â· Delete Â· Admin', 'Can: See Â· Add Â· Downloads Â· Delete']);
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
		await expect(page.getByTestId('ai-key-groups').first()).toHaveText('Can: See Â· Add Â· Downloads Â· Settings Â· Delete Â· Approve');
		await page.context().close();
	});
}

// The card and dialog templates hold whatever authors type: long and one-word names, 1 to 8+ access lines,
// long hosts and versions, huge and missing numbers, German and Arabic. Every card's footer sits at the same
// height and stays one line; the dialog's detail chips stay one row.
const stress = (() => {
	const base = { installed: false, running: false, enabled: false, updated: '2026-11-18', repo: 'https://github.com/CoderLuii/holyarrs-a-very-long-repository-name-for-tests', support_email: 'support@holyarrs.com', size: 2_500_000 };
	return [
		{ ...base, id: 'ultra', version: '10.20.300-beta.1', name: 'Ultra Extended Metadata Synchronisation', author: 'a-very-long-author-name-for-tests', developers: ['CoderLuii', 'HolyArrs', 'octocat'], description: 'Keeps every piece of metadata in your library aligned with several outside catalogues, including artwork, cast lists, ratings and plots in many languages.', reaches_app: true, access: { hosts: ['api.example-catalogue-one.net', 'images.example-catalogue-two.org'], folders: ['metadata', 'cache'], actions: ['library.read', 'watched.write', 'activity.read', 'requests.create', 'requests.read', 'custom.vendor-action'] }, stats: { downloads: 1_234_567, stars: 45_678, listed: new Date().toISOString().slice(0, 10) } },
		{ ...base, id: 'ok', version: '1.0.0', name: 'Ok', author: 'HolyArrs', description: 'Tiny.', access: null },
		{ ...base, id: 'host', version: '2.3.1', name: 'Long Host Relay', author: 'CoderLuii', description: 'Sends events to one very long web address.', access: { hosts: ['relay-endpoint.a-really-long-subdomain-name.example-hosting-provider-cloud.net'], folders: [], actions: [] }, stats: { downloads: 7 } },
		{ ...base, id: 'de', version: '0.9.0', name: 'Untertitel-Synchronisierungswerkzeug', author: 'CoderLuii', description: 'Synchronisiert Untertitel automatisch mit der Tonspur und korrigiert Zeitverschiebungen.', access: { hosts: ['untertitel-datenbank.beispiel.de'], folders: ['untertitel'], actions: ['library.read'] }, stats: { downloads: 4321, stars: 3 } },
		{ ...base, id: 'ar', version: '1.1.0', name: 'Ù…Ø²Ø§Ù…Ù†Ø© Ø§Ù„Ù…ÙƒØªØ¨Ø©', author: 'CoderLuii', description: 'ÙŠØ­Ø§ÙØ¸ Ø¹Ù„Ù‰ Ù…ÙƒØªØ¨ØªÙƒ Ù…ØªØ²Ø§Ù…Ù†Ø© Ù…Ø¹ Ø®Ø¯Ù…Ø© Ø§Ù„Ù…Ø´Ø§Ù‡Ø¯Ø© Ø¹Ø¨Ø± Ø§Ù„Ø¥Ù†ØªØ±Ù†Øª ÙˆÙŠØ¶ÙŠÙ Ø§Ù„Ø·Ù„Ø¨Ø§Øª Ø§Ù„Ø¬Ø¯ÙŠØ¯Ø© ØªÙ„Ù‚Ø§Ø¦ÙŠÙ‹Ø§.', access: { hosts: [], folders: [], actions: ['library.read', 'requests.create'] }, stats: { downloads: 640, stars: 41 } }
	];
})();
for (const [width, height, scheme] of [[1920, 1080, 'light'], [1530, 860, 'dark'], [1366, 768, 'light'], [1280, 800, 'dark'], [1024, 768, 'light'], [768, 1024, 'dark'], [390, 844, 'light']] as const) {
	for (const view of ['cards', 'large']) {
		test(`plugins: card template holds odd text in ${view} at ${width} ${scheme}`, async ({ browser }) => {
			const page = await (await browser.newContext({ viewport: { width, height }, colorScheme: scheme })).newPage();
			await signIn(page, owner());
			await page.addInitScript((v) => localStorage.setItem('plugins.view', v), view);
			await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: { ...fake, plugins: stress } }));
			await page.goto('/plugins');
			await page.evaluate((v) => localStorage.setItem('plugins.view', v), view);
			await page.reload();
			await expect(page.getByTestId('plugin-card-foot')).toHaveCount(stress.length);
			const m = await page.getByTestId('plugin-row').evaluateAll((rows) =>
				rows.map((r) => {
					const f = r.querySelector('[data-testid=plugin-card-foot]')!;
					const st = f.querySelector('.st')!.getBoundingClientRect();
					const chips = [...f.querySelectorAll<HTMLElement>('.chip')].filter((c) => c.offsetWidth > 0).map((c) => Math.round(c.getBoundingClientRect().top));
					return { off: Math.round(f.getBoundingClientRect().top - r.getBoundingClientRect().top), h: Math.round(f.getBoundingClientRect().height), btnTop: Math.round(st.top), chipTops: new Set(chips).size, overflow: f.scrollWidth - f.clientWidth };
				})
			);
			expect(new Set(m.map((x) => x.off)).size, JSON.stringify(m)).toBe(1);
			for (const x of m) {
				expect(x.h).toBeLessThanOrEqual(34);
				expect(x.chipTops).toBeLessThanOrEqual(1);
				expect(x.overflow).toBeLessThanOrEqual(1);
			}
			// Footer chips: no empty pill, none clipped (only a truly long version may shorten), the row never overflows,
			// and the grid's columns fit the content width.
			const chipCheck = () => page.locator('[data-testid=plugin-card-foot] .chips').evaluateAll((rows) =>
				rows.flatMap((row) => {
					const bad: string[] = [];
					if (row.scrollWidth > row.clientWidth + 1) bad.push(`row ${row.scrollWidth}>${row.clientWidth}`);
					for (const c of row.querySelectorAll<HTMLElement>('.chip')) {
						if (c.hidden || c.offsetWidth === 0) continue;
						if (!c.textContent?.trim()) bad.push('empty chip');
						if (c.scrollWidth > c.clientWidth + 1 || c.getBoundingClientRect().right > row.getBoundingClientRect().right + 1) bad.push(`clipped ${c.textContent}`);
					}
					return bad;
				})
			);
			await expect.poll(chipCheck).toEqual([]);
			await expect(page.locator('[data-testid=plugin-row][data-id=ok] [data-testid=plugin-downloads]')).toHaveCount(0);
			// The version is never shortened, on any card.
			const cut = await page.locator('[data-testid=plugin-row] .vchip').evaluateAll((vs) => vs.filter((v) => v.scrollWidth > v.clientWidth + 1).map((v) => v.textContent));
			expect(cut).toEqual([]);
			await expect(page.getByTestId('plugin-row').filter({ hasText: '10.20.300-beta.1' }).first()).toBeVisible();
			// A name too long to share its line with the New tag keeps two full lines; the tag moves under it.
			const names = await page.locator('[data-testid=plugin-row] .pname').evaluateAll((hs) => hs.filter((h) => h.scrollHeight > h.clientHeight + 1).map((h) => h.textContent));
			expect(names).toEqual([]);
			await expect(page.locator('[data-testid=plugin-row][data-id=ultra] .tag.new').filter({ visible: true })).toHaveCount(1);
			const grid = await page.getByTestId('plugins-grid').first().evaluate((g) => {
				const list = g.parentElement!.getBoundingClientRect();
				return Math.max(...[...g.children].map((c) => c.getBoundingClientRect().right)) - list.right;
			});
			expect(grid).toBeLessThanOrEqual(1);
			// The action pill is 32 px, the same as the dialog buttons.
			for (const b of await page.locator('[data-testid=plugin-card-foot] .st button').all()) expect(Math.round((await b.boundingBox())!.height)).toBe(32);
			// The date keeps its year.
			await expect(page.getByTestId('plugin-card-updated').first()).toContainText('2026');
			await noSidewaysScroll(page);
			await page.screenshot({ path: test.info().outputPath(`stress-${view}-${width}.png`), fullPage: true });
			if (view === 'large') return;
			for (const p of stress) {
				await page.goto(`/plugins#${p.id}`);
				const chips = page.getByTestId('plugin-detail').getByTestId('plugin-chips');
				await expect(chips).toBeVisible();
				const one = await chips.evaluate((c) => ({ rows: new Set([...c.querySelectorAll('dd')].map((d) => Math.round(d.getBoundingClientRect().top))).size, over: c.scrollWidth - c.clientWidth }));
				expect(one, p.id).toEqual({ rows: 1, over: 0 });
				for (const b of await page.getByTestId('plugin-acts').locator('.ui-btn').all()) expect(Math.abs((await b.boundingBox())!.height - 32)).toBeLessThanOrEqual(1.5);
			}
		});
	}
}

// With no plugins listed (only Language Packs), Add and the filter/view menu still show, and the Language Packs
// tile shows its own version 1.0.0, never the app's.
for (const [width, height] of [[1280, 800], [390, 844]] as const) {
	test(`plugins: Add and view tools with no plugins at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: { ...fake, plugins: [] } }));
		await page.goto('/plugins');
		await expect(page.getByTestId('plugins-add').first()).toBeVisible();
		await expect(page.getByTestId('plugins-filter').first()).toBeVisible();
		const tile = page.getByTestId('language-packs-card');
		await expect(tile).toBeVisible();
		await expect(tile).toContainText('1.0.0');
		await expect(tile).not.toContainText('1.3.0');
		await noSidewaysScroll(page);
		await page.screenshot({ path: test.info().outputPath(`no-plugins-${width}.png`), fullPage: true });
		await page.context().close();
	});
}

// The Language Packs sheet: all 44 languages fit one desktop window without scrolling (4 across); phones scroll.
const langCodes = ['ar', 'bg', 'bn', 'ca', 'cs', 'da', 'de', 'el', 'en', 'es', 'et', 'fa', 'fi', 'fil', 'fr', 'he', 'hi', 'hr', 'hu', 'id', 'is', 'it', 'ja', 'ko', 'lt', 'lv', 'ms', 'nb', 'nl', 'pl', 'pt-br', 'pt-pt', 'ro', 'ru', 'sk', 'sr', 'sv', 'ta', 'th', 'tr', 'uk', 'vi', 'zh-cn', 'zh-tw'];
for (const [width, height, fits] of [[1440, 900, true], [1280, 800, true], [390, 844, false]] as const) {
	test(`plugins: Language Packs sheet at ${width}x${height}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: { ...fake, plugins: [] } }));
		await page.route('**/api/v1/languages', (route) =>
			route.fulfill({ json: { languages: langCodes.map((code, i) => ({ code, state: code === 'en' ? 'built_in' : i % 9 === 1 ? 'installed' : 'available', size: 180_000 + i * 1000 })) } })
		);
		await page.goto('/plugins');
		await page.getByTestId('language-packs-card').locator('.open').click();
		const sheet = page.locator('.ui-sheet[open]');
		await expect(sheet.getByTestId('language-row')).toHaveCount(langCodes.length);
		await expect(sheet.getByTestId('language-install').first()).toBeVisible();
		await page.waitForTimeout(400);
		await page.screenshot({ path: test.info().outputPath(`language-sheet-${width}.png`) });
		// No language name or sub-line is ever cut short.
		const cut = await sheet.locator('[data-testid=language-row] .own, [data-testid=language-row] .en').evaluateAll((els) => els.filter((e) => e.scrollWidth > e.clientWidth + 1 || getComputedStyle(e).textOverflow === 'ellipsis').map((e) => e.textContent));
		expect(cut, 'cut-off language names').toEqual([]);
		await noSidewaysScroll(page);
		if (fits) {
			const over = await sheet.evaluate((el) => [...el.querySelectorAll<HTMLElement>('*')].some((n) => n.scrollHeight > n.clientHeight + 1 && /auto|scroll/.test(getComputedStyle(n).overflowY)));
			expect(over, 'the sheet scrolls at 1440x900').toBe(false);
			await sameHeight(page);
		}
		await page.context().close();
	});
}

// Guard: every plugin window fits one desktop screen. A heavy plugin (long description, many scopes, folders,
// approvals and settings) opens each window at 1440x900 and 1280x800; nothing in the open window may scroll.
// New plugin windows use the shared fit layout (app.css .fit-grid / .fit-clamp), so this test covers them too.
const heavy = {
	id: 'heavy', version: '12.34.56', installed: true, installed_version: '12.34.56', running: true, enabled: true, name: 'Heavy Everything Plugin With A Long Name',
	author: 'CoderLuii', developers: ['CoderLuii', 'HolyArrs', 'octocat'], updated: '2026-11-18', size: 9_800_000,
	repo: 'https://github.com/HolyArrs/HolyArrs-Plugins', support_email: 'support@holyarrs.com', reaches_app: true,
	description: 'Does a great many things for your library, downloads and requests, and explains each of them at length so the text runs long.',
	long_description: Array.from({ length: 6 }, (_, i) => `Paragraph ${i + 1}: this plugin keeps your library, downloads, requests and subtitles in step with several outside services, checks every change twice and writes a full report so you always know what it did and why it did it.`).join('\n'),
	how_it_works: ['Reads your library every hour.', 'Compares it with the outside services.', 'Writes the changes back and keeps a report.'],
	needs_approval: ['host:extra-one.example.net', 'host:extra-two.example.net', 'folder:reports'],
	access: {
		hosts: Array.from({ length: 12 }, (_, i) => `api-${i + 1}.a-long-outside-service-name.example.net`),
		folders: ['reports', 'cache', 'exports', 'imports'],
		actions: ['library.read', 'library.write', 'downloads.read', 'downloads.control', 'requests.read', 'requests.create', 'requests.approve', 'activity.read', 'watched.write', 'settings.read', 'schedules.run', 'subtitles.write']
	},
	installed_access: { hosts: [], folders: [], actions: [] },
	scope_groups: [{ id: 'network', scopes: ['network.forward'], change: true, warnings: ['forward'] }],
	settings: Array.from({ length: 16 }, (_, i) => ({ key: `opt${i}`, label: `Option number ${i + 1} with a longer label`, type: ['text', 'number', 'toggle', 'select', 'secret'][i % 5], options: ['One', 'Two', 'Three'], help: i % 3 ? undefined : 'A short line of help that explains what this option changes.', group: i < 8 ? 'Main' : 'Extra' }))
};
async function sameHeight(page: Page) {
	// The two columns start and end on one line: About (plus Your choices when a plugin waits for approval) beside Can access.
	const box = (sel: string) => page.locator(`dialog[open] ${sel}`).evaluateAll((es) => { const e = es.at(-1); if (!e) return null; const r = e.getBoundingClientRect(); return { top: r.top, bottom: r.bottom }; });
	const [about, reach, extra] = await Promise.all([box('.a-about'), box('.a-reach'), box('.a-extra:not(:has(.langs))')]);
	const left = extra && extra.top >= (about?.bottom ?? 0) - 1 && extra.top < (reach?.bottom ?? 0) ? extra.bottom : (about?.bottom ?? 0);
	expect(Math.abs((about?.top ?? 0) - (reach?.top ?? 1)), 'About and Can access start apart').toBeLessThanOrEqual(1);
	expect(Math.abs(left - (reach?.bottom ?? 1)), 'the two columns end apart').toBeLessThanOrEqual(1);
}
async function noScrollIn(page: Page, what: string) {
	const bad = await page.locator('dialog[open]').last().evaluate((root) => [root, ...root.querySelectorAll<HTMLElement>('*')]
		.filter((n) => n.scrollHeight > n.clientHeight + 1 && /auto|scroll/.test(getComputedStyle(n).overflowY))
		.map((n) => `${n.tagName.toLowerCase()}.${[...n.classList].join('.')} ${n.scrollHeight}>${n.clientHeight}`));
	expect(bad, `${what} scrolls`).toEqual([]);
}
for (const [width, height] of [[1440, 900], [1280, 800], [390, 844]] as const) {
	test(`plugins: every plugin window fits at ${width}x${height}`, async ({ browser }) => {
		const desk = width >= 1024;
		const page = await (await browser.newContext({ viewport: { width, height } })).newPage();
		await signIn(page, owner());
		await page.route('**/api/v1/plugins**', (route) => {
			const url = new URL(route.request().url());
			if (url.pathname.endsWith('/settings')) return route.fulfill({ json: { fields: heavy.settings, values: {}, secrets_set: {} } });
			if (url.pathname.endsWith('/activity')) return route.fulfill({ json: { calls: [], refused: 0, limited: 0 } });
			return route.fulfill({ json: { ...fake, plugins: [heavy] } });
		});
		await page.goto('/plugins');
		const shot = (n: string) => page.screenshot({ path: test.info().outputPath(`fit-${n}-${width}.png`) });
		// The plugin's own window (About, access, approvals).
		await page.getByTestId('plugin-row').first().locator('.open').click();
		await expect(page.getByTestId('plugin-detail')).toBeVisible();
		await page.waitForTimeout(400);
		await shot('info');
		// The red forwarding warning shows where the access is approved, with its icon.
		await expect(page.getByTestId('approve-warning')).toContainText('Sees everything sent through it');
		if (desk) await noScrollIn(page, 'plugin window');
		if (desk) await sameHeight(page);
		// Its settings.
		await page.getByTestId('plugin-settings').click();
		await expect(page.getByTestId('plugin-settings-form')).toBeVisible();
		await page.waitForTimeout(400);
		await shot('settings');
		if (desk) await noScrollIn(page, 'settings');
		await page.keyboard.press('Escape');
		// Add a plugin, with the AI prompt open.
		await page.getByTestId('plugins-add').first().getByRole('button').click();
		await page.getByTestId('plugins-add-ai').click();
		await page.waitForTimeout(400);
		await shot('add');
		if (desk) await noScrollIn(page, 'add sheet');
		await page.keyboard.press('Escape');
		await noSidewaysScroll(page);
		await page.context().close();
	});
}

// List view: the Language Packs row has the same columns as a plugin row (category, developer, version, its button with the others).
test('plugins: Language Packs row lines up in the list view', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
	await signIn(page, owner());
	await page.addInitScript(() => localStorage.setItem('plugins.view', 'list'));
	await page.route('**/api/v1/plugins**', (route) => route.fulfill({ json: fake }));
	await page.goto('/plugins');
	const row = page.getByTestId('language-packs-card');
	await expect(row).toContainText('1.0.0');
	const btn = await row.locator('.st').boundingBox();
	const other = await page.getByTestId('plugin-row').first().locator('.st').boundingBox();
	expect(Math.abs((btn?.x ?? 0) + (btn?.width ?? 0) - (other?.x ?? 0) - (other?.width ?? 0))).toBeLessThanOrEqual(2);
	await page.waitForTimeout(400);
	await page.screenshot({ path: test.info().outputPath('list-view-1280.png') });
	await page.context().close();
});
