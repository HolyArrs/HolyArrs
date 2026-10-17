const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const { createHash } = require('node:crypto');
const { mkdir, readFile, stat, writeFile } = require('node:fs/promises');
const path = require('node:path');

const [toolsRoot, outputRoot] = process.argv.slice(2);
if (!path.isAbsolute(toolsRoot || '') || !path.isAbsolute(outputRoot || '')) {
	throw new Error('Supply the absolute cached web/app tooling directory and a fresh absolute output directory.');
}

const repoRoot = path.resolve(__dirname, '..', '..', '..');
const appRoot = path.join(repoRoot, 'web', 'app');
const requireFromApp = createRequire(path.join(toolsRoot, 'package.json'));
const { chromium } = requireFromApp('playwright');
const axePath = requireFromApp.resolve('axe-core/axe.min.js');
const appOrigin = 'http://127.0.0.1:18746';
const permittedGetOrigins = new Set([appOrigin, 'https://image.tmdb.org']);

const viewports = [
	{ name: 'desktop', width: 1920, height: 1192 },
	{ name: 'tablet', width: 834, height: 1112 },
	{ name: 'phone', width: 390, height: 844 }
];
const requestedViewport = process.env.HOLYARRS_CAPTURE_VIEWPORT || '';
const selectedViewports = requestedViewport
	? viewports.filter(({ name }) => name === requestedViewport)
	: viewports;
if (requestedViewport && selectedViewports.length !== 1) {
	throw new Error(`Unknown HOLYARRS_CAPTURE_VIEWPORT: ${requestedViewport}`);
}
const captureDeadlineMs = Number(process.env.HOLYARRS_CAPTURE_DEADLINE_MS || 60_000);
if (!Number.isFinite(captureDeadlineMs) || captureDeadlineMs < 1_000 || captureDeadlineMs > 60_000) {
	throw new Error('HOLYARRS_CAPTURE_DEADLINE_MS must be between 1000 and 60000.');
}
const routes = [
	{ area: 'home', name: 'home', path: '/' },
	{ area: 'library', name: 'library-movies', path: '/library?kind=movie' },
	{ area: 'library', name: 'library-title-1', path: '/library/1' },
	{ area: 'library-management', name: 'library-held', path: '/library/held' },
	{ area: 'library-management', name: 'library-turned-down', path: '/library/turned-down' },
	{ area: 'library-management', name: 'library-copies', path: '/library/copies' },
	{ area: 'library-management', name: 'library-missing-subtitles', path: '/library/missing-subtitles' },
	{ area: 'library-management', name: 'library-rename', path: '/library/rename' },
	{ area: 'library-management', name: 'library-import', path: '/library/import' },
	{ area: 'library', name: 'library-collections', path: '/library/collections' },
	{ area: 'people', name: 'people', path: '/people' },
	{ area: 'search', name: 'search', path: '/search' },
	{ area: 'requests', name: 'wishlist', path: '/wishlist' },
	{ area: 'downloads', name: 'downloads', path: '/downloads' },
	{ area: 'downloads', name: 'downloads-decide', path: '/downloads/decide' },
	{ area: 'downloads', name: 'downloads-history', path: '/downloads/history' },
	{ area: 'calendar', name: 'calendar', path: '/calendar' },
	{ area: 'settings', name: 'settings', path: '/settings' },
	{ area: 'settings', name: 'settings-naming', path: '/settings/naming' },
	{ area: 'settings', name: 'settings-search', path: '/settings/search' },
	{ area: 'settings', name: 'settings-parts', path: '/settings/parts' },
	{ area: 'settings', name: 'settings-schedules', path: '/settings/schedules' },
	{ area: 'settings', name: 'settings-this-app', path: '/settings/this-app' },
	{ area: 'account', name: 'account', path: '/account' }
];
const sensitiveApiPaths = [
	/^\/api\/v1\/settings\/(?:tmdb-token|api|sign-in)(?:\/|$)/,
	/^\/api\/v1\/settings\/(?:sources|subtitles|downloaders|outside-lists|media-servers|alerts|users)(?:\/|$)/,
	/^\/api\/v1\/auth\/(?:sessions|api-keys)(?:\/|$)/
];
const intentionalBlockedNonGetPaths = [/^\/api\/v1\/settings\/naming\/example(?:\/|$)/];

