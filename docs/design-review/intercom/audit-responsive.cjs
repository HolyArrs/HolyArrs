const { chromium } = require('../../../web/app/node_modules/playwright');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const out = path.join(root, '.omx/artifacts/visual-ralph/intercom/responsive-pass', process.env.RESPONSIVE_BATCH || 'baseline');
const widths = (process.env.RESPONSIVE_WIDTHS || '360,834,1440,3440').split(',').map(Number);
const theme = process.env.RESPONSIVE_THEME === 'dark' ? 'dark' : 'light';
const forcedHeight = process.env.RESPONSIVE_HEIGHT ? Number(process.env.RESPONSIVE_HEIGHT) : null;
if (forcedHeight !== null && (!Number.isInteger(forcedHeight) || forcedHeight < 300 || forcedHeight > 4000)) throw Error('Invalid capture height');
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
function pageRoutes(dir, prefix = '') {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    if (entry.isDirectory()) return pageRoutes(path.join(dir, entry.name), prefix + '/' + entry.name);
    return entry.name === '+page.svelte' ? [prefix || '/'] : [];
  });
}
const templates = pageRoutes(path.join(root, 'web/app/src/routes'));
const routes = templates.map(template => {
  const dynamic = {
    '/library/[id]': '/library/76',
    '/library/[id]/albums/[album]': '/library/6/albums/7',
    '/library/collections/[id]': '/library/collections/900001',
    '/people/[id]': '/people/900001',
    '/settings/sources/[id]': '/settings/sources/900001',
    '/invite/[token]': '/invite/layout-check'
  };
  return { template, route: dynamic[template] || template, fixture: /900001|layout-check/.test(dynamic[template] || '') || template === '/login' };
});
routes.push(...['/library?kind=movie', '/library?kind=artist', '/library/1', '/library/141'].map(route => ({ template: route.includes('?') ? '/library' : '/library/[id]', route, fixture: false })));
routes.push(...[
  ['/settings/sources', 'add', 'Add a Source'],
  ['/settings/downloaders', 'add', 'Add a Downloader'],
  ['/settings/alerts', 'add', 'Add an Alert'],
  ['/settings/media-servers', 'add', 'Add a media server'],
  ['/settings/library-folders', 'add', 'Add a folder'],
  ['/settings/outside-lists', 'add', 'Add a list'],
  ['/settings/users', 'edit-user', '^Change '],
  ['/library/1', 'edit', 'Edit'],
  ['/library?kind=movie', 'requester-menu', 'Requested by'],
  ['/library?kind=movie', 'list', 'List']
].map(([route, state, button]) => ({ template: route.split('?')[0], route, state, button, fixture: ['/settings/users', '/settings/outside-lists'].includes(route) })));
const filter = process.env.RESPONSIVE_ROUTES?.split(',');
const selected = filter ? routes.filter(r => filter.includes(r.state ? r.route + '#' + r.state : r.route)) : routes;
const collection = { id: 900001, name: 'The Lord of the Rings collection', titles: [
  { id: 1, name: 'The Lord of the Rings: The Fellowship of the Ring — extended edition', year: 2001, version_ids: [1], files: [], protected: true },
  { id: 2, name: 'The Lord of the Rings: The Two Towers', year: 2002, version_ids: [2], files: [], protected: false }
] };
const person = { person: { id: '900001', name: 'Christopher Nolan', department: 'Directing', birthplace: 'London, England', biography: 'A filmmaker known for intricate storytelling and large-scale cinema. This browser-only layout fixture checks long biographies, filmography selection and responsive controls.', known_for: [] }, movies: [
  { name: 'The Dark Knight Rises — a deliberately extended title for layout coverage', year: 2012, overview: 'A long description checks readable filmography rows and selection controls across the supported screen sizes.', ids: { tmdb: '49026' }, roles: ['Director', 'Writer'], characters: [] },
  { name: 'Interstellar', year: 2014, overview: 'A science fiction film.', ids: { tmdb: '157336' }, roles: ['Director'], title_id: 1 }
], page: 1, page_size: 24, total_results: 2, total_pages: 1, complete: true, provider_pages: 1, attribution: 'Browser-only layout fixture' };
async function fixtures(route, descriptor) {
  const url = new URL(route.request().url());
  if (route.request().method() !== 'GET') return route.abort();
  if (!url.pathname.startsWith('/api/v1/')) return route.continue();
  let data;
  if (descriptor.template === '/login' && url.pathname.endsWith('/auth/me')) return route.fulfill({ status: 401, json: { error: 'Sign in required.' } });
  if (descriptor.template === '/login' && url.pathname.endsWith('/auth/state')) data = { needs_setup: false, ways: ['password'], min_password_length: 8 };
  if (url.pathname.endsWith('/auth/invites/layout-check')) data = { role: 'member' };
  if (url.pathname.endsWith('/collections/900001')) data = collection;
  if (url.pathname.endsWith('/people/900001')) data = person;
  if (url.pathname.endsWith('/sources/900001/stats')) data = { source: { id: 900001, name: 'Preview source with a long descriptive name', format: 'newznab', protocol: 'usenet', priority: 10, use_for: { feed: true, auto_search: true, hand_search: true }, health: { state: 'working', text: 'Available' }, summary: { avg_ms_24h: 128 } }, days: [], recent: [] };
  if (descriptor.state === 'add' && descriptor.route === '/settings/outside-lists') {
    if (url.pathname.endsWith('/outside-lists')) data = { outside_lists: [] };
    if (url.pathname.endsWith('/pick-rules')) data = { pick_rules: [] };
    if (url.pathname.endsWith('/never-add')) data = { never_add: [] };
    if (url.pathname.endsWith('/settings/trakt-key')) data = { key: '' };
  }
  if (descriptor.state === 'edit-user') {
    if (url.pathname.endsWith('/users')) data = { users: [{ id: 900001, name: 'Household member with a long display name', role: 'member', state: 'active', perms: ['ask'], two_factor: false, password: false, links: [], created_at: '2026-10-01T00:00:00Z' }], perms: ['ask', 'auto_approve', 'approve', 'manage_library', 'settings'] };
    if (url.pathname.endsWith('/users/invites')) data = { invites: [] };
  }
  if (data) return route.fulfill({ json: data });
  return route.continue();
}
async function layout(page) {
  return page.evaluate(() => {
    const scope = document.querySelector('dialog[open]') || document.querySelector('main') || document.body;
    const clipped = [], outside = [], overlaps = [];
    const visible = e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== 'hidden' && !e.closest('details:not([open]) > :not(summary)'); };
    const label = e => e.getAttribute('aria-label') || e.getAttribute('data-testid') || e.id || e.tagName.toLowerCase() + '.' + [...e.classList].join('.');
    const scrollable = e => { for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) { if (['auto', 'scroll'].includes(getComputedStyle(p).overflowX) && p.scrollWidth > p.clientWidth + 1) return true; } return false; };
    for (const e of scope.querySelectorAll('button,a,input,select,textarea,h1,h2,h3,label,code,th,td,p,span,small,summary,legend,dt,dd')) {
      if (!visible(e) || e.closest('.sr-only,[aria-hidden="true"]')) continue;
      const r = e.getBoundingClientRect(), s = getComputedStyle(e);
      if ((r.left < -1 || r.right > innerWidth + 1) && !scrollable(e)) outside.push({ element: label(e), left: r.left, right: r.right });
      if (e.scrollWidth > e.clientWidth + 2 && ['hidden', 'clip'].includes(s.overflowX) && s.textOverflow !== 'ellipsis' && s.webkitLineClamp === 'none' && e.tagName !== 'INPUT') clipped.push({ element: label(e), excess: e.scrollWidth - e.clientWidth });
    }
    const buttons = [...scope.querySelectorAll('button,.btn')].filter(visible);
    for (let i = 0; i < buttons.length; i++) for (let j = i + 1; j < buttons.length; j++) {
      const a = buttons[i], b = buttons[j];
      if (a.contains(b) || b.contains(a)) continue;
      const x = a.getBoundingClientRect(), y = b.getBoundingClientRect();
      if (Math.min(x.right, y.right) - Math.max(x.left, y.left) > 3 && Math.min(x.bottom, y.bottom) - Math.max(x.top, y.top) > 3) overlaps.push([label(a), label(b)]);
    }
    for (const control of scope.querySelectorAll('button,a.btn,input:not([type="checkbox"]):not([type="radio"]),select,textarea,summary')) {
      if (!visible(control)) continue;
      for (const sibling of control.parentElement.children) {
        if (!sibling.matches('p,span,small,label,code,h1,h2,h3,dt,dd') || !visible(sibling) || sibling.contains(control) || control.contains(sibling)) continue;
        const a = control.getBoundingClientRect(), b = sibling.getBoundingClientRect();
        if (Math.min(a.right, b.right) - Math.max(a.left, b.left) > 3 && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 3) overlaps.push([label(control), label(sibling)]);
      }
    }
    return { overflow: document.documentElement.scrollWidth > innerWidth + 1, pageWidth: document.documentElement.scrollWidth, height: document.documentElement.scrollHeight, clipped, outside, overlaps };
  });
}
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const before = identity(), checks = [];
  fs.writeFileSync(path.join(out, 'manifest.json'), JSON.stringify({ templates, allRoutes: routes, selected, widths, theme }, null, 2));
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of widths) for (const descriptor of selected) {
      const height = forcedHeight ?? (width >= 3840 ? 2160 : width >= 2560 ? 1440 : 900);
      const context = await browser.newContext({ viewport: { width, height }, deviceScaleFactor: 1, serviceWorkers: 'block', reducedMotion: 'no-preference' });
      await context.addInitScript(value => localStorage.setItem('holyarrs-theme', value), theme);
      await context.route('**/*', r => fixtures(r, descriptor));
      const page = await context.newPage(), errors = [];
      page.on('pageerror', e => errors.push(e.message));
      const key = width + '-' + (descriptor.route + (descriptor.state ? '-' + descriptor.state : '')).replace(/[^a-z0-9]/gi, '_');
      try {
        const response = await page.goto(origin + descriptor.route, { waitUntil: 'networkidle', timeout: 30000 });
        if (!response?.ok()) throw Error('Application document returned HTTP ' + (response?.status() ?? 'no response'));
        await page.evaluate(() => document.fonts.ready);
        await page.waitForTimeout(250);
        if (!await page.locator('h1').count()) throw Error('Expected application heading missing');
        if (new URL(page.url()).pathname !== new URL(descriptor.route, origin).pathname) throw Error('Unexpected route redirect');
        if (descriptor.button) {
          const trigger = descriptor.state === 'requester-menu' ? page.locator('summary').filter({ hasText: descriptor.button }).first() : page.getByRole('button', { name: descriptor.button.startsWith('^') ? new RegExp(descriptor.button) : descriptor.button, exact: !descriptor.button.startsWith('^') }).first();
          await trigger.click();
          await page.waitForTimeout(250);
        }
        const dialogFocus = await page.evaluate(() => {
          const dialog = document.querySelector('dialog[open]');
          const skip = document.querySelector('a.skip');
          return dialog ? { inside: dialog.contains(document.activeElement), active: document.activeElement?.tagName, name: document.activeElement?.getAttribute('aria-label') || document.activeElement?.id, skipTop: skip?.getBoundingClientRect().top, skipFocused: skip === document.activeElement } : null;
        });
        if (dialogFocus && !dialogFocus.inside) throw Error('Open dialog does not contain keyboard focus');
        const sensitive = page.locator('input[type="password"], [data-sensitive], .secret, .secret-value');
        const mask = await sensitive.count() ? [sensitive] : [];
        const measures = await layout(page);
        const screenshot = key + '.png';
        let viewport;
        if (dialogFocus) {
          viewport = key + '-viewport.png';
          await page.screenshot({ path: path.join(out, viewport), mask });
        }
        await page.screenshot({ path: path.join(out, screenshot), fullPage: measures.height <= 14000, mask });
        let bottom;
        if (measures.height > height) {
          await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight));
          await page.waitForTimeout(120);
          bottom = key + '-bottom.png';
          await page.screenshot({ path: path.join(out, bottom), mask });
        }
        let violations = [];
        if (process.env.RESPONSIVE_AXE !== '0') {
          await page.addScriptTag({ path: require.resolve('../../../web/app/node_modules/axe-core/axe.min.js') });
          violations = await page.evaluate(async () => (await axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'] } })).violations.map(v => ({ id: v.id, impact: v.impact, targets: v.nodes.map(n => n.target) })));
        }
        const environment = await page.evaluate(() => ({ cssWidth: innerWidth, cssHeight: innerHeight, dpr: devicePixelRatio, visualViewportScale: visualViewport?.scale, reducedMotion: matchMedia('(prefers-reduced-motion: reduce)').matches }));
        checks.push({ ...descriptor, capturedAt: new Date().toISOString(), width, height, theme, environment, browserZoom: 'Default isolated context; native browser zoom not manipulated', documentStatus: response?.status(), landed: new URL(page.url()).pathname, screenshot, bottom, viewport, dialogFocus, ...measures, errors, violations });
        console.log(width, descriptor.route, descriptor.state || '', 'overflow', measures.overflow, 'outside', measures.outside.length, 'clipped', measures.clipped.length, 'overlap', measures.overlaps.length, 'axe', violations.length, 'errors', errors.length);
      } catch (e) {
        checks.push({ ...descriptor, width, height, failure: e.message, errors });
        console.log(width, descriptor.route, 'FAILED', e.message.split('\n')[0]);
      } finally { await context.close(); }
      fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after: identity() }, null, 2));
    }
  } finally { await browser.close(); }
  const after = identity();
  fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, checks, after }, null, 2));
  if (before.sourceHash !== after.sourceHash || checks.some(c => c.failure || c.overflow || c.clipped.length || c.outside.length || c.overlaps.length || c.errors.length || c.violations.length)) process.exitCode = 1;
})().catch(e => { console.error(e); process.exitCode = 1; });
