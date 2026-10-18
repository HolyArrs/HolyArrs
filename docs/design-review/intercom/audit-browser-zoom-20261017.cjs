/* Isolated native browser-zoom audit. GET-only, preview-scoped and profile-disposable. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { chromium } = require('../../../web/app/node_modules/playwright');

const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const outputRoot = path.join(root, '.omx/artifacts/visual-ralph/intercom/audit20261017/zoom');
const runId = `${new Date().toISOString().replace(/[:.]/g, '-')}-${process.pid}`;
const output = path.join(outputRoot, runId);
const artworkHosts = new Set(['image.tmdb.org', 'static.tvmaze.com', 'is1-ssl.mzstatic.com']);
const routes = [
	{ path: '/library?kind=movie', name: 'library-list', prepare: async (page) => page.getByRole('button', { name: 'List' }).click() },
	{ path: '/wishlist', name: 'requests' },
	{ path: '/downloads', name: 'downloads' },
	{ path: '/settings', name: 'settings' }
];
const zoom200Preference = Math.log(2) / Math.log(1.2);

function identity() {
	const hash = createHash('sha256');
	function walk(directory) {
		for (const name of fs.readdirSync(directory).sort()) {
			const file = path.join(directory, name);
			if (fs.statSync(file).isDirectory()) walk(file);
			else hash.update(path.relative(root, file)).update(fs.readFileSync(file));
		}
	}
	walk(path.join(root, 'web/app/src'));
	return {
		commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(),
		sourceHash: hash.digest('hex')
	};
}

function makeProfile(label, zoomLevel) {
	const profile = fs.mkdtempSync(path.join(os.tmpdir(), `holyarrs-${label}-`));
	const defaultDirectory = path.join(profile, 'Default');
	fs.mkdirSync(defaultDirectory, { recursive: true });
	fs.writeFileSync(path.join(defaultDirectory, 'Preferences'), JSON.stringify({
		partition: { default_zoom_level: zoomLevel },
		profile: { default_content_setting_values: { notifications: 2 } }
	}));
	return profile;
}

function isOwnedProfile(profile) {
	const temporaryRoot = path.resolve(os.tmpdir());
	const resolved = path.resolve(profile);
	const relative = path.relative(temporaryRoot, resolved);
	return relative !== '' && !relative.startsWith('..') && !path.isAbsolute(relative) && path.basename(resolved).startsWith('holyarrs-');
}

function removeProfile(profile) {
	assert.ok(isOwnedProfile(profile), `Refusing to remove non-owned profile: ${profile}`);
	fs.rmSync(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
	assert.equal(fs.existsSync(profile), false, `Owned profile cleanup failed: ${profile}`);
}

async function lockContext(context) {
	if (typeof context.routeWebSocket === 'function') await context.routeWebSocket('**/*', (socket) => socket.close());
	await context.route('**/*', async (route) => {
		const request = route.request();
		const url = new URL(request.url());
		if (request.method() !== 'GET') return route.abort('blockedbyclient');
		if (url.origin === origin) return route.continue();
		if (request.resourceType() === 'image' && url.protocol === 'https:' && artworkHosts.has(url.hostname)) return route.continue();
		return route.abort('blockedbyclient');
	});
	await context.addInitScript(() => localStorage.setItem('holyarrs-theme', 'light'));
}

async function windowMetrics(context, page) {
	const cdp = await context.newCDPSession(page);
	const browserWindow = await cdp.send('Browser.getWindowForTarget');
	await cdp.detach();
	const pageMetrics = await page.evaluate(() => ({
		innerWidth,
		innerHeight,
		outerWidth,
		outerHeight,
		devicePixelRatio,
		visualViewport: visualViewport ? {
			width: visualViewport.width,
			height: visualViewport.height,
			scale: visualViewport.scale,
			offsetLeft: visualViewport.offsetLeft,
			offsetTop: visualViewport.offsetTop
		} : null
	}));
	return { ...pageMetrics, cdpWindow: browserWindow.bounds };
}

function compareZoom(baseline, zoomed) {
	const widthRatio = baseline.innerWidth / zoomed.innerWidth;
	const heightRatio = baseline.innerHeight / zoomed.innerHeight;
	const dprRatio = zoomed.devicePixelRatio / baseline.devicePixelRatio;
	const physicalWidthRatio = (zoomed.innerWidth * zoomed.devicePixelRatio) / (baseline.innerWidth * baseline.devicePixelRatio);
	const windowWidthRatio = zoomed.cdpWindow.width / baseline.cdpWindow.width;
	const near = (actual, expected, tolerance) => Math.abs(actual - expected) <= tolerance;
	const proven = near(widthRatio, 2, 0.2) && near(dprRatio, 2, 0.2) && near(physicalWidthRatio, 1, 0.12) && near(windowWidthRatio, 1, 0.02);
	return {
		proven,
		widthRatio,
		heightRatio,
		dprRatio,
		physicalWidthRatio,
		windowWidthRatio,
		reason: proven
			? 'CSS viewport width and devicePixelRatio changed by approximately 2x while the physical pixel width remained stable.'
			: 'The Chromium preference did not produce a measured 2x CSS-width and devicePixelRatio change; 200% browser zoom is unavailable in this run.'
	};
}

