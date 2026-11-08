import { expect, test, type Page } from '@playwright/test';
import { addFromSearch, noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

// AI apps and search in plain words: turn AI apps
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
	await page.locator('[data-testid="library-card"], [data-testid="library-row"]').or(page.getByRole('heading', { name: 'Nothing here yet' })).first().waitFor();
	if (!(await page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Driftwood Moon' }).count())) {
		const dialog = await addFromSearch(page, 'Driftwood Moon', 'Driftwood Moon');
		await dialog.getByLabel('Search when added').uncheck();
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(page).toHaveURL(/\/library\/\d+/);
	}
});

test('AI apps are off until the owner turns them on, then an AI key works only there', async () => {
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

const GUIDE = /^https:\/\/holyarrs\.coderluii\.dev\/docs\/ai\/((claude-app|chatgpt|gemini-app|claude-code|codex|cursor|claude-desktop-home|local-ai)\/|#(apps|any-other-app))$/;

test('the table lists where each app takes HolyArrs, and a row opens its steps', async () => {
	await page.goto('/settings/ai');
	const table = page.getByTestId('ai-table');
	for (const g of ['Chat apps', 'Agents and editors', 'On your computer']) await expect(table.getByText(g, { exact: true })).toBeVisible();
	// No row is picked until you pick one.
	await expect(page.getByTestId('ai-guide')).toHaveCount(0);
	// Every Guide link goes to one of the website's 8 guides or the main page.
	const guides = table.getByRole('link', { name: /^Guide for / });
	await expect(guides).toHaveCount(13);
	for (const href of await guides.evaluateAll((els) => els.map((e) => e.getAttribute('href') ?? ''))) expect(href).toMatch(GUIDE);
	await expect(table.getByRole('link', { name: 'Guide for Cursor' })).toHaveAttribute('href', 'https://holyarrs.coderluii.dev/docs/ai/cursor/');
	await expect(table.getByRole('link', { name: 'Guide for Any other app' })).toHaveAttribute('href', 'https://holyarrs.coderluii.dev/docs/ai/#any-other-app');
	await expect(page.getByRole('link', { name: 'All AI guides' })).toHaveAttribute('href', 'https://holyarrs.coderluii.dev/docs/ai/');
	await expect(page.getByRole('link', { name: 'Only on your home network?' }).first()).toHaveAttribute('href', 'https://holyarrs.coderluii.dev/docs/ai/#home-network');

	// The test server is on this computer, where apps sign in through the browser.
	await expect(page.getByTestId('ai-mode')).toContainText('You press Allow once');
	await expect(page.getByTestId('ai-steps')).toContainText('then press Allow');
	const origin = new URL(page.url()).origin;
	await expect(page.getByTestId('ai-prompt')).toHaveText(
		`Read https://holyarrs.coderluii.dev/docs/ai/ai.md and connect yourself to my HolyArrs at ${origin}. Only add the HolyArrs MCP server. Ask me before you change anything else.`
	);
	await expect(page.getByTestId('ai-check')).toContainText("What's downloading in HolyArrs?");

	// Picking a row opens its steps, and only its steps.
	const before = new URL(page.url());
	await table.getByRole('button', { name: 'Claude Code', exact: true }).click();
	await expect(page).toHaveURL(/\?app=claude-code$/);
	// Picking changes only ?app= in the address, never more (no key can reach it).
	const after = new URL(page.url());
	expect(after.pathname).toBe(before.pathname);
	expect([...after.searchParams.keys()]).toEqual(['app']);
	const guide = page.getByTestId('ai-guide');
	await expect(guide).toHaveCount(1);
	await expect(guide).toContainText('claude mcp add --transport http --scope user holyarrs');
	await expect(guide).not.toContainText('Bearer');
	await expect(guide).toContainText('Tested with HolyArrs on 3 Nov 2026');
	await table.getByRole('button', { name: 'Cursor', exact: true }).click();
	await expect(table.getByRole('button', { name: 'Claude Code', exact: true })).toHaveAttribute('aria-expanded', 'false');
	await expect(table.getByRole('button', { name: 'Cursor', exact: true })).toHaveAttribute('aria-expanded', 'true');
	await expect(guide).toHaveCount(1);
	await expect(guide).toContainText('~/.cursor/mcp.json');
	await expect(guide.getByRole('link', { name: 'Add to Cursor' })).toHaveAttribute('href', /^cursor:\/\/anysphere\.cursor-deeplink\/mcp\/install\?name=holyarrs&config=/);
	await expect(guide).toContainText("Not tested with HolyArrs yet. These steps follow Cursor's own help pages.");
	// On sign-in the table offers the same one-click buttons.
	await expect(table.getByRole('link', { name: 'Add to Cursor' })).toHaveAttribute('href', /^cursor:\/\//);
	await expect(table.getByRole('link', { name: 'Add to VS Code' })).toHaveAttribute('href', /^vscode:mcp\/install\?/);
	await noSidewaysScroll(page);
	// Picking it again closes it.
	await table.getByRole('button', { name: 'Cursor', exact: true }).click();
	await expect(guide).toHaveCount(0);
});

test('a link picks its row, old app names included', async () => {
	const expanded = (name: string) => page.getByTestId('ai-table').getByRole('button', { name, exact: true, expanded: true });
	await page.goto('/settings/ai?app=cursor');
	await expect(expanded('Cursor')).toBeVisible();
	await page.goto('/settings/ai?app=codex-ide');
	await expect(expanded('Codex')).toBeVisible();
	await expect(page.getByTestId('ai-guide').getByRole('heading')).toHaveText('Codex IDE extension');
	// LM Studio sits under the local row but shows its own steps, not Open WebUI's.
	await page.goto('/settings/ai?app=lm-studio');
	await expect(expanded('Open WebUI, LM Studio, Ollama')).toBeVisible();
	await expect(page.getByTestId('ai-guide').getByRole('heading')).toHaveText('LM Studio');
	await expect(page.getByTestId('ai-guide')).toContainText('Edit mcp.json');
	await expect(page.getByTestId('ai-guide')).not.toContainText('Admin Panel');
	// A row that covers several apps switches between their steps.
	const which = page.getByRole('group', { name: 'Which app' });
	await expect(which.getByRole('button', { name: 'LM Studio', pressed: true })).toBeVisible();
	await which.getByRole('button', { name: 'Open WebUI', exact: true }).click();
	await expect(page.getByTestId('ai-guide').getByRole('heading')).toHaveText('Open WebUI');
	await expect(page.getByTestId('ai-guide')).toContainText('Admin Panel');
	await expect(page).toHaveURL(/app=open-webui/);
	// An app without a row (the website's key steps say "pick Windsurf") still opens its steps.
	await page.goto('/settings/ai?app=windsurf');
	await expect(page.getByTestId('ai-guide').getByRole('heading')).toHaveText('Windsurf');
	await expect(page.getByTestId('ai-table').getByRole('button', { expanded: true })).toHaveCount(0);
	await page.goto('/settings/ai?app=nope');
	await expect(page.getByTestId('ai-table')).toBeVisible();
	await expect(page.getByTestId('ai-guide')).toHaveCount(0);
});

test('on a plain-http home address apps use an AI key, and cloud apps need https', async ({ browser }) => {
	// The page opens at a home-network address; its requests reach the test server.
	const real = new URL(page.url()).origin;
	const home = `http://10.254.254.254:${new URL(real).port}`;
	const ctx = await browser.newContext({ baseURL: home });
	// The live stream cannot pass this stand-in (it never ends); a quiet open one takes its place.
	await ctx.addInitScript(() => {
		window.EventSource = class {
			onopen: ((e: Event) => void) | null = null;
			onerror = null;
			onmessage = null;
			readyState = 1;
			constructor() {
				setTimeout(() => this.onopen?.(new Event('open')), 0);
			}
			close() {}
			addEventListener() {}
			removeEventListener() {}
		} as unknown as typeof EventSource;
	});
	await ctx.route(home + '/**', async (r) => {
		const req = r.request();
		const headers = { ...req.headers() };
		delete headers.origin;
		const res = await r.fetch({ url: real + req.url().slice(home.length), headers, maxRedirects: 0 });
		await r.fulfill({ response: res });
	});
	const p = await ctx.newPage();
	await signIn(p, owner());
	await p.goto('/settings/ai');
	await expect(p.getByTestId('ai-address')).toHaveText(home + '/api/v1/mcp');
	await expect(p.getByTestId('ai-mode')).toContainText('This is a plain-http address');
	await expect(p.getByTestId('ai-steps')).toContainText('Paste the AI key from Make a key');
	const table = p.getByTestId('ai-table');
	const row = (name: string) => table.getByRole('row').filter({ has: p.getByRole('button', { name, exact: true }) });
	await expect(row('Claude app')).toContainText('Needs a public https address.');
	await expect(row('ChatGPT')).toContainText('Needs a public https address.');
	await expect(row('Cursor')).not.toContainText('Needs a public https address.');
	// No one-click button and no header-less command on a home address.
	await expect(row('Cursor')).not.toContainText('Add to Cursor');
	await expect(row('Gemini CLI')).toContainText('--header "Authorization: Bearer <your AI key>"');
	await expect(p.getByTestId('ai-prompt')).toHaveText(
		`Read https://holyarrs.coderluii.dev/docs/ai/ai.md and connect yourself to my HolyArrs at ${home}. Only add the HolyArrs MCP server. Ask me before you change anything else. I'll give you an AI key when you ask.`
	);
	await expect(p.getByText('first to put your key in the prompt.')).toBeVisible();

	// Make a key fills the app's steps and the agent prompt.
	await table.getByRole('button', { name: 'Cursor', exact: true }).click();
	const guide = p.getByTestId('ai-guide');
	await expect(guide.getByRole('link', { name: 'Add to Cursor' })).toHaveCount(0);
	await guide.getByRole('button', { name: 'Make a key for Cursor' }).click();
	const fresh = p.getByRole('status').filter({ hasText: 'shown only once' });
	const k = (await fresh.locator('code').textContent())!.trim();
	expect(k.length).toBeGreaterThan(10);
	await expect(guide).toContainText(`"Authorization": "Bearer ${k}"`);
	await expect(guide.getByRole('button', { name: 'Make a key for Cursor' })).toHaveCount(0);
	await expect(p.getByTestId('ai-prompt')).toContainText(`Use this AI key: ${k}. Only add`);
	await expect(p.getByText('Contains your key. Paste only into your own agent.')).toBeVisible();
	expect([...new URL(p.url()).searchParams.keys()]).toEqual(['app']);
	expect(p.url()).not.toContain(k);

	// The key is Cursor's only: another app's steps and its prompt never get it.
	await table.getByRole('button', { name: 'Claude Code', exact: true }).click();
	await expect(guide.getByRole('heading')).toHaveText('Claude Code');
	await expect(guide).not.toContainText(k);
	await expect(guide.getByRole('button', { name: 'Make a key for Claude Code' })).toBeVisible();
	await expect(p.getByTestId('ai-prompt')).not.toContainText(k);

	// A cloud app says it needs a public https address.
	await table.getByRole('button', { name: 'Claude app', exact: true }).click();
	await expect(p.getByTestId('ai-unreachable')).toHaveText('Needs a public https address.');

	// Revoke asks first, then the key is gone, from the list and from Cursor's steps.
	const keys = p.locator('#ai-keys');
	await keys.getByRole('listitem').filter({ hasText: 'Cursor' }).getByRole('button', { name: 'Revoke' }).click();
	await p.getByRole('dialog', { name: 'Revoke Cursor?' }).getByRole('button', { name: 'Revoke' }).click();
	await expect(keys.getByRole('listitem').filter({ hasText: 'Cursor' })).toHaveCount(0);
	await table.getByRole('button', { name: 'Cursor', exact: true }).click();
	await expect(guide).not.toContainText(k);
	await expect(guide.getByRole('button', { name: 'Make a key for Cursor' })).toBeVisible();
	await expect(p.getByTestId('ai-prompt')).not.toContainText(k);
	await ctx.close();
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
	await expect(page).toHaveURL(/\/ai\?/);
	const card = page.getByTestId('ai-connect');
	await expect(card).toContainText('wants to use HolyArrs');
	await expect(card).toContainText("Says it's Test Agent");
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
	const ai = page.getByRole('group', { name: 'Who' }).getByRole('button', { name: 'AI apps', exact: true });
	await ai.click();
	await expect(ai).toHaveAttribute('aria-pressed', 'true');
	await page.keyboard.press('Escape');
});

test('AI apps and plain-words search fit a phone screen', async () => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/settings/ai');
	await expect(page.getByRole('checkbox', { name: 'Let AI apps connect' })).toBeVisible();
	await noSidewaysScroll(page);
	// On a phone a picked row opens its steps right under it; long config still fits.
	for (const name of ['Claude Desktop at home', 'Gemini CLI', 'Any other app']) {
		await page.getByTestId('ai-table').getByRole('button', { name, exact: true }).click();
		await expect(page.getByTestId('ai-table').getByTestId('ai-guide')).toBeVisible();
		await noSidewaysScroll(page);
	}
	await page.goto('/search?q=' + encodeURIComponent('driftwood moon movie'));
	await expect(page.getByTestId('search-plain')).toBeVisible();
	await noSidewaysScroll(page);
	// Leave it off for the other specs.
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.goto('/settings/ai');
	await page.getByRole('checkbox', { name: 'Let AI apps connect' }).uncheck();
});
