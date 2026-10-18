// Read-only checks against the owned disposable preview. No API writes or credentials.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { createRequire } = require('node:module');

const repo = path.resolve(__dirname, '../../..');
const appRequire = createRequire(path.join(repo, 'web/app/package.json'));
const { chromium, expect } = appRequire('@playwright/test');
const { default: AxeBuilder } = appRequire('@axe-core/playwright');
const baseline = process.argv.includes('--before');
const run = `${baseline ? 'before' : 'after'}-${crypto.randomUUID()}`;
const output = path.join(repo, '.omx/artifacts/visual-ralph/intercom/mobile-library-status', run);
const origin = 'http://10.10.25.25:18746';
const cases = [
  { kind: 'movie', name: 'Copper Lighthouse', id: 161 },
  { kind: 'show', name: 'Moss Parade', id: 162 },
  { kind: 'artist', name: 'Copper Wren', id: 163 }
];
const widths = baseline ? [360] : [320, 360, 390, 430, 600, 601, 834, 1440];
const receipt = { run, started: new Date().toISOString(), baseline, source: {}, checks: [], screenshots: [], pageErrors: [], blocked: [] };
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
  return {
    commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8', windowsHide: true }).trim(),
    frontend: hash.digest('hex')
  };
}

function check(name, passed, evidence = {}) {
  receipt.checks.push({ name, passed, evidence });
  if (!passed && !baseline) throw new Error(name);
}

async function capture(page, name) {
  const file = `${name}.png`;
  await page.screenshot({ path: path.join(output, file), fullPage: false, animations: 'disabled' });
  receipt.screenshots.push({ file, viewport: page.viewportSize(), sha256: crypto.createHash('sha256').update(fs.readFileSync(path.join(output, file))).digest('hex') });
}

async function geometry(page, row) {
  return row.evaluate((element) => {
    const region = element.closest('.list-scroll');
    const bounds = region.getBoundingClientRect();
    const badges = [...element.querySelectorAll('.chip')].map((badge) => {
      const rect = badge.getBoundingClientRect();
      return { text: badge.textContent.trim(), left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom,
        fits: rect.left >= bounds.left - 1 && rect.right <= Math.min(bounds.right, innerWidth) + 1 && badge.scrollWidth <= badge.clientWidth + 1 && badge.scrollHeight <= badge.clientHeight + 1 };
    });
    const title = element.querySelector('.row-link');
    return { regionWidth: region.clientWidth, regionContentWidth: region.scrollWidth,
      documentWidth: document.documentElement.clientWidth, documentContentWidth: document.documentElement.scrollWidth,
      rowLayout: getComputedStyle(element).display, titleBottom: title.getBoundingClientRect().bottom, badges };
  });
}

