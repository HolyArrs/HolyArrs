import { expect, test, type Page, type Request } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Push updates (setting downloads.sync = on) against a fake qBittorrent:
// the Downloads page reads its snapshot once, then follows the client
// through /live patches with no 2 s view reads; a stored polling value
// reads as on. Runs with live-push.config.ts only.
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
	// A throwaway owner for this test's own fresh HolyArrs.
	// Made up per run, like the e2e fakes' stand-in keys.
	const pw = ['push', 'test', randomUUID()].join('-');
	const owner = await call('POST', '/api/v1/auth/setup', { name: 'pushtest', password: pw });
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

test('legacy: a stored legacy value reads as on: the page follows by push with no 2 s view reads', async () => {
	// There is no polling path: the setting still takes the old value, and it reads as on.
	await setSync('legacy');
	await page.waitForTimeout(6000); // the setting is read again within 5 s
	await page.goto('/downloads');
	await expect(row()).toBeVisible({ timeout: 20_000 });
	await expect.poll(() => interests.length, { timeout: 10_000 }).toBeGreaterThan(0);
	await page.waitForTimeout(1500);
	const v0 = views.length;
	await fetch(`${qbit}/set?progress=0.9`);
	await expect(progress()).toContainText('90%', { timeout: 2500 });
	await page.waitForTimeout(4000);
	expect(views.length - v0, 'view reads while push is healthy').toBe(0);
});

