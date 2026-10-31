// Actual preview reads and non-saving interactions only; isolated browser contexts.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const repo = path.resolve(__dirname, '../../..');
const appRequire = createRequire(path.join(repo, 'web/app/package.json'));
const { chromium, expect } = appRequire('@playwright/test');
const before = process.argv.includes('--before');
const run = `${before ? 'before' : 'after'}-${crypto.randomUUID()}`;
const output = path.join(repo, '.omx/artifacts/visual-ralph/intercom/mobile-activity-fit', run);
const origin = 'http://10.10.25.25:18746';
const receipt = { run, before, source: {}, checks: [], screenshots: [], pageErrors: [], blocked: [] };
let browser;
function source() {
  const hash = crypto.createHash('sha256');
  function walk(dir) {
    for (const name of fs.readdirSync(dir).sort()) {
      const file = path.join(dir, name);
      if (fs.statSync(file).isDirectory()) walk(file);
      else hash.update(path.relative(repo, file)).update(fs.readFileSync(file));
    }
  }
  walk(path.join(repo, 'web/app/src'));
  return { commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8', windowsHide: true }).trim(), frontend: hash.digest('hex') };
}
function check(name, passed, evidence = {}) {
  receipt.checks.push({ name, passed, evidence });
  if (!passed && !before) throw new Error(name);
}
async function shot(page, name, locator) {
  const file = `${name}.png`;
  if (locator) await locator.screenshot({ path: path.join(output, file), animations: 'disabled' });
  else await page.screenshot({ path: path.join(output, file), animations: 'disabled' });
  receipt.screenshots.push(file);
}
async function fit(page, selector, label, width) {
  const result = await page.locator(selector).evaluateAll((rows) => rows.map((row, index) => {
    const outer = row.getBoundingClientRect();
    const controls = [...row.querySelectorAll('.chip, button, .actions .btn')].filter((el) => el.getClientRects().length).map((el) => {
      const r = el.getBoundingClientRect();
      return { text: el.textContent.trim(), fits: r.left >= Math.max(0, outer.left) - 1 && r.right <= Math.min(innerWidth, outer.right) + 1 && el.scrollWidth <= el.clientWidth + 1 && el.scrollHeight <= el.clientHeight + 1 && (el.tagName !== 'BUTTON' || r.height <= 64), height: r.height };
    });
    return { index, fits: row.scrollWidth <= row.clientWidth + 1 && controls.every((c) => c.fits), controls };
  }));
  check(`${width}/${label}: all statuses and buttons fit`, result.length > 0 && result.every((r) => r.fits), { rows: result.length, failed: result.filter((r) => !r.fits) });
  const doc = await page.evaluate(() => ({ width: document.documentElement.clientWidth, content: document.documentElement.scrollWidth }));
  check(`${width}/${label}: no document sideways scrolling`, doc.content <= doc.width + 1, doc);
}
(async () => {
  fs.mkdirSync(output, { recursive: true });
  receipt.source = source();
  browser = await chromium.launch({ headless: true });
  for (const width of before ? [320, 360] : [320, 360, 390, 430, 600, 620, 621, 834, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: 1000 }, serviceWorkers: 'block', reducedMotion: 'reduce', hasTouch: width <= 620 });
    await context.route('**/*', (route) => {
      const request = route.request(), url = new URL(request.url());
      const image = request.resourceType() === 'image' && request.method() === 'GET' && ((url.protocol === 'https:' && /^(image\.tmdb\.org|static\.tvmaze\.com|is[1-5]-ssl\.mzstatic\.com)$/.test(url.hostname)) || url.origin === 'http://127.0.0.1:28764');
      if ((['GET', 'HEAD'].includes(request.method()) && url.origin === origin) || image) return route.continue();
      receipt.blocked.push({ method: request.method(), path: url.pathname }); return route.abort();
    });
    const page = await context.newPage();
    page.on('pageerror', (error) => receipt.pageErrors.push(error.message));
    await page.goto(`${origin}/requests`);
    await expect(page.getByTestId('ask').first()).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await fit(page, '.ask', 'requests-waiting', width);
    await page.getByTestId('ask').first().scrollIntoViewIfNeeded();
    await shot(page, `${width}-requests-waiting`);
    if (!before && width === 360) {
      await page.getByTestId('ask').first().getByRole('button', { name: /^Review / }).click();
      await expect(page.getByRole('button', { name: 'Close review' })).toBeVisible();
      await page.getByRole('button', { name: 'Close review' }).click();
      const approve = page.getByTestId('ask').first().getByRole('button', { name: 'Approve', exact: true });
      await approve.click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await shot(page, '360-approve-dialog');
      await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).click();
      check('Requests review and approval dialog open/cancel without saving', true);
    }
    await page.getByRole('button', { name: /^Everyone's/ }).click();
    await fit(page, '.ask', 'requests-all', width);
    await shot(page, `${width}-requests-all-row`, page.getByTestId('ask').first());
    if (width === 320) {
      await page.getByTestId('ask').first().evaluate((row) => {
        row.querySelector('.h3').textContent = 'AnExtremelyLongUnbrokenTitleWithASpecialExtendedEdition';
        row.querySelector('[data-testid="ask-state"]').textContent = 'Waiting for a decision from the household owner';
      });
      await fit(page, '.ask', 'requests-browser-only-long-text', width);
      await shot(page, '320-requests-browser-only-long-text', page.getByTestId('ask').first());
    }
    await page.goto(`${origin}/downloads`);
    await expect(page.locator('.dl').first()).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    if (!before && width === 360) {
      const summaries = page.locator('.eps > summary');
      const count = await summaries.count();
      for (let index = 0; index < count; index++) await summaries.nth(index).click();
      check('Download episode/track disclosures open without saving', count > 0 && await page.locator('.eps[open]').count() === count, { count });
      await shot(page, '360-download-expanded-episodes', page.locator('.dl').filter({ has: page.locator('.eps[open]') }).first());
    }
    await fit(page, '.dl, .imports li', 'downloads', width);
    await page.locator('.dl').first().scrollIntoViewIfNeeded();
    await shot(page, `${width}-downloads-row`, page.locator('.dl').first());
    if (width === 320) {
      await page.locator('.dl').first().evaluate((row) => {
        row.querySelector('.nm').textContent = 'AnExtremelyLongUnbrokenEpisodeOrAlbumTitle';
        row.querySelector('[data-testid="row-state"]').textContent = 'Waiting for a decision from the household owner';
      });
      await fit(page, '.dl, .imports li', 'downloads-browser-only-long-text', width);
      await shot(page, '320-downloads-browser-only-long-text', page.locator('.dl').first());
    }
    if (!before && width === 360) {
      await page.locator('.dl').first().getByRole('button', { name: /^Details for/ }).click();
      await expect(page.getByRole('complementary', { name: 'Selected download details' })).toBeVisible();
      const remove = page.locator('.dl .acts').first().getByRole('button', { name: 'Remove', exact: true });
      await remove.click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await shot(page, '360-remove-dialog');
      await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).click();
      check('Download details and removal dialog open/cancel without saving', true);
    }
    await context.close();
  }
  check('No runtime errors or out-of-scope/write attempts', !receipt.pageErrors.length && !receipt.blocked.length);
  check('Frontend stayed frozen', source().frontend === receipt.source.frontend);
})().catch((error) => { receipt.error = error.message; process.exitCode = 1; }).finally(async () => {
  await browser?.close();
  fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
  console.log(JSON.stringify({ run, checks: receipt.checks.length, failed: receipt.checks.filter((c) => !c.passed).length, screenshots: receipt.screenshots.length, error: receipt.error }));
});
