import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { plainForm } from '../helpers/helpers.ts';

type Kind = 'qbittorrent' | 'sabnzbd' | 'transmission' | 'nzbget' | 'deluge' | 'rtorrent';

const configDir = process.env.HOLYARRS_OPTIONAL_UI_CONFIG;
const libraryDir = process.env.HOLYARRS_OPTIONAL_UI_LIBRARY;
const screenshotDir = process.env.HOLYARRS_OPTIONAL_UI_SCREENSHOTS;
// A hand-prepared run (a fresh app plus its folders, given in these variables);
// without them the spec is skipped, with the reason, instead of failing to load.
test.skip(!process.env.HOLYARRS_OPTIONAL_UI_URL || !configDir || !libraryDir || !screenshotDir, 'needs a prepared optional-downloaders run: HOLYARRS_OPTIONAL_UI_URL, _CONFIG, _LIBRARY and _SCREENSHOTS');

const owner = { name: 'optional-ui-owner', password: `pw-${crypto.randomUUID()}` };
const savedSecrets = new Map<Kind, string>();

const kinds: { kind: Kind; label: string; username: boolean; secretLabel: 'Password' | 'API key'; category: string }[] = [
	{ kind: 'qbittorrent', label: 'qBittorrent', username: true, secretLabel: 'Password', category: 'holyarrs' },
	{ kind: 'transmission', label: 'Transmission', username: true, secretLabel: 'Password', category: 'holyarrs' },
	{ kind: 'deluge', label: 'Deluge', username: false, secretLabel: 'Password', category: 'holyarrs' },
	{ kind: 'rtorrent', label: 'rTorrent', username: true, secretLabel: 'Password', category: 'holyarrs' },
	{ kind: 'sabnzbd', label: 'SABnzbd', username: false, secretLabel: 'API key', category: 'movies' },
	{ kind: 'nzbget', label: 'NZBGet', username: true, secretLabel: 'Password', category: 'holyarrs' }
];

async function noSidewaysScroll(page: Page) {
	const width = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
	expect(width.scroll, `page width ${width.scroll}px exceeds viewport ${width.client}px`).toBeLessThanOrEqual(width.client);
}

async function seriousA11y(page: Page, label: string) {
	await page.evaluate(() => Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => undefined))));
	const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = result.violations.filter((violation) => violation.impact === 'serious' || violation.impact === 'critical');
	expect(bad.map((violation) => `${violation.id}: ${violation.nodes.slice(0, 3).map((node) => node.target.join(' ')).join(', ')}`), label).toEqual([]);
}

async function signIn(page: Page) {
	await page.goto('/login');
	await plainForm(page);
	await page.getByLabel('Name').fill(owner.name);
	await page.getByLabel('Password', { exact: true }).fill(owner.password);
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(page).toHaveURL(/\/$/);
}

async function bootstrap(page: Page) {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible();
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(owner.name);
	await page.getByLabel('Password', { exact: true }).fill(owner.password);
	await page.getByLabel('Password again').fill(owner.password);
	await page.getByRole('button', { name: 'Create account' }).click();

	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	const firstRun = await page.evaluate(async ({ folder }) => {
		const me = await fetch('/api/v1/auth/me');
		if (!me.ok) throw new Error(`first-run session check returned ${me.status}`);
		const { csrf } = (await me.json()) as { csrf: string };
		async function save(path: string, method: 'POST' | 'PUT', body: object) {
			const response = await fetch(`/api/v1${path}`, {
				method,
				headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
				body: JSON.stringify(body)
			});
			if (!response.ok) throw new Error(`first-run API ${path} returned ${response.status}`);
		}
		await save('/library-folders', 'POST', { path: folder, kind: 'movie', label: 'Optional UI Library' });
		return true;
	}, { folder: libraryDir! });
	expect(firstRun).toBe(true);
}

async function openAdd(page: Page) {
	await page.getByRole('button', { name: 'Add a Downloader' }).first().click();
	await expect(page.getByRole('heading', { name: 'Add a Downloader' })).toBeVisible();
}

async function choose(page: Page, label: string) {
	await page.getByRole('radio', { name: new RegExp(`^${label}`) }).check();
}