const receipt = {
	schema: 1,
	kind: 'intercom-actual-app-read-only-responsive-accessibility-capture',
	milestone: process.env.HOLYARRS_CAPTURE_MILESTONE || 'intermediate-baseline',
	certification: 'Intermediate design evidence only; this does not certify the final combined application.',
	startedUtc: new Date().toISOString(),
	appOrigin,
	policy: {
		browser: 'one isolated headless Chromium context with one page',
		requests: 'GET only; same-origin app resources and public TMDB images only',
		sensitiveData: 'credential, key, provider, user, and session endpoints are blocked before response; response bodies are never recorded',
		serviceWorkers: 'blocked',
		reducedMotion: 'reduce',
		credentials: 'no credentials read, supplied, or stored',
		mutations: 'no form submission or state-changing interaction'
	},
	source: {},
	screenshots: [],
	audits: [],
	console: [],
	sensitiveConsoleEvents: [],
	intentionalConsoleEvents: [],
	pageErrors: [],
	requestFailures: [],
	httpErrors: [],
	nonGetAttempts: [],
	intentionalCaptureGaps: [],
	blockedOrigins: [],
		sensitiveRequestsBlocked: [],
	authBlockers: [],
	fixtureChecks: [],
	runErrors: []
};

const sensitiveBlockedUrls = new Set();
const intentionalBlockedUrls = new Set();

let browser;
let context;
let page;
let deadlineReached = false;
const deadline = setTimeout(() => {
	deadlineReached = true;
	receipt.runErrors.push(`${captureDeadlineMs}-millisecond capture deadline reached`);
	void browser?.close();
}, captureDeadlineMs);

function git(args, encoding = 'utf8') {
	return execFileSync('git', ['-C', repoRoot, ...args], { encoding, windowsHide: true });
}

function sha256(parts) {
	const hash = createHash('sha256');
	for (const part of parts) hash.update(part);
	return hash.digest('hex');
}

async function sourceIdentity() {
	const commit = git(['rev-parse', 'HEAD']).trim();
	const status = git(['status', '--porcelain=v1', '-z', '--', 'web/app'], 'buffer');
	const diff = git(['diff', '--binary', 'HEAD', '--', 'web/app'], 'buffer');
	const untrackedRaw = git(['ls-files', '--others', '--exclude-standard', '-z', '--', 'web/app'], 'buffer');
	const untracked = untrackedRaw
		.toString('utf8')
		.split('\0')
		.filter(Boolean)
		.sort();
	const dirtyParts = [status, diff];
	for (const relative of untracked) {
		dirtyParts.push(Buffer.from(`\0${relative}\0`, 'utf8'));
		dirtyParts.push(await readFile(path.join(repoRoot, relative)));
	}

	const trackedAndOther = git(['ls-files', '-co', '--exclude-standard', '-z', '--', 'web/app'], 'buffer')
		.toString('utf8')
		.split('\0')
		.filter(Boolean)
		.sort();
	const tree = createHash('sha256');
	for (const relative of trackedAndOther) {
		const absolute = path.join(repoRoot, relative);
		const info = await stat(absolute).catch(() => null);
		if (!info?.isFile()) continue;
		tree.update(`\0${relative}\0`);
		tree.update(await readFile(absolute));
	}

	return {
		commit,
		dirty: status.length > 0,
		dirtySha256: sha256(dirtyParts),
		appTreeSha256: tree.digest('hex'),
		appTreeFiles: trackedAndOther.length,
		untrackedAppFiles: untracked.length
	};
}

function compactUrl(raw) {
	try {
		const value = new URL(raw);
		if (value.pathname.startsWith('/api/')) return `${value.origin}${value.pathname}`;
		const safeQuery = new URLSearchParams();
		for (const [name, field] of value.searchParams) {
			if (name === 'kind' && ['movie', 'show', 'music'].includes(field)) safeQuery.set(name, field);
		}
		const query = safeQuery.size ? `?${safeQuery}` : '';
		return `${value.origin}${value.pathname}${query}`;
	} catch {
		return String(raw);
	}
}

