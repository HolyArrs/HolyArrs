import { expect, test } from '@playwright/test';
import { noSidewaysScroll, reachSetupAccount } from '../helpers/helpers.ts';

// Skip setup for now: after the account step the owner can leave the wizard for Home, and a Finish setup card
// brings it back at the first unfinished step. Own fresh app: npx playwright test -c ../tests/setups/skip-setup.config.ts
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;

test('setup: skip after the account step, land on Home, resume from the card', async ({ page }) => {
	test.setTimeout(240_000);
	await expect(async () => {
		await page.goto('/');
		await reachSetupAccount(page);
	}).toPass({ timeout: 120_000 });
	// Language, Welcome and Account stay required: no skip before the account exists.
	await expect(page.getByTestId('skip-setup')).toHaveCount(0);
	await page.getByLabel('Username').fill('luis');
	await page.getByLabel('Password', { exact: true }).fill(pw);
	await page.getByLabel('Confirm password').fill(pw);
	await page.getByRole('button', { name: 'Create account and continue' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1/);
	const skip = page.getByTestId('skip-setup');
	await expect(skip).toBeVisible();
	await skip.click();
	// Home opens, with the Finish setup card; the wizard gate no longer holds the owner.
	await expect(page).toHaveURL(/\/$/, { timeout: 20_000 });
	const card = page.getByTestId('finish-setup');
	await expect(card).toBeVisible();
	await page.goto('/library');
	await expect(page).not.toHaveURL(/welcome/);
	await page.goto('/settings');
	await expect(page.getByTestId('finish-setup')).toBeVisible();
	// Phone width: the card fits.
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/');
	await expect(card).toBeVisible();
	await noSidewaysScroll(page);
	await page.screenshot({ path: test.info().outputPath('home-finish-setup-390.png') });
	await page.setViewportSize({ width: 1280, height: 800 });
	// Resume: the wizard reopens at a step after the account (never Language, Welcome or Account again).
	await card.click();
	await expect(page).toHaveURL(/\/welcome\?first=1/);
	await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/Your HolyArrs account|Welcome to HolyArrs/);
	await page.screenshot({ path: test.info().outputPath('resume-1280.png') });
});
