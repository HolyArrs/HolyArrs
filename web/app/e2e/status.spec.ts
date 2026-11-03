import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { noSidewaysScroll, signIn } from './helpers.ts';

// Status (#71, #78): every tab and every Jobs shape at desktop width and at
// 390, the tabs work from the keyboard, and the Jobs table keeps its labels
// on a phone. Own fresh app: npx playwright test -c e2e/status.config.ts
test.describe.configure({ mode: 'serial' });

const tmp = join(dirname(fileURLToPath(import.meta.url)), '.tmp-phone');
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
	await page.getByLabel('Setup code').fill(readFileSync(join(tmp, 'config', 'setup-code.txt'), 'utf8').trim());
	await page.getByLabel('Name').fill(owner().name);
	await page.getByLabel('Password', { exact: true }).fill(owner().password);
	await page.getByLabel('Password again').fill(owner().password);
	await page.getByRole('button', { name: 'Make the owner and sign in' }).click();
	await page.getByLabel('Music', { exact: true }).check();
	await page.getByLabel('Yes', { exact: true }).check();
	await page.getByRole('button', { name: 'Continue' }).click();
	await page.getByLabel('Folder path').fill(join(tmp, 'movies'));
	await page.getByLabel(/^Name/).fill('Movies');
	await page.getByRole('button', { name: 'Add folder and open Home' }).click();
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
	test(`status: every tab and shape at ${width}`, async ({ browser }) => {
		const page = await (await browser.newContext({ viewport: { width, height: 844 } })).newPage();
		await signIn(page, owner());
		for (const t of tabs) {
			await open(page, `/status?tab=${t}`);
			await expect(page.getByRole('tablist', { name: 'Status view' }).getByRole('tab', { selected: true })).toHaveAttribute('id', `st-tab-${t}`);
			await expect(page.locator('#st-panel-' + t)).toBeVisible();
			await noSidewaysScroll(page);
		}
		for (let n = 1; n <= 10; n++) {
			await open(page, `/status?tab=jobs&live=${n}`);
			await expect(page.getByTestId('status-live-pills')).toBeVisible();
			await noSidewaysScroll(page);
		}
		// The last job row scrolls clear of the pill bar.
		await open(page, '/status?tab=jobs');
		const last = page.getByTestId('status-row').last();
		await last.scrollIntoViewIfNeeded();
		await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
		const row = await last.boundingBox();
		const pills = await page.getByTestId('status-live-pills').boundingBox();
		expect(row && pills && row.y + row.height <= pills.y + 1, 'last job row sits above the pills').toBeTruthy();
		if (width === 390) await expect(last.getByText('Next run', { exact: true })).toBeVisible();
		await page.context().close();
	});
}

test('status: tabs move with the arrow keys and keep focus', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(page, owner());
	await open(page, '/status');
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
	await expect(page.getByText('Checks ran.')).toBeVisible();
	await page.context().close();
});
