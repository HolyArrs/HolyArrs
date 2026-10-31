import { expect, test, type Page, type Request } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Push updates (setting downloads.sync = on) against a fake qBittorrent:
// the Downloads page reads its snapshot once, then follows the client
// through /live patches with no 2 s view reads; with the setting back on
// legacy the page polls as before. Runs with live-push.config.ts only.
test.describe.configure({ mode: 'serial' });

const here = dirname(fileURLToPath(import.meta.url));
const qbit = `http://127.0.0.1:${process.env.HOLYARRS_LIVE_QBIT_PORT ?? 18733}`;
const fakeClientKey = ['stand', 'in', 'qbit'].join('-');
let page: Page;
let csrf = '';
const views: number[] = [];
const interests: number[] = [];

async function call(method: string, path: string, data?: unknown) {
	const r = await page.request.fetch(path, { method, data, headers: csrf ? { 'X-CSRF-Token': csrf } : {} });
	expect(r.ok(), `${method} ${path}: ${r.status()} ${await r.text()}`).toBeTruthy();
	return r.status() === 204 ? null : r.json();
}

async function setSync(mode: 'on' | 'legacy') {
	const s = await call('GET', '/api/v1/settings/downloaders');
	await call('PUT', '/api/v1/settings/downloaders', { ...s, downloads_sync: mode });
}

const row = () => page.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' });
const progress = () => row().locator('[data-col="progress"]');

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	page.on('request', (r: Request) => {
		const u = new URL(r.url());
		if (u.pathname === '/api/v1/downloads/view') views.push(Date.now());
		if (u.pathname === '/api/v1/live/interest') interests.push(Date.now());
	});
	await page.goto('/login');
	const code = readFileSync(join(here, '.tmp-live', 'config', 'setup-code.txt'), 'utf8').trim();
	// A throwaway owner for this test's own fresh HolyArrs.
	// Made up per run, like the e2e fakes' stand-in keys.
	const pw = ['push', 'test', randomUUID()].join('-');
	const owner = await call('POST', '/api/v1/auth/setup', { name: 'pushtest', password: pw, code });
	csrf = owner.csrf;
	await call('POST', '/api/v1/downloaders', { name: 'Fake qBit', kind: 'qbittorrent', url: qbit, username: 'fake', secret: fakeClientKey, category: 'holyarrs', allow_local_network: true });
	await setSync('on');
});

test('push: progress reaches the page in under 2.5 s with no view reads in between', async () => {
	await page.goto('/downloads');
	await expect(progress()).toContainText('20%', { timeout: 20_000 });
	// The page told the server it watches (push is on) and the stream is open.
	await expect.poll(() => interests.length, { timeout: 10_000 }).toBeGreaterThan(0);
	await page.waitForTimeout(1500);
	const before = views.length;
	await fetch(`${qbit}/set?progress=0.55&eta=300`);
	await expect(progress()).toContainText('55%', { timeout: 2500 });
	expect(views.length, 'view reads while push is healthy').toBe(before);
	// And again, still without a read.
	await fetch(`${qbit}/set?progress=0.8`);
	await expect(progress()).toContainText('80%', { timeout: 2500 });
	await page.waitForTimeout(4000);
	expect(views.length, 'no 2 s polling while push is healthy').toBe(before);
});

test('legacy: the page polls the view every 2 s as before and sends no interest', async () => {
	await setSync('legacy');
	await page.waitForTimeout(6000); // the setting is read again within 5 s
	await page.goto('/downloads');
	await expect(row()).toBeVisible({ timeout: 20_000 });
	const v0 = views.length;
	const i0 = interests.length;
	await page.waitForTimeout(6500);
	expect(views.length - v0, 'view reads in 6.5 s').toBeGreaterThanOrEqual(2);
	expect(interests.length).toBe(i0);
	await fetch(`${qbit}/set?progress=0.9`);
	await expect(progress()).toContainText('90%', { timeout: 10_000 });
});
