import { expect, test } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// Home (roadmap #3): the owner's overview, and the setup bar while setup is open, on a desktop and a phone.
for (const width of [1280, 390]) {
	test(`home at ${width}px: Library and Needs attention, setup steps fill the bar`, async ({ browser }) => {
		const ctx = await browser.newContext({ viewport: { width, height: 900 } });
		const page = await ctx.newPage();
		await signIn(page, owner());
		await page.goto('/');
		await expect(page.getByRole('heading', { name: 'Home', level: 1 })).toBeVisible();
		const welcome = page.getByTestId('home-welcome');
		if (await welcome.isVisible()) {
			// One segment per step, all the same width, filling the bar.
			const w = await welcome.locator('.segs i').evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().width)));
			const bar = await welcome.locator('.segs').evaluate((e) => e.getBoundingClientRect().width);
			expect(w.length).toBe(await welcome.locator('.steps li').count());
			expect(Math.max(...w) - Math.min(...w)).toBeLessThanOrEqual(1);
			expect(w.reduce((t, x) => t + x, 0) + 4 * (w.length - 1)).toBeGreaterThan(bar - 3);
		} else {
			await expect(page.getByTestId('home-attention')).toBeVisible();
		}
		await noSidewaysScroll(page);
		await ctx.close();
	});
}
