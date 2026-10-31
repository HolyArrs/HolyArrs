const { createRequire } = require('node:module');
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const requireApp = createRequire(path.join(root, 'web/app/package.json'));
const { chromium } = requireApp('playwright');
const output = path.join(root, '.omx/artifacts/visual-ralph/intercom/full-audit', process.env.AUDIT_BATCH || 'final');
const widths = (process.env.AUDIT_WIDTHS || '390,834,1440').split(',').map(Number);
const routes = (process.env.AUDIT_ROUTES || '/,/library?kind=movie,/requests,/downloads,/downloads/decide,/downloads/history,/settings').split(',');
const theme = process.env.AUDIT_THEME || 'light';
function identity() {
  const hash = createHash('sha256');
  function walk(dir) {
    for (const name of fs.readdirSync(dir).sort()) {
      const file = path.join(dir, name);
      if (fs.statSync(file).isDirectory()) walk(file);
      else hash.update(path.relative(root, file)).update(fs.readFileSync(file));
    }
  }
  walk(path.join(root, 'web/app/src'));
  return { commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(), sourceHash: hash.digest('hex') };
}
(async () => {
  fs.mkdirSync(output, { recursive: true });
  const before = identity();
  const checks = [];
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of widths) {
      const height = width >= 3840 ? 2160 : width >= 2560 ? 1440 : 900;
      const context = await browser.newContext({ viewport: { width, height }, serviceWorkers: 'block', reducedMotion: 'reduce' });
      await context.addInitScript((value) => localStorage.setItem('holyarrs-theme', value), theme);
      await context.route('**/*', (route) => route.request().method() === 'GET' ? route.continue() : route.abort());
      for (const route of routes) {
        const page = await context.newPage();
        const errors = [];
        const onError = (error) => errors.push(error.message);
        page.on('pageerror', onError);
        const response = await page.goto('http://127.0.0.1:18746' + route, { waitUntil: 'networkidle' });
        const expectedPath = new URL(route, 'http://127.0.0.1:18746').pathname;
        if (!response || response.status() >= 400) throw new Error(`Document failed for ${route}`);
        if (new URL(page.url()).pathname !== expectedPath) throw new Error(`Unexpected redirect for ${route}`);
        for (const [key, value] of new URL(route, 'http://127.0.0.1:18746').searchParams) {
          if (new URL(page.url()).searchParams.get(key) !== value) throw new Error(`Lost route filter ${key} for ${route}`);
        }
        const heading = await page.locator('main h1').first().innerText();
        if (!heading.trim() || /not found|not in your library|sign in|404/i.test(heading)) throw new Error(`Expected app screen missing for ${route}`);
        await page.screenshot({ path: path.join(output, width + '-' + route.replace(/[^a-z0-9]/gi, '_') + '.png') });
        await page.addScriptTag({ path: requireApp.resolve('axe-core/axe.min.js') });
        const violations = await page.evaluate(async () => (await axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'] } })).violations.map((v) => ({ id: v.id, impact: v.impact, targets: v.nodes.map((n) => n.target) })));
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
        checks.push({ route, width, height, theme, documentStatus: response.status(), heading, overflow, errors, violations });
        page.off('pageerror', onError);
        fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify({ before, checks, after: identity() }, null, 2));
        console.log(width, route, theme, 'violations', violations.length, 'overflow', overflow);
        await page.close();
      }
      await context.close();
    }
  } finally { await browser.close(); }
  const after = identity();
  fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify({ before, checks, after }, null, 2));
  if (before.sourceHash !== after.sourceHash) throw new Error('Source changed during capture; repeat against stable source.');
  if (checks.some((c) => c.overflow || c.errors.length || c.violations.length)) process.exitCode = 1;
})().catch((error) => { console.error(error); process.exitCode = 1; });
