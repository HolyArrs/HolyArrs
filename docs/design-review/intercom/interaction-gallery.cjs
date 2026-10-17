const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '../../..');
const evidenceRoot = path.join(root, '.omx/artifacts/visual-ralph/intercom/interaction-pass');
const finalDirectory = path.join(evidenceRoot, 'final');
const touchDirectory = path.join(evidenceRoot, 'touch');
const staticAuditRoot = path.resolve(root, 'web/app/static/preview-audit');
const outputDirectory = path.resolve(staticAuditRoot, 'interaction-pass');
const pngSignature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function fail(message) {
  throw new Error(`Interaction gallery refused to publish: ${message}`);
}

function requireGeneratedPath(candidate, label) {
  const resolved = path.resolve(candidate);
  if (resolved !== staticAuditRoot && !resolved.startsWith(staticAuditRoot + path.sep)) {
    fail(`${label} escapes the static preview-audit directory.`);
  }
  return resolved;
}

function readJson(directory, name) {
  const file = path.join(directory, name);
  if (!fs.existsSync(file)) fail(`missing ${path.relative(root, file)}.`);
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (error) {
    fail(`${path.relative(root, file)} is not valid JSON (${error.message}).`);
  }
}

function list(value, label) {
  if (!Array.isArray(value)) fail(`${label} must be an array.`);
  return value;
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function checkKey(value) {
  return `${value.width}|${value.group}|${value.name}`;
}

function scenarioKey(value) {
  return `${value.group}|${value.name}`;
}

function unique(values, label) {
  const set = new Set(values);
  if (set.size !== values.length) fail(`${label} contains duplicate entries.`);
  return set;
}

function equalArray(actual, expected, label) {
  if (actual.length !== expected.length || actual.some((value, index) => value !== expected[index])) {
    fail(`${label} does not match the final expected inventory.`);
  }
}

function validatePng(directory, filename, label) {
  if (typeof filename !== 'string' || filename !== path.basename(filename) || path.extname(filename).toLowerCase() !== '.png') {
    fail(`${label} has an unsafe or non-PNG screenshot path.`);
  }
  const file = path.join(directory, filename);
  if (!fs.existsSync(file) || !fs.statSync(file).isFile()) fail(`${label} is missing screenshot ${filename}.`);
  const descriptor = fs.openSync(file, 'r');
  const header = Buffer.alloc(pngSignature.length);
  try {
    if (fs.readSync(descriptor, header, 0, header.length, 0) !== header.length || !header.equals(pngSignature)) {
      fail(`${label} screenshot ${filename} is not a valid PNG file.`);
    }
  } finally {
    fs.closeSync(descriptor);
  }
  return file;
}

function eligible(scenario, width) {
  return !(scenario.minWidth && width < scenario.minWidth
    || scenario.maxWidth && width > scenario.maxWidth
    || scenario.supportedWidths && !scenario.supportedWidths.includes(width));
}

function assertExactKeySet(actual, expected, label) {
  const actualSet = unique(actual, `${label} keys`);
  const expectedSet = unique(expected, 'expected keys');
  const missing = [...expectedSet].filter(key => !actualSet.has(key));
  const extra = [...actualSet].filter(key => !expectedSet.has(key));
  if (missing.length || extra.length) {
    fail(`${label} coverage differs from expected (missing ${missing.length}, extra ${extra.length}).`);
  }
}

if (!fs.existsSync(finalDirectory) || !fs.statSync(finalDirectory).isDirectory()) {
  fail(`missing final evidence directory ${path.relative(root, finalDirectory)}.`);
}

const expected = readJson(finalDirectory, 'expected.json');
const manifest = readJson(finalDirectory, 'manifest.json');
const receipt = readJson(finalDirectory, 'receipt.json');
const widths = list(expected.widths, 'expected widths');
const expectedChecks = list(expected.checks, 'expected checks');
const checks = list(receipt.checks, 'receipt checks');
const scenarios = list(manifest.scenarios, 'manifest scenarios');
const skipped = list(manifest.skipped, 'manifest skipped list');

if (!expected.sourceHash || !receipt.before?.sourceHash || !receipt.after?.sourceHash || !manifest.before?.sourceHash) {
  fail('source identity is incomplete.');
}
if (expected.sourceHash !== receipt.before.sourceHash
  || expected.sourceHash !== receipt.after.sourceHash
  || expected.sourceHash !== manifest.before.sourceHash) {
  fail('source hash drifted between expected inventory, manifest, and receipt.');
}
if (receipt.before.commit !== receipt.after.commit || receipt.before.commit !== manifest.before.commit) {
  fail('Git identity changed during the final interaction run.');
}
if (!widths.length || widths.some(width => !Number.isInteger(width))) fail('expected widths are empty or invalid.');
unique(widths, 'expected widths');
equalArray(list(manifest.widths, 'manifest widths'), widths, 'manifest widths');
if (manifest.writes !== 'Every non-GET request is intercepted; no backend mutation is permitted.') {
  fail('manifest does not declare the required non-GET interception boundary.');
}

for (const check of expectedChecks) {
  if (check.key !== checkKey(check) || !check.name || !check.group || !check.route || !widths.includes(check.width)) {
    fail(`invalid expected check ${JSON.stringify(check.key)}.`);
  }
}
assertExactKeySet(expectedChecks.map(check => check.key), expectedChecks.map(checkKey), 'expected');

const manifestScenarioKeys = unique(scenarios.map(scenarioKey), 'manifest scenarios');
for (const scenario of scenarios) {
  if (!scenario.name || !scenario.group || !scenario.route) fail('manifest contains an invalid scenario.');
  if (scenario.supportedWidths && !Array.isArray(scenario.supportedWidths)) fail(`${scenarioKey(scenario)} has invalid supportedWidths.`);
}
const manifestEligible = widths.flatMap(width => scenarios
  .filter(scenario => eligible(scenario, width))
  .map(scenario => checkKey({ ...scenario, width })));
assertExactKeySet(manifestEligible, expectedChecks.map(check => check.key), 'manifest eligible matrix');

const manifestRoutes = new Map(scenarios.map(scenario => [scenarioKey(scenario), scenario.route]));
for (const check of expectedChecks) {
  if (!manifestScenarioKeys.has(scenarioKey(check)) || manifestRoutes.get(scenarioKey(check)) !== check.route) {
    fail(`expected metadata differs from manifest for ${check.key}.`);
  }
}
const manifestSkipped = skipped.map(item => checkKey(item));
const expectedSkipped = widths.flatMap(width => scenarios
  .filter(scenario => !eligible(scenario, width))
  .map(scenario => checkKey({ ...scenario, width })));
assertExactKeySet(manifestSkipped, expectedSkipped, 'manifest skipped matrix');

assertExactKeySet(checks.map(checkKey), expectedChecks.map(check => check.key), 'receipt');
const expectedByKey = new Map(expectedChecks.map(check => [check.key, check]));
const screenshotNames = [];
for (const check of checks) {
  const key = checkKey(check);
  const planned = expectedByKey.get(key);
  if (!planned || check.route !== planned.route) fail(`receipt metadata differs from expected for ${key}.`);
  if (check.failure) fail(`${key} failed: ${String(check.failure).split('\n')[0]}`);
  if (!Array.isArray(check.errors) || check.errors.length) fail(`${key} recorded browser runtime errors.`);
  if (!Array.isArray(check.steps) || !Array.isArray(check.attempts) || !Array.isArray(check.fixtures)
    || !Array.isArray(check.nativeDialogs) || !Array.isArray(check.controls)) {
    fail(`${key} has incomplete interaction evidence.`);
  }
  if (check.attempts.some(attempt => attempt?.intercepted !== true)) fail(`${key} contains an unguarded write attempt.`);
  if (!Number.isInteger(check.fixtureResponses) || check.fixtureResponses !== check.fixtures.length) {
    fail(`${key} has inconsistent fixture-response evidence.`);
  }
  validatePng(finalDirectory, check.screenshot, key);
  screenshotNames.push(check.screenshot);
}
unique(screenshotNames, 'final screenshots');

const touchManifest = readJson(touchDirectory, 'manifest.json');
const touchReceipt = readJson(touchDirectory, 'receipt.json');
const touchWidths = list(touchManifest.widths, 'touch manifest widths');
const touchScenarios = list(touchManifest.scenarios, 'touch manifest scenarios');
const touchChecks = list(touchReceipt.checks, 'touch receipt checks');
equalArray(touchWidths, [390, 834], 'touch manifest widths');
if (touchScenarios.length !== 4) fail('touch manifest must contain exactly four scenarios.');
unique(touchScenarios.map(scenario => scenario.name), 'touch manifest scenarios');
for (const scenario of touchScenarios) {
  if (!scenario.name || !scenario.route) fail('touch manifest contains an invalid scenario.');
}
if (touchManifest.writes !== 'Every non-GET request is intercepted; no backend mutation is permitted.'
  || touchManifest.input !== 'Navigation, buttons, checkboxes and radios use Playwright touch input; text and select values use standard form automation.') {
  fail('touch manifest does not declare the required input and interception boundaries.');
}
if (touchManifest.before?.sourceHash !== expected.sourceHash
  || touchReceipt.before?.sourceHash !== expected.sourceHash
  || touchReceipt.after?.sourceHash !== expected.sourceHash) {
  fail('touch evidence does not match the final frontend source hash.');
}
if (touchManifest.before?.commit !== touchReceipt.before?.commit || touchReceipt.before?.commit !== touchReceipt.after?.commit) {
  fail('Git identity changed during the touch interaction run.');
}
const touchKey = value => `${value.width}|${value.name}`;
const expectedTouchKeys = touchWidths.flatMap(width => touchScenarios.map(scenario => touchKey({ ...scenario, width })));
assertExactKeySet(touchChecks.map(touchKey), expectedTouchKeys, 'touch receipt');
const touchRoutes = new Map(touchScenarios.map(scenario => [scenario.name, scenario.route]));
const touchScreenshots = [];
for (const check of touchChecks) {
  const key = touchKey(check);
  if (touchRoutes.get(check.name) !== check.route) fail(`touch receipt metadata differs from manifest for ${key}.`);
  if (check.failure) fail(`touch ${key} failed: ${String(check.failure).split('\n')[0]}`);
  if (!Array.isArray(check.errors) || check.errors.length) fail(`touch ${key} recorded browser runtime errors.`);
  if (!Array.isArray(check.steps) || !Array.isArray(check.attempts) || !Array.isArray(check.fixtures)) {
    fail(`touch ${key} has incomplete interaction evidence.`);
  }
  if (check.attempts.some(attempt => attempt?.intercepted !== true)) fail(`touch ${key} contains an unguarded write attempt.`);
  validatePng(touchDirectory, check.screenshot, `touch ${key}`);
  touchScreenshots.push(check.screenshot);
}
unique(touchScreenshots, 'touch screenshots');

const galleryChecks = [
  ...checks.map(check => ({ ...check, sourceDirectory: finalDirectory })),
  ...touchChecks.map(check => ({
    ...check,
    group: 'Touch',
    controls: [],
    fixtureResponses: check.fixtures.length,
    nativeDialogs: [],
    sourceDirectory: touchDirectory
  }))
];

function historicalFailures() {
  const preferred = fs.readdirSync(evidenceRoot, { withFileTypes: true })
    .filter(entry => entry.isDirectory() && (entry.name.startsWith('red-') || ['bindings-fixed', 'green-second', 'matrix-first', 'recovery-third'].includes(entry.name)))
    .map(entry => entry.name)
    .sort((a, b) => {
      const rank = name => name.startsWith('red-') ? 0 : name === 'bindings-fixed' ? 1 : name === 'green-second' ? 2 : name === 'matrix-first' ? 3 : 4;
      return rank(a) - rank(b) || a.localeCompare(b);
    });
  const failures = new Map();
  for (const batch of preferred) {
    const directory = path.join(evidenceRoot, batch);
    const receiptFile = path.join(directory, 'receipt.json');
    if (!fs.existsSync(receiptFile)) continue;
    let previous;
    try { previous = JSON.parse(fs.readFileSync(receiptFile, 'utf8')); } catch { continue; }
    for (const check of Array.isArray(previous.checks) ? previous.checks : []) {
      if (!check.failure || failures.has(checkKey(check))) continue;
      let screenshot;
      try { screenshot = validatePng(directory, check.screenshot, `${batch} ${checkKey(check)}`); } catch { screenshot = undefined; }
      failures.set(checkKey(check), { batch, directory, screenshot, filename: check.screenshot, failure: check.failure });
    }
  }
  return failures;
}

const failures = historicalFailures();
const temporary = requireGeneratedPath(outputDirectory + '.building', 'temporary gallery path');
requireGeneratedPath(outputDirectory, 'gallery output path');
fs.rmSync(temporary, { recursive: true, force: true });
fs.mkdirSync(path.join(temporary, 'screens'), { recursive: true });
fs.mkdirSync(path.join(temporary, 'before'), { recursive: true });

const widthOptions = [...new Set(galleryChecks.map(check => check.width))].sort((a, b) => a - b);
const groupOptions = [...new Set(galleryChecks.map(check => check.group))].sort();
let totalSteps = 0, totalAttempts = 0, totalFixtures = 0, totalControls = 0, pairedFailures = 0;
const cards = galleryChecks
  .slice()
  .sort((a, b) => a.width - b.width || a.group.localeCompare(b.group) || a.name.localeCompare(b.name))
  .map((check, index) => {
    totalSteps += check.steps.length;
    totalAttempts += check.attempts.length;
    totalFixtures += Number(check.fixtureResponses || 0);
    totalControls += check.controls.length;
    const finalName = `${String(index + 1).padStart(3, '0')}-${path.basename(check.screenshot)}`;
    fs.copyFileSync(path.join(check.sourceDirectory, check.screenshot), path.join(temporary, 'screens', finalName));
    const before = failures.get(checkKey(check));
    let beforeMarkup = '';
    if (before) {
      pairedFailures++;
      let beforeImage = '';
      if (before.screenshot) {
        const beforeName = `${String(index + 1).padStart(3, '0')}-${before.batch}-${path.basename(before.filename)}`;
        fs.copyFileSync(before.screenshot, path.join(temporary, 'before', beforeName));
        beforeImage = `<a class="shot" href="before/${encodeURIComponent(beforeName)}"><img loading="lazy" src="before/${encodeURIComponent(beforeName)}" alt="Initial failed state for ${escapeHtml(check.name)} at ${check.width}px"></a>`;
      }
      beforeMarkup = `<details class="before"><summary>Earlier failed diagnostic · ${escapeHtml(before.batch)}</summary>${beforeImage}<pre>${escapeHtml(String(before.failure).slice(0, 1200))}</pre></details>`;
    }
    const steps = check.steps.length
      ? `<ol>${check.steps.map(step => `<li>${escapeHtml(step)}</li>`).join('')}</ol>`
      : '<p>No named intermediate steps; the scenario completion and final state were asserted.</p>';
    return `<article class="card" data-width="${check.width}" data-group="${escapeHtml(check.group)}">
      <header><div><p class="group">${escapeHtml(check.group)}</p><h2>${escapeHtml(check.name)}</h2><p class="route">${escapeHtml(check.route)}</p></div><span class="pass">Passed</span></header>
      <a class="shot" href="screens/${encodeURIComponent(finalName)}"><img loading="lazy" src="screens/${encodeURIComponent(finalName)}" alt="Final state for ${escapeHtml(check.name)} at ${check.width}px"></a>
      <div class="metrics"><span>${check.width}px</span><span>${check.steps.length} steps</span><span>${check.controls.length} visible controls</span><span>${check.attempts.length} intercepted writes</span><span>${Number(check.fixtureResponses || 0)} fixture responses</span></div>
      ${beforeMarkup}
      <details><summary>Recorded interaction steps</summary>${steps}</details>
    </article>`;
  }).join('\n');

const sourceShort = expected.sourceHash.slice(0, 12);
const generated = new Date().toISOString();
const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs interaction pass</title>
<style>:root{color-scheme:light;--ink:#20211f;--muted:#696b66;--line:#d9dad6;--surface:#fff;--wash:#f4f4f1;--accent:#176b55;--danger:#9d331d}*{box-sizing:border-box}body{margin:0;background:var(--wash);color:var(--ink);font:15px/1.5 system-ui,-apple-system,sans-serif}main{max-width:1680px;margin:auto;padding:28px}h1{font-size:32px;letter-spacing:-.03em;margin:4px 0 8px}.eyebrow,.intro,.route,.group{color:var(--muted)}.intro{max-width:92ch}.notice{max-width:105ch;border-left:3px solid var(--accent);padding:10px 14px;background:var(--surface)}.limits{border-color:#ad641b}.summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:10px;margin:22px 0}.stat{background:var(--surface);border:1px solid var(--line);border-radius:12px;padding:14px}.stat strong{display:block;font-size:24px}.stat span{color:var(--muted);font-size:12px}.controls{position:sticky;top:0;z-index:3;display:flex;gap:8px;flex-wrap:wrap;padding:12px 0;background:color-mix(in srgb,var(--wash) 92%,transparent);backdrop-filter:blur(12px);border-bottom:1px solid var(--line)}button{min-height:42px;padding:0 13px;border:1px solid var(--line);border-radius:9px;background:var(--surface);font:inherit;cursor:pointer}button[aria-pressed=true]{border-color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent);color:var(--accent)}.count{margin-left:auto;align-self:center;color:var(--muted)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:18px;margin-top:18px}.card{min-width:0;background:var(--surface);border:1px solid var(--line);border-radius:14px;padding:14px;box-shadow:0 1px 2px #1820180d}.card[hidden]{display:none}.card header{display:flex;justify-content:space-between;gap:12px}.card h2{font-size:17px;line-height:1.3;margin:2px 0}.group{text-transform:uppercase;letter-spacing:.08em;font-size:11px;margin:0}.route{font-size:12px;margin:4px 0;overflow-wrap:anywhere}.pass{height:fit-content;border-radius:999px;padding:3px 8px;background:#eaf7f1;color:var(--accent);font-size:12px}.shot{display:block;height:340px;margin:12px 0;border:1px solid var(--line);border-radius:9px;overflow:hidden;background:#eee}.shot img{display:block;width:100%;height:auto;object-position:top}.metrics{display:flex;gap:6px;flex-wrap:wrap}.metrics span{font-size:11px;border:1px solid var(--line);border-radius:999px;padding:3px 7px}.before{margin-top:12px;border-left:3px solid var(--danger);padding-left:10px}.before .shot{height:240px}.before pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:11px;color:var(--muted);max-height:170px;overflow:auto}details>summary{min-height:44px;align-content:center;cursor:pointer}ol{padding-left:22px}.empty{display:none;text-align:center;padding:50px;color:var(--muted)}a{color:#175b92}@media(max-width:700px){main{padding:18px}.controls{position:static}.count{width:100%;margin:0}.card header{display:block}.pass{display:inline-block}.shot{height:280px}}</style></head><body><main>
<p class="eyebrow">Preview 18746 · final interaction evidence · generated ${escapeHtml(generated)}</p><h1>Buttons, forms, menus, and interaction pass</h1>
<p class="intro">Every eligible scenario in the final runner manifest is represented once at each declared phone, tablet, or desktop width. A separate touch matrix adds four primary touch workflows at 390px and 834px. The gallery was generated only after matching both manifests, receipts, screenshots, and frozen frontend source identity.</p>
<p class="notice">Final frontend source <code>${escapeHtml(sourceShort)}</code> · all ${checks.length} main checks and ${touchChecks.length} touch checks passed with no recorded browser runtime errors or frontend-source drift.</p>
<section class="summary"><div class="stat"><strong>${galleryChecks.length}</strong><span>checks</span></div><div class="stat"><strong>${totalSteps}</strong><span>named steps</span></div><div class="stat"><strong>${totalControls}</strong><span>visible controls recorded</span></div><div class="stat"><strong>${totalAttempts}</strong><span>intercepted write attempts</span></div><div class="stat"><strong>${totalFixtures}</strong><span>fixture responses</span></div><div class="stat"><strong>${pairedFailures}</strong><span>before/after failure pairs</span></div></section>
<p class="notice limits"><strong>Proof limits.</strong> Non-GET requests were intercepted, so these checks validate frontend behavior against controlled success and failure contracts rather than persisted backend writes. The source hash covers <code>web/app/src</code>; it does not bind backend or test-harness files. Screenshots and browser assertions do not certify exact licensed font rendering, frame-by-frame motion timing, every possible data shape, WebSocket behavior, exhaustive accessibility, the installed app, or a combined future integration build.</p>
<p><a href="/">Open preview</a> · Raw evidence: <code>.omx/artifacts/visual-ralph/intercom/interaction-pass/final</code> and <code>.omx/artifacts/visual-ralph/intercom/interaction-pass/touch</code></p>
<div class="controls" aria-label="Interaction evidence filters"><button type="button" data-width="all" aria-pressed="true">All widths</button>${widthOptions.map(width => `<button type="button" data-width="${width}" aria-pressed="false">${width}px</button>`).join('')}<button type="button" data-group="all" aria-pressed="true">All groups</button>${groupOptions.map(group => `<button type="button" data-group="${escapeHtml(group)}" aria-pressed="false">${escapeHtml(group)}</button>`).join('')}<span class="count" id="count"></span></div>
<section class="grid" id="gallery">${cards}</section><p class="empty" id="empty">No interaction checks match these filters.</p>
</main><script>let width='all',group='all';const cards=[...document.querySelectorAll('.card')],count=document.querySelector('#count'),empty=document.querySelector('#empty');function apply(){let shown=0;for(const card of cards){const visible=(width==='all'||card.dataset.width===width)&&(group==='all'||card.dataset.group===group);card.hidden=!visible;if(visible)shown++}count.textContent=shown+' of '+cards.length+' checks';empty.style.display=shown?'none':'block'}document.querySelectorAll('.controls button[data-width]').forEach(button=>button.addEventListener('click',()=>{width=button.dataset.width;document.querySelectorAll('.controls button[data-width]').forEach(item=>item.setAttribute('aria-pressed',String(item===button)));apply()}));document.querySelectorAll('.controls button[data-group]').forEach(button=>button.addEventListener('click',()=>{group=button.dataset.group;document.querySelectorAll('.controls button[data-group]').forEach(item=>item.setAttribute('aria-pressed',String(item===button)));apply()}));apply();</script></body></html>`;

fs.writeFileSync(path.join(temporary, 'index.html'), html);
fs.rmSync(outputDirectory, { recursive: true, force: true });
fs.mkdirSync(outputDirectory, { recursive: true });
fs.cpSync(temporary, outputDirectory, { recursive: true, force: true });
fs.rmSync(temporary, { recursive: true, force: true });
console.log(JSON.stringify({
  output: path.join(outputDirectory, 'index.html'),
  sourceHash: expected.sourceHash,
  checks: galleryChecks.length,
  mainChecks: checks.length,
  touchChecks: touchChecks.length,
  widths: widthOptions,
  groups: groupOptions,
  steps: totalSteps,
  visibleControls: totalControls,
  interceptedWrites: totalAttempts,
  fixtureResponses: totalFixtures,
  beforeAfterPairs: pairedFailures
}, null, 2));