function sanitizeConsoleText(text) {
	return String(text)
		.replace(/([?&](?:token|key|secret|password|code|state|ticket|auth)=)[^&\s]+/gi, '$1[redacted]')
		.replace(/\b(?:api[_ -]?key|token|secret|password)\s*[:=]\s*[^\s,;]+/gi, '[redacted credential field]');
}

function isSensitiveApi(raw) {
	try {
		const pathname = new URL(raw).pathname;
		return sensitiveApiPaths.some((pattern) => pattern.test(pathname));
	} catch {
		return false;
	}
}

function isIntentionalBlockedNonGet(raw) {
	try {
		const pathname = new URL(raw).pathname;
		return intentionalBlockedNonGetPaths.some((pattern) => pattern.test(pathname));
	} catch {
		return false;
	}
}

async function waitForSettledScreen() {
	await page.waitForLoadState('domcontentloaded');
	await page.locator('main').first().waitFor({ state: 'visible', timeout: 10_000 }).catch(() => {});
	await page.evaluate(async () => {
		await document.fonts.ready;
		await Promise.all(
			[...document.images]
				.filter((image) => !image.complete)
				.map((image) => new Promise((resolve) => {
					const done = () => resolve(undefined);
					image.addEventListener('load', done, { once: true });
					image.addEventListener('error', done, { once: true });
					setTimeout(done, 2_000);
				}))
		);
	});
	await page.waitForTimeout(150);
}

async function authBlocker() {
	const pathname = new URL(page.url()).pathname;
	const signInHeading = await page.getByRole('heading', { name: /sign in/i }).count();
	return pathname.endsWith('/login') || signInHeading > 0;
}

async function layoutAudit(label) {
	return page.evaluate((auditLabel) => {
		const root = document.documentElement;
		const body = document.body;
		const clientWidth = root.clientWidth;
		const protruding = [...document.querySelectorAll('body *')]
			.map((element) => ({ element, rect: element.getBoundingClientRect() }))
			.filter(({ rect }) => rect.right > clientWidth + 1 || rect.left < -1)
			.filter(({ element, rect }) => ![...element.children].some((child) => {
				const childRect = child.getBoundingClientRect();
				return childRect.right > rect.right + 0.5 || childRect.left < rect.left - 0.5;
			}))
			.slice(0, 8)
			.map(({ element, rect }) => ({
				tag: element.tagName.toLowerCase(),
				id: element.id || null,
				classes: [...element.classList].slice(0, 5),
				left: Math.round(rect.left * 10) / 10,
				right: Math.round(rect.right * 10) / 10
			}));
		return {
			label: auditLabel,
			url: location.href,
			title: document.title,
			viewport: { width: innerWidth, height: innerHeight },
			document: {
				clientWidth,
				scrollWidth: root.scrollWidth,
				scrollHeight: root.scrollHeight,
				bodyScrollWidth: body.scrollWidth
			},
			hasHorizontalOverflow: root.scrollWidth > clientWidth + 1,
			protruding
		};
	}, label);
}

async function axeAudit(label) {
	await page.addScriptTag({ path: axePath });
	const result = await page.evaluate(async () => window.axe.run(document, {
		runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'] },
		resultTypes: ['violations', 'incomplete']
	}));
	return {
		label,
		violations: result.violations.map((violation) => ({
			id: violation.id,
			impact: violation.impact,
			help: violation.help,
			helpUrl: violation.helpUrl,
			nodes: violation.nodes.map((node) => ({
				target: node.target,
				failureSummary: node.failureSummary
			}))
		})),
		incomplete: result.incomplete.map((item) => ({
			id: item.id,
			impact: item.impact,
			help: item.help,
			nodes: item.nodes.map((node) => ({ target: node.target }))
		}))
	};
}

async function screenshot(label) {
	const filename = `${label}.png`;
	const absolute = path.join(outputRoot, filename);
	await page.screenshot({ path: absolute, fullPage: false, animations: 'disabled' });
	const bytes = await readFile(absolute);
	receipt.screenshots.push({
		label,
		file: filename,
		sha256: sha256([bytes]),
		bytes: bytes.length,
		url: compactUrl(page.url()),
		viewport: page.viewportSize()
	});
}

