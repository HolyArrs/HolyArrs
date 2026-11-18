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
	// A language kept on the account wins over this browser's; an earlier spec may have kept one, so clear it and let each test pick.
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	expect((await page.request.put('/api/v1/my/language', { data: { language: '' }, headers: { 'X-CSRF-Token': me.csrf } })).ok()).toBe(true);
});
test.afterAll(async () => {
	// Standard is shared by every later spec: a tag left on it limits searches to tagged Sources, so take it off again.
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const d = (await (await page.request.get(`/api/v1/pick-rules/${id}`)).json()) as { name: string; kinds: string[]; body: { tags?: string[] } };
	d.body.tags = [];
	const put = await page.request.put(`/api/v1/pick-rules/${id}`, { data: { name: d.name, kinds: d.kinds, body: d.body }, headers: { 'X-CSRF-Token': me.csrf } });
	expect(put.ok(), await put.text()).toBe(true);
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
	// The save's own toast (the save bar keeps a hidden "not saved yet" line that also says saved).
	await expect(page.getByRole('status')).toContainText('Quality set saved');
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