async function openMeasuredPage(context, route) {
	const page = await context.newPage();
	const errors = [];
	page.on('pageerror', (error) => errors.push(error.message));
	const response = await page.goto(origin + route, { waitUntil: 'networkidle', timeout: 30_000 });
	assert.ok(response && response.status() >= 200 && response.status() < 400, `Document failed for ${route}`);
	await page.locator('body').waitFor();
	await page.evaluate(() => document.fonts.ready);
	return { page, response, errors };
}

async function keyboardEvidence(page) {
	await page.evaluate(() => document.activeElement instanceof HTMLElement && document.activeElement.blur());
	for (let attempt = 0; attempt < 20; attempt++) {
		await page.keyboard.press('Tab');
		const usable = await page.evaluate(() => {
			const element = document.activeElement;
			if (!(element instanceof HTMLElement) || element === document.body) return false;
			const bounds = element.getBoundingClientRect();
			return bounds.width > 0 && bounds.height > 0 && getComputedStyle(element).visibility !== 'hidden';
		});
		if (usable) break;
	}
	const forward = await page.evaluate(() => {
		const element = document.activeElement;
		const bounds = element?.getBoundingClientRect();
		return {
			tag: element?.tagName,
			label: (element?.getAttribute?.('aria-label') || element?.textContent || '').trim().slice(0, 120),
			focusVisible: element?.matches?.(':focus-visible') ?? false,
			bounds: bounds ? { x: bounds.x, y: bounds.y, width: bounds.width, height: bounds.height } : null
		};
	});
	assert.ok(forward.tag && forward.tag !== 'BODY' && forward.bounds?.width > 0 && forward.bounds?.height > 0, 'Tab did not reach a visible control');
	await page.keyboard.press('Shift+Tab');
	const reverseTag = await page.evaluate(() => document.activeElement?.tagName ?? '');
	return { forward, reverseTag };
}

async function routeCheck(context, descriptor) {
	const startedAt = new Date().toISOString();
	const { page, response, errors } = await openMeasuredPage(context, descriptor.path);
	try {
		if (descriptor.prepare) await descriptor.prepare(page);
		const expected = new URL(descriptor.path, origin);
		const actual = new URL(page.url());
		assert.equal(actual.pathname, expected.pathname, `Unexpected redirect for ${descriptor.path}`);
		for (const [key, value] of expected.searchParams) assert.equal(actual.searchParams.get(key), value, `Lost ${key} on ${descriptor.path}`);
		const heading = (await page.locator('main h1').first().innerText()).trim();
		assert.ok(heading && !/not found|sign in|404/i.test(heading), `Expected app screen missing for ${descriptor.path}`);
		const geometry = await page.evaluate(() => ({
			innerWidth,
			innerHeight,
			dpr: devicePixelRatio,
			scrollWidth: document.documentElement.scrollWidth,
			overflow: document.documentElement.scrollWidth > innerWidth + 1
		}));
		assert.equal(geometry.overflow, false, `Horizontal document overflow on ${descriptor.path}`);
		assert.deepEqual(errors, [], `Browser runtime errors on ${descriptor.path}`);
		const keyboard = await keyboardEvidence(page);
		const screenshot = `${descriptor.name}-zoom200.png`;
		await page.screenshot({
			path: path.join(output, screenshot),
			fullPage: false,
			mask: [page.locator('input[type="password"], [data-sensitive], .secret, .secret-value, .fresh code')]
		});
		return { ...descriptor, startedAt, finishedAt: new Date().toISOString(), status: response.status(), heading, geometry, keyboard, errors, screenshot };
	} finally {
		await page.close();
	}
}

async function launchProfile(label, zoomLevel) {
	const profile = makeProfile(label, zoomLevel);
	let context;
	try {
		context = await chromium.launchPersistentContext(profile, {
			headless: true,
			viewport: null,
			serviceWorkers: 'block',
			reducedMotion: 'no-preference',
			args: ['--window-size=1280,900']
		});
		await lockContext(context);
		for (const initial of context.pages()) await initial.close();
		return { profile, context };
	} catch (error) {
		if (context) await context.close().catch(() => {});
		removeProfile(profile);
		throw error;
	}
}

