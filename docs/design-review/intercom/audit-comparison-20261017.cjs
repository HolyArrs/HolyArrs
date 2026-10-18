// Measured product captures; source raster dimensions do not establish source CSS scale.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { chromium } = require('../../../web/app/node_modules/playwright');
const root = path.resolve(__dirname, '../../..');
const origin = 'http://127.0.0.1:18746';
const out = path.join(root, '.omx/artifacts/visual-ralph/intercom/audit20261017/comparison');
function identity() {
  const h = crypto.createHash('sha256');
  function walk(d) { for (const n of fs.readdirSync(d).sort()) { const f = path.join(d,n); if (fs.statSync(f).isDirectory()) walk(f); else h.update(path.relative(root,f)).update(fs.readFileSync(f)); } }
  walk(path.join(root,'web/app/src'));
  return { commit: execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(), sourceHash:h.digest('hex') };
}
(async () => {
  fs.mkdirSync(out,{recursive:true});
  fs.writeFileSync(path.join(out,'receipt.json'),JSON.stringify({captureComplete:false,status:'pending',checks:[]}));
  const before = identity(), checks = [], browser = await chromium.launch({headless:true});
  try {
    for (const theme of ['light','dark']) for (const route of ['/library','/wishlist','/downloads','/settings']) {
      const context = await browser.newContext({viewport:{width:1920,height:1192},deviceScaleFactor:1,serviceWorkers:'block'});
      await context.addInitScript(value=>localStorage.setItem('holyarrs-theme',value),theme);
      await context.route('**/*',r=>{ const q=r.request(),u=new URL(q.url()); if(q.method()!=='GET'||u.origin!==origin&&!(q.resourceType()==='image'&&['image.tmdb.org','static.tvmaze.com','is1-ssl.mzstatic.com'].includes(u.hostname))) return r.abort(); return r.continue(); });
      if(context.routeWebSocket) await context.routeWebSocket('**/*',s=>s.close());
      const page=await context.newPage(),errors=[]; page.on('pageerror',e=>errors.push(e.message));
      try {
        const response=await page.goto(origin+route,{waitUntil:'networkidle'}); await page.evaluate(()=>document.fonts.ready);
        const landed=new URL(page.url()).pathname, heading=await page.locator('main h1').first().textContent();
        if(landed!==route||!heading?.trim()||/sign in|not found|404/i.test(heading)) throw Error('Expected application route/heading missing');
        const components=await page.evaluate(()=>Object.fromEntries(['.global-rail','.context-sidebar','.shell','.topbar','h1','button','.card','input','select','table'].map(selector=>{
          const e=document.querySelector(selector); if(!e)return[selector,null]; const r=e.getBoundingClientRect(),s=getComputedStyle(e);
          return[selector,{visible:!!e.getClientRects().length,x:r.x,y:r.y,width:r.width,height:r.height,font:s.fontFamily,fontSize:s.fontSize,lineHeight:s.lineHeight,fontWeight:s.fontWeight,color:s.color,background:s.backgroundColor,border:s.border,borderRadius:s.borderRadius,boxShadow:s.boxShadow,padding:s.padding,transition:s.transition,animation:s.animation}];
        })));
        const screenshot=theme+route.replaceAll('/','-')+'.png'; await page.screenshot({path:path.join(out,screenshot),animations:'disabled',mask:[page.locator('input[type=password],[data-sensitive],.secret,.secret-value')]});
        checks.push({width:1920,height:1192,dpr:1,theme,route,landed,heading,capturedAt:new Date().toISOString(),status:response.status(),components,errors,screenshot});
      } catch(e) { checks.push({width:1920,theme,route,failure:e.message,errors}); }
      finally { await context.close(); }
    }
  } finally { await browser.close(); }
  const after=identity(); const captureComplete=before.sourceHash===after.sourceHash&&checks.length===8&&checks.every(c=>!c.failure&&c.status===200&&!c.errors.length);
  fs.writeFileSync(path.join(out,'receipt.json'),JSON.stringify({before,after,captureComplete,visualMatchAsserted:false,animationCapturePolicy:'Screenshot animations disabled; motion measured separately',checks,limitations:['This capture gate performs no automated reference pixel or visual-match assertion; a separate scoped visual-ralph verdict is required.','Source images are 1920x1320 raster pixels with approximately 128px Mobbin footer; CSS viewport, DPR and zoom unknown.','Computed HolyArrs styles are measured; source styles, authenticated font and timed motion are unavailable.','Media-manager content, branding, responsive navigation and permissions are intentional adaptations.']},null,2));
  console.log(JSON.stringify({captureComplete,captures:checks.length,before,after})); if(!captureComplete)process.exitCode=1;
})().catch(e=>{console.error(e.message);process.exitCode=1;});
