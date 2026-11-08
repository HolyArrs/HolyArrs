const { chromium } = require('../../../web/app/node_modules/playwright');
const fs = require('node:fs');
const path = require('node:path');
const out = path.resolve(__dirname, '../../../.omx/artifacts/visual-ralph/intercom/full-audit/repair-interactions');
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const checks = [];
  try {
    for (const width of [360, 390, 834, 1440]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block', reducedMotion: 'reduce' });
      await context.route('**/*', (route) => route.request().method() === 'GET' ? route.continue() : route.abort());
      const page = await context.newPage();
      await page.goto('http://127.0.0.1:18746/requests', { waitUntil: 'networkidle' });
      if (width < 600) {
        const review = page.getByRole('button', { name: /^Review / }).first();
        await review.focus();
        await page.keyboard.press('Enter');
        await page.locator('#wishlist-review').waitFor({ state: 'visible' });
        if (await review.getAttribute('aria-expanded') !== 'true') throw Error('Review disclosure state');
        const close = page.getByRole('button', { name: 'Close review', exact: true });
        await close.focus();
        await page.keyboard.press('Enter');
        if (!await review.evaluate((element) => element === document.activeElement)) throw Error('Review focus return');
        const box = await review.boundingBox();
        if (!box || box.y < 0 || box.y > 900) throw Error('Review focus offscreen');
        checks.push({ width, keyboardReviewDisclosure: true, focusReturnedOnscreen: true });
        await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight));
        await page.screenshot({ path: path.join(out, width + '-requests-end.png') });
      }
      for (const id of [76, 141, 1]) {
        await page.goto('http://127.0.0.1:18746/library/' + id, { waitUntil: 'networkidle' });
        const more = page.getByRole('button', { name: 'Read more', exact: true });
        if (width <= 600 && await more.isVisible()) {
          await more.focus();
          await page.keyboard.press('Enter');
          const less = page.getByRole('button', { name: 'Show less', exact: true });
          if (await less.getAttribute('aria-expanded') !== 'true') throw Error('Overview disclosure');
          await page.keyboard.press('Enter');
          checks.push({ width, id, keyboardOverviewDisclosure: true });
        }
        await page.screenshot({ path: path.join(out, width + '-detail-' + id + '.png') });
      }
      await page.goto('http://127.0.0.1:18746/downloads/history?from=2026-10-17&to=2026-10-01', { waitUntil: 'networkidle' });
      await page.locator('#history-date-error').waitFor();
      if (await page.locator('#h-from').getAttribute('aria-invalid') !== 'true') throw Error('Invalid range not associated');
      await page.screenshot({ path: path.join(out, width + '-history-invalid.png') });
      await page.locator('#h-to').fill('2026-10-31');
      await page.locator('#h-to').blur();
      await page.locator('#history-date-error').waitFor({ state: 'hidden' });
      checks.push({ width, historyValidationRecovery: true });
      await context.close();
    }
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify(checks, null, 2));
  }
  console.log(checks);
})().catch((error) => { console.error(error); process.exitCode = 1; });
