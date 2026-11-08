import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, noSidewaysScroll, finishFirstRun } from '../helpers/helpers.ts';

// Settings > Metadata says what the metadata server gets and links to
// the Count this install switch, at desktop and phone width.
test('Settings > Metadata shows what the metadata server gets', async ({ page }) => {
	await page.goto('/');
	const o = newOwner();
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await finishFirstRun(page, moviesDir);
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();

	for (const width of [1280, 390]) {
		await page.setViewportSize({ width, height: width > 500 ? 900 : 844 });
		await page.goto('/settings/tmdb');
		const card = page.getByRole('region', { name: 'What we send' });
		await expect(card).toBeVisible();
		for (const words of ['An install token.', 'What you look up.', 'Display choices.', 'Small request headers.', 'Install count by country.', 'public IP address', 'Never sent', 'Your files, folder paths or file names.']) {
			await expect(card.getByText(words, { exact: false }).first()).toBeVisible();
		}
		await expect(card.locator('li')).toHaveCount(9);
		await noSidewaysScroll(page);
		const axe = await new AxeBuilder({ page }).include('#metadata-sent').withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
		expect(axe.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
	}
	await page.getByRole('link', { name: 'Count this install' }).click();
	await expect(page).toHaveURL(/\/settings\/this-app#count-install$/);
	await expect(page.getByText('Count this install').first()).toBeVisible();
});
