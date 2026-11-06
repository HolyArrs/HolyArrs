import { expect, test } from '@playwright/test';
import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { finishFirstRun } from './helpers.ts';

// The whole app at http://127.0.0.1:18735/holyarrs behind a reverse proxy
// that answers nothing outside /holyarrs: first run, sign-in, pages, the
// API, the live stream, downloads and sign-out all stay under the base.
test.describe.configure({ mode: 'serial' });

const here = dirname(fileURLToPath(import.meta.url));
// The proxy's port: a slot from e2e-run.ps1 hands its own in the environment.
const proxy = `127.0.0.1:${process.env.HOLYARRS_E2E_BASE_PROXY_PORT ?? '18735'}`;
const root = `http://${proxy}/holyarrs`;

test('the app works under /holyarrs behind a proxy', async ({ page }) => {
	const outside: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (u.host === proxy && !u.pathname.startsWith('/holyarrs')) outside.push(u.pathname);
	});
	page.on('response', (r) => {
		if (r.status() === 404 && r.url().includes('/_app/')) outside.push(`404 ${r.url()}`);
	});

	// healthz answers while the app is still starting; wait until it is ready.
	await expect.poll(async () => (await page.request.get(`${root}/readyz`)).status(), { timeout: 120_000 }).toBe(200);
	await page.goto(root);
	await expect(page).toHaveURL(/\/holyarrs\/login/);
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill('luis');
	await page.getByLabel('Password', { exact: true }).fill('base-test-password');
	await page.getByLabel('Password again').fill('base-test-password');
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(`${root}/welcome?first=1`);
	const movies = join(here, '.tmp-base', 'movies');
	mkdirSync(movies, { recursive: true });
	await finishFirstRun(page, movies);
	await expect(page).toHaveURL(`${root}/`);

	await page.goto(`${root}/settings`);
	await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
	await page.getByRole('main').locator('.links').getByRole('link').filter({ hasText: 'This app' }).click();
	await expect(page).toHaveURL(`${root}/settings/this-app`);
	await expect(page.getByTestId('settings-links').getByRole('link', { name: /^Logs/ })).toHaveAttribute('href', '/holyarrs/settings/logs');
	await expect(page.getByTestId('url-base')).toContainText('/holyarrs');

	await page.goto(`${root}/settings/api`);
	await expect(page.getByTestId('api-base')).toHaveText(`${root}/api/v1`);
	const spec = await (await page.request.get(`${root}/api/v1/openapi.json`)).json();
	expect(spec.servers[0].url).toBe('/holyarrs/');

	// A reload of a deep page, the menu and Back stay under the base.
	await page.goto(`${root}/library`);
	await page.reload();
	await expect(page).toHaveURL(`${root}/library`);
	await page.getByRole('navigation', { name: 'Main' }).first().getByRole('link', { name: /^Settings(, needs attention)?$/ }).click();
	await expect(page).toHaveURL(`${root}/settings`);
	await page.goBack();
	await expect(page).toHaveURL(`${root}/library`);

	// Outside the base there is nothing.
	expect((await page.request.get(`http://${proxy}/api/v1/auth/state`)).status()).toBe(404);
	expect(outside, 'requests outside /holyarrs').toEqual([]);
});
