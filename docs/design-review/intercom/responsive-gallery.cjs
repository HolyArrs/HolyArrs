const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '../../..');
const evidenceRoot = path.join(root, '.omx/artifacts/visual-ralph/intercom/responsive-pass');
const batch = process.env.RESPONSIVE_GALLERY_BATCH || 'final';
if (!/^[a-z0-9][a-z0-9._-]*$/i.test(batch)) throw new Error(`Invalid responsive gallery batch ${JSON.stringify(batch)}.`);
const outputRoot = path.join(root, 'web/app/static/preview-audit/responsive-pass');
const outputBatch = path.join(outputRoot, batch);

function readReceipt(name) {
  const file = path.join(evidenceRoot, name, 'receipt.json');
  if (!fs.existsSync(file)) return null;
  return { name, file, directory: path.dirname(file), receipt: JSON.parse(fs.readFileSync(file, 'utf8')) };
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function safeAsset(sourceDirectory, file, destinationDirectory, prefix = '') {
  if (!file || path.extname(file).toLowerCase() !== '.png') return null;
  const source = path.resolve(sourceDirectory, file);
  const allowed = path.resolve(sourceDirectory) + path.sep;
  if (!source.startsWith(allowed) || !fs.existsSync(source)) return null;
  fs.mkdirSync(destinationDirectory, { recursive: true });
  const destinationName = `${prefix}${path.basename(file)}`;
  fs.copyFileSync(source, path.join(destinationDirectory, destinationName));
  return destinationName;
}

function list(value) {
  return Array.isArray(value) ? value : [];
}

function diagnostics(check) {
  const items = [];
  if (check.failure) items.push({ kind: 'Capture failure', detail: check.failure });
  if (check.overflow) items.push({ kind: 'Horizontal overflow', detail: `Document width ${check.pageWidth ?? 'unknown'}px` });
  for (const item of list(check.clipped)) items.push({ kind: 'Clipped content', detail: JSON.stringify(item) });
  for (const item of list(check.outside)) items.push({ kind: 'Outside viewport', detail: JSON.stringify(item) });
  for (const item of list(check.overlaps)) items.push({ kind: 'Overlapping controls', detail: JSON.stringify(item) });
  for (const item of list(check.errors)) items.push({ kind: 'Page error', detail: item });
  for (const item of list(check.violations)) items.push({ kind: `Accessibility: ${item.id || 'violation'}`, detail: JSON.stringify(item) });
  return items;
}

function key(check) {
  return `${check.width}|${check.route}|${check.state || ''}`;
}

const current = readReceipt(batch);
if (!current) throw new Error(`Responsive receipt not found for batch ${JSON.stringify(batch)}.`);
const baseline = batch === 'baseline' ? current : readReceipt('baseline');
const checks = list(current.receipt.checks);
if (!checks.length) throw new Error(`Responsive receipt for ${JSON.stringify(batch)} has no checks.`);
const manifestPath = path.join(current.directory, 'manifest.json');
const manifest = fs.existsSync(manifestPath) ? JSON.parse(fs.readFileSync(manifestPath, 'utf8')) : null;
function matrixKeys(routes, widths) { return widths.flatMap(width => routes.map(route => key({ ...route, width }))); }
const recordedKeys = checks.map(key);
const expectedKeys = manifest ? matrixKeys(list(manifest.selected), list(manifest.widths)) : [];
const captureSetValid = expectedKeys.length > 0 && new Set(recordedKeys).size === recordedKeys.length && expectedKeys.length === recordedKeys.length && expectedKeys.every(value => recordedKeys.includes(value));
const requiredWidths = [360, 834, 1440, 3440];
const fullKeys = manifest ? matrixKeys(list(manifest.allRoutes), requiredWidths) : [];
const fullCoverage = fullKeys.length > 0 && fullKeys.every(value => recordedKeys.includes(value));

fs.mkdirSync(outputBatch, { recursive: true });
const currentAssets = path.join(outputBatch, 'screens');
const baselineAssets = path.join(outputBatch, 'baseline');
const baselineChecks = new Map(list(baseline?.receipt.checks).map(check => [key(check), check]));
const stateBaseline = readReceipt('states-before');
for (const check of list(stateBaseline?.receipt.checks)) {
  if (!check.failure) baselineChecks.set(key(check), { ...check, sourceDirectory: stateBaseline.directory });
}
const previousIteration = readReceipt('iteration-two');
for (const check of list(previousIteration?.receipt.checks)) {
  if (!check.failure && ['/library/rename', '/settings/sign-in'].includes(check.route) && !baselineChecks.has(key(check))) {
    baselineChecks.set(key(check), { ...check, sourceDirectory: previousIteration.directory });
  }
}

const screens = checks.map((check, index) => {
  const screen = safeAsset(current.directory, check.screenshot, currentAssets);
  const bottom = safeAsset(current.directory, check.bottom, currentAssets);
  const viewport = safeAsset(current.directory, check.viewport, currentAssets);
  const before = baselineChecks.get(key(check));
  const baselineScreen = batch !== 'baseline' && baseline
    ? safeAsset(before?.sourceDirectory || baseline.directory, before?.screenshot, baselineAssets, 'before-')
    : null;
  const baselineBottom = batch !== 'baseline' && baseline
    ? safeAsset(before?.sourceDirectory || baseline.directory, before?.bottom, baselineAssets, 'before-')
    : null;
  const issues = diagnostics(check);
  return {
    id: `screen-${index + 1}`,
    route: check.route,
    template: check.template,
    state: check.state,
    fixture: Boolean(check.fixture),
    width: check.width,
    height: check.height,
    screen,
    bottom,
    viewport,
    baselineScreen,
    baselineBottom,
    issues,
    clean: issues.length === 0
  };
});

const widths = [...new Set(screens.map(screen => screen.width))].sort((a, b) => a - b);
const issueCount = screens.filter(screen => !screen.clean).length;
const captureFailures = screens.filter(screen => screen.issues.some(issue => issue.kind === 'Capture failure')).length;
const sourceStable = current.receipt.before?.sourceHash && current.receipt.before.sourceHash === current.receipt.after?.sourceHash;
const fullyValidated = issueCount === 0 && captureFailures === 0 && sourceStable && captureSetValid && (batch !== 'final' || fullCoverage);

function mediaLink(file, folder, label) {
  return file ? `<a class="media-link" href="${folder}/${encodeURIComponent(file)}">${escapeHtml(label)}</a>` : '';
}

const cards = screens.map(screen => {
  const issueMarkup = screen.issues.length
    ? `<ul class="diagnostics">${screen.issues.map(issue => `<li><strong>${escapeHtml(issue.kind)}</strong><code>${escapeHtml(issue.detail)}</code></li>`).join('')}</ul>`
    : '<p class="clean-note">No automated layout, page-error, or accessibility findings in this capture.</p>';
  const image = screen.screen
    ? `<a class="shot" href="screens/${encodeURIComponent(screen.screen)}"><img loading="lazy" src="screens/${encodeURIComponent(screen.screen)}" alt="${escapeHtml(screen.route)} at ${screen.width}px"></a>`
    : '<div class="missing">No screenshot was produced.</div>';
  const baselineLinks = [
    mediaLink(screen.baselineScreen, 'baseline', 'Baseline top'),
    mediaLink(screen.baselineBottom, 'baseline', 'Baseline bottom')
  ].filter(Boolean).join('');
  return `<article class="card" id="${screen.id}" data-width="${screen.width}" data-status="${screen.clean ? 'clean' : 'issue'}" data-search="${escapeHtml(`${screen.route} ${screen.template}`.toLowerCase())}">
    <header><div><p class="route">${escapeHtml(screen.route)}${screen.state ? ` · ${escapeHtml(screen.state)}` : ''}</p><p class="template">${escapeHtml(screen.template)}</p></div><div class="badges"><span>${screen.width}px viewport · ${screen.height}px page</span>${screen.fixture ? '<span class="fixture">Browser-only fixture</span>' : ''}<span class="status ${screen.clean ? 'ok' : 'warn'}">${screen.clean ? 'Clean' : `${screen.issues.length} finding${screen.issues.length === 1 ? '' : 's'}`}</span></div></header>
    ${image}
    <div class="links">${mediaLink(screen.screen, 'screens', 'Open full page')}${mediaLink(screen.viewport, 'screens', 'Open viewport')}${mediaLink(screen.bottom, 'screens', 'Open bottom')}${baselineLinks}</div>
    ${issueMarkup}
  </article>`;
}).join('');

const data = JSON.stringify(screens.map(({ id, route, template, width, clean, fixture }) => ({ id, route, template, width, clean, fixture }))).replaceAll('<', '\\u003c');
const comparisonRoutes = ['/downloads', '/downloads/history', '/settings/parts', '/settings/this-app', '/library/import', '/people/900001'];
const comparisons = batch === 'final' ? screens.filter(screen => !screen.state && screen.baselineScreen && (screen.width === 360 && comparisonRoutes.includes(screen.route) || screen.width === 1440 && ['/library/rename', '/settings/sign-in'].includes(screen.route))).map(screen =>
  `<article class="comparison"><h3>${escapeHtml(screen.route)} · ${screen.width}px</h3><div class="comparison-pair"><figure><figcaption>Before</figcaption><a href="baseline/${encodeURIComponent(screen.baselineScreen)}"><img loading="lazy" src="baseline/${encodeURIComponent(screen.baselineScreen)}" alt="Before: ${escapeHtml(screen.route)}"></a></figure><figure><figcaption>After</figcaption><a href="screens/${encodeURIComponent(screen.screen)}"><img loading="lazy" src="screens/${encodeURIComponent(screen.screen)}" alt="After: ${escapeHtml(screen.route)}"></a></figure></div></article>`
).join('') : '';
const summary = fullyValidated
  ? `All ${screens.length} captured screens passed the recorded automated checks and the source stayed unchanged during capture.${fullCoverage ? ' The complete route/state inventory is covered at all four required widths.' : ' This is a targeted or intermediate capture set.'}`
  : `${screens.length} screens recorded. ${issueCount} screen${issueCount === 1 ? '' : 's'} had diagnostics or capture failures${sourceStable ? '' : '; the source changed during capture'}${captureSetValid ? '' : '; capture inventory is incomplete or duplicated'}${batch === 'final' && !fullCoverage ? '; the full four-width matrix is incomplete' : ''}. Review the marked cards.`;

fs.writeFileSync(path.join(outputBatch, 'index.html'), `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs responsive review — ${escapeHtml(batch)}</title>
<style>
:root{color-scheme:light;--ink:#20211f;--muted:#666963;--line:#d9dbd6;--surface:#fff;--wash:#f5f5f2;--accent:#176b55;--warn:#9a4f12}*{box-sizing:border-box}body{margin:0;background:var(--wash);color:var(--ink);font:15px/1.5 system-ui,-apple-system,sans-serif}main{max-width:1680px;margin:auto;padding:28px}.eyebrow{color:var(--muted);font-size:13px;margin:0}h1{font-size:30px;letter-spacing:-.02em;margin:5px 0 8px}.intro{max-width:88ch;color:var(--muted)}nav,.controls,.links,.badges{display:flex;gap:8px;flex-wrap:wrap}a{color:#175b92}.controls{position:sticky;top:0;z-index:4;background:color-mix(in srgb,var(--wash) 92%,transparent);backdrop-filter:blur(12px);padding:12px 0;margin:20px 0;border-bottom:1px solid var(--line)}button,input{min-height:40px;border:1px solid var(--line);border-radius:9px;background:var(--surface);color:var(--ink);font:inherit}button{padding:0 14px;cursor:pointer}button[aria-pressed="true"]{border-color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent);color:var(--accent)}input{padding:0 12px;min-width:min(330px,100%)}.count{margin-left:auto;align-self:center;color:var(--muted)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr));gap:18px}.card{min-width:0;background:var(--surface);border:1px solid var(--line);border-radius:14px;padding:14px;box-shadow:0 1px 2px #1820180d}.card[hidden]{display:none}.card header{display:flex;justify-content:space-between;gap:12px;align-items:flex-start}.route{font-weight:650;margin:0;overflow-wrap:anywhere}.template{color:var(--muted);font-size:12px;margin:2px 0 0}.badges{justify-content:flex-end}.badges span,.media-link{font-size:12px;border:1px solid var(--line);border-radius:999px;padding:3px 8px;text-decoration:none}.fixture{background:#f1eefc}.status.ok{color:var(--accent);background:#edf8f3}.status.warn{color:var(--warn);background:#fff4e8}.shot{display:block;margin:12px 0;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:#eee}.shot{height:360px}.shot img{display:block;width:100%;height:auto;object-position:top;background:#fafafa}.missing{margin:12px 0;padding:44px 16px;border:1px dashed var(--line);border-radius:10px;text-align:center;color:var(--warn)}.diagnostics{padding-left:20px}.diagnostics li{margin:8px 0}.diagnostics code{display:block;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--muted);font-size:11px}.clean-note{font-size:12px;color:var(--muted)}.notice{border-left:3px solid ${fullyValidated ? 'var(--accent)' : 'var(--warn)'};padding:9px 12px;background:var(--surface);max-width:100ch}.empty{display:none;text-align:center;padding:60px;color:var(--muted)}@media(max-width:700px){main{padding:18px}.controls{position:static}.count{width:100%;margin:0}.card header{display:block}.badges{justify-content:flex-start;margin-top:8px}.shot{height:280px}}
.comparisons{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,480px),1fr));gap:20px;margin:20px 0}.comparison{background:var(--surface);border:1px solid var(--line);border-radius:12px;padding:12px}.comparison h3{font-size:14px;overflow-wrap:anywhere}.comparison-pair{display:grid;grid-template-columns:1fr 1fr;gap:10px}.comparison figure{margin:0;min-width:0}.comparison figcaption{font-size:12px;color:var(--muted);margin-bottom:6px}.comparison a{display:block;height:400px;overflow:hidden;border:1px solid var(--line);border-radius:8px}.comparison img{display:block;width:100%;height:auto}details>summary{cursor:pointer;min-height:44px;align-content:center}
</style></head><body><main>
<p class="eyebrow">Preview 18746 · responsive batch ${escapeHtml(batch)}</p><h1>Every-screen responsive review</h1>
<p class="intro">Actual HolyArrs preview captures across phone, tablet, desktop and ultrawide widths. Browser-only fixtures are marked; magenta blocks mask sensitive fields in screenshots. Automated diagnostics cover overflow, content outside the viewport, clipping, control overlap, page errors and recorded accessibility violations.</p>
<p class="notice">${escapeHtml(summary)}</p>
<nav><a href="/">Live preview</a><a href="/preview-audit/typography-motion/index.html">Typography and motion review</a>${baseline && batch !== 'baseline' ? '<a href="../baseline/index.html">Baseline gallery</a>' : ''}</nav>
${comparisons ? `<details><summary>Before and after: representative phone and desktop repairs</summary><p class="intro">Phone pairs use the initial or corrected baseline. Desktop pairs show the final Rename and Passwords spacing fixes. Select an image to inspect its full size.</p><section class="comparisons">${comparisons}</section></details>` : ''}
<div class="controls" aria-label="Gallery filters"><button type="button" data-width="all" aria-pressed="true">All viewports</button>${widths.map(width => `<button type="button" data-width="${width}" aria-pressed="false">${width}px</button>`).join('')}<button type="button" data-status="issue" aria-pressed="false">Findings only</button><input id="search" type="search" aria-label="Filter routes" placeholder="Filter route or template"><span class="count" id="count"></span></div>
<section class="grid" id="gallery">${cards}</section><p class="empty" id="empty">No screens match these filters.</p>
<p class="intro">Captured from the disposable preview with isolated browser contexts. Screenshot review can reveal visual problems but does not certify every interaction, every data shape, licensed Intercom font parity, or the installed application.</p>
</main><script>const screens=${data};let width='all',issuesOnly=false;const cards=[...document.querySelectorAll('.card')],search=document.querySelector('#search'),count=document.querySelector('#count'),empty=document.querySelector('#empty');function apply(){const q=search.value.trim().toLowerCase();let shown=0;for(const card of cards){const item=screens.find(x=>x.id===card.id);const visible=(width==='all'||String(item.width)===width)&&(!issuesOnly||!item.clean)&&(!q||card.dataset.search.includes(q));card.hidden=!visible;if(visible)shown++}count.textContent=shown+' of '+cards.length+' screens';empty.style.display=shown?'none':'block'}document.querySelectorAll('button[data-width]').forEach(button=>button.addEventListener('click',()=>{width=button.dataset.width;document.querySelectorAll('button[data-width]').forEach(item=>item.setAttribute('aria-pressed',String(item===button)));apply()}));document.querySelector('[data-status]').addEventListener('click',event=>{issuesOnly=!issuesOnly;event.currentTarget.setAttribute('aria-pressed',String(issuesOnly));apply()});search.addEventListener('input',apply);apply();</script></body></html>`);

// Keep a standalone baseline link available after the final gallery is generated.
if (batch !== 'baseline' && baseline && !fs.existsSync(path.join(outputRoot, 'baseline', 'index.html'))) {
  console.warn('Baseline receipt is available, but its gallery has not been generated. Run with RESPONSIVE_GALLERY_BATCH=baseline first for the baseline link.');
}

console.log(JSON.stringify({ batch, output: path.join(outputBatch, 'index.html'), screens: screens.length, findings: issueCount, sourceStable, captureSetValid, fullCoverage, fullyValidated }, null, 2));
if (batch === 'final' && !fullyValidated) process.exitCode = 1;
