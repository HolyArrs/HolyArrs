// Real writes are restricted to this preview's newly created disposable members and requests.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { createRequire } = require('node:module');

const repo = path.resolve(__dirname, '../../..');
const appRequire = createRequire(path.join(repo, 'web/app/package.json'));
const { chromium, expect: baseExpect } = appRequire('@playwright/test');
const expect = baseExpect.configure({ timeout: 10_000 });
const base = 'http://10.10.25.25:18746';
const runtime = path.join(os.tmpdir(), 'holyarrs-intercom-preview-18747');
const resuming = process.env.HOLYARRS_REAL_SAVE_RESUME || '';
if (resuming && !/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(resuming)) throw new Error('Invalid continuation run id');
const runId = resuming || crypto.randomUUID();
const evidenceDir = path.join(repo, '.omx/artifacts/visual-ralph/intercom/real-save', runId);
const tokens = [crypto.randomBytes(32).toString('hex'), crypto.randomBytes(32).toString('hex')];
const passwords = [crypto.randomBytes(32).toString('base64url'), crypto.randomBytes(32).toString('base64url')];
const sessionTokens = [crypto.randomBytes(32).toString('base64url'), crypto.randomBytes(32).toString('base64url')];
const csrfTokens = [crypto.randomBytes(32).toString('base64url'), crypto.randomBytes(32).toString('base64url')];
const namePrefix = `e2e-${runId.slice(0, 8)}-`;
const names = { member: `${namePrefix}one`, second: `${namePrefix}two` };
const askIDs = new Set();
const originalAskIDs = new Set();
const titleIDs = new Set();
const references = new Set(['tmdb:900008', 'tmdb:800003', 'tmdb:900009', 'tmdb:900007', 'mbid:a3333333-0000-4000-8000-000000000003']);
let results = { runId, base, started: new Date().toISOString(), checks: [], screenshots: [], requests: [], writes: [], blocked: [], pageErrors: [] };
let browser;
const pages = [];
let fixtureCreated = false;
let finalized = false;

