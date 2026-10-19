import { expect, test } from '@playwright/test';
import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// The whole app at http://127.0.0.1:18735/holyarrs behind a reverse proxy
// that answers nothing outside /holyarrs: first run, sign-in, pages, the
// API, the live stream, downloads and sign-out all stay under the base.
test.describe.configure({ mode: 'serial' });

const here = dirname(fileURLToPath(import.meta.url));
const root = 'http://127.0.0.1:18735/holyarrs';

test('the app works under /holyarrs behind a proxy', async ({ page }) => {
	const outside: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (u.host === '127.0.0.1:18735' && !u.pathname.startsWith('/holyarrs')) outside.push(u.pathname);
	});
	page.on('response', (r) => {
		if (r.status() === 404 && r.url().includes('/_app/')) outside.push(`404 ${r.url()}`);
	});

	await page.goto(root);
	await expect(page).toHaveURL(/\/holyarrs\/login/);
	const code = readFileSync(join(here, '.tmp-base', 'config', 'setup-code.txt'), 'utf8').trim();
	await page.getByLabel('Setup code').fill(code);
	await page.getByLabel('Name').fill('luis');
	await page.getByLabel('Password', { exact: true }).fill('base-test-password');
	await page.getByLabel('Password again').fill('base-test-password');
	await page.getByRole('button', { name: 'Make the owner and sign in' }).click();
	await expect(page).toHaveURL(`${root}/welcome?first=1`);
	await page.getByRole('button', { name: 'Continue' }).click();
	const movies = join(here, '.tmp-base', 'movies');
	mkdirSync(movies, { recursive: true });
	await page.getByLabel('Folder path').fill(movies);
	await page.getByLabel(/^Name/).fill('Movies');
	await page.getByRole('button', { name: 'Add folder and open Home' }).click();
	await expect(page).toHaveURL(`${root}/`);
	await expect(page.getByText('Live', { exact: true })).toBeVisible();

	await page.goto(`${root}/settings`);
	await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
	await page.getByRole('main').locator('.links').getByRole('link').filter({ hasText: 'This app' }).click();
	await expect(page).toHaveURL(`${root}/settings/this-app`);
	await expect(page.getByRole('link', { name: 'Download the log' }).first()).toHaveAttribute('href', '/holyarrs/api/v1/logs/download');
	await expect(page.getByTestId('url-base')).toContainText('/holyarrs');

	await page.goto(`${root}/settings/api`);
	await expect(page.getByTestId('api-base')).toHaveText(`${root}/api/v1`);
	const spec = await (await page.request.get(`${root}/api/v1/openapi.json`)).json();
	expect(spec.servers[0].url).toBe('/holyarrs/');

	// A reload of a deep page, the menu and Back stay under the base.
	await page.goto(`${root}/library`);
	await page.reload();
	await expect(page).toHaveURL(`${root}/library`);
	await page.getByRole('navigation', { name: 'Main' }).first().getByRole('link', { name: 'Settings', exact: true }).click();
	await expect(page).toHaveURL(`${root}/settings`);
	await page.goBack();
	await expect(page).toHaveURL(`${root}/library`);

	// Outside the base there is nothing.
	expect((await page.request.get('http://127.0.0.1:18735/api/v1/auth/state')).status()).toBe(404);
	expect(outside, 'requests outside /holyarrs').toEqual([]);
});