async function captureRoute(viewport, route) {
	const label = `${viewport.name}-${route.name}`;
	await page.setViewportSize({ width: viewport.width, height: viewport.height });
	await page.goto(`${appOrigin}${route.path}`, { waitUntil: 'domcontentloaded', timeout: 15_000 });
	await waitForSettledScreen();
	if (route.name === 'library-title-1') {
		const evidence = await page.evaluate(() => {
			const main = document.querySelector('main');
			const heading = main?.querySelector('h1')?.textContent?.trim() || '';
			const text = main?.textContent || '';
			return {
				heading,
				notFound: text.includes('This title is not in your Library'),
				hasErrorAlert: !!main?.querySelector('[role="alert"]')
			};
		});
		receipt.fixtureChecks.push({
			label,
			passed: !!evidence.heading && !evidence.notFound && !evidence.hasErrorAlert,
			evidence
		});
	}

	if (await authBlocker()) {
		const blocker = {
			label,
			requested: route.path,
			landed: compactUrl(page.url()),
			viewport: { width: viewport.width, height: viewport.height }
		};
		receipt.authBlockers.push(blocker);
		await screenshot(`${viewport.name}-login-blocker`);
		return false;
	}

	const layout = await layoutAudit(label);
	layout.url = compactUrl(layout.url);
	const accessibility = await axeAudit(label);
	receipt.audits.push({ label, area: route.area, requested: route.path, layout, accessibility });
	await screenshot(label);
	return true;
}

