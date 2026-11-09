import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { noSidewaysScroll, signIn, finishFirstRun } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

// Status: every tab at desktop width and at
// 390, the tabs work from the keyboard, and the Jobs table keeps its labels
// on a phone. About hands an admin the support bundle; a member never gets it. Own fresh app: npx playwright test -c e2e/status.config.ts
test.describe.configure({ mode: 'serial' });

const tmp = dataDir('phone');
// A fresh password each run, as helpers.ts newOwner() makes.
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
const owner = () => ({ name: 'luis', password: pw });

test.beforeAll(async ({ browser }) => {
	test.setTimeout(240_000);
	const page = await browser.newPage();
	// The first start may show "starting" for a while; reload until set-up shows.
	await expect(async () => {
		await page.goto('/');
		await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible({ timeout: 2_000 });
	}).toPass({ timeout: 120_000 });
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(owner().name);
	await page.getByLabel('Password', { exact: true }).fill(owner().password);
	await page.getByLabel('Password again').fill(owner().password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await page.getByLabel('Music', { exact: true }).check();
	await page.getByLabel('Yes', { exact: true }).check();
	await page.getByRole('button', { name: 'Continue' }).click();
	await finishFirstRun(page, join(tmp, 'movies'));
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();
	await page.context().close();
});

const tabs = ['parts', 'jobs', 'updates', 'about'];

async function open(page: Page, url: string) {
	await page.goto(url);
	await expect(page.getByTestId('status-sections')).toBeVisible();
	await page.waitForLoadState('networkidle');
}

for (const width of [1280, 390]) {
	test(`status: every tab at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		for (const t of tabs) {
			await open(page, `/health?tab=${t}`);
			await expect(page.getByRole('tablist', { name: 'Status view' }).getByRole('tab', { selected: true })).toHaveAttribute('id', `st-tab-${t}`);
			await expect(page.locator('#st-panel-' + t)).toBeVisible();
			await noSidewaysScroll(page);
		}
		// The Jobs tab draws the coming day beside the timeline; the last job row stays reachable.
		await open(page, '/health?tab=jobs');
		await expect(page.getByTestId('status-live')).toBeVisible();
		await expect(page.getByTestId('status-live-pills')).toHaveCount(0);
		const last = page.getByTestId('status-row').last();
		await last.scrollIntoViewIfNeeded();
		await expect(last).toBeInViewport();
		if (width === 390) await expect(last.getByText('Next run', { exact: true })).toBeVisible();
		await page.context().close();
	});
}

for (const width of [1280, 390]) {
	test(`status: About downloads the support bundle at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 }, acceptDownloads: true })).newPage();
		await signIn(page, owner());
		await open(page, '/health?tab=about');
		const button = page.getByTestId('status-support-bundle');
		await expect(button).toBeVisible();
		await button.scrollIntoViewIfNeeded();
		const [download] = await Promise.all([page.waitForEvent('download'), button.click()]);
		expect(download.suggestedFilename()).toMatch(/^holyarrs-support-.+\.zip$/);
		// A zip keeps each file name in plain text in its directory.
		const zip = readFileSync((await download.path())!);
		expect(zip.subarray(0, 2).toString()).toBe('PK');
		for (const name of ['log.txt', 'about.json', 'doctor.txt', 'health.json']) expect(zip.includes(name), name).toBe(true);
		await noSidewaysScroll(page);
		await page.context().close();
	});
}

test('status: a member never gets the support bundle, even with Status open', async ({ browser }) => {
	const own = await (await browser.newContext()).newPage();
	await signIn(own, owner());
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('Uma');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const uma = await (await browser.newContext()).newPage();
	const umaPw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
	await uma.goto(new URL(link).pathname);
	await uma.getByLabel('Name').fill('uma');
	await uma.getByLabel('Password', { exact: true }).fill(umaPw);
	await uma.getByLabel('Confirm password').fill(umaPw);
	await uma.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(uma).toHaveURL(/\/$/);
	await uma.goto('/health?tab=about');
	await expect(uma).not.toHaveURL(/\/health/);
	expect((await uma.request.get('/api/v1/support/bundle')).status()).toBe(403);
	// Run the Library opens Status, but the bundle stays with the owner and admins.
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Change uma' }).click();
	await own.getByRole('dialog').getByLabel('Run the Library').check();
	await own.getByRole('dialog').getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('uma saved')).toBeVisible();
	await open(uma, '/health?tab=about');
	await expect(uma.locator('#st-panel-about')).toBeVisible();
	await expect(uma.getByTestId('status-support-bundle')).toHaveCount(0);
	expect((await uma.request.get('/api/v1/support/bundle')).status()).toBe(403);
	await uma.context().close();
	await own.context().close();
});

test('status: tabs move with the arrow keys and keep focus', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(page, owner());
	await open(page, '/health');
	await page.getByRole('tablist', { name: 'Status view' }).getByRole('tab', { name: /^Status/ }).focus();
	await page.keyboard.press('ArrowRight');
	await expect(page).toHaveURL(/tab=jobs/);
	await expect(page.getByRole('tablist', { name: 'Status view' }).getByRole('tab', { name: /^Jobs/ })).toBeFocused();
	await page.keyboard.press('End');
	await expect(page).toHaveURL(/tab=about/);
	await expect(page.getByText('Draft')).toHaveCount(0);
	await page.keyboard.press('Home');
	await expect(page).not.toHaveURL(/tab=(jobs|updates|about)/);
	await page.getByRole('button', { name: 'Check again' }).click();
	await expect(page.getByText('Checks ran')).toBeVisible();
	await page.context().close();
});