async function main() {
	fs.mkdirSync(output, { recursive: true });
	const before = identity();
	const startedAt = new Date().toISOString();
	const engine = { executablePath: chromium.executablePath() };
	let baselineProfile;
	let zoomProfile;
	let baseline;
	let zoomed;
	let zoomComparison;
	let baselineErrors = [];
	let zoomErrors = [];
	let baselineScreenshot;
	let zoomScreenshot;
	const checks = [];
	try {
		const baselineRun = await launchProfile('zoom-baseline', 0);
		baselineProfile = baselineRun.profile;
		try {
			engine.baselineVersion = baselineRun.context.browser()?.version();
			const opened = await openMeasuredPage(baselineRun.context, '/library?kind=movie');
			try {
				baseline = await windowMetrics(baselineRun.context, opened.page);
				baselineErrors = opened.errors;
				baselineScreenshot = 'baseline-100.png';
				await opened.page.screenshot({ path: path.join(output, baselineScreenshot), fullPage: false, mask: [opened.page.locator('input[type="password"], [data-sensitive], .secret, .secret-value, .fresh code')] });
			}
			finally { await opened.page.close(); }
		} finally {
			await baselineRun.context.close();
			removeProfile(baselineProfile);
			baselineProfile = undefined;
		}

		const zoomRun = await launchProfile('zoom-200', zoom200Preference);
		zoomProfile = zoomRun.profile;
		try {
			engine.zoomVersion = zoomRun.context.browser()?.version();
			assert.equal(engine.zoomVersion, engine.baselineVersion, 'Baseline and zoom runs used different Chromium versions');
			const opened = await openMeasuredPage(zoomRun.context, '/library?kind=movie');
			try {
				zoomed = await windowMetrics(zoomRun.context, opened.page);
				zoomErrors = opened.errors;
				zoomScreenshot = 'probe-zoom200.png';
				await opened.page.screenshot({ path: path.join(output, zoomScreenshot), fullPage: false, mask: [opened.page.locator('input[type="password"], [data-sensitive], .secret, .secret-value, .fresh code')] });
			}
			finally { await opened.page.close(); }
			zoomComparison = compareZoom(baseline, zoomed);
			if (zoomComparison.proven) {
				for (const descriptor of routes) checks.push(await routeCheck(zoomRun.context, descriptor));
			}
		} finally {
			await zoomRun.context.close();
			removeProfile(zoomProfile);
			zoomProfile = undefined;
		}
	} finally {
		if (baselineProfile && fs.existsSync(baselineProfile)) removeProfile(baselineProfile);
		if (zoomProfile && fs.existsSync(zoomProfile)) removeProfile(zoomProfile);
	}

	const after = identity();
	assert.equal(before.sourceHash, after.sourceHash, 'Source changed during zoom capture; repeat against stable source');
	assert.deepEqual(baselineErrors, [], 'Baseline zoom probe had browser runtime errors');
	assert.deepEqual(zoomErrors, [], '200% zoom probe had browser runtime errors');
	if (zoomComparison?.proven) assert.equal(checks.length, routes.length, 'Proven zoom run did not cover every requested route');

	const finalStatus = zoomComparison?.proven ? 'verified' : 'unavailable';
	const receipt = {
		runId,
		startedAt,
		finishedAt: new Date().toISOString(),
		origin,
		before,
		after,
		engine,
		requestedWindow: { width: 1280, height: 900 },
		preferences: { baselineDefaultZoomLevel: 0, zoom200DefaultZoomLevel: zoom200Preference },
		baseline,
		zoomed,
		probeErrors: { baseline: baselineErrors, zoomed: zoomErrors },
		probeScreenshots: { baseline: baselineScreenshot, zoomed: zoomScreenshot },
		zoomComparison,
		status: finalStatus,
		certification: finalStatus === 'verified'
			? {
				scope: 'native-browser-zoom-and-document-horizontal-overflow-only',
				claim: 'The isolated Chromium profile measurably applied approximately 200% native browser zoom, and the listed routes had no document-level horizontal overflow at that measured zoom.',
				excludes: ['component-level clipping', 'all keyboard-accessibility behavior', 'physical 4K or TV behavior', 'browser zoom in other engines']
			}
			: null,
		checks,
		profileCleanup: 'Both harness-owned profiles were removed and verified absent under the operating-system temporary directory.',
		limitations: zoomComparison?.proven ? [] : ['Chromium ignored or did not measurably apply partition.default_zoom_level in this headless environment, so no route is certified at 200% browser zoom.']
	};
	fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2));
	console.log(`Native 200% zoom: ${receipt.status}; ${checks.length} route checks`);
}

if (require.main === module) main().catch((error) => { console.error(error); process.exitCode = 1; });
module.exports = { compareZoom, identity, isOwnedProfile, routes, runId, zoom200Preference };