async function verifyFieldMatrix(page: Page) {
	await openAdd(page);
	for (const item of kinds) {
		await choose(page, item.label);
		await expect(page.getByLabel('Username')).toHaveCount(item.username ? 1 : 0);
		await expect(page.getByLabel('API key', { exact: true })).toHaveCount(item.secretLabel === 'API key' ? 1 : 0);
		await expect(page.getByLabel('Password', { exact: true })).toHaveCount(item.secretLabel === 'Password' ? 1 : 0);
		if (item.kind === 'deluge') await expect(page.getByText(/connected to a daemon with the Label plugin on/)).toBeVisible();
		if (item.kind === 'rtorrent') await expect(page.getByText(/plugins\/httprpc\/action\.php/)).toBeVisible();
	}
	await noSidewaysScroll(page);
	await seriousA11y(page, 'desktop add form');
	await page.getByRole('button', { name: 'Cancel' }).click();
}

async function addDownloader(page: Page, item: (typeof kinds)[number]) {
	await openAdd(page);
	await choose(page, item.label);
	const secret = `secret-${crypto.randomUUID()}`;
	savedSecrets.set(item.kind, secret);
	await page.getByLabel('Name', { exact: true }).fill(`UI ${item.label}`);
	await page.getByLabel('Address', { exact: true }).fill('http://127.0.0.1:9');
	if (item.username) await page.getByLabel('Username').fill(`user-${item.kind}`);
	await page.getByLabel(item.secretLabel, { exact: true }).fill(secret);
	await page.getByLabel('Category', { exact: true }).fill(item.category);

	const responsePromise = page.waitForResponse((response) => {
		const request = response.request();
		return request.method() === 'POST' && new URL(response.url()).pathname === '/api/v1/downloaders';
	});
	await page.locator('form.sc-form').first().getByRole('button', { name: 'Save', exact: true }).click();
	const response = await responsePromise;
	expect([200, 201]).toContain(response.status());
	const body = (await response.json()) as { kind: Kind; username: string; secret: string };
	expect(body.kind).toBe(item.kind);
	expect(body.username).toBe(item.username ? `user-${item.kind}` : '');
	if (!body.secret || body.secret === secret) throw new Error('downloader API did not return a masked stored secret');

	const row = page.getByTestId('downloader').filter({ hasText: `UI ${item.label}` });
	await expect(row).toBeVisible({ timeout: 30_000 });
	if ((await row.innerText()).includes(secret)) throw new Error('stored downloader secret rendered in the page');
}

async function editWithMaskedSecret(page: Page, item: (typeof kinds)[number], priority: number) {
	const row = page.getByTestId('downloader').filter({ hasText: `UI ${item.label}` });
	await row.hover(); // row buttons show on hover
	await row.getByRole('button', { name: 'Change' }).click();
	const secretInput = page.getByLabel(item.secretLabel, { exact: true });
	const masked = await secretInput.inputValue();
	if (!masked || masked === savedSecrets.get(item.kind)) throw new Error('stored secret was not masked in the edit form');
	await page.getByLabel('Priority', { exact: true }).fill(String(priority));
	const responsePromise = page.waitForResponse((response) => response.request().method() === 'PUT' && /\/api\/v1\/downloaders\/\d+$/.test(new URL(response.url()).pathname));
	await page.locator('form.sc-form').first().getByRole('button', { name: 'Save', exact: true }).click();
	const response = await responsePromise;
	expect(response.status()).toBe(200);
	const body = (await response.json()) as { priority: number; secret: string };
	expect(body.priority).toBe(priority);
	if (!body.secret || body.secret === savedSecrets.get(item.kind)) throw new Error('masked edit did not preserve the stored secret');
	await expect(row).toContainText(`priority ${priority}`);
}

async function verifyStoredAPI(page: Page) {
	const stored = await page.evaluate(async () => {
		const response = await fetch('/api/v1/downloaders');
		if (!response.ok) throw new Error('downloaders API failed');
		return (await response.json()).downloaders as { kind: Kind; username: string; secret: string }[];
	});
	expect(stored.map((item) => item.kind).sort()).toEqual(kinds.map((item) => item.kind).sort());
	for (const item of kinds) {
		const got = stored.find((candidate) => candidate.kind === item.kind);
		if (!got || !got.secret || got.secret === savedSecrets.get(item.kind)) throw new Error('downloaders API exposed or lost a stored secret');
		expect(got.username).toBe(item.username ? `user-${item.kind}` : '');
	}
}

