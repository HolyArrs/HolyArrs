import { expect, test } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

test('phone: bottom menu, one column, no sideways scroll on any page', async ({ page }) => {
	await signIn(page, owner());
	const tabs = page.locator('nav.tabbar');
	await expect(tabs).toBeVisible();
	await expect(page.locator('aside.ui-sidebar')).toBeHidden();

	const urls = ['/', '/library', '/library/1', '/search', '/downloads', '/calendar', '/settings'];
	for (const url of urls) {
		await page.goto(url);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(page);
	}
	for (const url of [
		'/settings/library-folders',
		'/settings/naming',
		'/library/rename',
		'/library/import',
		'/settings/sources',
		'/settings/downloaders',
		'/settings/tmdb',
		'/settings/backups',
		'/settings/sign-in',
		'/settings/account',
		'/settings/users',
		'/settings/music',
		'/settings/wishlist',
		'/settings/alerts',
		'/account',
		'/requests',
		'/settings/this-app'
	]) {
		await page.goto(url);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(page);
	}

	// Downloads lives in the Manage sheet behind the last tab.
	await tabs.getByRole('button', { name: /^Manage/ }).click();
	await page.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: 'Downloads', exact: true }).click();
	await expect(page).toHaveURL(/\/downloads$/);
	await expect(tabs.getByRole('button', { name: /^Manage/ })).toHaveClass(/active/);
	await tabs.getByRole('button', { name: /^Manage/ }).click();
	await expect(page.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: 'Downloads', exact: true })).toHaveAttribute('aria-current', 'page');
});
