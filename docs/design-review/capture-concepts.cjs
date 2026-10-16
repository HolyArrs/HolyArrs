const { createRequire } = require('node:module');
const { readFile, mkdir, writeFile } = require('node:fs/promises');
const { createHash } = require('node:crypto');
const path = require('node:path');
const assert = require('node:assert/strict');

// Run only after explicit shared heavy-slot release/allocation. One browser, one page.
const [toolsRoot, output] = process.argv.slice(2);
if (!path.isAbsolute(toolsRoot || '') || !path.isAbsolute(output || '')) {
	throw new Error('Supply absolute cached tooling directory and fresh output directory.');
}
const cached = createRequire(path.join(toolsRoot, 'package.json'));
const { chromium } = cached('playwright');
const axePath = cached.resolve('axe-core/axe.min.js');
const origin = 'http://127.0.0.1:18744';
const receipt = { kind: 'review-prototypes-only', sourceHashes: {}, screenshots: [], checks: [], errors: [], blockedRequests: [], violations: [] };
let browser;
const deadline = setTimeout(() => {
	receipt.errors.push('150-second internal deadline reached');
	void browser?.close();
}, 150_000);

async function audit(page, label) {
	assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${label}: horizontal document overflow`);
	await page.addScriptTag({ path: axePath });
	const result = await page.evaluate(() => axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'] } }));
	receipt.violations.push(...result.violations.map(v => ({ label, id: v.id, impact: v.impact, nodes: v.nodes.map(n => ({ target: n.target, summary: n.failureSummary })) })));
	receipt.checks.push(`${label}: overflow and axe checks executed`);
}

async function capture(page, label) {
	await page.evaluate(() => document.fonts.ready);
	await page.screenshot({ path: path.join(output, `${label}.png`), fullPage: true, animations: 'disabled' });
	receipt.screenshots.push({ file: `${label}.png`, viewport: page.viewportSize() });
	await audit(page, label);
}

(async () => {
	await mkdir(output, { recursive: false });
	for (const file of ['index.html', 'concepts/one.html', 'concepts/two.html', 'concepts/three.html', 'assets/artwork.png']) {
		receipt.sourceHashes[file] = createHash('sha256').update(await readFile(path.join(__dirname, file))).digest('hex');
	}
	try {
		browser = await chromium.launch({ headless: true });
		const context = await browser.newContext({ reducedMotion: 'reduce', serviceWorkers: 'block' });
		await context.route('**/*', route => {
			if (new URL(route.request().url()).origin === origin) return route.continue();
			receipt.blockedRequests.push(route.request().url());
			return route.abort();
		});
		const page = await context.newPage();
		page.setDefaultTimeout(5000);
		page.on('pageerror', error => receipt.errors.push(error.message));
		for (const concept of ['one', 'two', 'three']) {
			for (const [device, width, height] of [['desktop', 1440, 1050], ['tablet', 834, 1112], ['phone', 390, 844], ['narrow', 320, 740]]) {
				await page.setViewportSize({ width, height });
				await page.goto(`${origin}/concepts/${concept}.html`);
				await capture(page, `${concept}-${device}`);
			}
		}
		await page.setViewportSize({ width: 390, height: 844 });
		await page.goto(`${origin}/concepts/one.html`);
		await page.locator('.poster-button').first().click();
		await capture(page, 'one-phone-title');
		await page.keyboard.press('Escape');
		assert(await page.evaluate(() => document.activeElement.matches('.poster-button')), 'one: title focus return');
		await page.locator('.dock [data-view="activity"]').click();
		await capture(page, 'one-phone-downloads');
		await page.getByRole('button', { name: 'Mark failed', exact: true }).first().click();
		assert(await page.locator('#confirm').evaluate(d => d.open), 'one: consequence dialog');
		await page.keyboard.press('Escape');
		await page.locator('.dock [data-view="menu"]').click();
		await page.locator('[data-menu="settings"]').click();
		await page.getByRole('button', { name: 'Save, then test', exact: true }).click();
		assert((await page.locator('#saveStatus').textContent()).includes('save precedes test'));
		await capture(page, 'one-phone-settings');
		receipt.checks.push('one: detail focus return, download consequence dialog, menu/settings save sequence');

		await page.goto(`${origin}/concepts/two.html`);
		await page.locator('.nav-item[data-view="movies"]').click();
		await page.locator('.row').first().click();
		await capture(page, 'two-phone-title');
		await page.locator('#detailBack').click();
		await page.locator('#navToggle').click();
		assert(await page.evaluate(() => !document.activeElement.closest('[inert]')), 'two: active phone navigation focus');
		await page.locator('.nav-item[data-view="downloads"]').click();
		await capture(page, 'two-phone-downloads');
		await page.locator('.row[data-id="weather"]').click();
		await page.locator('#removeDownload').click();
		assert(await page.locator('#removeDialog').evaluate(d => d.open), 'two: removal confirmation');
		await page.keyboard.press('Escape');
		await page.locator('#removeDownload').click();
		await page.locator('#confirmRemove').click();
		assert(await page.evaluate(() => !document.activeElement.closest('[inert]')), 'two: confirmation focus remains on active surface');
		await page.locator('#detailBack').click();
		await page.locator('#navToggle').click();
		await page.locator('.nav-item[data-view="settings"]').click();
		await page.locator('.row[data-id="downloader"]').click();
		await page.locator('#saveConfig').click();
		await page.locator('#testConfig').click();
		await capture(page, 'two-phone-settings');
		receipt.checks.push('two: phone push/back focus, download confirmation, saved settings test');

		await page.goto(`${origin}/concepts/three.html`);
		await page.locator('.poster').first().click();
		await capture(page, 'three-phone-title');
		await page.locator('.back').click();
		await page.locator('.mobilebar [data-action="activity"]').click();
		await capture(page, 'three-phone-downloads');
		await page.locator('#activity [data-mode="remove"]').first().click();
		await page.keyboard.press('Escape');
		await page.locator('#activity [data-close]').click();
		assert(await page.evaluate(() => !document.activeElement.closest('dialog:not([open])')), 'three: nested dialog focus return');
		await page.locator('#profile').click();
		await page.locator('#profilemenu [data-go="settings"]').click();
		await page.getByRole('button', { name: 'Save and test example', exact: true }).click();
		await capture(page, 'three-phone-settings');
		receipt.checks.push('three: full title navigation, nested dialog focus return, saved settings test');
		await context.close();
	} catch (error) {
		receipt.errors.push(error.stack);
	} finally {
		await browser?.close();
		clearTimeout(deadline);
		receipt.pass = !receipt.errors.length && !receipt.violations.length && !receipt.blockedRequests.length;
		await writeFile(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2));
		process.stdout.write(JSON.stringify({ pass: receipt.pass, screenshots: receipt.screenshots.length, checks: receipt.checks.length, errors: receipt.errors.length, violations: receipt.violations.length, output }));
		if (!receipt.pass) process.exitCode = 1;
	}
})();
