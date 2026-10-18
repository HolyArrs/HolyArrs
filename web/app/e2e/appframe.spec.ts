import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// The rest of the app frame (3.4.3, 3.5.7-3.5.12): signed-in devices, This
// app (storage, log download, update notice), Schedules, the log viewer and
// the API page; each at phone width and through axe.
test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
	page = await ctx.newPage();
	await signIn(page, owner());
});

async function axe(p: Page, where: string) {
	// Scan settled screens: toasts and sheets animate in.
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(r.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`), where).toEqual([]);
}

async function phone(p: Page, path: string) {
	await p.setViewportSize({ width: 390, height: 844 });
	await p.goto(path);
	await p.waitForLoadState('networkidle');
	await noSidewaysScroll(p);
	await p.setViewportSize({ width: 1280, height: 900 });
}

test('signed-in devices: see them and sign the others out', async ({ browser }) => {
	const other = await (await browser.newContext()).newPage();
	await signIn(other, owner());
	await page.goto('/account');
	const list = page.getByTestId('sessions');
	// Earlier tests left sessions too: at least this one and the other.
	await expect(list.locator('[data-current="true"]')).toContainText('This device');
	expect(await list.getByTestId('session').count()).toBeGreaterThanOrEqual(2);
	await list.getByRole('button', { name: 'Sign out all other devices' }).click();
	await expect(page.getByRole('status').filter({ hasText: /Signed out \d+ devices?\./ })).toBeVisible();
	await expect(list.getByTestId('session')).toHaveCount(1);
	await other.goto('/library');
	await expect(other).toHaveURL(/\/login/);
	await axe(page, '/account');
	await phone(page, '/account');
	await other.context().close();
});

test('the sign-in page says how to reset the owner', async ({ browser }) => {
	const p = await (await browser.newContext()).newPage();
	await p.goto('/login');
	await p.getByText('Forgot the password?').click();
	await expect(p.getByTestId('reset-owner-help')).toContainText('reset-owner');
	await p.context().close();
});

test('This app shows where things are kept, and the log', async () => {
	await page.goto('/settings/this-app');
	const store = page.getByTestId('storage');
	await expect(store.getByTestId('storage-path').filter({ hasText: 'Main database' })).toContainText('holyarrs.db');
	await expect(store.getByTestId('storage-media').first()).toContainText('Library folder');
	const dl = page.getByRole('link', { name: 'Download the log' });
	await expect(dl).toHaveAttribute('href', /\/api\/v1\/logs\/download$/);
	const [file] = await Promise.all([page.waitForEvent('download'), dl.click()]);
	expect(file.suggestedFilename()).toMatch(/^holyarrs-log-.*\.txt$/);
	await page.getByRole('link', { name: 'Read the log' }).click();
	await expect(page.getByRole('heading', { name: 'Logs', level: 1 })).toBeVisible();
	await expect(page.getByTestId('log-line').first()).toBeVisible();
	await page.getByLabel('Text').fill('listening on');
	await page.getByRole('button', { name: 'Show', exact: true }).click();
	await expect(page.getByTestId('log-line').first()).toContainText('listening on');
	await page.getByLabel('Level').selectOption('error');
	await expect(page.getByTestId('log-lines').getByRole('status')).toBeVisible();
	for (const l of await page.getByTestId('log-line').all()) await expect(l).toHaveAttribute('data-level', 'error');
	// The first-run setup code is in the log, hidden.
	await page.getByRole('button', { name: 'Clear' }).click();
	await page.getByLabel('Text').fill('setup code');
	await page.getByRole('button', { name: 'Show', exact: true }).click();
	await expect(page.getByTestId('log-line').first()).toContainText('setup code [hidden]');
	await axe(page, '/settings/logs');
	await phone(page, '/settings/logs');
	await axe(page, '/settings/this-app');
	await phone(page, '/settings/this-app');
});

test('Schedules: next feed check, run it now, change how often', async () => {
	await page.goto('/settings');
	await page.getByRole('main').locator('.links').getByRole('link').filter({ hasText: 'Schedules' }).click();
	await expect(page.getByRole('heading', { name: 'Schedules', level: 1 })).toBeVisible();
	const feed = page.locator('[data-testid="schedule"][data-kind="feed_check"]');
	await expect(feed).toContainText('Feed check');
	await expect(feed.getByTestId('next')).toContainText('Next:');
	await feed.getByRole('button', { name: 'Run Feed check now' }).click();
	await expect(page.getByRole('status').filter({ hasText: /Feed check (started|is already waiting or running)\./ })).toBeVisible();
	await feed.getByRole('button', { name: 'Change how often Feed check runs' }).click();
	// The box holds the bounds (10 to 120 minutes); the server checks them too.
	await expect(feed.getByLabel('Every (minutes)')).toHaveAttribute('min', '10');
	await feed.getByLabel('Every (minutes)').fill('30');
	await feed.getByRole('button', { name: 'Save' }).click();
	await expect(feed).toContainText('Runs every 30 minutes');
	await expect(page.getByTestId('loop').first()).toBeVisible();
	await axe(page, '/settings/schedules');
	await phone(page, '/settings/schedules');
});

test('a newer release shows a notice; off asks nothing', async () => {
	await page.goto('/settings/this-app');
	await page.getByLabel('The release feed may be on the local network').check();
	await page.getByRole('button', { name: 'Save' }).click();
	await page.getByRole('button', { name: 'Check for updates now' }).click();
	await expect(page.getByTestId('update-words')).toContainText('HolyArrs v9.9.9 is out');
	await expect(page.getByRole('link', { name: 'What is new in v9.9.9' })).toHaveAttribute('href', 'https://github.com/CoderLuii/holyarrs/releases/tag/v9.9.9');
	await expect(page.getByTestId('update-privacy')).toContainText('api.github.com');
	await page.getByLabel('Check once a day for a newer HolyArrs').uncheck();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByTestId('update-words')).toContainText('The update check is off');
	await expect(page.getByRole('button', { name: 'Check for updates now' })).toHaveCount(0);
	await page.getByLabel('Check once a day for a newer HolyArrs').check();
	await page.getByRole('button', { name: 'Save' }).click();
	await axe(page, '/settings/this-app');
});

test('the API page: key, scopes, base address, examples and every route', async () => {
	await page.goto('/settings');
	await page.getByRole('main').locator('.links').getByRole('link').filter({ hasText: 'API for other apps' }).click();
	await expect(page.getByRole('heading', { name: 'API for other apps', level: 1 })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Settings > Sign-in > API keys' })).toHaveAttribute('href', '/settings/sign-in');
	await expect(page.getByTestId('api-base')).toHaveText(/^http:\/\/127\.0\.0\.1:\d+\/api\/v1$/);
	await expect(page.getByTestId('api-scopes')).toContainText('control');
	expect(await page.getByTestId('api-op').count()).toBeGreaterThan(150);
	await page.getByLabel('Find a route').fill('schedules');
	await expect(page.getByTestId('api-op').first()).toContainText('/api/v1/schedules');
	await page.getByTestId('api-op').first().getByRole('button').click();
	await expect(page.getByTestId('api-op').first()).toContainText('Scope');
	const raw = await page.request.get('/api/v1/openapi.json');
	expect(raw.ok()).toBeTruthy();
	expect((await raw.json()).openapi).toMatch(/^3\./);
	await axe(page, '/settings/api');
	await phone(page, '/settings/api');
});
