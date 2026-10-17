const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const { createHash } = require('node:crypto');
const { mkdir, readFile, writeFile } = require('node:fs/promises');
const path = require('node:path');

const [toolsRoot, outputRoot] = process.argv.slice(2);
if (!path.isAbsolute(toolsRoot || '') || !path.isAbsolute(outputRoot || '')) {
	throw new Error('Supply the absolute app tooling directory and a fresh absolute output directory.');
}

const repoRoot = path.resolve(__dirname, '..', '..', '..');
const appOrigin = 'http://127.0.0.1:18746';
const requireFromApp = createRequire(path.join(toolsRoot, 'package.json'));
const { chromium } = requireFromApp('playwright');
const relevantSource = [
	'web/app/src/routes/+layout.svelte',
	'web/app/src/routes/library/+page.svelte',
	'web/app/src/app.css',
	'web/app/src/lib/tokens.css',
	'web/app/src/lib/Icon.svelte'
];
const widths = [
	{ name: 'phone', width: 390, height: 844 },
	{ name: 'tablet', width: 834, height: 1112 },
	{ name: 'desktop', width: 1440, height: 1000 }
];
const expectedKinds = new Map([
	['Movies', 'movie'],
	['TV Shows', 'show'],
	['Music', 'artist']
]);

const receipt = {
	schema: 1,
	kind: 'intercom-library-navigation-read-only-regression',
	startedUtc: new Date().toISOString(),
	policy: {
		browser: 'one isolated headless Chromium context with one page',
		requests: 'GET only; same-origin app resources and public TMDB images only',
		interactions: 'navigation dialog, filter links, collapse toggle, Escape, focus, and skip-link checks only',
		credentials: 'no credentials read, supplied, or stored',
		responseBodies: 'never recorded'
	},
	source: {},
	checks: [],
	screenshots: [],
	nonGetAttempts: [],
	blockedOrigins: [],
	httpErrors: [],
	consoleErrors: [],
	pageErrors: [],
	runErrors: []
};

let browser;
let context;
let page;
let deadlineReached = false;
const deadline = setTimeout(() => {
	deadlineReached = true;
	receipt.runErrors.push('60000-millisecond regression deadline reached');
	void browser?.close();
}, 60_000);

function git(args, encoding = 'utf8') {
	return execFileSync('git', ['-C', repoRoot, ...args], { encoding, windowsHide: true });
}

function sha256(parts) {
	const hash = createHash('sha256');
	for (const part of parts) hash.update(part);
	return hash.digest('hex');
}

async function sourceIdentity() {
	const hash = createHash('sha256');
	for (const relative of relevantSource) {
		hash.update(`\0${relative}\0`);
		hash.update(await readFile(path.join(repoRoot, relative)));
	}
	return {
		commit: git(['rev-parse', 'HEAD']).trim(),
		relevantTreeSha256: hash.digest('hex'),
		files: relevantSource
	};
}

function compactUrl(raw) {
	try {
		const value = new URL(raw);
		const safe = new URLSearchParams();
		const kind = value.searchParams.get('kind');
		if (kind && ['movie', 'show', 'artist'].includes(kind)) safe.set('kind', kind);
		return `${value.origin}${value.pathname}${safe.size ? `?${safe}` : ''}`;
	} catch {
		return String(raw);
	}
}

function check(name, passed, evidence = {}) {
	receipt.checks.push({ name, passed: !!passed, evidence });
	if (!passed) throw new Error(`Check failed: ${name}`);
}

async function shot(name) {
	const filename = `${name}.png`;
	const absolute = path.join(outputRoot, filename);
	await page.screenshot({ path: absolute, fullPage: false, animations: 'disabled' });
	const bytes = await readFile(absolute);
	receipt.screenshots.push({ name, file: filename, bytes: bytes.length, sha256: sha256([bytes]), url: compactUrl(page.url()), viewport: page.viewportSize() });
}

async function settle() {
	await page.waitForLoadState('domcontentloaded');
	await page.locator('main').first().waitFor({ state: 'visible', timeout: 10_000 });
	await page.evaluate(() => document.fonts.ready);
	await page.waitForTimeout(100);
}

function triggerFor(viewport) {
	if (viewport.name === 'phone') return page.locator('nav.tabbar').getByRole('button', { name: 'Library', exact: true });
	return page.locator('aside.global-rail').getByRole('button', { name: 'Library', exact: true });
}

