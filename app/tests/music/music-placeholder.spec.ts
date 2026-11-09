import { expect, test } from '@playwright/test';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { moviesDir, newOwner, noSidewaysScroll, owner, signIn, finishFirstRun } from '../helpers/helpers.ts';

// A cover or photo no source has shows the one shared placeholder art (no
// broken picture), on desktop and at phone widths 390 and 360. The fake
// metadata server has no pictures for the lab artists.
const artist = 'a2222222-0000-4000-8000-000000000002'; // Moss Choir
const album = 'b2222222-0000-4000-8000-000000000001'; // Slow Green

test.describe.configure({ mode: 'serial' });

// Run alone (no other project first): make the owner the first-run way,
// with Music on, when no earlier test did.
test.beforeAll(async ({ browser }) => {
	if (existsSync(join(moviesDir, '..', 'owner.json'))) return;
	const page = await browser.newPage();
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible();
	const o = newOwner();
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await finishFirstRun(page, moviesDir, { music: true });
	await expect(page).toHaveURL(/\/$/);
	await page.close();
});

for (const width of [1280, 390, 360]) {
	test(`missing photo and cover show the placeholder at ${width}`, async ({ browser }) => {
		const ctx = await browser.newContext({ viewport: { width, height: width < 600 ? 844 : 900 } });
		const p = await ctx.newPage();
		await signIn(p, owner());
		await p.goto(`/music/artist/${artist}`);
		const photo = p.getByTestId('artist-photo-placeholder');
		await expect(photo).toBeVisible({ timeout: 30_000 });
		await expect(photo.getByTestId('art-placeholder')).toHaveCount(1);
		const round = await photo.evaluate((el) => getComputedStyle(el).borderRadius);
		expect(round).toBe('50%');
		const box = await photo.boundingBox();
		expect(box && Math.abs(box.width - box.height) < 2 && box.width > 40).toBeTruthy();
		await noSidewaysScroll(p);
		await p.goto(`/music/album/${album}?artist=${artist}`);
		const hero = p.getByRole('main').getByTestId('art-placeholder').first();
		await expect(hero).toBeVisible({ timeout: 30_000 });
		await expect(p.locator('main img[alt$=" cover"]')).toHaveCount(0);
		await noSidewaysScroll(p);
		await ctx.close();
	});
}
