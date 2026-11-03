import { expect, test, type Page } from '@playwright/test';
import { addFromSearch, noSidewaysScroll, owner, signIn } from './helpers.ts';

// AI connections (#89) and search in plain words (#90): turn AI connections
// on, make an AI key, use it over MCP, and find a title by describing it.
test.describe.configure({ mode: 'serial' });

let page: Page;
let key = '';

const mcp = (body: object, k = key) =>
	page.request.post('/api/v1/mcp', { headers: { 'X-Api-Key': k, 'Content-Type': 'application/json', 'MCP-Protocol-Version': '2025-11-25' }, data: { jsonrpc: '2.0', id: 1, ...body } });

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
	// A Library movie to find; earlier specs may have added it already.
	await page.goto('/library');
	await page.locator('[data-testid="library-card"], [data-testid="library-row"]').or(page.getByRole('heading', { name: 'Your Library is empty' })).first().waitFor();
	if (!(await page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Driftwood Moon' }).count())) {
		const dialog = await addFromSearch(page, 'Driftwood Moon', 'Driftwood Moon');
		await dialog.getByLabel('Search when added').uncheck();
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(page).toHaveURL(/\/library\/\d+/);
	}
});

test('AI connections are off until the owner turns them on, then an AI key works only there', async () => {
	await page.goto('/settings/ai');
	const sw = page.getByRole('checkbox', { name: 'Let AI apps connect' });
	await expect(sw).not.toBeChecked();
	await page.getByText('Make a key by hand').click();
	await page.getByLabel('Key name').fill('Claude Desktop');
	await page.getByRole('button', { name: 'Make an AI key' }).click();
	const fresh = page.getByRole('status').filter({ hasText: 'shown only once' });
	key = (await fresh.locator('code').textContent())!.trim();
	expect(key.length).toBeGreaterThan(10);

	// Off: the route is not there.
	expect((await mcp({ method: 'ping' })).status()).toBe(404);
	// The switch flips at once; wait for the save before calling.
	const saved = page.waitForResponse((r) => r.url().endsWith('/api/v1/settings/ai') && r.request().method() === 'PUT');
	await sw.check();
	expect((await saved).ok()).toBe(true);
	await expect(sw).toBeChecked();

	const list = await (await mcp({ method: 'tools/list' })).json();
	const names = list.result.tools.map((t: { name: string }) => t.name);
	expect(names).toContain('search_titles');
	expect(names).toContain('ask_for_title');

	// The AI key reaches nothing else.
	const other = await page.request.get('/api/v1/titles', { headers: { 'X-Api-Key': key } });
	expect(other.status()).toBe(403);
});