async function verifyUncertainSendRecovery(page: Page) {
	const nzbgetID = await page.evaluate(async () => {
		const response = await fetch('/api/v1/downloaders');
		if (!response.ok) throw new Error('downloaders API failed before attempt mock');
		const body = (await response.json()) as { downloaders: { id: number; kind: Kind }[] };
		const nzbget = body.downloaders.find((item) => item.kind === 'nzbget');
		if (!nzbget) throw new Error('NZBGet row is missing');
		return nzbget.id;
	});
	let pending = true;
	let abandonPosts = 0;
	const attemptID = 901;
	const releaseID = 42;
	const slotID = 84;
	const pattern = '**/api/v1/downloaders/*/attempts**';
	await page.route(pattern, async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname;
		if (request.method() === 'GET') {
			const match = path.match(/^\/api\/v1\/downloaders\/(\d+)\/attempts$/);
			if (!match) throw new Error(`unexpected attempt GET ${path}`);
			const attempts = Number(match[1]) === nzbgetID && pending
				? [{ id: attemptID, downloader_id: nzbgetID, release_id: releaseID, slot_id: slotID, state: 'pending', created_at: '2026-10-15T20:00:00Z', updated_at: '2026-10-15T20:00:00Z' }]
				: [];
			await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ attempts }) });
			return;
		}
		if (request.method() === 'POST' && path === `/api/v1/downloaders/${nzbgetID}/attempts/${attemptID}/abandon`) {
			abandonPosts++;
			pending = false;
			await route.fulfill({ status: 204 });
			return;
		}
		throw new Error(`unexpected attempt request ${request.method()} ${path}`);
	});

	await page.reload();
	const row = page.getByTestId('downloader').filter({ hasText: 'UI NZBGet' });
	const warning = row.getByTestId('downloader-attempts');
	await expect(warning.getByRole('heading', { name: 'Check whether this download arrived' })).toBeVisible();
	await expect(warning).toContainText(`Release ${releaseID} · Slot ${slotID}`);
	await expect(warning).toContainText('does not cancel anything in NZBGet');
	await expect(warning).toContainText('stops HolyArrs from retrying this exact Release and Slot');
	await expect(warning.getByRole('button', { name: 'Give up on it' })).toBeVisible();
	await expect(warning.getByRole('button', { name: /retry/i })).toHaveCount(0);

	await noSidewaysScroll(page);
	for (const width of [320, 390]) {
		await page.setViewportSize({ width, height: 844 });
		await noSidewaysScroll(page);
		await expect(warning.getByRole('button', { name: 'Give up on it' })).toBeVisible();
	}
	const posts: string[] = [];
	const rememberPost = (request: { method(): string; url(): string }) => {
		if (request.method() === 'POST') posts.push(new URL(request.url()).pathname);
	};
	page.on('request', rememberPost);
	page.once('dialog', async (dialog) => {
		expect(dialog.message()).toContain(`Release ${releaseID}`);
		expect(dialog.message()).toContain(`Slot ${slotID}`);
		expect(dialog.message()).toContain('does not cancel anything in NZBGet');
		await dialog.accept();
	});
	await warning.getByRole('button', { name: 'Give up on it' }).click();
	await expect(warning).toHaveCount(0);
	page.off('request', rememberPost);
	expect(abandonPosts).toBe(1);
	expect(posts).toEqual([`/api/v1/downloaders/${nzbgetID}/attempts/${attemptID}/abandon`]);
	await page.unroute(pattern);
}

async function phoneCheck(browser: Browser) {
	const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
	const page = await context.newPage();
	await signIn(page);
	await page.goto('/settings/downloaders');
	await expect(page.getByTestId('downloader')).toHaveCount(5);
	await noSidewaysScroll(page);
	await seriousA11y(page, 'phone downloader list');
	await page.screenshot({ path: join(screenshotDir!, 'phone-downloaders.png'), fullPage: true });
	await openAdd(page);
	for (const item of kinds) await expect(page.getByRole('radio', { name: new RegExp(`^${item.label}`) })).toBeVisible();
	await noSidewaysScroll(page);
	await context.close();
}

test('optional downloader settings work in a fresh real app', async ({ browser, page }) => {
	test.setTimeout(180_000);
	await bootstrap(page);
	await page.goto('/settings/downloaders');
	await expect(page.getByRole('heading', { level: 1, name: 'Downloaders' })).toBeVisible();
	await verifyFieldMatrix(page);
	for (const item of kinds) await addDownloader(page, item);
	for (const [index, item] of kinds.entries()) await editWithMaskedSecret(page, item, index + 2);
	await verifyStoredAPI(page);
	await noSidewaysScroll(page);
	await seriousA11y(page, 'desktop downloader list');
	await page.screenshot({ path: join(screenshotDir!, 'desktop-downloaders.png'), fullPage: true });
	await verifyUncertainSendRecovery(page);
	await phoneCheck(browser);
});
