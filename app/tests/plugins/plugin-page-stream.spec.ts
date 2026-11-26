import { expect, test } from '@playwright/test';

// A plugin page is a sandboxed frame with no cookie and never sees a stream address. For video it says
// play({file_id}); HolyArrs shows its own player over the page and plays the file with the person's own sign-in,
// and the page gets events back. Own app with a stub player plugin:
// npx playwright test -c ../tests/setups/plugin-page-stream.config.ts
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
const owner = { name: 'luis', password: pw };

test('a plugin page asks the app to play: the app streams a range, the page gets events and never an address', async ({ page }) => {
	test.setTimeout(300_000);
	// The first-run wizard needs the part switches of the full program, which this app does not have:
	// make the owner and finish setup the way the wizard's last step does.
	const made = await page.request.post('/api/v1/auth/setup', { data: owner });
	expect(made.status()).toBe(201);
	const { csrf } = await made.json();
	expect((await page.request.post('/api/v1/auth/setup/done', { headers: { 'X-CSRF-Token': csrf } })).status()).toBe(200);

	const streams: { status: number; range?: string; cookie?: string }[] = [];
	page.on('response', async (r) => {
		if (!r.url().includes('/api/v1/plugin-stream/')) return;
		const h = await r.request().allHeaders();
		streams.push({ status: r.status(), range: h['range'], cookie: h['cookie'] });
	});

	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/plugins/player/player');
	const frame = page.frameLocator('iframe');
	const out = frame.locator('#out');

	// The app's own player shows over the page, with the person's session on its range request.
	const player = page.getByTestId('plugin-player');
	await expect(player).toBeVisible();
	await expect.poll(() => streams.find((s) => s.status === 206)?.status).toBe(206);
	const got = streams.find((s) => s.status === 206)!;
	expect(got.range).toMatch(/^bytes=\d+-/);
	expect(got.cookie).toBeTruthy();

	// At phone width the player fits and the page does not scroll sideways.
	const box = await player.locator('video').boundingBox();
	expect(box!.width).toBeLessThanOrEqual(390);
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);

	// The page was told it worked, and nothing it can read holds an address or token.
	await expect(out).toContainText('"status":200');
	const seen = await frame.locator('html').evaluate((el) => el.outerHTML + ' ' + location.href.split('/').slice(0, 4).join('/'));
	expect(seen).not.toContain('plugin-stream');
	expect(seen).not.toMatch(/hms_[A-Z0-9]{20,}\?|"url"/);

	// Keyboard close: Escape closes the player and the page hears about it.
	await page.keyboard.press('Escape');
	await expect(player).toBeHidden();
	await expect(out).toContainText('"type":"closed"');

});
