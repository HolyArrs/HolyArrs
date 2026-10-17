const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const evidence = path.join(root, '.omx/artifacts/visual-ralph/intercom/typography-motion');
const out = path.join(root, 'web/app/static/preview-audit/typography-motion');
fs.mkdirSync(out, { recursive: true });
for (const name of ['library-before-after-overlay.png', 'library-before-after-diff.png']) fs.copyFileSync(path.join(evidence, name), path.join(out, name));
let sections = '';
for (const width of [1440, 834, 390]) {
  let panels = '';
  for (const state of ['library', 'dialog']) {
    panels += `<h3>${state === 'library' ? 'Library typography' : 'Dialog typography'}</h3><div class="pair">`;
    for (const batch of ['before', 'after']) {
      const file = `${width}-no-preference-${state}.png`;
      const name = `${batch}-${file}`;
      fs.copyFileSync(path.join(evidence, batch === 'after' ? 'final-delivery' : batch, file), path.join(out, name));
      panels += `<figure><figcaption>${batch}</figcaption><a href="${name}"><img loading="lazy" src="${name}" alt="${batch} ${state} at ${width}px"></a></figure>`;
    }
    panels += '</div>';
  }
  panels += '<h3>Recorded interactions</h3><p>Library, dialog opening and Escape, focus return, then temporary browser-only diagnostic messages. Native dialog close remains immediate.</p><div class="pair">';
  for (const batch of ['before', 'after']) {
    const name = `${batch}-${width}-no-preference.webm`;
    fs.copyFileSync(path.join(evidence, batch === 'after' ? 'final-delivery' : batch, `${width}-no-preference.webm`), path.join(out, name));
    panels += `<figure><figcaption>${batch} · normal motion</figcaption><video controls preload="none" src="${name}"></video></figure>`;
  }
  const reduced = `after-${width}-reduce.webm`;
  fs.copyFileSync(path.join(evidence, 'final-delivery', `${width}-reduce.webm`), path.join(out, reduced));
  panels += `</div><details><summary>Updated reduced-motion recording</summary><video controls preload="none" src="${reduced}"></video></details>`;
  sections += `<section id="w${width}"><h2>${width}px ${width === 1440 ? 'desktop' : width === 834 ? 'tablet' : 'phone'}</h2>${panels}</section>`;
}
fs.writeFileSync(path.join(out, 'index.html'), `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>HolyArrs typography and motion</title><style>body{margin:0;background:#f5f5f2;color:#20211f;font:16px/1.5 system-ui,sans-serif}main{max-width:1500px;margin:auto;padding:24px}nav{display:flex;gap:20px;flex-wrap:wrap}a{color:#205f9c}section{margin-top:48px;border-top:1px solid #ccc;padding-top:20px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:20px}figure{margin:0;min-width:0}figcaption{text-transform:capitalize;font-weight:600;margin-bottom:8px}img,video{max-width:100%;height:auto;border:1px solid #ddd;border-radius:12px}video{max-height:700px;background:#fff}details{margin-top:24px}summary{cursor:pointer;padding:12px}h1{font-size:28px}p{max-width:85ch}@media(max-width:720px){.pair{grid-template-columns:1fr}main{padding:16px}}</style><main><h1>Typography and motion — preview18746</h1><p>Actual preview application, before and after. Larger body and navigation text, calmer hierarchy, shared timing, stationary list reveals, short dialog entrances and message fades. Reduced-motion checks preserve keyboard and focus behavior.</p><p>Font remains bundled Geist. Intercom’s public app declares licensed Graphik; exact font files and timed Mobbin animation evidence were unavailable. These are tuned transitions, not a certified exact clone. Recordings use isolated browsers and do not change stored data.</p><nav><a href="/library?kind=movie">Live Library</a><a href="../index.html">Previous full audit</a><a href="#w1440">Desktop</a><a href="#w834">Tablet</a><a href="#w390">Phone</a></nav>${sections}</main></html>`);
console.log(out);
