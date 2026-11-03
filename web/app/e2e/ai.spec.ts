import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

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
});

test('AI connections are off until the owner turns them on, then an AI key works only there', async () => {
	await page.goto('/settings/ai');
	const sw = page.getByRole('checkbox', { name: 'Let AI apps connect' });
	await expect(sw).not.toBeChecked();
	await page.getByLabel('Key name').fill('Claude Desktop');
	await page.getByRole('button', { name: 'Make an AI key' }).click();
	const fresh = page.getByRole('status').filter({ hasText: 'shown only once' });
	key = (await fresh.locator('code').textContent())!.trim();
	expect(key.length).toBeGreaterThan(10);

	// Off: the route is not there.
	expect((await mcp({ method: 'ping' })).status()).toBe(404);
	await sw.check();
	await expect(sw).toBeChecked();

	const list = await (await mcp({ method: 'tools/list' })).json();
	const names = list.result.tools.map((t: { name: string }) => t.name);
	expect(names).toContain('search_titles');
	expect(names).toContain('ask_for_title');

	// The AI key reaches nothing else.
	const other = await page.request.get('/api/v1/titles', { headers: { 'X-Api-Key': key } });
	expect(other.status()).toBe(403);
});

test('an AI app finds a title from a description', async () => {
	const r = await (await mcp({ method: 'tools/call', params: { name: 'search_titles', arguments: { query: 'quiet orchard movie' } } })).json();
	expect(r.result.isError).toBe(false);
	expect(JSON.stringify(r.result.structuredContent)).toContain('Quiet Orchard');
});

test('search in plain words shows what it understood', async () => {
	await page.goto('/search?q=' + encodeURIComponent('quiet orchard movie'));
	const plain = page.getByTestId('search-plain');
	await expect(plain).toContainText('Understood as');
	await expect(plain).toContainText('Quiet Orchard');
});

test('History has an AI filter', async () => {
	await page.goto('/downloads/history');
	await expect(page.getByRole('button', { name: 'AI', exact: true }).or(page.getByRole('checkbox', { name: 'AI', exact: true })).or(page.getByText('AI', { exact: true })).first()).toBeVisible();
});

test('AI connections and plain-words search fit a phone screen', async () => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/settings/ai');
	await expect(page.getByRole('checkbox', { name: 'Let AI apps connect' })).toBeVisible();
	await noSidewaysScroll(page);
	await page.goto('/search?q=' + encodeURIComponent('quiet orchard movie'));
	await expect(page.getByTestId('search-plain')).toBeVisible();
	await noSidewaysScroll(page);
	// Leave it off for the other specs.
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.goto('/settings/ai');
	await page.getByRole('checkbox', { name: 'Let AI apps connect' }).uncheck();
});
