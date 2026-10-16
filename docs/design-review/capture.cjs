const { createRequire } = require('node:module');
const { mkdir, writeFile, readFile } = require('node:fs/promises');
const { createHash } = require('node:crypto');
const path = require('node:path');
const assert = require('node:assert/strict');

const [toolsRoot, output] = process.argv.slice(2);
if (!path.isAbsolute(toolsRoot || '') || !path.isAbsolute(output || '')) {
	throw new Error('Supply absolute cached web/app tooling directory and capture output directory.');
}
const cached = createRequire(path.join(toolsRoot, 'package.json'));
const { chromium } = cached('playwright');
const axePath = cached.resolve('axe-core/axe.min.js');
const origin = 'http://127.0.0.1:18744';
const checks = [];
const pages = [];
const violations = [];
let browser;
let context;
const deadline = setTimeout(() => {
	process.stderr.write('Capture deadline reached\n');
	void browser?.close();
	process.exitCode = 1;
}, 150_000);

async function check(name, fn) {
	await fn();
	checks.push(name);
}
async function audit(page, label) {
	assert(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), `${label}: document overflow`);
	await page.addScriptTag({ path: axePath });
	const result = await page.evaluate(async () => axe.run(document, {
		runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'] }
	}));
	violations.push(...result.violations.map(v => ({ label, id: v.id, impact: v.impact, nodes: v.nodes.map(n => ({ target: n.target, summary: n.failureSummary })) })));
	checks.push(`${label}: no document overflow; axe executed`);
}

(async () => {
	await mkdir(output, { recursive: true });
	const artifact = await readFile(path.join(__dirname, 'index.html'));
	const errors = [];
	const blocked = [];
	try {
		browser = await chromium.launch({ headless: true, args: ['--renderer-process-limit=1', '--disable-background-networking'] });
		context = await browser.newContext({ reducedMotion: 'reduce', serviceWorkers: 'block' });
		await context.route('**/*', route => {
			if (new URL(route.request().url()).origin === origin) return route.continue();
			blocked.push(route.request().url());
			return route.abort();
		});
		const page = await context.newPage();
		page.on('pageerror', e => errors.push(e.message));
		for (const [device, width, height] of [['desktop', 1440, 1050], ['tablet', 834, 1112], ['phone', 390, 844]]) {
			await page.setViewportSize({ width, height });
			for (const screen of ['library', 'title', 'downloads', 'settings']) {
				await page.goto(`${origin}/?screen=${screen}`, { waitUntil: 'networkidle' });
				await page.evaluate(() => document.fonts.ready);
				await audit(page, `${device}/${screen}`);
				const file = `${device}-${screen}.png`;
				await page.screenshot({ path: path.join(output, file) });
				pages.push({ device, width, height, screen, file });
			}
		}
		await page.setViewportSize({ width: 320, height: 740 });
		for (const screen of ['library', 'title', 'downloads', 'settings']) {
			await page.goto(`${origin}/?screen=${screen}`);
			await audit(page, `narrow/${screen}`);
		}
		await page.setViewportSize({ width: 390, height: 844 });
		for (const state of ['empty', 'loading', 'error']) {
			await page.goto(`${origin}/?screen=library&state=${state}`);
			await audit(page, `phone/library/${state}`);
			await page.screenshot({ path: path.join(output, `phone-library-${state}.png`) });
		}
		await page.goto(`${origin}/?screen=library`);
		await check('Kind filtering returns two music fixtures', async () => {
			await page.getByRole('button', { name: 'Music 2' }).click();
			assert.equal(await page.locator('.media-link').count(), 2);
		});
		await check('Empty search exposes clear filters and recovers', async () => {
			await page.getByRole('searchbox', { name: 'Find in Library' }).fill('no-such-title');
			await page.getByRole('heading', { name: 'No titles match' }).waitFor();
			await page.getByRole('button', { name: 'Clear filters' }).click();
			assert.equal(await page.locator('.media-link').count(), 10);
		});
		await check('Poster/list toggle retains all titles', async () => {
			await page.getByRole('button', { name: 'List view', exact: true }).click();
			assert(await page.locator('#media-grid').evaluate(e => e.classList.contains('list-layout')));
			assert.equal(await page.locator('.media-link').count(), 10);
		});
		await check('More opens, Escape closes and returns focus', async () => {
			await page.getByRole('button', { name: 'More', exact: true }).click();
			assert.equal(await page.locator('#more').getAttribute('aria-expanded'), 'true');
			await page.keyboard.press('Escape');
			assert.equal(await page.locator('#more').getAttribute('aria-expanded'), 'false');
			assert(await page.locator('#more').evaluate(e => e === document.activeElement));
		});
		await check('Review action dialog closes with Escape and returns focus', async () => {
			const add = page.getByRole('button', { name: 'Add title', exact: true });
			await add.click();
			await page.getByRole('dialog').waitFor();
			await page.keyboard.press('Escape');
			assert(await add.evaluate(e => e === document.activeElement));
		});
		await check('Light theme at phone passes automated audit', async () => {
			await page.getByRole('button', { name: 'Switch to light theme' }).click();
			await audit(page, 'phone/library/light');
			await page.screenshot({ path: path.join(output, 'phone-library-light.png') });
		});
		await page.goto(`${origin}/?screen=settings`);
		await check('Example form reports local-only outcomes', async () => {
			await page.getByRole('button', { name: 'Test saved example' }).click();
			assert((await page.locator('#form-result').textContent()).includes('No linked app was contacted'));
			await page.getByRole('button', { name: 'Save and test example' }).click();
			assert((await page.locator('#form-result').textContent()).includes('Installed configuration unchanged'));
		});
		await check('Skip link reaches main', async () => {
			await page.goto(`${origin}/?screen=library`);
			await page.keyboard.press('Tab');
			assert.equal(await page.evaluate(() => document.activeElement.className), 'skip');
			await page.keyboard.press('Enter');
			assert.equal(await page.evaluate(() => document.activeElement.id), 'main');
		});
		await check('200 percent text has no document overflow', async () => {
			await page.evaluate(() => {
				const sizes = [...document.querySelectorAll('body *')].map(e => [e, parseFloat(getComputedStyle(e).fontSize)]);
				for (const [e, size] of sizes) e.style.fontSize = `${size * 2}px`;
			});
			assert(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth));
			await page.screenshot({ path: path.join(output, 'phone-library-text-200.png') });
		});
		assert.equal(errors.length, 0, 'Browser script errors');
		assert.equal(blocked.length, 0, 'Unexpected external network requests');
		assert.equal(violations.length, 0, 'Automated accessibility violations');
	} catch (e) {
		checks.push(`FAILED: ${e.message}`);
		process.exitCode = 1;
	} finally {
		await context?.close();
		await browser?.close();
		clearTimeout(deadline);
		const receipt = {
			schema: 1, observedUtc: new Date().toISOString(), artifactSha256: createHash('sha256').update(artifact).digest('hex'),
			baseCommit: '4b660863ab8c73977e9e2a58aa24811a4d64660b', scope: 'review HTML only; no product or installed-app acceptance',
			status: process.exitCode ? 'failed' : 'passed', checks, pages, violations, errors, blocked,
			cleanup: 'Own browser and isolated context closed; static review server retained separately'
		};
		await writeFile(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
		process.stdout.write(JSON.stringify({ status: receipt.status, checks: checks.length, captures: pages.length, violations: violations.length, last: checks.at(-1) }) + '\n');
	}
})();
