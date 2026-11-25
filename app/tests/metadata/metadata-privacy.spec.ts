import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, noSidewaysScroll, finishFirstRun, reachSetupAccount } from '../helpers/helpers.ts';

// Settings > Metadata says what the metadata server gets and links to
// the Count this install switch, at desktop and phone width.
test('Settings > Metadata shows what the metadata server gets', async ({ page }) => {
	await page.goto('/');
	const o = newOwner();
	await reachSetupAccount(page);
	await page.getByLabel('Username').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Confirm password').fill(o.password);
	await page.getByRole('button', { name: 'Create account and continue' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1/);
	await finishFirstRun(page, moviesDir);
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();

	for (const width of [1280, 390]) {
		await page.setViewportSize({ width, height: width > 500 ? 900 : 844 });
		await page.goto('/settings/tmdb');
		const card = page.getByRole('region', { name: 'What we send' });
		await expect(card).toBeVisible();
		for (const words of ['An install token.', 'What you look up.', 'Display choices.', 'Small request headers.', 'Install count by country.', 'public IP address']) {
			await expect(card.getByText(words, { exact: false }).first()).toBeVisible();
		}
		await expect(card.locator('li')).toHaveCount(5);
		// What never leaves is its own card.
		const never = page.getByRole('region', { name: 'Never sent' });
		await expect(never.getByText('Your files, folder paths or file names.')).toBeVisible();
		await expect(never.locator('li')).toHaveCount(4);
		await noSidewaysScroll(page);
		const axe = await new AxeBuilder({ page }).include('#metadata-sent').withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
		expect(axe.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
	}
	await page.getByRole('link', { name: 'Count this install' }).click();
	await expect(page).toHaveURL(/\/settings\/this-app#count-install$/);
	await expect(page.getByText('Count this install').first()).toBeVisible();
});
