import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openAdvanced } from '../helpers/helpers.ts';

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
	const useAccount = p.getByRole('button', { name: 'Use a HolyArrs account' });
	if (await useAccount.isVisible()) await useAccount.click();
	await p.getByText("Can't sign in?").click();
	await expect(p.getByTestId('reset-owner-help')).toContainText('reset-owner');
	await p.context().close();
});

test('This app shows where things are kept, and the log', async () => {
	await page.goto('/settings/this-app');
	await page.getByTestId('more-options').getByText('More options').click();
	const store = page.getByTestId('storage');
	await expect(store.getByTestId('storage-path').filter({ hasText: 'Main database' })).toContainText('holyarrs.db');
	await expect(store.getByTestId('storage-media').first()).toContainText('Library folder');
	await page.goto('/settings');
	await page.getByRole('link', { name: /^Server/ }).click();
	await page.getByRole('link', { name: /^Logs/ }).click();
	await expect(page.getByRole('heading', { name: 'Logs', level: 1 })).toBeVisible();
	const dl = page.getByRole('link', { name: 'Download', exact: true });
	await expect(dl).toHaveAttribute('href', /\/api\/v1\/logs\/download$/);
	const [file] = await Promise.all([page.waitForEvent('download'), dl.click()]);
	expect(file.suggestedFilename()).toMatch(/^holyarrs-log-.*\.txt$/);
	await expect(page.getByTestId('log-line').first()).toBeVisible();
	await page.getByLabel('Text').fill('listening on');
	await page.getByRole('button', { name: 'Apply', exact: true }).click();
	await expect(page.getByTestId('log-line').first()).toContainText('listening on');
	await page.getByLabel('Level').selectOption('error');
	// Wait for the lines that answer the Level filter, not the ones shown before.
	await expect(page.getByTestId('log-lines')).toHaveAttribute('data-query', /(^|&)level=error(&|$)/);
	await expect(page.getByTestId('log-lines')).toHaveAttribute('aria-busy', 'false');
	for (const l of await page.getByTestId('log-line').all()) await expect(l).toHaveAttribute('data-level', 'error');
	await axe(page, '/settings/logs');
	await phone(page, '/settings/logs');
	await axe(page, '/settings/this-app');
	await phone(page, '/settings/this-app');
});

test('Schedules: next feed check, run it now, change how often', async () => {
	await page.goto('/settings');
	// Settings home: open the Downloads card, then its Schedules capsule.
	await page.getByTestId('settings-group').filter({ hasText: 'Downloads' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Schedules', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Schedules', level: 1 })).toBeVisible();
	const feed = page.locator('[data-testid="schedule"][data-kind="feed_check"]');
	await expect(feed).toContainText('Feed check');
	await expect(feed.getByTestId('next')).toContainText('Next run');
	await feed.getByRole('button', { name: 'Run Feed check now' }).click();
	await expect(page.getByRole('status').filter({ hasText: /Feed check (started|already in progress)/ })).toBeVisible();
	await feed.getByRole('button', { name: 'Change how often Feed check runs' }).click();
	// The box holds the bounds (10 to 120 minutes); the server checks them too.
	await expect(feed.getByLabel('Every (minutes)')).toHaveAttribute('min', '10');
	await feed.getByLabel('Every (minutes)').fill('30');
	await feed.getByRole('button', { name: 'Save' }).click();
	await expect(feed).toContainText('Every 30 minutes');
	// Back to the default, which later projects (search) expect.
	await feed.getByRole('button', { name: 'Change how often Feed check runs' }).click();
	await feed.getByLabel('Every (minutes)').fill('15');
	await feed.getByRole('button', { name: 'Save' }).click();
	await expect(feed).toContainText('Every 15 minutes');
	await expect(page.getByTestId('loop').first()).toBeVisible();
	await axe(page, '/settings/schedules');
	await phone(page, '/settings/schedules');
});

test('a newer release shows a notice; off asks nothing', async () => {
	// The update check settings moved to System > Health checks; the notice stays on This app.
	const saved = async () => {
		await page.getByRole('button', { name: 'Save' }).click();
		await expect(page.getByTestId('savebar')).toHaveCount(0);
	};
	await page.goto('/settings/health');
	// The page shows its settings once they load; then open Advanced.
	await page.locator('details.advanced').first().waitFor();
	await openAdvanced(page);
	await page.getByLabel('Feed may be on the local network').check();
	await saved();
	await page.goto('/health?tab=updates');
	const upd = page.getByTestId('status-updates');
	await upd.getByRole('button', { name: /^Check (now|again)$/ }).click();
	await expect(upd).toContainText('9.9.9');
	await expect(upd.getByRole('link', { name: 'What changed' })).toHaveAttribute('href', 'https://github.com/HolyArrs/HolyArrs/releases/tag/v9.9.9');
	await page.goto('/settings/health');
	await expect(page.getByTestId('update-privacy')).toContainText('api.github.com');
	await page.getByLabel('Check for a newer HolyArrs').uncheck();
	await saved();
	await page.goto('/health?tab=updates');
	await expect(page.getByTestId('status-updates')).toContainText('Update check is off');
	await expect(page.getByTestId('status-updates').getByRole('button', { name: /^Check (now|again)$/ })).toHaveCount(0);
	await page.goto('/settings/health');
	await page.getByLabel('Check for a newer HolyArrs').check();
	await saved();
	await axe(page, '/settings/health');
	await page.goto('/settings/this-app');
	await axe(page, '/settings/this-app');
});

test('the API page: key, scopes, base address, examples and every route', async () => {
	await page.goto('/settings');
	// Settings home: open the Linked apps card, then its API capsule.
	await page.getByTestId('settings-group').filter({ hasText: 'Linked apps' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'API', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'API', level: 1 })).toBeVisible();
	await expect(page.locator('#api-start').getByRole('link', { name: 'API keys' })).toHaveAttribute('href', '#keys-card');
	await expect(page.getByLabel('Key name')).toBeVisible();
	await expect(page.getByTestId('api-base')).toHaveText(/^http:\/\/127\.0\.0\.1:\d+\/api\/v1$/);
	await expect(page.getByTestId('api-scopes')).toContainText('control');
	expect(await page.getByTestId('api-op').count()).toBeGreaterThan(150);
	await page.getByTestId('more-options').getByText('Reference').click();
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
