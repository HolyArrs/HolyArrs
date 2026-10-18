const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { chromium, expect: locatorExpect } = require('../../../web/app/node_modules/@playwright/test');

const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const artworkHosts = new Set(['image.tmdb.org', 'static.tvmaze.com', 'is1-ssl.mzstatic.com']);
const batch = process.env.INTERACTION_BATCH || 'baseline';
if (!/^[a-z0-9][a-z0-9._-]*$/i.test(batch)) throw Error('Invalid evidence batch');
const out = path.join(root, '.omx/artifacts/visual-ralph/intercom/interaction-pass', batch);
const widths = (process.env.INTERACTION_WIDTHS || '360,834,1440').split(',').map(Number);
const modules = ['navigation', 'library', 'management', 'activity', 'settings', 'extra-settings', 'account', 'forms', 'stress', 'album', 'subtitles', 'roles'];
const scenarios = modules.flatMap(group => {
  const file = path.join(__dirname, `interaction-${group}.cjs`);
  if (!fs.existsSync(file)) throw Error(`Missing required scenario module: ${group}`);
  const loaded = require(file);
  const cases = Array.isArray(loaded) ? loaded : loaded.scenarios;
  if (!Array.isArray(cases)) throw Error(`Invalid scenario module: ${group}`);
  return cases.map(scenario => ({ ...scenario, group }));
}).filter(scenario => !process.env.INTERACTION_MATCH || new RegExp(process.env.INTERACTION_MATCH, 'i').test(scenario.name));
if (!scenarios.length || widths.some(width => !Number.isInteger(width) || width < 320 || width > 3840)) throw Error('Invalid or empty interaction selection');
if (new Set(widths).size !== widths.length) throw Error('Duplicate viewport widths');
const eligible = (scenario, width) => !(scenario.minWidth && width < scenario.minWidth || scenario.maxWidth && width > scenario.maxWidth || scenario.supportedWidths && !scenario.supportedWidths.includes(width));
const checkKey = ({ width, group, name }) => `${width}|${group}|${name}`;

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

function assertion(actual, message) {
  if (typeof actual === 'boolean') { assert.ok(actual, message); return; }
  return locatorExpect(actual, message);
}
assertion.poll = locatorExpect.poll;

async function screenshot(page, file) {
  const sensitive = page.locator('input[type="password"], [data-sensitive], .secret, .secret-value, .fresh code');
  await page.screenshot({ path: file, mask: [sensitive], fullPage: false });
}

