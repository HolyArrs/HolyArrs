import { expect, test } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

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
		'/settings/requests',
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

test('phone: What\'s new rows fit and read at 360 and 390', async ({ page }) => {
	await signIn(page, owner());
	for (const width of [360, 390]) {
		await page.setViewportSize({ width, height: 800 });
		await page.goto('/health?tab=updates');
		const box = page.getByTestId('whats-new');
		await expect(box).toBeVisible();
		await expect(box.getByRole('heading', { name: /^What's new/ })).toBeVisible();
		await noSidewaysScroll(page);
		// Each row head stays inside the screen, and its version is real, readable text.
		const rows = await box.locator('li.rel .rh').evaluateAll((els, w) =>
			els.map((el) => {
				const r = el.getBoundingClientRect();
				const v = el.querySelector('.rv');
				const vr = v?.getBoundingClientRect();
				return {
					inside: r.left >= -1 && r.right <= w + 1,
					text: (v?.textContent ?? '').trim(),
					size: v ? parseFloat(getComputedStyle(v).fontSize) : 0,
					seen: !!vr && vr.width > 0 && vr.right <= w + 1
				};
			}), width);
		for (const row of rows) {
			expect(row.inside, `row sticks out at ${width}px`).toBe(true);
			expect(row.text.length, `empty version at ${width}px`).toBeGreaterThan(0);
			expect(row.size, `version text too small at ${width}px`).toBeGreaterThanOrEqual(12);
			expect(row.seen, `version cut off at ${width}px`).toBe(true);
		}
	}
});
