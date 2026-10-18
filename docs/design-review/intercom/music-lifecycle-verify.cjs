// Read-only final-checkout verification and local screenshot gallery.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium, expect } = require('../../../web/app/node_modules/@playwright/test');
const { identity } = require('./audit-environments-20261017.cjs');
const root = path.resolve(__dirname, '../../..');
const runId = '6d60338b-2a0f-4b57-859b-460851b88fb5';
const evidence = path.join(root, '.omx/artifacts/visual-ralph/intercom/real-save', runId);
const out = path.join(root, 'web/app/static/preview-audit/music-lifecycle-20261017');
const base = 'http://10.10.25.25:18746';
const escape = value => String(value).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;');

(async () => {
	fs.writeFileSync(path.join(evidence, 'final-checkout.json'), JSON.stringify({ passed: false, status: 'pending' }));
	const before = identity(), lifecycle = JSON.parse(fs.readFileSync(path.join(evidence, 'receipt.json')));
	assert.equal(lifecycle.passed, true);
	assert.equal(lifecycle.cleanup.verified, true);
	assert.equal(lifecycle.cleanup.originalRowsPreserved, true);
	assert.equal(lifecycle.cleanup.originalMediaPreserved, true);
	assert.equal(lifecycle.sourceAfter.frontend, before.sourceHash);
	const request = lifecycle.requests[0];
	assert.equal(request.reference, 'mbid:a4444444-0000-4000-8000-000000000004');
	fs.mkdirSync(out, { recursive: true });
	const checks = [], screenshots = [];
	const browser = await chromium.launch({ headless: true });
	try {
		for (const width of [360, 834, 1440]) {
			const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block' });
			await context.route('**/*', route => route.request().method() === 'GET' && new URL(route.request().url()).origin === base ? route.continue() : route.abort());
			const page = await context.newPage(), errors = [];
			page.on('pageerror', error => errors.push(error.message));
			await page.goto(`${base}/library?kind=artist`, { waitUntil: 'networkidle' });
			await page.getByRole('searchbox', { name: 'Find in the Library' }).fill('Amber Satellites');
			for (const [view, testId] of [['Posters', 'library-card'], ['List', 'library-row']]) {
				await page.getByRole('button', { name: view, exact: true }).click();
				const item = page.getByTestId(testId).filter({ hasText: 'Amber Satellites' });
				await expect(item.getByTestId('request-approved')).toHaveText('Approved');
				await expect(item).toContainText('3 missing');
				await expect(item.getByTestId('requested-by')).toContainText(request.requesterNames[0]);
				await page.reload({ waitUntil: 'networkidle' });
				await expect(item).toBeVisible();
				assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
				assert.deepEqual(errors, []);
				const file = `${width}-${view.toLowerCase()}.png`;
				await page.screenshot({ path: path.join(out, file), animations: 'disabled' });
				screenshots.push({ file, label: `${width}px ${view}: Approved, missing tracks and requester after refresh` });
				checks.push({ width, view, passed: true });
			}
			await context.close();
		}
	} finally { await browser.close(); }
	for (const image of lifecycle.screenshots.filter(image => ['music-album-asked', 'music-album-approved', 'Amber Satellites-album-scope'].includes(image.label))) {
		fs.copyFileSync(path.join(evidence, image.file), path.join(out, image.file));
		screenshots.push({ file: image.file, label: image.label });
	}
	fs.writeFileSync(path.join(out, 'index.html'), `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Music approval to Library</title><style>body{font:16px/1.5 system-ui;margin:24px;background:#f4f5f1;color:#20221f}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr));gap:20px}figure{margin:0;padding:16px;background:white;border:1px solid #ddd;border-radius:12px}img{width:100%;height:440px;object-fit:contain;object-position:top}p,figcaption{overflow-wrap:anywhere}</style><h1>Music request → approval → Library</h1><p>Preview only. Amber Satellites / Evening Signals. The other album remains off. Real saved request ${request.id} maps to Library title ${request.titleId}. Four guarded writes; original samples, credentials and media preserved. One initial count-readiness failure was retained before the successful continuation. Synthetic metadata, no download or external music-provider certification.</p><p>Final checkout ${escape(before.commit)}; frontend ${escape(before.sourceHash)}.</p><main>${screenshots.map(image => `<figure><figcaption>${escape(image.label)}</figcaption><a href="${escape(image.file)}"><img src="${escape(image.file)}" alt="${escape(image.label)}" loading="lazy"></a></figure>`).join('')}</main></html>`);
	const after = identity();
	assert.deepEqual(after, before);
	assert.equal(checks.length, 6);
	fs.writeFileSync(path.join(evidence, 'final-checkout.json'), JSON.stringify({ passed: true, before, after, checks, screenshots, capturedAt: new Date().toISOString() }, null, 2));
	console.log(`Music final checkout: ${checks.length} checks passed; title ${request.titleId}`);
})().catch(error => {
	fs.writeFileSync(path.join(evidence, 'final-checkout.json'), JSON.stringify({ passed: false, status: 'failed', error: error.message }));
	console.error(error.message); process.exitCode = 1;
});
