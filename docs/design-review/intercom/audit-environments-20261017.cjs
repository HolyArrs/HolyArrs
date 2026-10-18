/* Bounded isolated responsive environment capture; GET-only and preview-scoped. */
const { chromium } = require('../../../web/app/node_modules/playwright');
const { createHash } = require('node:crypto'); const { execFileSync } = require('node:child_process'); const fs=require('node:fs'); const path=require('node:path');
const root=path.resolve(__dirname,'../../..'), origin='http://10.10.25.25:18746', out=path.join(root,'.omx/artifacts/visual-ralph/intercom/audit20261017/env');
const boundaryWidths=[599,600,601,619,620,621,767,768,769,1023,1024,1025]; const representativeRoutes=['/library','/wishlist','/downloads','/settings','/library/76']; const cases=[...boundaryWidths.flatMap((width)=>representativeRoutes.map((route,i)=>({width,route,theme:i%2?'dark':'light'}))),{width:640,route:'/library',theme:'light',zoomEquivalent:true},{width:1280,route:'/library/76',theme:'dark',dpr:2}];
function identity(){const h=createHash('sha256'),walk=d=>fs.readdirSync(d).sort().forEach(n=>{const f=path.join(d,n),s=fs.statSync(f);s.isDirectory()?walk(f):h.update(path.relative(root,f)).update(fs.readFileSync(f));});walk(path.join(root,'web/app/src'));return {commit:execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(),sourceHash:h.digest('hex')};}
async function geometry(p){return p.evaluate(()=>{const v=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'},es=[...document.querySelectorAll('button,a,input,select,textarea,h1,h2,h3,label,summary')].filter(v),outside=[],overlaps=[];for(const e of es){const r=e.getBoundingClientRect();if(r.left< -1||r.right>innerWidth+1)outside.push(e.getAttribute('aria-label')||e.textContent?.trim().slice(0,50));}for(let i=0;i<es.length;i++)for(let j=i+1;j<es.length;j++){const a=es[i].getBoundingClientRect(),b=es[j].getBoundingClientRect();if(Math.min(a.right,b.right)>Math.max(a.left,b.left)+3&&Math.min(a.bottom,b.bottom)>Math.max(a.top,b.top)+3&&!es[i].contains(es[j])&&!es[j].contains(es[i]))overlaps.push([es[i].tagName,es[j].tagName]);}return {innerWidth,innerHeight,scrollWidth:document.documentElement.scrollWidth,overflow:document.documentElement.scrollWidth>innerWidth+1,outside:outside.slice(0,20),overlaps:overlaps.slice(0,20)};});}
async function main() {
  fs.mkdirSync(out, { recursive: true });
  const before = identity(), checks = [];
  const browser = await chromium.launch({ headless: true });
  try {
    for (const descriptor of cases) {
      const context = await browser.newContext({ viewport: { width: descriptor.width, height: 900 }, deviceScaleFactor: descriptor.dpr || 1, reducedMotion: 'no-preference', serviceWorkers: 'block' });
      await context.addInitScript(theme => localStorage.setItem('holyarrs-theme', theme), descriptor.theme);
      await context.route('**/*', route => {
        const request = route.request(), url = new URL(request.url());
        if (request.method() !== 'GET') return route.abort();
        if (url.origin !== new URL(origin).origin && !(url.origin === 'https://image.tmdb.org' && request.resourceType() === 'image')) return route.abort();
        return route.continue();
      });
      const page = await context.newPage(), errors = [];
      page.on('pageerror', error => errors.push(error.message));
      const key = `${descriptor.width}-${descriptor.theme}-${descriptor.route.replace(/[^a-z0-9]/gi, '_')}-${descriptor.dpr || 1}`;
      try {
        const response = await page.goto(origin + descriptor.route, { waitUntil: 'networkidle', timeout: 30000 });
        await page.evaluate(() => document.fonts.ready);
        const measures = await geometry(page);
        await page.screenshot({ path: path.join(out, key + '.png'), mask: [page.locator('input[type=password],[data-sensitive],.secret,.secret-value')] });
        checks.push({ ...descriptor, timestamp: new Date().toISOString(), status: response?.status() || 0, geometry: measures, errors, screenshot: key + '.png', actualDpr: await page.evaluate(() => devicePixelRatio), visualViewport: await page.evaluate(() => ({ width: visualViewport?.width, height: visualViewport?.height, scale: visualViewport?.scale })), nativeBrowserZoomProven: false });
        console.log(key, 'overflow', measures.overflow, 'outside', measures.outside.length, 'overlap diagnostics', measures.overlaps.length);
      } catch (error) {
        checks.push({ ...descriptor, timestamp: new Date().toISOString(), status: 0, failure: error.message, errors });
      } finally { await context.close(); }
    }
  } finally { await browser.close(); }
  const after = identity();
  fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ before, after, origin, cases, checks, limitations: ['640 CSS viewport represents reflow of a 1280px window at 200%; native browser zoom and physical scaling are unproven.', 'Overlap/outside diagnostics need inspection for intentional nested or horizontally scrollable controls.', 'Reduced motion, keyboard, touch and full journeys remain covered elsewhere.'] }, null, 2));
  if (before.sourceHash !== after.sourceHash || checks.length !== cases.length || checks.some(check => check.failure || check.status < 200 || check.status >= 400 || check.errors.length || check.geometry.overflow || check.geometry.innerWidth !== check.width || check.actualDpr !== (check.dpr || 1))) throw Error('Environment gate failed; inspect receipt');
}
if (require.main === module) main().catch(error => { console.error(error); process.exitCode = 1; });
module.exports = { cases, identity };