test('the guide has steps for every major AI app, nothing to paste on this address', async () => {
	// A link to one app opens its family with that app chosen.
	await page.goto('/settings/ai?app=cursor');
	const guide = page.getByTestId('ai-guide');
	const families = page.getByRole('group', { name: 'Kind of AI app' });
	await expect(families.getByRole('button', { name: 'Code editors', pressed: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Cursor', pressed: true })).toBeVisible();
	await expect(guide).toContainText('~/.cursor/mcp.json');
	await expect(page.getByTestId('ai-full-guide')).toHaveAttribute('href', 'https://holyarrs.coderluii.dev/docs/ai/cursor/');
	// Every family has steps and a link to its guide on the website.
	for (const name of ['Claude', 'ChatGPT & Codex', 'Gemini', 'Copilot', 'Grok & more', 'Code editors', 'VS Code add-ons', 'Terminal tools', 'Self-hosted chat', 'Desktop AI', 'Local agents', 'Local runners', 'Any other app']) {
		await families.getByRole('button', { name, exact: true }).click();
		await expect(guide).toBeVisible();
		await expect(page.getByTestId('ai-full-guide')).toHaveAttribute('href', /^https:\/\/holyarrs\.coderluii\.dev\/docs\/ai\/[a-z0-9-]+\/$/);
	}
	await families.getByRole('button', { name: 'Desktop AI', exact: true }).click();
	await page.getByRole('button', { name: 'LM Studio', exact: true }).click();
	await expect(guide).toContainText('mcpServers');
	await families.getByRole('button', { name: 'Claude', exact: true }).click();
	// The test server is on this computer, where apps sign in through the browser.
	await expect(page.getByTestId('ai-mode')).toContainText('Nothing to paste');
	await expect(guide).toContainText('claude mcp add --transport http --scope user holyarrs');
	await expect(guide).not.toContainText('Bearer');
	// A cloud app cannot reach a local address, and says so.
	await families.getByRole('button', { name: 'ChatGPT & Codex', exact: true }).click();
	await expect(page.getByTestId('ai-unreachable')).toBeVisible();
});

test('an AI app signs in through the browser and gets its own key', async () => {
	const origin = new URL(page.url()).origin;
	const reg = await (
		await page.request.post('/oauth/register', { data: { client_name: 'Test Agent', redirect_uris: ['http://localhost:45999/callback'] } })
	).json();
	const verifier = 'e2e-verifier-' + 'x'.repeat(40);
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
	const challenge = Buffer.from(digest).toString('base64url');
	let back = '';
	await page.route('http://localhost:45999/**', (r) => {
		back = r.request().url();
		return r.fulfill({ body: 'back in the app' });
	});
	const q = new URLSearchParams({ response_type: 'code', client_id: reg.client_id, redirect_uri: 'http://localhost:45999/callback', code_challenge: challenge, code_challenge_method: 'S256', state: 's1', resource: origin + '/api/v1/mcp' });
	await page.goto('/oauth/authorize?' + q);
	await expect(page).toHaveURL(/\/ai-connect\?/);
	const card = page.getByTestId('ai-connect');
	await expect(card).toContainText('Test Agent wants to use HolyArrs');
	await card.getByRole('button', { name: 'Allow' }).click();
	await expect.poll(() => back).toContain('code=');
	const code = new URL(back).searchParams.get('code')!;
	expect(new URL(back).searchParams.get('state')).toBe('s1');
	const tok = await (
		await page.request.post('/oauth/token', { form: { grant_type: 'authorization_code', code, client_id: reg.client_id, redirect_uri: 'http://localhost:45999/callback', code_verifier: verifier } })
	).json();
	expect(tok.token_type).toBe('Bearer');
	const list = await (
		await page.request.post('/api/v1/mcp', {
			headers: { Authorization: 'Bearer ' + tok.access_token, 'Content-Type': 'application/json', 'MCP-Protocol-Version': '2025-11-25' },
			data: { jsonrpc: '2.0', id: 1, method: 'tools/list' }
		})
	).json();
	expect(list.result.tools.length).toBeGreaterThan(0);
	await page.unroute('http://localhost:45999/**');
	// It shows among the connected apps, where it can be revoked.
	await page.goto('/settings/ai');
	await expect(page.locator('#ai-keys')).toContainText('Test Agent');
});

test('an AI app finds a title from a description', async () => {
	const r = await (await mcp({ method: 'tools/call', params: { name: 'search_titles', arguments: { query: 'driftwood moon movie' } } })).json();
	expect(r.result.isError).toBe(false);
	expect(JSON.stringify(r.result.structuredContent)).toContain('Driftwood Moon');
});

test('search in plain words shows what it understood', async () => {
	await page.goto('/search?q=' + encodeURIComponent('driftwood moon movie'));
	const plain = page.getByTestId('search-plain');
	await expect(plain).toContainText('Understood as');
	await expect(plain).toContainText('Driftwood Moon');
});

test('History offers AI apps in its Filter', async () => {
	await page.goto('/downloads/history');
	await page.getByRole('button', { name: 'Filter' }).click();
	// AI actions are their own "who" next to You, Members and HolyArrs on its own.
	const who = page.getByRole('combobox', { name: 'Who' });
	await who.selectOption({ label: 'AI apps' });
	await expect(who).toHaveValue('ai');
	await page.keyboard.press('Escape');
});

test('AI connections and plain-words search fit a phone screen', async () => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/settings/ai');
	await expect(page.getByRole('checkbox', { name: 'Let AI apps connect' })).toBeVisible();
	await noSidewaysScroll(page);
	await page.goto('/search?q=' + encodeURIComponent('driftwood moon movie'));
	await expect(page.getByTestId('search-plain')).toBeVisible();
	await noSidewaysScroll(page);
	// Leave it off for the other specs.
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.goto('/settings/ai');
	await page.getByRole('checkbox', { name: 'Let AI apps connect' }).uncheck();
});
