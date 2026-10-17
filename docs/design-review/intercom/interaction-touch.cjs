const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { chromium, expect } = require('../../../web/app/node_modules/@playwright/test');

const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const evidenceRoot = path.resolve(root, '.omx/artifacts/visual-ralph/intercom/interaction-pass');
const out = path.resolve(evidenceRoot, 'touch');
const widths = [390, 834];
const artworkHosts = new Set(['image.tmdb.org', 'static.tvmaze.com', 'is1-ssl.mzstatic.com']);

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

async function screenshot(page, file) {
	const sensitive = page.locator('input[type="password"], [data-sensitive], .secret, .secret-value, .fresh code');
	await page.screenshot({ path: file, mask: [sensitive], fullPage: false });
}

const owner = {
	id: 1,
	name: 'Preview owner',
	via: 'session',
	csrf: 'touch-fixture',
	role: 'owner',
	perms: ['ask', 'approve', 'manage_library', 'settings']
};

const libraryChooser = {
	name: 'Library chooser opens and selects Movies by touch',
	route: '/library?kind=show',
	run: async ({ page, width, step }) => {
		await step('tap the responsive Library navigation control', async () => {
			const trigger = width >= 768
				? page.locator('aside.global-rail').getByRole('button', { name: 'Library', exact: true })
				: page.getByRole('button', { name: 'Library', exact: true }).last();
			await trigger.tap();
			const chooser = page.locator('dialog.navigation-dialog');
			await expect(chooser).toBeVisible();
			await chooser.getByRole('link', { name: 'Movies', exact: true }).tap();
			await expect.poll(() => new URL(page.url()).searchParams.get('kind')).toBe('movie');
			assert.equal(new URL(page.url()).pathname, '/library');
		});
	}
};

const wishlistReview = {
	name: 'Wishlist review opens and restores focus by touch',
	route: '/wishlist',
	setup: ({ intercept }) => {
		const ask = {
			id: 71,
			kind: 'movie',
			tmdb_id: '9071',
			name: 'Touch Review Movie',
			year: 2026,
			state: 'asked',
			state_label: 'Waiting',
			asked_by: { id: 22, name: 'Preview member' },
			asked_at: '2026-10-17T16:00:00Z',
			people: [{ id: 22, name: 'Preview member', asked_at: '2026-10-17T16:00:00Z' }],
			auto_approved: false,
			mine: false
		};
		intercept(({ method, path }) => {
			if (method === 'GET' && path === '/asks') return { json: { asks: [ask] } };
			if (method === 'GET' && path === '/messages') return { json: { unread: 0, waiting: 1, messages: [] } };
			if (method === 'GET' && path === '/library-folders') return { json: { library_folders: [] } };
			if (method === 'GET' && path === '/pick-rules') return { json: { pick_rules: [] } };
			return null;
		});
	},
	run: async ({ page, width, step }) => {
		await step('tap Review and verify its responsive focus behavior', async () => {
			const review = page.getByTestId('ask').filter({ hasText: 'Touch Review Movie' }).getByRole('button', { name: 'Review Touch Review Movie' });
			await review.tap();
			const heading = page.locator('#wishlist-review').getByRole('heading', { name: /Touch Review Movie/ });
			await expect.poll(() => heading.evaluate((node) => document.activeElement === node)).toBe(true);
			if (width <= 600) {
				const close = page.getByRole('button', { name: 'Close review' });
				await expect(close).toBeVisible();
				await close.tap();
				await expect(close).toBeHidden();
				await expect.poll(() => review.evaluate((node) => document.activeElement === node)).toBe(true);
			} else {
				await expect(review).toHaveAttribute('aria-pressed', 'true');
			}
		});
	}
};

const musicSettings = {
	name: 'Music settings retain touch edits after rejected save',
	route: '/settings/music',
	setup: ({ intercept }) => {
		intercept(({ method, path }) => {
			if (method === 'GET' && path === '/settings/music') return { json: {
				album_choice: 'all',
				new_albums: 'all',
				album_types: ['album', 'ep'],
				secondary_types: ['studio'],
				release_statuses: ['official'],
				pressing_rule: 'common',
				musicbrainz_url: '',
				cache_hours: 24
			} };
			if (method === 'PUT' && path === '/settings/music') return { delay: 180, status: 503, json: { error: 'Preview touch save rejected' } };
			return null;
		});
	},
	run: async ({ page, step }) => {
		await step('edit bound controls with touch and expose save recovery', async () => {
			const address = page.getByLabel(/MusicBrainz address/);
			const live = page.getByRole('checkbox', { name: 'Live', exact: true });
			const pressing = page.getByLabel('Which Pressing an album follows');
			const save = page.getByRole('button', { name: 'Save', exact: true });
			await address.fill('https://music.example.test');
			await live.tap();
			await pressing.selectOption('earliest');
			assert.equal(await address.inputValue(), 'https://music.example.test');
			assert.equal(await live.isChecked(), true);
			assert.equal(await pressing.inputValue(), 'earliest');
			await save.tap();
			await expect(save).toBeDisabled();
			await page.getByText('Preview touch save rejected').waitFor();
			await expect(save).toBeEnabled();
			assert.equal(await address.inputValue(), 'https://music.example.test');
			assert.equal(await live.isChecked(), true);
			assert.equal(await pressing.inputValue(), 'earliest');
		});
	}
};

