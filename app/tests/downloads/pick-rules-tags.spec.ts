import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

// Pick rules tags: the Sources card takes tags, saves them and shows them
// after a reload; the card fits at desktop, tablet and phone widths in
// English, German and Arabic.
test.describe.configure({ mode: 'serial' });

let page: Page;
let id = 0;
test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
	const res = await page.request.get('/api/v1/pick-rules');
	const body = await res.json();
	const list = (body.pick_rules ?? body) as { id: number; name: string }[];
	id = list.find((r) => r.name === 'Standard')!.id;
});
test.afterAll(async () => {
	await page.context().close();
});

test('tags on Pick rules save and come back after a reload', async () => {
	await page.goto(`/settings/pick-rules/${id}`);
	const card = page.locator('#rules-sources');
	await expect(card).toBeVisible();
	const input = card.locator('#pr-tags');
	await input.fill('anime');
	await input.press('Enter');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Saved', { exact: false }).first()).toBeVisible();
	await page.reload();
	await expect(page.locator('#rules-sources')).toContainText('anime');
	const saved = await (await page.request.get(`/api/v1/pick-rules/${id}`)).json();
	expect(JSON.stringify(saved)).toContain('"tags":["anime"]');
});

for (const lang of ['en', 'de', 'ar']) {
	for (const [w, h] of [[1440, 900], [1024, 768], [390, 844], [360, 740]]) {
		test(`the Sources card fits at ${w} in ${lang}`, async () => {
			await page.setViewportSize({ width: w, height: h });
			await page.goto('/');
			await page.evaluate((l) => localStorage.setItem('holyarrs-lang', l), lang);
			await page.goto(`/settings/pick-rules/${id}`);
			const card = page.locator('#rules-sources');
			await expect(card).toBeVisible();
			await card.scrollIntoViewIfNeeded();
			await expect(card.locator('#pr-tags')).toBeVisible();
			const box = await card.boundingBox();
			expect(box!.x).toBeGreaterThanOrEqual(0);
			expect(box!.x + box!.width).toBeLessThanOrEqual(w + 1);
			await noSidewaysScroll(page);
			if (lang === 'ar') await expect(page.locator('html')).toHaveAttribute('dir', 'rtl');
		});
	}
}