(async () => {
  fs.mkdirSync(out, { recursive: true });
  const before = identity(), checks = [];
  const expectedChecks = widths.flatMap(width => scenarios.filter(scenario => eligible(scenario, width)).map(({ name, route, group }) => ({ key: checkKey({ width, group, name }), name, route, group, width })));
  assert.equal(new Set(expectedChecks.map(check => check.key)).size, expectedChecks.length, 'Duplicate scenario keys');
  assert.ok(expectedChecks.length, 'No eligible scenarios');
  fs.writeFileSync(path.join(out, 'expected.json'), JSON.stringify({ sourceHash: before.sourceHash, widths, checks: expectedChecks }, null, 2));
  fs.writeFileSync(path.join(out, 'manifest.json'), JSON.stringify({ before, widths, scenarios: scenarios.map(({ name, route, group, minWidth, maxWidth, supportedWidths }) => ({ name, route, group, minWidth, maxWidth, supportedWidths })), skipped: widths.flatMap(width => scenarios.filter(scenario => !eligible(scenario, width)).map(({ name, group }) => ({ width, name, group, reason: 'Control exists only in the declared viewport range.' }))), writes: 'Every non-GET request is intercepted; no backend mutation is permitted.' }, null, 2));
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of widths) for (const scenario of scenarios) {
      if (!eligible(scenario, width)) continue;
      const name = `${width}-${scenario.group}-${scenario.name.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}`;
      const context = await browser.newContext({ viewport: { width, height: 900 }, isMobile: width < 600, hasTouch: width < 1024, serviceWorkers: 'block', reducedMotion: 'no-preference' });
      await context.addInitScript(() => localStorage.setItem('holyarrs-theme', 'light'));
      const page = await context.newPage();
      page.setDefaultTimeout(6000);
      const attempts = [], steps = [], errors = [], handlers = [], nativeDialogs = [], fixtures = [], bootstrapFailures = [], visitedRoutes = new Set();
      page.on('requestfailed', request => { const u = new URL(request.url()); if (u.origin === origin && ['script','document'].includes(request.resourceType())) bootstrapFailures.push({path:u.pathname,error:request.failure()?.errorText}); });
      page.on('response', response => { const u = new URL(response.url()); if (u.origin === origin && response.status() >= 400 && response.request().resourceType() === 'script') bootstrapFailures.push({path:u.pathname,status:response.status()}); });
      let fixtureResponses = 0, nextConfirmation = false;
      page.on('pageerror', error => errors.push(error.message));
      page.on('framenavigated', frame => {
        if (frame !== page.mainFrame()) return;
        const url = new URL(frame.url());
        if (url.origin === origin) visitedRoutes.add(url.pathname);
      });
      if (typeof context.routeWebSocket === 'function') await context.routeWebSocket('**/*', socket => socket.close());
      page.on('dialog', async dialog => {
        const accept = nextConfirmation;
        nextConfirmation = false;
        nativeDialogs.push({ type: dialog.type(), action: accept ? 'accept' : 'dismiss' });
        if (accept) await dialog.accept();
        else await dialog.dismiss();
      });
      await context.route('**/*', async route => {
        const request = route.request(), url = new URL(request.url()), method = request.method();
        const own = url.origin === origin;
        const api = own && url.pathname.startsWith('/api/v1/');
        const apiPath = api ? url.pathname.slice('/api/v1'.length) : url.pathname;
        if (method !== 'GET') attempts.push({ method, path: apiPath, intercepted: true });
        if (api) {
          let body;
          try { body = request.postDataJSON(); } catch { body = undefined; }
          for (const handler of [...handlers].reverse()) {
            const response = await handler({ request, url, method, path: apiPath, body });
            if (response != null) {
              fixtureResponses++;
              fixtures.push({ method, path: apiPath, status: response.status || 200 });
              if (response.delay) await new Promise(resolve => setTimeout(resolve, response.delay));
              const payload = response.status === 204 || response.emptyBody
                ? { status: response.status || 204, body: '' }
                : { status: response.status || 200, json: response.json ?? {} };
              try { await route.fulfill(payload); } catch (error) { if (!page.isClosed() && !/closed|handled|cancel/i.test(error.message)) throw error; }
              return;
            }
          }
        }
        if (method !== 'GET') {
          if (api) return route.fulfill({ status: 503, json: { error: 'Preview interaction check: service unavailable. Try again.' } });
          return route.abort('blockedbyclient');
        }
        if (!own && (request.resourceType() !== 'image' || url.protocol !== 'https:' || !artworkHosts.has(url.hostname))) return route.abort('blockedbyclient');
        return route.continue();
      });
      const ctx = {
        page, width, expect: assertion, attempts,
        confirmNext(accept = false) { nextConfirmation = accept; },
        intercept(handler) { handlers.push(handler); },
        routeJSON(method, matcher, json, status = 200) {
          handlers.push(async data => {
            if (data.method !== method || !(matcher instanceof RegExp ? matcher.test(data.path) : matcher === data.path)) return null;
            return { status, json: typeof json === 'function' ? await json(data) : json };
          });
        },
        async step(label, action) {
          await action();
          steps.push(label);
        },
        async capture(label) {
          const file = name + '-' + label.replace(/[^a-z0-9]+/gi, '-').toLowerCase() + '.png';
          await screenshot(page, path.join(out, file));
          return file;
        }
      };
      let failure, image, controls = [];
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
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, 'Interaction caused horizontal page overflow');
        controls = await page.locator('main button, main input, main select, main textarea, dialog[open] button').evaluateAll(nodes => nodes.filter(node => node.getClientRects().length).map(node => ({ tag: node.tagName, type: node.getAttribute('type'), label: (node.getAttribute('aria-label') || node.labels?.[0]?.textContent || node.textContent || '').trim().slice(0, 100), disabled: node.disabled })));
        image = name + '.png';
        await screenshot(page, path.join(out, image));
      } catch (error) {
        failure = error.message;
        image = name + '-failed.png';
        try { await screenshot(page, path.join(out, image)); } catch { image = undefined; }
      } finally {
        checks.push({ name: scenario.name, group: scenario.group, route: scenario.route, width, steps, attempts, fixtureResponses, fixtures, nativeDialogs, bootstrapFailures, controls, visitedRoutes: [...visitedRoutes], errors, screenshot: image, ...(failure ? { failure } : {}) });
        await context.close();
        fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after: identity() }, null, 2));
        console.log(`${width} ${scenario.name}: ${failure ? 'FAIL ' + failure.split('\n')[0] : 'pass'}`);
      }
    }
  } finally { await browser.close(); }
  const after = identity();
  assert.deepEqual(checks.map(checkKey).sort(), expectedChecks.map(check => check.key).sort(), 'Interaction coverage is incomplete');
  fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after }, null, 2));
  if (checks.some(check => check.failure) || before.sourceHash !== after.sourceHash) process.exitCode = 1;
})().catch(error => { console.error(error.message); process.exitCode = 1; });
