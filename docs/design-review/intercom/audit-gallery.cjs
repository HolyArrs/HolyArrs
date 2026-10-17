const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const source = path.join(root, '.omx/artifacts/visual-ralph/intercom/full-audit');
const target = path.join(root, 'web/app/static/preview-audit');
fs.mkdirSync(target, { recursive: true });
const groups = [
  ['Library desktop — before', 'before/1440-_library_kind_movie.png'],
  ['Library desktop — after', 'final-complete/1440-_library_kind_movie.png'],
  ['Library phone — before', 'before/390-_library_kind_movie.png'],
  ['Library phone — after', 'final-complete/390-_library_kind_movie.png'],
  ['Downloads — before', 'before/390-_downloads.png'],
  ['Downloads — after', 'final-complete/390-_downloads.png'],
  ['Requester filter', 'states/390-requester.png'],
  ['Empty search', 'states/390-empty-search.png'],
  ['Recoverable error', 'states/library-error.png'],
  ['Library loading — phone', 'loading/390-library-loading.png'],
  ['TV detail — before', 'route-matrix/390-_library_76.png'],
  ['TV detail — after', 'final-complete/390-_library_76.png'],
  ['Music detail — before', 'route-matrix/390-_library_141.png'],
  ['Music detail — after', 'final-complete/390-_library_141.png'],
  ['Requests phone — before', 'wide-matrix/360-_wishlist.png'],
  ['Requests phone — after', 'final-responsive/360-_wishlist.png'],
  ['Music last album — phone', 'clearance/360-music-album.png'],
  ['Music page end — phone', 'clearance/360-music-end.png'],
  ['History — invalid date recovery', 'repair-interactions/390-history-invalid.png'],
  ['Requests desktop', 'wide-matrix/1920-_wishlist.png'],
  ['Ultrawide Library', 'wide-matrix/3440-_library_kind_show.png'],
  ['4K Downloads', 'final-responsive/3840-_downloads.png']
];
const cards = groups.filter(([, file]) => fs.existsSync(path.join(source, file))).map(([label, file], i) => {
  const name = `${i}.png`;
  fs.copyFileSync(path.join(source, file), path.join(target, name));
  return `<figure><figcaption>${label}</figcaption><a href="${name}" target="_blank"><img src="${name}" loading="lazy" alt="${label}"></a></figure>`;
});
fs.writeFileSync(path.join(target, 'index.html'), `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs preview audit</title><style>body{margin:0;background:#efefeb;color:#20201e;font:16px/1.5 system-ui}header,main{padding:24px;max-width:1600px;margin:auto}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,440px),1fr));gap:24px}figure{margin:0;background:white;border:1px solid #ddd;border-radius:14px;overflow:hidden}figcaption{padding:16px;font-weight:600}img{width:100%;height:520px;object-fit:contain;object-position:top;background:#f7f7f5}a:focus-visible{outline:3px solid #b33b12}h1{margin:0}p{max-width:80ch}</style><header><h1>Preview audit — screenshots</h1><p>Before/after captures show Library controls, status contrast, mobile requests, TV details and music album repairs. Click any image for full resolution. Later screenshots cover representative responsive states; they do not certify every interaction or pixel-exact Intercom parity.</p><a href="/library?kind=movie">Open the actual Library</a></header><main>${cards.join('')}</main></html>`);
console.log(`Saved ${cards.length} screenshots to the preview-only gallery.`);