(async () => {
	await mkdir(path.dirname(outputRoot), { recursive: true });
	await mkdir(outputRoot, { recursive: false });
	receipt.source.start = await sourceIdentity();
	receipt.source.captureScriptSha256 = sha256([await readFile(__filename)]);
	receipt.routePlan = routes.map(({ area, name, path: routePath }) => ({ area, name, path: routePath }));
	receipt.viewportPlan = selectedViewports;

	try {
		browser = await chromium.launch({
			headless: true,
			args: ['--renderer-process-limit=1', '--disable-background-networking']
		});
		context = await browser.newContext({
			reducedMotion: 'reduce',
			serviceWorkers: 'block',
			colorScheme: 'light',
			locale: 'en-US'
		});
		await context.route('**/*', async (route) => {
			const request = route.request();
			const method = request.method().toUpperCase();
			const requestUrl = request.url();
			if (method !== 'GET') {
				const event = { method, url: compactUrl(requestUrl), resourceType: request.resourceType() };
				if (isIntentionalBlockedNonGet(requestUrl)) {
					intentionalBlockedUrls.add(requestUrl);
					receipt.intentionalCaptureGaps.push({ ...event, reason: 'naming preview calculator requires a POST; blocked by the read-only capture policy' });
				} else receipt.nonGetAttempts.push(event);
				return route.abort('blockedbyclient');
			}
			if (isSensitiveApi(requestUrl)) {
				sensitiveBlockedUrls.add(requestUrl);
				receipt.sensitiveRequestsBlocked.push({
					method,
					endpoint: compactUrl(requestUrl),
					resourceType: request.resourceType()
				});
				return route.abort('blockedbyclient');
			}
			let origin;
			try {
				origin = new URL(requestUrl).origin;
			} catch {
				receipt.blockedOrigins.push({ url: String(requestUrl), reason: 'invalid URL' });
				return route.abort('blockedbyclient');
			}
			if (!permittedGetOrigins.has(origin)) {
				receipt.blockedOrigins.push({ url: compactUrl(requestUrl), reason: 'origin outside explicit GET allowlist' });
				return route.abort('blockedbyclient');
			}
			return route.continue();
		});

		page = await context.newPage();
		page.setDefaultTimeout(10_000);
		page.on('console', (message) => {
			const event = {
				type: message.type(),
				text: sanitizeConsoleText(message.text()),
				location: {
					url: compactUrl(message.location().url || ''),
					lineNumber: message.location().lineNumber,
					columnNumber: message.location().columnNumber
				}
			};
			if (isIntentionalBlockedNonGet(message.location().url || '')) receipt.intentionalConsoleEvents.push(event);
			else if (isSensitiveApi(message.location().url || '')) receipt.sensitiveConsoleEvents.push(event);
			else receipt.console.push(event);
		});
		page.on('pageerror', (error) => receipt.pageErrors.push({ message: sanitizeConsoleText(error.message) }));
		page.on('requestfailed', (request) => {
			if (sensitiveBlockedUrls.has(request.url()) || intentionalBlockedUrls.has(request.url())) return;
			receipt.requestFailures.push({
				method: request.method(),
				url: compactUrl(request.url()),
				resourceType: request.resourceType(),
				error: request.failure()?.errorText || 'unknown'
			});
		});
		page.on('response', (response) => {
			if (response.status() >= 400) {
				receipt.httpErrors.push({ status: response.status(), url: compactUrl(response.url()) });
			}
		});

		let blocked = false;
		for (const viewport of selectedViewports) {
			for (const route of routes) {
				if (!(await captureRoute(viewport, route))) {
					blocked = true;
					break;
				}
			}
			if (blocked) break;
		}
	} catch (error) {
		receipt.runErrors.push(error instanceof Error ? error.stack || error.message : String(error));
	} finally {
		await context?.close().catch(() => {});
		await browser?.close().catch(() => {});
		clearTimeout(deadline);
		receipt.source.end = await sourceIdentity().catch((error) => ({ error: String(error) }));
		receipt.source.changedDuringCapture =
			receipt.source.start?.commit !== receipt.source.end?.commit ||
			receipt.source.start?.dirtySha256 !== receipt.source.end?.dirtySha256 ||
			receipt.source.start?.appTreeSha256 !== receipt.source.end?.appTreeSha256;
		receipt.deadlineReached = deadlineReached;
		receipt.finishedUtc = new Date().toISOString();

		const violations = receipt.audits.reduce(
			(total, audit) => total + audit.accessibility.violations.length,
			0
		);
		const overflow = receipt.audits.filter((audit) => audit.layout.hasHorizontalOverflow).map((audit) => audit.label);
		const consoleErrors = receipt.console.filter((message) => message.type === 'error');
		const fixtureFailures = receipt.fixtureChecks.filter((item) => !item.passed);
		receipt.summary = {
			expectedScreenshots: selectedViewports.length * routes.length,
			capturedScreenshots: receipt.screenshots.length,
			audits: receipt.audits.length,
			axeViolations: violations,
			horizontalOverflow: overflow,
			consoleErrors: consoleErrors.length,
			pageErrors: receipt.pageErrors.length,
			httpErrors: receipt.httpErrors.length,
			requestFailures: receipt.requestFailures.length,
			blockedOrigins: receipt.blockedOrigins.length,
			sensitiveRequestsBlocked: receipt.sensitiveRequestsBlocked.length,
			sensitiveConsoleEvents: receipt.sensitiveConsoleEvents.length,
			intentionalCaptureGaps: receipt.intentionalCaptureGaps.length,
			intentionalConsoleEvents: receipt.intentionalConsoleEvents.length,
			fixtureChecks: receipt.fixtureChecks.length,
			fixtureFailures: fixtureFailures.length,
			nonGetAttempts: receipt.nonGetAttempts.length,
			authBlocked: receipt.authBlockers.length > 0,
			sourceChangedDuringCapture: !!receipt.source.changedDuringCapture
		};
		receipt.status =
			receipt.authBlockers.length > 0
				? 'blocked-authentication'
				: receipt.runErrors.length || deadlineReached || receipt.source.changedDuringCapture ||
				  receipt.screenshots.length !== selectedViewports.length * routes.length ||
				  violations || overflow.length || consoleErrors.length || receipt.pageErrors.length ||
				  receipt.httpErrors.length || receipt.requestFailures.length || receipt.blockedOrigins.length ||
				  receipt.nonGetAttempts.length || fixtureFailures.length
					? 'failed'
					: 'passed';
		receipt.cleanup = 'The isolated browser context and browser were closed. The existing preview processes were not changed.';
		await writeFile(path.join(outputRoot, 'receipt.json'), `${JSON.stringify(receipt, null, 2)}\n`);
		process.stdout.write(`${JSON.stringify({ status: receipt.status, ...receipt.summary, outputRoot })}\n`);
		if (receipt.status !== 'passed') process.exitCode = 1;
	}
})();
