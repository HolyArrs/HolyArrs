import { expect, test } from '@playwright/test';
import { owner, signIn } from './helpers.ts';

test('the Downloaders Test button reaches the preserved lab qBittorrent without a login', async ({ page }) => {
	await signIn(page, owner());
	await page.goto('/settings/downloaders');
	await page.getByRole('button', { name: 'Add a Downloader' }).click();
	await page.getByRole('radio', { name: /qBittorrent/ }).check();
	await page.getByLabel('Name', { exact: true }).fill('Preserved lab qBittorrent');
	await page.getByLabel('Address', { exact: true }).fill('http://10.10.25.25:18708');
	await page.getByLabel('Category', { exact: true }).fill('holyarrs-acceptance');
	await page.getByLabel('May reach the local network').check();
	await expect(page.getByLabel('Sign-in name')).toHaveValue('');
	await expect(page.getByLabel('Password', { exact: true })).toHaveValue('');
	await page.getByRole('button', { name: 'Save Downloader' }).click();

	const downloader = page.getByTestId('downloader').filter({ hasText: 'Preserved lab qBittorrent' });
	await expect(downloader.getByTestId('downloader-test')).toHaveText('It answered: qbittorrent v5.2.4.');
	await expect(downloader.getByTestId('downloader-state')).toHaveText('Working');
	await expect(downloader).toContainText('may reach the local network');
	await expect(downloader).not.toContainText('password');

	await downloader.getByRole('button', { name: 'Test', exact: true }).click();
	await expect(downloader.getByTestId('downloader-test')).toHaveText('It answered: qbittorrent v5.2.4.');
	await expect(downloader.getByTestId('downloader-state')).toHaveText('Working');
});