const backupPreset = {
	name: 'Backup preset exposes pending state and rolls back by touch',
	route: '/settings/backups',
	setup: ({ intercept }) => {
		intercept(({ method, path }) => {
			if (method === 'GET' && path === '/backups') return { json: { backups: [], preset: 'balanced', restore_waiting: false, key_note: '' } };
			if (method === 'PUT' && path === '/backups/settings') return { delay: 180, status: 503, json: { error: 'Preview preset save rejected' } };
			return null;
		});
	},
	run: async ({ page, step }) => {
		await step('tap a preset, observe pending controls, then verify rollback', async () => {
			const balanced = page.getByRole('radio', { name: /Balanced/ });
			const light = page.getByRole('radio', { name: /Light/ });
			await expect(balanced).toBeChecked();
			await light.tap();
			await expect(light).toBeChecked();
			await expect(balanced).toBeDisabled();
			await expect(light).toBeDisabled();
			await page.getByText('Preview preset save rejected').waitFor();
			await expect(balanced).toBeChecked();
			await expect(light).not.toBeChecked();
			await expect(light).toBeEnabled();
		});
	}
};

const scenarios = [libraryChooser, wishlistReview, musicSettings, backupPreset];

(async () => {
	const outputRelative = path.relative(evidenceRoot, out);
	assert.ok(outputRelative && !outputRelative.startsWith('..') && !path.isAbsolute(outputRelative), 'Touch output must stay inside the interaction evidence directory');
	fs.rmSync(out, { recursive: true, force: true });
	fs.mkdirSync(out, { recursive: true });
	const before = identity();
	const checks = [];
	fs.writeFileSync(path.join(out, 'manifest.json'), JSON.stringify({
		before,
		widths,
		scenarios: scenarios.map(({ name, route }) => ({ name, route })),
		writes: 'Every non-GET request is intercepted; no backend mutation is permitted.',
		input: 'Navigation, buttons, checkboxes and radios use Playwright touch input; text and select values use standard form automation.'
	}, null, 2));
	const browser = await chromium.launch({ headless: true });
	try {
		for (const width of widths) for (const scenario of scenarios) {
			const context = await browser.newContext({
				viewport: { width, height: 900 },
				isMobile: width < 600,
				hasTouch: true,
				serviceWorkers: 'block',
				reducedMotion: 'no-preference'
			});
			await context.addInitScript(() => localStorage.setItem('holyarrs-theme', 'light'));
			if (typeof context.routeWebSocket === 'function') await context.routeWebSocket('**/*', (socket) => socket.close());
			const page = await context.newPage();
			page.setDefaultTimeout(7000);
			const handlers = [];
			const attempts = [];
			const fixtures = [];
			const steps = [];
			const errors = [];
			page.on('pageerror', (error) => errors.push(error.message));
			await context.route('**/*', async (route) => {
				const request = route.request();
				const url = new URL(request.url());
				const method = request.method();
				const own = url.origin === origin;
				const api = own && url.pathname.startsWith('/api/v1/');
				const apiPath = api ? url.pathname.slice('/api/v1'.length) : url.pathname;
				if (method !== 'GET') attempts.push({ method, path: apiPath, intercepted: true });
				if (api) {
					let body;
					try { body = request.postDataJSON(); } catch { body = undefined; }
					for (const handler of [...handlers].reverse()) {
						const response = await handler({ request, url, method, path: apiPath, body });
						if (response == null) continue;
						fixtures.push({ method, path: apiPath, status: response.status || 200 });
						if (response.delay) await new Promise((resolve) => setTimeout(resolve, response.delay));
						await route.fulfill({ status: response.status || 200, json: response.json ?? {} });
						return;
					}
				}
				if (method !== 'GET') {
					if (api) return route.fulfill({ status: 503, json: { error: 'Preview touch check: service unavailable. Try again.' } });
					return route.abort('blockedbyclient');
				}
				if (!own && (request.resourceType() !== 'image' || url.protocol !== 'https:' || !artworkHosts.has(url.hostname))) return route.abort('blockedbyclient');
				return route.continue();
			});
			handlers.push(({ method, path: apiPath }) => method === 'GET' && apiPath === '/auth/me' ? { json: owner } : null);
			const ctx = {
				page,
				width,
				intercept(handler) { handlers.push(handler); },
				async step(name, action) { await action(); steps.push(name); }
			};
			let failure;
			const filename = `${width}-${scenario.name.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}.png`;
			try {
				await scenario.setup?.(ctx);
				const response = await page.goto(origin + scenario.route, { waitUntil: 'networkidle' });
				assert.ok(response?.ok(), `Page returned ${response?.status()}`);
				await page.locator('body').waitFor();
				await page.evaluate(() => document.fonts.ready);
				await scenario.run(ctx);
				await page.waitForTimeout(180);
				assert.equal(new URL(page.url()).origin, origin, 'Navigation escaped preview');
				assert.deepEqual(errors, [], 'Browser runtime errors');
				assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, 'Touch interaction caused horizontal overflow');
				await screenshot(page, path.join(out, filename));
			} catch (error) {
				failure = error.message;
				try { await screenshot(page, path.join(out, filename)); } catch {}
			} finally {
				checks.push({ name: scenario.name, route: scenario.route, width, steps, attempts, fixtures, errors, screenshot: filename, ...(failure ? { failure } : {}) });
				await context.close();
				fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after: identity() }, null, 2));
				console.log(`${width} ${scenario.name}: ${failure ? `FAIL ${failure.split('\n')[0]}` : 'pass'}`);
			}
		}
	} finally {
		await browser.close();
	}
	const after = identity();
	assert.equal(checks.length, widths.length * scenarios.length, 'Touch interaction coverage is incomplete');
	fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after }, null, 2));
	if (checks.some((check) => check.failure) || before.sourceHash !== after.sourceHash) process.exitCode = 1;
})().catch((error) => {
	console.error(error.message);
	process.exitCode = 1;
});
