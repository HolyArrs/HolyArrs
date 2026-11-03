import { expect, test } from '@playwright/test';
import { libraryTool, noSidewaysScroll, owner, signIn } from './helpers.ts';

// Library tools added in November 2026: Movie series, Recently deleted and
// Import files by hand open from the Add menu, and fit a phone.
test('Library tools open from the Add menu', async ({ page }) => {
	await signIn(page, owner());
	for (const [tool, heading] of [
		['Movie series', 'Movie series'],
		['Recently deleted', 'Recently deleted'],
		['Import files by hand', 'Import files by hand']
	]) {
		await page.goto('/library');
		await libraryTool(page, tool);
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
	}
});

test('Library tools fit a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 360, height: 780 } });
	const page = await ctx.newPage();
	await signIn(page, owner());
	for (const path of ['/library', '/library/collections', '/library/deleted', '/library/import-files']) {
		await page.goto(path);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(page);
	}
	await ctx.close();
});