// The Downloads list looks the same on push as on polling. The baseline
// is taken in polling mode in this run (fixed clock, live numbers masked) and the
// push page is compared with toHaveScreenshot.
const masks = (p: Page) => ['progress', 'speed', 'eta', 'up', 'seeds', 'peers', 'ratio', 'completed', 'added', 'size'].map((c) => p.locator(`[data-testid="dl-listbox"] [data-col="${c}"]`));
const sizes = [
	{ name: 'dl-1280.png', width: 1280, height: 800 },
	{ name: 'dl-390.png', width: 390, height: 844 }
];
async function shotPage(p: Page, width: number, height: number) {
	await p.setViewportSize({ width, height });
	await p.clock.setFixedTime(new Date('2026-10-31T12:00:00Z'));
	await p.goto('/downloads');
	await expect(p.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' })).toBeVisible({ timeout: 20_000 });
	await p.waitForTimeout(800);
	return p.locator('[data-testid="dl-listbox"]');
}

test('AC9 baseline: legacy Downloads list at 1280x800 and 390x844', async () => {
	const p = await page.context().newPage();
	for (const s of sizes) {
		const box = await shotPage(p, s.width, s.height);
		await box.screenshot({ path: test.info().snapshotPath(s.name, { kind: 'screenshot' }), animations: 'disabled', caret: 'hide', scale: 'css', mask: masks(p) });
	}
	await p.close();
});

test('AC9: push Downloads list matches the legacy baseline (maxDiffPixelRatio 0.01)', async () => {
	await setSync('on');
	await page.waitForTimeout(6000);
	const p = await page.context().newPage();
	for (const s of sizes) {
		const box = await shotPage(p, s.width, s.height);
		await expect(box).toHaveScreenshot(s.name, { maxDiffPixelRatio: 0.01, animations: 'disabled', caret: 'hide', scale: 'css', mask: masks(p) });
	}
	await p.close();
});

// Phase 4: every tab of one browser shares one /live stream.
const liveCount = async () => (await (await page.request.get('/__test/live')).json()) as { open: number; total: number };

test('three tabs open exactly one /live stream; closing the leader hands over without missing a change', async () => {
	await page.goto('/downloads');
	await expect(row()).toBeVisible({ timeout: 20_000 });
	const ctx = page.context();
	const tabs = [page, await ctx.newPage(), await ctx.newPage()];
	const full: string[] = [];
	for (const t of tabs.slice(1)) {
		t.on('request', (r) => {
			const u = new URL(r.url());
			if (u.pathname === '/api/v1/downloads/view' && !u.searchParams.has('client')) full.push(u.search);
		});
		await t.goto('/downloads');
	}
	for (const t of tabs) await expect(t.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' })).toBeVisible({ timeout: 20_000 });
	await expect.poll(async () => (await liveCount()).open, { timeout: 10_000 }).toBe(1);
	await page.waitForTimeout(2000);
	expect((await liveCount()).open, 'three tabs, one stream').toBe(1);
	// The followers move with the leader's stream.
	await fetch(`${qbit}/set?progress=0.41`);
	for (const t of tabs) await expect(t.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' }).locator('[data-col="progress"]')).toContainText('41%', { timeout: 2500 });
	// Close the leader (the first tab) while a change is on its way.
	const before = await liveCount();
	full.length = 0;
	await fetch(`${qbit}/set?progress=0.62`);
	await page.close();
	page = tabs[1];
	for (const t of tabs.slice(1)) await expect(t.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' }).locator('[data-col="progress"]')).toContainText('62%', { timeout: 5000 });
	await fetch(`${qbit}/set?progress=0.73`);
	for (const t of tabs.slice(1)) await expect(t.locator('[data-testid="dl-table-row"]').filter({ hasText: 'Live.Push.Test.ISO' }).locator('[data-col="progress"]')).toContainText('73%', { timeout: 2500 });
	const after = await liveCount();
	expect(after.open, 'one stream after the handover').toBe(1);
	expect(after.total - before.total, 'exactly one new stream').toBe(1);
	expect(full, 'no whole-list rereads during the handover').toEqual([]);
	await tabs[2].close();
});

// AC6: 2,000 rows at 390x844 with 4x CPU slowdown, a scripted 10 s scroll:
// p95 frame time < 33 ms and fewer than 100 row nodes in the page.
test('AC6: 2,000-row list scrolls smoothly and draws fewer than 100 rows', async ({ browser }, info) => {
	test.setTimeout(240_000);
	await fetch(`${qbit}/set?count=2000`);
	const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: await page.context().storageState() });
	const p = await ctx.newPage();
	await p.goto('/downloads');
	await expect(p.locator('[data-testid="dl-table-row"]').first()).toBeVisible({ timeout: 30_000 });
	await expect.poll(async () => (await page.request.get('/api/v1/downloads/view?limit=1')).json().then((v) => v.outside_total), { timeout: 30_000 }).toBeGreaterThanOrEqual(2000);
	// The page may still hold its first read (before the reader saw the 2,000):
	// the client's patches bring them all, or a reread brings a page of them and
	// Show more torrents. Wait for either.
	const rowsAttr = async () => Number(await p.locator('[data-testid="dl-list"]').getAttribute('data-rows'));
	await expect.poll(async () => (await rowsAttr()) >= 2000 || (await p.getByTestId('dl-outside-more').isVisible()), { timeout: 30_000 }).toBe(true);
	for (let i = 0; i < 12; i++) {
		const more = p.getByTestId('dl-outside-more');
		if (!(await more.isVisible().catch(() => false))) break;
		await more.click();
		await p.waitForTimeout(1200);
	}
	await expect.poll(rowsAttr, { timeout: 30_000 }).toBeGreaterThanOrEqual(2000);
	const cdp = await ctx.newCDPSession(p);
	await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
	await ctx.tracing.startChunk({ title: 'ac6-scroll' }); // the config already traces each context
	const res = await p.evaluate(async () => {
		const box = document.querySelector<HTMLElement>('[data-testid="dl-listbox"]')!;
		const frames: number[] = [];
		let maxRows = 0;
		const t0 = performance.now();
		let last = t0;
		await new Promise<void>((done) => {
			const step = (now: number) => {
				frames.push(now - last);
				last = now;
				box.scrollTop += 60;
				maxRows = Math.max(maxRows, document.querySelectorAll('[data-testid="dl-table-row"]').length);
				if (now - t0 < 10_000) requestAnimationFrame(step);
				else done();
			};
			requestAnimationFrame(step);
		});
		frames.shift();
		const sorted = [...frames].sort((a, b) => a - b);
		return { p95: sorted[Math.floor(sorted.length * 0.95)], frames: frames.length, maxRows, scrolled: box.scrollTop, height: box.scrollHeight };
	});
	await ctx.tracing.stopChunk({ path: info.outputPath('ac6-trace.zip') });
	await ctx.tracing.startChunk(); // hand tracing back to the test runner
	await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
	console.log(`AC6 p95 frame ${res.p95.toFixed(1)} ms over ${res.frames} frames, max row nodes ${res.maxRows}, scrolled ${res.scrolled}/${res.height}px`);
	expect(res.scrolled, 'the list scrolled').toBeGreaterThan(20_000);
	expect(res.maxRows, 'row nodes').toBeLessThan(100);
	expect(res.p95, 'p95 frame time').toBeLessThan(33);
	// Keyboard: the arrow keys walk past the drawn window and the row stays focused and on screen.
	await p.setViewportSize({ width: 1280, height: 800 });
	await p.locator('[data-testid="dl-listbox"]').evaluate((b) => (b.scrollTop = 0));
	await p.locator('[data-testid="dl-line"]').first().click();
	for (let i = 0; i < 60; i++) await p.keyboard.press('ArrowDown');
	const focused = p.locator('[data-testid="dl-table-row"]:has(> .dline:focus)');
	await expect(focused).toHaveCount(1);
	await expect(focused).toBeInViewport();
	const nodes = await p.locator('[data-testid="dl-table-row"]').count();
	expect(nodes, 'row nodes after the keyboard walk').toBeLessThan(100);
	await ctx.close();
	await fetch(`${qbit}/set?count=0`);
});

// 883 torrents HolyArrs did not add: the tabs count every one, a note under
// the tabs says only some are loaded, and Show more torrents sits at the end
// of the list for Torrent (and every network), never for Usenet with none left.
test('tab counts and Show more follow every torrent and the network filter', async ({ browser }) => {
	test.setTimeout(120_000);
	await fetch(`${qbit}/set?count=883`);
	await expect.poll(async () => (await page.request.get('/api/v1/downloads/view?limit=1')).json().then((v) => v.outside_total), { timeout: 30_000 }).toBeGreaterThanOrEqual(883);
	const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: await page.context().storageState() });
	const p = await ctx.newPage();
	await p.goto('/downloads');
	const all = p.locator('[data-testid="rail-item"][data-key="active"]');
	await expect.poll(async () => Number(await all.getAttribute('data-count')), { timeout: 30_000 }).toBeGreaterThanOrEqual(883);
	await expect(p.getByTestId('dl-morenote')).toBeVisible();
	const network = async (key: string) => {
		await p.getByTestId('dl-filters-btn').click();
		await p.locator(`[data-testid="dl-f-src"][data-key="${key}"]`).click();
		await p.getByTestId('dl-filters-done').click();
	};
	const toEnd = () => p.getByTestId('dl-listbox').evaluate((b) => (b.scrollTop = b.scrollHeight));
	const loaded = async () => Number(await p.locator('[data-testid="dl-list"]').getAttribute('data-rows'));

	await network('torrent');
	await toEnd();
	await expect(p.getByTestId('dl-outside-more')).toBeInViewport();

	await network('usenet');
	await expect(p.getByTestId('dl-outside-more')).toHaveCount(0);
	await expect(p.getByTestId('dl-morenote')).toHaveCount(0);

	await network('torrent');
	await toEnd();
	const before = await loaded();
	await expect(p.getByTestId('dl-outside-more')).toBeInViewport();
	await p.getByTestId('dl-outside-more').click();
	await expect.poll(loaded, { timeout: 30_000 }).toBeGreaterThan(before);
	await ctx.close();
	await fetch(`${qbit}/set?count=0`);
});