function sanitize(value) {
  let text = String(value ?? '');
  for (const secret of [...tokens, ...passwords, ...sessionTokens, ...csrfTokens]) text = text.split(secret).join('[redacted]');
  return text.replace(/\/(auth\/)?invites?\/[a-zA-Z0-9_-]+/g, '/$1invite/[redacted]');
}
function saveReceipt() {
  fs.writeFileSync(path.join(evidenceDir, 'receipt.json'), sanitize(JSON.stringify(results, null, 2)) + '\n');
}
function sourceIdentity() {
  const root = path.join(repo, 'web/app/src');
  const hash = crypto.createHash('sha256');
  function walk(dir) {
    for (const name of fs.readdirSync(dir).sort()) {
      const file = path.join(dir, name);
      if (fs.statSync(file).isDirectory()) walk(file);
      else hash.update(path.relative(repo, file)).update(fs.readFileSync(file));
    }
  }
  walk(root);
  return {
    commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
    branch: execFileSync('git', ['branch', '--show-current'], { cwd: repo, encoding: 'utf8' }).trim(),
    frontend: hash.digest('hex')
  };
}
function processIdentity() {
  // Read only the required processes. Command lines are checked in memory, never emitted.
  const script = String.raw`
$ErrorActionPreference = 'Stop'
$ports = @(18730,18745,18746,18747) + @(28760..28772)
$rows = @(Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in $ports })
$front = @($rows | Where-Object LocalPort -eq 18746)
$back = @($rows | Where-Object LocalPort -eq 18747)
if ($front.Count -ne 1 -or $back.Count -ne 1) { throw 'Preview listeners are not unique' }
if ($back[0].LocalAddress -ne '127.0.0.1') { throw 'Backend is not loopback only' }
$frontProcess = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $front[0].OwningProcess)
$backProcess = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $back[0].OwningProcess)
$expectedConfig = Join-Path $env:TEMP 'holyarrs-intercom-preview-18747\config'
if (-not $frontProcess.CommandLine.Contains('vite.intercom-preview.config.ts')) { throw 'Unexpected frontend command' }
if (-not $backProcess.CommandLine.Contains($expectedConfig) -or -not $backProcess.CommandLine.Contains('127.0.0.1:18747')) { throw 'Unexpected disposable backend' }
$binaryStream = [IO.File]::OpenRead($backProcess.ExecutablePath)
$sha = [Security.Cryptography.SHA256]::Create()
try { $hash = [BitConverter]::ToString($sha.ComputeHash($binaryStream)).Replace('-','') }
finally { $binaryStream.Dispose(); $sha.Dispose() }
if ($hash -ne 'D1FA075167517E84FD834EC112C211EE84F27AA8BE8191E07DB9AE62E316C914') { throw 'Unexpected backend binary' }
foreach ($port in @(28760..28772)) {
  $fake = @($rows | Where-Object LocalPort -eq $port)
  if ($fake.Count -ne 1 -or $fake[0].LocalAddress -ne '127.0.0.1') { throw 'Fake service listener mismatch' }
}
@{ listeners = @($rows | Sort-Object LocalPort,LocalAddress | Select-Object LocalAddress,LocalPort,OwningProcess); backendHash = $hash; isolated = $true } | ConvertTo-Json -Depth 5 -Compress
`;
  return JSON.parse(execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8', timeout: 20_000 }));
}
function data(action, extra = {}) {
  const input = { action, runId, evidenceDir, ...extra };
  try {
    const output = execFileSync('python', [path.join(__dirname, 'real-save-data.py')], {
      input: JSON.stringify(input), encoding: 'utf8', timeout: 30_000, maxBuffer: 1024 * 1024
    });
    return JSON.parse(output);
  } catch (error) {
    let reason = 'Helper refused the operation';
    try { reason = JSON.parse(String(error.stdout)).error || reason; } catch { /* No raw input or SQL values in the error. */ }
    throw new Error(sanitize(reason));
  }
}
async function directGet(apiPath) {
  const response = await fetch(`${base}/api/v1${apiPath}`, { signal: AbortSignal.timeout(10_000) });
  if (!response.ok) throw new Error(`Preflight GET ${apiPath.split('?')[0]} returned ${response.status}`);
  return response.json();
}
async function get(page, apiPath) {
  if (!apiPath.startsWith('/') || apiPath.startsWith('//')) throw new Error('API-relative paths required');
  return page.evaluate(async (apiPath) => {
    const response = await fetch(`/api/v1${apiPath}`, { credentials: 'same-origin' });
    if (!response.ok) throw new Error(`GET ${apiPath.split('?')[0]} returned ${response.status}`);
    return response.json();
  }, apiPath);
}
async function step(name, fn) {
  const check = { name, started: new Date().toISOString(), passed: false };
  results.checks.push(check);
  process.stdout.write(`Starting: ${name}\n`);
  try {
    await fn();
    check.passed = true;
    process.stdout.write(`Passed: ${name}\n`);
  } catch (error) {
    check.error = sanitize(error.stack || error);
    throw error;
  } finally {
    check.finished = new Date().toISOString();
    saveReceipt();
  }
}
async function capture(page, label) {
  if (/\/invite\//.test(page.url())) throw new Error('Refusing invitation screenshot');
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  const viewport = page.viewportSize();
  const file = `${String(results.screenshots.length + 1).padStart(2, '0')}-${viewport.width}-${label.replace(/[^a-z0-9-]/gi, '-').toLowerCase()}.png`;
  await page.screenshot({ path: path.join(evidenceDir, file), fullPage: true,
    mask: [page.locator('input[type="password"], [data-sensitive], .secret-value, .setup-code')], animations: 'disabled' });
  results.screenshots.push({ file, label, viewport, route: new URL(page.url()).pathname + new URL(page.url()).search });
  saveReceipt();
}
function allowedWrite(who, request, url) {
  const method = request.method();
  const apiPath = url.pathname.slice('/api/v1'.length);
  let body;
  try { body = request.postDataJSON(); } catch { return false; }
  if (method === 'POST' && who !== 'owner' && apiPath === `/auth/invites/${tokens[who === 'member' ? 0 : 1]}`) {
    return body?.name === names[who] && body?.password === passwords[who === 'member' ? 0 : 1]
      && Object.keys(body).every(key => ['name', 'password'].includes(key));
  }
  if (method === 'POST' && apiPath === '/asks' && who !== 'owner') {
    return references.has(body?.id) && Object.keys(body).every(key => ['id', 'kind', 'seasons', 'albums'].includes(key))
      && (!body.kind || ['movie', 'show', 'artist'].includes(body.kind))
      && (!body.seasons || JSON.stringify(body.seasons) === '[1]')
      && (!body.albums || JSON.stringify(body.albums) === '["b3333333-0000-4000-8000-000000000001"]');
  }
  const decision = apiPath.match(/^\/asks\/(\d+)\/(approve|turn-down)$/);
  if (method === 'POST' && decision && who === 'owner' && askIDs.has(Number(decision[1]))) {
    return Object.keys(body || {}).every(key => ['note', 'library_folder_id', 'pick_rules_id'].includes(key));
  }
  const withdrawal = apiPath.match(/^\/asks\/(\d+)$/);
  if (method === 'DELETE' && withdrawal && who !== 'owner' && askIDs.has(Number(withdrawal[1]))) return true;
  if (method === 'PUT' && apiPath === '/my/alerts' && who !== 'owner') {
    return Object.keys(body || {}).every(key => ['email', 'discord_id', 'push_user', 'push_token'].includes(key))
      && Object.values(body || {}).every(value => value === '' || value === 'preview-e2e@example.test');
  }
  return method === 'POST' && apiPath === '/messages/read' && who !== 'owner';
}
async function context(who, viewport) {
  const ctx = await browser.newContext({ viewport, serviceWorkers: 'block', reducedMotion: 'reduce' });
  const allowedExternalImages = new Set(['image.tmdb.org', 'static.tvmaze.com', 'is1-ssl.mzstatic.com', 'is2-ssl.mzstatic.com', 'is3-ssl.mzstatic.com', 'is4-ssl.mzstatic.com', 'is5-ssl.mzstatic.com']);
  await ctx.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const safeMethod = ['GET', 'HEAD', 'OPTIONS'].includes(request.method());
    if (url.origin === base && safeMethod) return route.continue();
    if (url.origin === base && url.pathname.startsWith('/api/v1/') && allowedWrite(who, request, url)) {
      results.writes.push({ who, method: request.method(), path: sanitize(url.pathname), passedThrough: true });
      return route.continue();
    }
    if (safeMethod && request.resourceType() === 'image' && (
      (url.protocol === 'https:' && allowedExternalImages.has(url.hostname)) || url.origin === 'http://127.0.0.1:28764')) return route.continue();
    results.blocked.push({ who, method: request.method(), path: sanitize(url.origin + url.pathname) });
    return route.abort('blockedbyclient');
  });
  const page = await ctx.newPage();
  pages.push(page);
  page.setDefaultTimeout(10_000);
  page.setDefaultNavigationTimeout(20_000);
  page.on('pageerror', error => results.pageErrors.push({ who, error: sanitize(error.message) }));
  page.on('response', response => {
    const request = response.request();
    if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method())) {
      const item = [...results.writes].reverse().find(item => item.who === who && item.method === request.method() && item.path === sanitize(new URL(request.url()).pathname) && item.status === undefined);
      if (item) item.status = response.status();
    }
  });
  return page;
}
async function join(page, who, index) {
  await page.goto(`${base}/invite/${tokens[index]}`);
  await page.getByLabel('Name', { exact: true }).fill(names[who]);
  await page.getByLabel('Password', { exact: true }).fill(passwords[index]);
  await page.getByLabel('Password again', { exact: true }).fill(passwords[index]);
  await page.getByRole('button', { name: 'Join and sign in', exact: true }).click();
  await page.waitForURL('**/library');
  const me = await get(page, '/auth/me');
  expect(me.user?.name ?? me.name).toBe(names[who]);
  expect(me.role ?? me.user?.role).toBe('member');
  expect(me.via).toBe('session');
  expect(me.perms).toContain('ask');
  expect(me.perms).not.toContain('manage_library');
  return me.user?.id ?? me.id;
}

