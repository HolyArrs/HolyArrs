const { chromium } = require('../../../web/app/node_modules/playwright');
const fs = require('node:fs');
const path = require('node:path');
const out = path.resolve(__dirname, '../../../.omx/artifacts/visual-ralph/intercom/full-audit/loading');
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const checks = [];
  try {
    for (const width of [390, 834, 1440]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block', reducedMotion: 'reduce' });
      await context.route('**/*', (route) => route.request().method() === 'GET' ? route.continue() : route.abort());
      let resume;
      const gate = new Promise((resolve) => { resume = resolve; });
      await context.route('**/api/v1/titles', async (route) => { await gate; await route.continue(); });
      const page = await context.newPage();
      await page.goto('http://127.0.0.1:18746/library?kind=movie', { waitUntil: 'domcontentloaded' });
      await page.locator('.library-content .sk').first().waitFor({ state: 'visible' });
      await page.screenshot({ path: path.join(out, width + '-library-loading.png') });
      resume();
      await page.getByTestId('library-card').first().waitFor({ state: 'visible' });
      checks.push({ width, loadingSkeleton: true, realResponseRecovery: true });
      await context.close();
    }
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify(checks, null, 2));
  }
  console.log(checks);
})().catch((error) => { console.error(error); process.exitCode = 1; });