async function dialogChecks(viewport) {
	await page.setViewportSize({ width: viewport.width, height: viewport.height });
	await page.goto(`${appOrigin}/library?kind=movie`, { waitUntil: 'domcontentloaded', timeout: 12_000 });
	await settle();
	const trigger = triggerFor(viewport);
	const dialog = page.locator('dialog.navigation-dialog');
	const before = page.url();
	await trigger.click();
	await dialog.waitFor({ state: 'visible' });
	check(`${viewport.name}: Library trigger opens native chooser`, await dialog.evaluate((element) => element instanceof HTMLDialogElement && element.open), { url: compactUrl(page.url()) });
	check(`${viewport.name}: opening chooser preserves URL`, page.url() === before, { before: compactUrl(before), after: compactUrl(page.url()) });

	const links = dialog.getByRole('link');
	const linkEvidence = [];
	for (const [label, expectedKind] of expectedKinds) {
		const link = links.filter({ hasText: label }).first();
		if (!(await link.count())) continue;
		const href = await link.getAttribute('href');
		const resolved = new URL(href, appOrigin);
		const valid = resolved.origin === appOrigin && resolved.pathname === '/library' && resolved.searchParams.get('kind') === expectedKind;
		linkEvidence.push({ label, expectedKind, href: compactUrl(resolved.href), valid });
	}
	check(`${viewport.name}: available media kinds use real Library filter URLs`, linkEvidence.length > 0 && linkEvidence.every((item) => item.valid), { links: linkEvidence });
	await shot(`${viewport.name}-library-chooser-open`);

	await page.keyboard.press('Escape');
	await dialog.waitFor({ state: 'hidden' });
	check(`${viewport.name}: Escape closes chooser`, !(await dialog.evaluate((element) => element.open)));
	check(`${viewport.name}: Escape returns focus to Library trigger`, await trigger.evaluate((element) => document.activeElement === element));

	for (const item of linkEvidence) {
		await trigger.click();
		await dialog.waitFor({ state: 'visible' });
		await Promise.all([
			page.waitForURL((url) => url.pathname === '/library' && url.searchParams.get('kind') === item.expectedKind),
			dialog.getByRole('link', { name: item.label, exact: true }).click()
		]);
		await dialog.waitFor({ state: 'hidden' });
		const landed = new URL(page.url());
		check(`${viewport.name}: selecting ${item.label} closes chooser and applies filter`, !await dialog.evaluate((element) => element.open) && landed.pathname === '/library' && landed.searchParams.get('kind') === item.expectedKind, { landed: compactUrl(landed.href) });
	}
}

async function desktopContextChecks() {
	const viewport = widths.find((item) => item.name === 'desktop');
	await page.setViewportSize({ width: viewport.width, height: viewport.height });
	await page.goto(`${appOrigin}/library?kind=movie`, { waitUntil: 'domcontentloaded', timeout: 12_000 });
	await settle();
	const context = page.locator('aside.context-sidebar');
	const parent = context.getByRole('button', { name: 'Library', exact: true });
	const reveal = context.locator('.library-reveal');
	const before = page.url();
	check('desktop: context Library begins expanded', await parent.getAttribute('aria-expanded') === 'true');
	check('desktop: context Library has its library icon without an arrow control', await parent.locator('svg').count() === 1 && !(await parent.locator('.chevron, [data-icon="chevron"], [data-icon="arrow"]').count()), { svgCount: await parent.locator('svg').count() });
	await parent.click();
	check('desktop: Library toggle preserves URL', page.url() === before, { url: compactUrl(page.url()) });
	check('desktop: collapsed Library marks children inert', await parent.getAttribute('aria-expanded') === 'false' && await reveal.evaluate((element) => element.inert), { ariaExpanded: await parent.getAttribute('aria-expanded'), inert: await reveal.evaluate((element) => element.inert) });
	await shot('desktop-library-context-collapsed');
	await parent.click();
	check('desktop: Library toggle expands children', await parent.getAttribute('aria-expanded') === 'true' && !await reveal.evaluate((element) => element.inert));
	await shot('desktop-library-context-expanded');
}

function channel(value) {
	const scaled = value / 255;
	return scaled <= 0.04045 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
}

function luminance(css) {
	const values = css.match(/[\d.]+/g)?.slice(0, 3).map(Number);
	if (!values || values.length !== 3) throw new Error(`Unsupported computed color: ${css}`);
	return 0.2126 * channel(values[0]) + 0.7152 * channel(values[1]) + 0.0722 * channel(values[2]);
}