async function main() {
  if (process.env.HOLYARRS_REAL_SAVE_ALLOW !== 'preview-18746') throw new Error('Explicit disposable preview opt-in is required');
  if (path.resolve(process.cwd()) !== repo) throw new Error('Run only from the owned preview worktree');
  if (resuming) {
    results = JSON.parse(fs.readFileSync(path.join(evidenceDir, 'receipt.json'), 'utf8'));
    if (results.runId !== runId || results.passed || !results.cleanup?.verified || results.cleanup.disabledMembers !== 2) throw new Error('Continuation requires a safely finalized incomplete run');
    fs.writeFileSync(path.join(evidenceDir, `attempt-${(results.previousAttempts?.length || 0) + 1}.json`), JSON.stringify(results, null, 2));
    results.previousAttempts = [...(results.previousAttempts || []), { checks: results.checks, error: results.error, cleanup: results.cleanup, finished: results.finished }];
    results.checks = [];
    results.resumed = new Date().toISOString();
    delete results.error;
    delete results.cleanupError;
    delete results.cleanup;
    delete results.finished;
    delete results.passed;
    expect(sourceIdentity()).toEqual(results.sourceBefore);
    expect(processIdentity()).toEqual(results.processBefore);
  }
  results.sourceBefore = sourceIdentity();
  if (results.sourceBefore.branch !== 'codex/mobbin-product-review') throw new Error('Unexpected branch');
  results.processBefore = processIdentity();
  const document = await fetch(base, { signal: AbortSignal.timeout(10_000) }).then(response => response.text());
  if (!document.includes(`/@fs/${repo.replaceAll('\\', '/')}/web/app/.svelte-kit/generated/dev/client/app.js`)) throw new Error('Preview document is not served from the owned worktree');
  const proxy = fs.readFileSync(path.join(repo, 'web/app/vite.intercom-preview.config.ts'), 'utf8');
  if (!proxy.includes("const target = 'http://127.0.0.1:18747';")) throw new Error('Unexpected preview API target');
  const me = await directGet('/auth/me');
  if (me.role !== 'owner' || me.via !== 'local_network') throw new Error('Expected local preview owner bypass');
  const settings = await directGet('/settings/asks');
  if (settings.search_on_approve !== false) throw new Error('Approval must not start a download search');
  const titles = (await directGet('/titles')).titles;
  if (!Array.isArray(titles)) throw new Error('Library read failed');
  const ownedMemberIDs = new Set(Object.values(results.memberIDs || {}));
  const pending = (await directGet('/asks')).asks;
  if (!Array.isArray(pending)) throw new Error('Request read failed');
  const ownedAsks = resuming ? pending.filter(ask => ownedMemberIDs.has(ask.asked_by?.id)
    && (ask.people || []).every(person => ownedMemberIDs.has(person.id))) : [];
  const ownedRefs = new Set(ownedAsks.map(ask => ask.ref));
  const ownedTitleIDs = new Set(ownedAsks.map(ask => ask.title_id).filter(Number.isInteger));
  for (const title of titles.filter(title => !ownedTitleIDs.has(title.id))) {
    if ([900008, 800003, 900009, 900007].includes(Number(title.ids?.tmdb))
      || [title.ids?.mbid, title.ids?.musicbrainz].includes('a3333333-0000-4000-8000-000000000003')
      || ['Copper Lighthouse', 'Moss Parade', 'Copper Wren', 'Velvet Tide', 'Northern Kettle'].includes(title.name)) throw new Error('Lifecycle candidate already exists in Library');
  }
  for (const ask of pending.filter(ask => !ownedRefs.has(ask.ref))) originalAskIDs.add(ask.id);
  if (pending.filter(ask => !ownedRefs.has(ask.ref)).some(ask => references.has(ask.ref) || references.has(ask.reference)
    || [900008, 800003, 900009, 900007].includes(Number(ask.tmdb_id))
    || ['Copper Lighthouse', 'Moss Parade', 'Copper Wren', 'Velvet Tide', 'Northern Kettle'].includes(ask.name))) throw new Error('Lifecycle candidate already has a request');
  fs.mkdirSync(evidenceDir, { recursive: true });
  if (resuming) {
    results.continuation = data('resume', { sessionTokens, csrfTokens, expectedRequests: results.requests, allowPartial: true });
    originalAskIDs.clear();
    for (const id of results.continuation.originalAskIDs) originalAskIDs.add(id);
  } else {
    results.fixture = data('init', { inviteTokens: tokens, namePrefix });
  }
  fixtureCreated = true;
  saveReceipt();
  browser = await chromium.launch({ headless: true });
  const owner = await context('owner', { width: 1440, height: 1000 });
  const member = await context('member', { width: 360, height: 800 });
  const second = await context('second', { width: 834, height: 1100 });
  await owner.goto(`${base}/wishlist`);
  if (resuming) {
    await step('Resume only the two disposable test members without changing passwords', async () => {
      for (const [index, [who, page]] of [['member', member], ['second', second]].entries()) {
        await page.context().addCookies([{ name: 'holyarrs_intercom_preview_session', value: sessionTokens[index], url: base, httpOnly: true, sameSite: 'Lax' }]);
        await page.goto(`${base}/library`);
        const identity = await get(page, '/auth/me');
        expect(identity.id).toBe(results.memberIDs[who]);
        expect(identity.name).toBe(names[who]);
        expect(identity.role).toBe('member');
        expect(identity.via).toBe('session');
      }
    });
  } else {
    await step('Create two disposable members through real invitation forms', async () => {
      results.memberIDs = { member: await join(member, 'member', 0), second: await join(second, 'second', 1) };
    });
  }
  await require('./real-save-flows.cjs')({ owner, member, second, expect, step, capture, get, names, base, results,
    registerAsk(id) { if (!Number.isInteger(id) || id <= 0 || originalAskIDs.has(id)) throw new Error('Request must be new and run-owned'); askIDs.add(id); },
    registerTitle(id) { if (!Number.isInteger(id) || id <= 0) throw new Error('Invalid title id'); titleIDs.add(id); }
  });
  expect(results.pageErrors, 'No browser runtime errors').toEqual([]);
  expect(results.blocked, 'No attempted writes outside the explicit test scope').toEqual([]);
  expect(results.writes.every(write => write.status >= 200 && write.status < 300), 'All authorized real saves accepted').toBe(true);
  results.database = data('verify', { expectedRequests: results.requests });
  results.cleanup = data('finalize', { expectedRequests: results.requests });
  finalized = true;
  results.processAfter = processIdentity();
  expect(results.processAfter).toEqual(results.processBefore);
  results.sourceAfter = sourceIdentity();
  expect(results.sourceAfter).toEqual(results.sourceBefore);
  results.passed = true;
}

let deadline;
Promise.race([main(), new Promise((_, reject) => { deadline = setTimeout(() => reject(new Error('Real-save run exceeded ten minutes')), 600_000); })])
  .catch(async error => {
    results.passed = false;
    results.error = sanitize(error.stack || error);
    process.stderr.write(sanitize(error.message) + '\n');
    process.exitCode = 1;
    for (const [index, page] of pages.entries()) {
      if (!page.isClosed() && page.url().startsWith(base) && !page.url().includes('/invite/')) {
        try { await capture(page, `diagnostic-${index}`); } catch { /* Preserve the original failure. */ }
      }
    }
  })
  .finally(async () => {
    clearTimeout(deadline);
    if (browser) await browser.close();
    if (fixtureCreated && !finalized) {
      try { results.cleanup = data('finalize', { expectedRequests: results.requests, allowPartial: true }); }
      catch (error) { results.cleanupError = sanitize(error.message); }
    }
    results.finished = new Date().toISOString();
    if (fs.existsSync(evidenceDir)) saveReceipt();
    process.stdout.write(JSON.stringify({ runId, passed: results.passed === true, checks: results.checks.length, screenshots: results.screenshots.length, realWrites: results.writes.length, evidenceDir }) + '\n');
  });
