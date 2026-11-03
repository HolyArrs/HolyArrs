import { expect, test } from '@playwright/test';
import { libraryTool, noSidewaysScroll, owner, signIn } from './helpers.ts';

// Library tools added in November 2026: Movie series (Movies grouped by
// series), Recently deleted and Import files by hand open from the Add menu,
// and fit a phone.
test('Library tools open from the Add menu', async ({ page }) => {
	await signIn(page, owner());
	// Movie series opens Movies grouped by series; each series opens its own page.
	await page.goto('/library');
	await libraryTool(page, 'Movie series');
	await expect(page).toHaveURL(/kind=movie.*group=series|group=series.*kind=movie/);
	for (const tool of ['Recently deleted', 'Import files by hand']) {
		await page.goto('/library');
		await libraryTool(page, tool);
		await expect(page.getByRole('heading', { level: 1, name: tool })).toBeVisible();
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
