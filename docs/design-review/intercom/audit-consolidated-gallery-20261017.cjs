// GET-only verification of the local evidence pages, separate from product acceptance.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium, expect } = require('../../../web/app/node_modules/@playwright/test');
const { identity } = require('./audit-environments-20261017.cjs');
const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const output = path.join(root, '.omx/artifacts/visual-ralph/intercom/audit20261017/gallery');
const frozenHash = '45144ef19d962c8cd785e124d352c7f00f42821ea359558b919415073e6f2928';
(async () => {
  fs.mkdirSync(output, { recursive: true });
  const before = identity(), checks = [];
  fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify({ passed:false, status:'pending', before, checks }));
  assert.equal(before.sourceHash, frozenHash, 'Final smoke requires the approved frozen frontend');
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of [360, 834, 1440]) for (const file of ['index.html', 'before-after.html']) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block' });
      await context.route('**/*', r => r.request().method() === 'GET' && new URL(r.request().url()).origin === origin ? r.continue() : r.abort());
      const page = await context.newPage(), errors = [];
      page.on('pageerror', e => errors.push(e.message));
      try {
        const response = await page.goto(`${origin}/preview-audit/consolidated-20261017/${file}`, { waitUntil: 'networkidle' });
        assert.equal(response.status(), 200);
        await expect(page.locator('h1')).toBeVisible();
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        if (file === 'index.html') {
          await page.getByLabel('Group', { exact: true }).selectOption('Comparison');
          await expect(page.locator('article:visible')).toHaveCount(8);
          await page.getByLabel('Find', { exact: true }).fill('/wishlist');
          await expect(page.locator('article:visible')).toHaveCount(2);
          await page.getByLabel('CSS width', { exact: true }).selectOption('1920');
          await expect(page.locator('article:visible')).toHaveCount(2);
          await page.getByLabel('Find', { exact: true }).fill('no-matching-audit-route');
          await expect(page.locator('article:visible')).toHaveCount(0);
          await page.getByLabel('Find', { exact: true }).fill('');
        } else {
          await expect(page.locator('section')).toHaveCount(4);
          await expect(page.locator('figure')).toHaveCount(8);
        }
        await page.locator('article:visible img, figure img').evaluateAll(images => images.forEach(image => image.loading = 'eager'));
        await expect.poll(() => page.locator('article:visible img, figure img').evaluateAll(images => images.length > 0 && images.every(image => image.complete && image.naturalWidth > 0))).toBe(true);
        assert.deepEqual(errors, []);
        const screenshot = `${width}-${file}.png`;
        await page.screenshot({ path: path.join(output, screenshot) });
        checks.push({ width, file, passed: true, screenshot });
      } finally { await context.close(); }
    }
    for (const width of [390, 1440]) for (const route of ['/', '/library', '/wishlist', '/downloads']) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block' });
      await context.route('**/*', r => r.request().method() === 'GET' && new URL(r.request().url()).origin === origin ? r.continue() : r.abort());
      const page = await context.newPage(), errors = [];
      page.on('pageerror', e => errors.push(e.message));
      try {
        const response = await page.goto(origin + route, { waitUntil: 'networkidle' });
        assert.equal(response.status(), 200);
        assert.equal(new URL(page.url()).pathname, route);
        await expect(page.locator('main h1').first()).toBeVisible();
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        assert.deepEqual(errors, []);
        const screenshot = `${width}-product-${route.replaceAll('/', '_')}.png`;
        await page.screenshot({ path: path.join(output, screenshot) });
        checks.push({ width, route, passed: true, screenshot, scope: 'final-checkout-smoke-only' });
      } finally { await context.close(); }
    }
  } finally { await browser.close(); }
  const after = identity(); assert.equal(before.sourceHash, after.sourceHash); assert.equal(after.sourceHash, frozenHash); assert.equal(checks.length, 14);
  fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify({ passed:true, status:'verified', before, after, checks, finishedAt:new Date().toISOString() }, null, 2));
  console.log(`Evidence gallery: ${checks.length} checks passed`);
})().catch(e => { fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify({ passed:false, status:'failed', error:e.message, finishedAt:new Date().toISOString() }, null, 2)); console.error(e); process.exitCode = 1; });
