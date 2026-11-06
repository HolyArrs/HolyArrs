import { expect, test } from '@playwright/test';
import { owner, signIn } from './helpers.ts';

test('the Downloaders Test button reaches the preserved lab qBittorrent with its API key', async ({ page }) => {
	// The lab's own qBittorrent (research\lab); without it the test is skipped with the reason.
	const up = await fetch('http://10.10.25.25:18708/api/v2/app/version', { signal: AbortSignal.timeout(3000) }).then(() => true, () => false);
	test.skip(!up, 'needs the lab qBittorrent on 18708: python research\lab\labctl.py up qbittorrent (stop it after)');
	// A qBittorrent is saved with an API key or a sign-in, never neither (a3301b5d).
	const key = process.env.HOLYARRS_LAB_QBIT_KEY ?? '';
	test.skip(!key, 'needs the lab API key: python research\lab\labctl.py qbt-apikey -- <the e2e command>');
	await signIn(page, owner());
	await page.goto('/settings/downloaders');
	await page.getByRole('button', { name: 'Add a Downloader' }).first().click();
	await page.getByRole('radio', { name: /qBittorrent/ }).check();
	await page.getByLabel('Name', { exact: true }).fill('Preserved lab qBittorrent');
	await page.getByLabel('Address', { exact: true }).fill('http://10.10.25.25:18708');
	await expect(page.getByRole('radio', { name: 'API key' })).toBeChecked();
	await page.getByRole('textbox', { name: 'API key', exact: true }).fill(key);
	await page.getByText('More options').click();
	await page.getByLabel('Category', { exact: true }).fill('holyarrs-acceptance');
	await page.getByLabel('May reach the local network').check();
	await page.locator('form.sc-form').first().getByRole('button', { name: 'Save', exact: true }).click();

	const downloader = page.getByTestId('downloader').filter({ hasText: 'Preserved lab qBittorrent' });
	await expect(downloader.getByTestId('downloader-version')).toHaveText('qBittorrent v5.2.4');
	await expect(downloader.getByTestId('downloader-state')).toHaveText('Working');
	await expect(downloader).toContainText('may reach the local network');
	await expect(downloader).not.toContainText('password');

	await downloader.getByRole('button', { name: 'Test', exact: true }).click();
	await expect(downloader.getByTestId('downloader-version')).toHaveText('qBittorrent v5.2.4');
	await expect(downloader.getByTestId('downloader-state')).toHaveText('Working');
});