function contrast(foreground, background) {
	const a = luminance(foreground);
	const b = luminance(background);
	return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

async function skipLinkContrastChecks() {
	await page.setViewportSize({ width: 1440, height: 1000 });
	for (const theme of ['light', 'dark']) {
		await page.goto(`${appOrigin}/library?kind=movie`, { waitUntil: 'domcontentloaded', timeout: 12_000 });
		await settle();
		const skip = page.getByRole('link', { name: 'Skip to content', exact: true });
		await page.evaluate((value) => { document.documentElement.dataset.theme = value; }, theme);
		await page.keyboard.press('Tab');
		check(`${theme}: skip link receives first keyboard focus`, await skip.evaluate((element) => document.activeElement === element));
		const colors = await skip.evaluate((element) => {
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			return { foreground: style.color, background: style.backgroundColor, top: rect.top, bottom: rect.bottom };
		});
		const ratio = contrast(colors.foreground, colors.background);
		check(`${theme}: skip-link foreground contrast is at least 4.5:1`, ratio >= 4.5, { ...colors, ratio: Math.round(ratio * 100) / 100 });
		check(`${theme}: focused skip link is visible`, colors.top >= 0 && colors.bottom <= 1000, { top: colors.top, bottom: colors.bottom });
		await shot(`desktop-${theme}-skip-link`);
	}
}

(async () => {
	await mkdir(path.dirname(outputRoot), { recursive: true });
	await mkdir(outputRoot, { recursive: false });
	receipt.source.start = await sourceIdentity();
	receipt.source.scriptSha256 = sha256([await readFile(__filename)]);
	try {
		browser = await chromium.launch({ headless: true, args: ['--renderer-process-limit=1', '--disable-background-networking'] });
		context = await browser.newContext({ reducedMotion: 'reduce', serviceWorkers: 'block', colorScheme: 'light', locale: 'en-US' });
		await context.route('**/*', async (route) => {
			const request = route.request();
			const method = request.method().toUpperCase();
			if (method !== 'GET') {
				receipt.nonGetAttempts.push({ method, url: compactUrl(request.url()), resourceType: request.resourceType() });
				return route.abort('blockedbyclient');
			}
			const origin = new URL(request.url()).origin;
			if (origin !== appOrigin && origin !== 'https://image.tmdb.org') {
				receipt.blockedOrigins.push({ url: compactUrl(request.url()), resourceType: request.resourceType() });
				return route.abort('blockedbyclient');
			}
			return route.continue();
		});
		page = await context.newPage();
		page.setDefaultTimeout(10_000);
		page.on('console', (message) => {
			if (message.type() === 'error') receipt.consoleErrors.push({ text: message.text(), url: compactUrl(message.location().url || '') });
		});
		page.on('pageerror', (error) => receipt.pageErrors.push({ message: error.message }));
		page.on('response', (response) => {
			if (response.status() >= 400) receipt.httpErrors.push({ status: response.status(), url: compactUrl(response.url()) });
		});

		await dialogChecks(widths[0]);
		await dialogChecks(widths[1]);
		await desktopContextChecks();
		await skipLinkContrastChecks();
	} catch (error) {
		receipt.runErrors.push(error instanceof Error ? error.stack || error.message : String(error));
	} finally {
		await context?.close().catch(() => {});
		await browser?.close().catch(() => {});
		clearTimeout(deadline);
		receipt.source.end = await sourceIdentity().catch((error) => ({ error: String(error) }));
		receipt.source.changedDuringRun = receipt.source.start?.relevantTreeSha256 !== receipt.source.end?.relevantTreeSha256;
		receipt.deadlineReached = deadlineReached;
		receipt.finishedUtc = new Date().toISOString();
		receipt.summary = {
			checks: receipt.checks.length,
			passedChecks: receipt.checks.filter((item) => item.passed).length,
			screenshots: receipt.screenshots.length,
			nonGetAttempts: receipt.nonGetAttempts.length,
			blockedOrigins: receipt.blockedOrigins.length,
			httpErrors: receipt.httpErrors.length,
			consoleErrors: receipt.consoleErrors.length,
			pageErrors: receipt.pageErrors.length,
			sourceChangedDuringRun: !!receipt.source.changedDuringRun
		};
		receipt.status = receipt.runErrors.length || deadlineReached || receipt.source.changedDuringRun || receipt.nonGetAttempts.length || receipt.blockedOrigins.length || receipt.httpErrors.length || receipt.consoleErrors.length || receipt.pageErrors.length || receipt.checks.some((item) => !item.passed) ? 'failed' : 'passed';
		receipt.cleanup = 'The isolated browser context and browser were closed. The preview server and application data were not changed.';
		await writeFile(path.join(outputRoot, 'receipt.json'), `${JSON.stringify(receipt, null, 2)}\n`);
		process.stdout.write(`${JSON.stringify({ status: receipt.status, ...receipt.summary, outputRoot })}\n`);
		if (receipt.status !== 'passed') process.exitCode = 1;
	}
})();
