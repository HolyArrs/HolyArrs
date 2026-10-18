// Local, ignored review gallery. No product code, credentials or reference ZIPs are exported.
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const destination = path.join(root, 'web/app/static/preview-audit/consolidated-20261017');
const artifactRoot = path.join(root, '.omx/artifacts/visual-ralph/intercom');
const coverage = require('./summarize-final-audit-20261017.cjs').build();
if (coverage.status !== 'covered-checks-clear-with-limitations') throw Error('Final coverage is incomplete; review receipts before generating the gallery');
const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const sources = [
  ['Responsive', 'responsive-pass/consolidated-final-v2-20261017'],
  ['Interaction', 'interaction-pass/consolidated-final-20261017'],
  ['Touch', 'interaction-pass/touch'],
  ['Environment', 'audit20261017/env-v2'],
  ['Comparison', 'audit20261017/comparison']
];
fs.mkdirSync(destination, { recursive: true });
const cards = [], receipts = [];
for (const [group, folder] of sources) {
  const directory = path.join(artifactRoot, folder), receiptFile = path.join(directory, 'receipt.json');
  if (!fs.existsSync(receiptFile)) throw Error('Missing required receipt: ' + receiptFile);
  const receipt = JSON.parse(fs.readFileSync(receiptFile, 'utf8'));
  if(group === 'Comparison' && receipt.captureComplete !== true) throw Error('Comparison captures are incomplete');
  if(receipt.before?.sourceHash !== receipt.after?.sourceHash) throw Error('Source changed during '+group);
  if((receipt.checks||[]).some(c=>c.failure||c.errors?.length||c.overflow||c.violations?.length)) throw Error('Failed required checks in '+group);
  if (!(receipt.checks || []).some(check => check.screenshot)) throw Error('No screenshots in required receipt: ' + receiptFile);
  receipts.push({ group, before: receipt.before, after: receipt.after, count: receipt.checks?.length });
  for (const [index, check] of (receipt.checks || []).entries()) {
    if (!check.screenshot) continue;
    const file = path.join(directory, check.screenshot);
    if (!fs.existsSync(file)) throw Error('Missing evidence screenshot: ' + file);
    const target = `${group.toLowerCase()}-${index}.png`;
    fs.copyFileSync(file, path.join(destination, target));
    const findings = check.failure || ((check.overflow || check.geometry?.overflow || check.errors?.length || check.violations?.length || check.clipped?.length || check.outside?.length || check.overlaps?.length || check.geometry?.clipped?.length || check.geometry?.outside?.length || check.geometry?.overlaps?.length) ? 'Inspect receipt findings' : 'Automated checks clear; visual scope recorded separately');
    cards.push(`<article data-group="${group}" data-width="${check.width}" data-search="${escape([check.route, check.name, check.state, group].join(' ').toLowerCase())}"><h2>${escape(group)} · ${escape(check.width)} CSS px</h2><p>${escape(check.route)} ${escape(check.state || check.name || '')}</p><a href="${target}" target="_blank"><img loading="lazy" src="${target}" alt="${escape(check.route || check.name)} at ${escape(check.width)} CSS pixels"></a><p>${escape(findings)}</p></article>`);
  }
}
fs.writeFileSync(path.join(destination, 'index.html'), `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs measured Intercom audit</title><style>body{margin:0;background:#efefeb;color:#1f201e;font:15px/1.5 system-ui}header{padding:24px;position:sticky;top:0;background:#fafbf8;border-bottom:1px solid #ccc;z-index:1}h1{font-size:24px;margin:0}h2{font-size:16px}p{max-width:90ch}input,select{font:inherit;padding:10px;max-width:100%;box-sizing:border-box}main{padding:20px;display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr));gap:20px}article{background:white;padding:16px;border:1px solid #ddd;border-radius:14px;min-width:0}article p{overflow-wrap:anywhere}img{width:100%;height:350px;object-fit:contain;object-position:top;background:#eee}article[hidden]{display:none}a:focus-visible,input:focus-visible,select:focus-visible{outline:3px solid #2364b5;outline-offset:3px}label{display:inline-block;margin:5px 12px 5px 0}details{margin-top:12px}pre{white-space:pre-wrap;overflow-wrap:anywhere}</style><header><h1>HolyArrs · measured Intercom audit</h1><p>Preview 18746 only. Screenshots certify their recorded source and scenario, not every possible state. Source font identity, timed Intercom motion and native-device parity remain unverified. Browser fixtures are distinguished in the durable report. No production or installed-app acceptance is claimed.</p><label>Find <input id="find" type="search" placeholder="Route or scenario"></label><label>Group <select id="group"><option>All</option>${sources.map(([g]) => `<option>${g}</option>`).join('')}</select></label><label>CSS width <select id="width"><option>All</option>${[...new Set(cards.map(c => /data-width="([^"]+)/.exec(c)[1]))].sort((a,b)=>Number(a)-Number(b)).map(w=>`<option>${escape(w)}</option>`).join('')}</select></label><output id="count" aria-live="polite"></output><details><summary>Receipt identities</summary><pre>${escape(JSON.stringify(receipts, null, 2))}</pre></details></header><main>${cards.join('\n')}</main><script>const find=document.getElementById('find'),group=document.getElementById('group'),width=document.getElementById('width'),count=document.getElementById('count'),cards=[...document.querySelectorAll('article')];function filter(){let n=0;for(const c of cards){c.hidden=!(c.dataset.search.includes(find.value.toLowerCase())&&(group.value==='All'||c.dataset.group===group.value)&&(width.value==='All'||c.dataset.width===width.value));if(!c.hidden)n++}count.textContent=n+' captures'}for(const id of ['find','group','width'])document.getElementById(id).addEventListener('input',filter);filter()</script></html>`);
console.log(JSON.stringify({ gallery: destination, cards: cards.length, groups: receipts }));
const pairs = [
  ['Desktop Library · light · 1920×1192', 'responsive-pass/consolidated-before-light-20261017', '1920-_library.png', 'audit20261017/comparison', 'light-library.png', 'Context sidebar now uses its own surface token.'],
  ['Desktop Library · dark · 1920×1192', 'responsive-pass/consolidated-before-dark-20261017', '1920-_library.png', 'audit20261017/comparison', 'dark-library.png', 'Dark sidebar is separated from the workspace surface.'],
  ['Home · tablet · 768×900', 'responsive-pass/consolidated-final-20261017', '768-_.png', 'responsive-pass/consolidated-final-v2-20261017', '768-_.png', 'Activity sections stack to avoid an empty column beside long events.'],
  ['Home · compact desktop · 1280×900', 'responsive-pass/home-stack-after-20261017', '1280-_.png', 'responsive-pass/consolidated-final-v2-20261017', '1280-_.png', 'Health moves below Activity so event text retains usable width.']
];
const pairCards = pairs.map(([label,beforeFolder,beforeName,afterFolder,afterName,note],i)=>{
  const identities=[];
  const figures=[[beforeFolder,beforeName,'Before'],[afterFolder,afterName,'After']].map(([folder,name,stage],j)=>{
    const file=path.join(artifactRoot,folder,name),receipt=JSON.parse(fs.readFileSync(path.join(artifactRoot,folder,'receipt.json'),'utf8'));
    if(!fs.existsSync(file))throw Error('Missing before/after capture '+file);
    identities.push({stage,folder,source:receipt.before});
    const target=`before-after-${i}-${j}.png`;fs.copyFileSync(file,path.join(destination,target));
    return `<figure><figcaption>${stage}</figcaption><a href="${target}" target="_blank"><img src="${target}" alt="${escape(stage+' '+label)}"></a></figure>`;
  });
  return `<section><h2>${escape(label)}</h2><p>${escape(note)}</p><div class="pair">${figures.join('')}</div><details><summary>Capture identities</summary><pre>${escape(JSON.stringify(identities,null,2))}</pre></details></section>`;
});
fs.writeFileSync(path.join(destination,'before-after.html'),`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs before and after</title><style>body{margin:24px;background:#efefeb;color:#20211f;font:16px/1.5 system-ui}section{background:white;border:1px solid #ddd;border-radius:14px;padding:20px;margin:24px 0}.pair{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}figure{margin:0;min-width:0}img{width:100%;height:600px;object-fit:contain;object-position:top;background:#eee}pre{white-space:pre-wrap;overflow-wrap:anywhere}@media(max-width:767px){.pair{grid-template-columns:1fr}}a:focus-visible{outline:3px solid #2364b5}</style><h1>Preview changes · before and after</h1><p>Matching CSS viewports, DPR 1. Before captures intentionally show older source identities; only After belongs to the final frontend hash. Click any image for its full capture.</p><a href="index.html">All audit captures</a>${pairCards.join('')}</html>`);
