import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, finishFirstRun } from '../helpers/helpers.ts';

const tiny = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

test('no CSP violations on Explore, More info, a trailer and a title page', async ({ page }) => {
	test.setTimeout(180_000);
	const bad: string[] = [];
	page.on('console', (m) => { if (/Content.Security.Policy|Refused to/i.test(m.text())) bad.push(m.text()); });
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (e) => console.error('Refused to (CSP) ' + e.violatedDirective + ' ' + e.blockedURI));
	});
	const res = await page.goto('/');
	expect(res?.headers()['content-security-policy']).toContain("frame-src https://www.youtube-nocookie.com");
	const o = newOwner();
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await finishFirstRun(page, moviesDir);
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();

	await page.route('**/api/v1/artwork/tmdb/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	const item = (i: number) => ({ tmdb_id: String(900002 + i), kind: 'movie', name: `Lab `, year: 2023, overview: 'A lab title.', poster: `https://image.tmdb.org/t/p/w500/p.jpg`, backdrop: `https://image.tmdb.org/t/p/original/b.jpg`, trailer_key: 'dQw4w9WgXcQ' });
	await page.route('https://image.tmdb.org/**', (r) => r.fulfill({ body: tiny, contentType: 'image/png' }));
	await page.route('**/api/v1/trailer/**', (r) => r.fulfill({ status: 404, json: { error: 'no direct stream' } }));
	await page.route('**/api/v1/explore/head?**', (r) => r.fulfill({ json: { type: 'all', building: false, seeds: 0, tail_cap: 0, tail_rows: 0, hero: [item(0), item(1)], head: [{ id: 'row0', type: 'genre', title: 'Row 0', items: Array.from({ length: 8 }, (_, i) => item(i)) }] } }));
	await page.goto('/explore');
	await expect(page.getByRole('heading', { level: 1, name: 'Explore' })).toBeVisible();
	// The hero comes with the head answer, which can be slow on a busy machine:
	// wait for its More info control, then give its trailer time to start.
	const info = page.getByRole('link', { name: 'More info' }).or(page.getByRole('button', { name: /^More info/ })).first();
	await expect(info, 'the Explore hero shows its More info control').toBeVisible({ timeout: 30_000 });
	await page.waitForTimeout(9000);
	console.log('hero iframes', JSON.stringify(await page.locator('iframe').evaluateAll((els) => els.map((e) => (e as HTMLIFrameElement).src.slice(0, 50) + ' ' + e.getAttribute('sandbox')))));
	await info.click();
	// More info opens a sheet, or the hero's link goes to the title page.
	await expect(page.getByRole('dialog').first().or(page.getByTestId('title-detail'))).toBeVisible();
	await page.waitForTimeout(3000);
	await page.keyboard.press('Escape');
	const heroFrames = await page.locator('iframe').evaluateAll((els) => els.map((e) => (e as HTMLIFrameElement).src.slice(0, 40) + ' ' + e.getAttribute('sandbox')));
	console.log('after More info', JSON.stringify(heroFrames));
	const frames = await page.locator('iframe').evaluateAll((els) => els.map((e) => e.getAttribute('sandbox')));
	console.log('iframes seen', JSON.stringify(frames));
	await page.goto('/title/movie/900002');
	await page.waitForTimeout(4000);
	const frames2 = await page.locator('iframe').evaluateAll((els) => els.map((e) => (e as HTMLIFrameElement).src + ' ' + e.getAttribute('sandbox')));
	console.log('title iframes', JSON.stringify(frames2), 'url', page.url());
	expect(bad).toEqual([]);
});
