// Public reference inspection only; never sign in or submit data.
const { chromium } = require('../../../web/app/node_modules/playwright');
const fs = require('node:fs');
const path = require('node:path');
const out = path.resolve(__dirname, '../../../.omx/artifacts/visual-ralph/intercom/audit20261017/flows');
const ids = ['27b1d962-dee6-4cbb-ba51-f3d667477e9d', '6211d06a-abe1-4112-92c9-a37a676704f4', '670d481a-5adb-4c2e-ae5d-9f48210d3c4f'];
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const checks = [];
  try {
    for (const id of ids) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1, serviceWorkers: 'block' });
      await context.route('**/*', route => route.request().method() === 'GET' ? route.continue() : route.abort());
      const page = await context.newPage();
      const row = { id, url: `https://mobbin.com/flows/${id}`, capturedAt: new Date().toISOString(), viewport: { width: 1440, height: 1000 }, dpr: 1 };
      try {
        const response = await page.goto(row.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
        await page.waitForTimeout(1800);
        row.status = response?.status();
        Object.assign(row, await page.evaluate(() => ({
          landed: location.href, title: document.title,
          tabs: [...document.querySelectorAll('[role=tab]')].map(e => e.textContent?.trim()),
          controls: [...document.querySelectorAll('button')].map(e => e.textContent?.trim()).filter(Boolean).slice(0, 60),
          media: [...document.querySelectorAll('video,audio')].map(e => ({ tag: e.tagName, duration: Number.isFinite(e.duration) ? e.duration : null, readyState: e.readyState })),
          loginForm: !!document.querySelector('input[type=password]'),
          text: document.body.innerText.slice(0, 2600)
        })));
        await page.screenshot({ path: path.join(out, `${id}.png`), mask: [page.locator('input')] });
        row.timedMotionVerified = false;
        row.limit = 'Public rendered page plus local ordered flow stills; no authenticated computed product styles or timed source recording measured.';
      } catch (e) { row.unavailable = e.message.split('\n')[0]; }
      checks.push(row);
      await context.close();
    }
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ checks }, null, 2));
  }
  console.log(checks.map(({ id, status, title, media, loginForm, unavailable }) => ({ id, status, title, media, loginForm, unavailable })));
})().catch(e => { console.error(e); process.exitCode = 1; });