(async () => {
  fs.mkdirSync(output, { recursive: true });
  receipt.source = source();
  browser = await chromium.launch({ headless: true });
  for (const width of widths) {
    const context = await browser.newContext({ viewport: { width, height: 1000 }, serviceWorkers: 'block', reducedMotion: 'reduce', hasTouch: width <= 600 });
    await context.route('**/*', async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const image = request.resourceType() === 'image' && request.method() === 'GET' &&
        ((url.protocol === 'https:' && /^(image\.tmdb\.org|static\.tvmaze\.com|is[1-5]-ssl\.mzstatic\.com)$/.test(url.hostname)) || url.origin === 'http://127.0.0.1:28764');
      if ((['GET', 'HEAD'].includes(request.method()) && url.origin === origin) || image) return route.continue();
      receipt.blocked.push({ method: request.method(), path: url.pathname });
      return route.abort();
    });
    await context.addInitScript(() => localStorage.setItem('holyarrs.library-view', 'list'));
    const page = await context.newPage();
    page.on('pageerror', (error) => receipt.pageErrors.push(error.message));
    for (const item of baseline ? cases.slice(0, 1) : cases) {
      await page.goto(`${origin}/library?kind=${item.kind}&q=${encodeURIComponent(item.name)}`);
      const row = page.getByTestId('library-row').filter({ has: page.locator(`a.row-link[href^="/library/${item.id}"]`) });
      await expect(row).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      await expect(row.getByTestId('request-approved')).toHaveText('Approved');
      await expect(row.locator('.chip').filter({ hasText: /missing/i })).toBeVisible();
      await expect(row.getByTestId('requested-by')).toBeVisible();
      await row.scrollIntoViewIfNeeded();
      const measured = await geometry(page, row);
      await capture(page, `${width}-${item.kind}`);
      check(`${width}/${item.kind}: badges fully visible without sideways scrolling`, measured.regionContentWidth <= measured.regionWidth + 1 && measured.documentContentWidth <= measured.documentWidth + 1 && measured.badges.every((badge) => badge.fits), measured);
      if (!baseline) {
        check(`${width}/${item.kind}: responsive row layout`, width <= 600 ? measured.rowLayout === 'grid' && measured.badges.every((badge) => badge.top >= measured.titleBottom) : measured.rowLayout === 'table-row');
        await expect(page.getByRole('table', { name: 'Titles in the Library' })).toBeVisible();
        await expect(page.getByRole('columnheader', { name: 'State', exact: true })).toBeAttached();
        if (width === 360 && item.kind === 'movie') {
          await row.locator('.row-link').focus();
          await page.keyboard.press('Enter');
          await expect(page).toHaveURL(new RegExp(`/library/${item.id}(?:\\?|$)`));
          await page.goBack();
          await expect(row.getByTestId('request-approved')).toBeVisible();
          await expect(row.getByTestId('last-opened')).toBeVisible();
          check('360/movie: keyboard open and return preserve list, query and last-opened marker', page.url().includes('q=Copper'));
          await capture(page, '360-keyboard-return');
        }
      }
    }
    if (!baseline && width === 360) {
      const audit = await new AxeBuilder({ page }).include('.list-scroll').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
      check('360: Library list automated accessibility', audit.violations.length === 0, { violations: audit.violations.map(({ id, impact, nodes }) => ({ id, impact, nodes: nodes.length })) });
    }
    if (!baseline && [320, 360].includes(width)) {
      // Browser-only stress content. It never reaches the preview API or database.
      const row = page.getByTestId('library-row').first();
      await row.evaluate((element) => {
        element.querySelector('.row-link b').textContent = 'A Very Long Artist and Album Name With an UnbrokenIdentifierThatMustWrapWithinThePhoneRow';
        element.querySelector('[data-testid="requested-by"]').textContent = 'Requested by Alexandria-With-A-Long-Household-Name, AnotherVeryLongUnbrokenRequesterName';
        const status = element.querySelector('.state-cell');
        for (const text of ['Waiting for a suitable release', 'AnUnbrokenStatusLabelThatMustWrapInsideTheBadge']) {
          const badge = document.createElement('span');
          badge.className = status.querySelector('.chip').className;
          badge.textContent = text;
          status.append(badge);
        }
      });
      await row.scrollIntoViewIfNeeded();
      const measured = await geometry(page, row);
      check(`${width}: browser-only long title/requester/multiple-status wrapping`, measured.regionContentWidth <= measured.regionWidth + 1 && measured.documentContentWidth <= measured.documentWidth + 1 && measured.badges.every((badge) => badge.fits), measured);
      await capture(page, `${width}-browser-only-long-text`);
    }
    await context.close();
  }
  check('No browser runtime errors or blocked out-of-scope/write attempts', receipt.pageErrors.length === 0 && receipt.blocked.length === 0);
  check('Frontend source stayed frozen throughout this run', source().frontend === receipt.source.frontend);
})().catch((error) => {
  receipt.error = error.message;
  process.exitCode = 1;
}).finally(async () => {
  await browser?.close();
  receipt.finished = new Date().toISOString();
  fs.mkdirSync(output, { recursive: true });
  fs.writeFileSync(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
  console.log(JSON.stringify({ run, checks: receipt.checks.length, failed: receipt.checks.filter((check) => !check.passed).length, screenshots: receipt.screenshots.length, error: receipt.error }));
});
