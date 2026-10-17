const {createRequire}=require('node:module');
const {readFile,mkdir,writeFile}=require('node:fs/promises');
const {createHash}=require('node:crypto');
const path=require('node:path');
const assert=require('node:assert/strict');
// One isolated browser/page. Requires a separately recorded shared-machine allocation.
const [toolsRoot,output]=process.argv.slice(2);
if(!path.isAbsolute(toolsRoot||'')||!path.isAbsolute(output||''))throw Error('Absolute tools and fresh output paths required');
const cached=createRequire(path.join(toolsRoot,'package.json'));
const {chromium}=cached('playwright');
const origin='http://127.0.0.1:18744';
const receipt={kind:'desktop-direction-review-only',sourceHashes:{},screenshots:[],checks:[],errors:[],blockedRequests:[],violations:[]};
let browser;
const deadline=setTimeout(()=>{receipt.errors.push('150-second deadline');void browser?.close()},150000);
async function capture(page,label){
 await page.evaluate(()=>document.fonts.ready);
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${label}: document overflow`);
 await page.screenshot({path:path.join(output,label+'.png'),fullPage:true,animations:'disabled'});
 receipt.screenshots.push({file:label+'.png',viewport:page.viewportSize()});
 await page.addScriptTag({path:cached.resolve('axe-core/axe.min.js')});
 const scan=await page.evaluate(()=>window.axe.run(document,{runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21aa','wcag22aa']}}));
 receipt.violations.push(...scan.violations.map(v=>({label,id:v.id,impact:v.impact,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))})));
 receipt.checks.push(label+': overflow and axe completed');
}
async function nav(page,concept,view){
 const selector=concept==='two'?`.primary-nav [data-view="${view}"]`:concept==='one'?`.sidebar [data-view="${view}"]`:`.icon-rail [data-page="${view}"]`;
 await page.locator(selector).click();
}
(async()=>{
 await mkdir(output,{recursive:false});
 for(const file of ['index.html','concepts/one.html','concepts/two.html','concepts/three.html',...['dune','interstellar','blade-runner','arrival','breaking-bad','dark'].map(x=>'assets/'+x+'.jpg')])receipt.sourceHashes[file]=createHash('sha256').update(await readFile(path.join(__dirname,file))).digest('hex');
 try{
  browser=await chromium.launch({headless:true});
  const context=await browser.newContext({reducedMotion:'reduce',serviceWorkers:'block'});
  await context.route('**/*',r=>{if(new URL(r.request().url()).origin===origin)return r.continue();receipt.blockedRequests.push(r.request().url());return r.abort()});
  const page=await context.newPage();page.setDefaultTimeout(5000);
  page.on('pageerror',e=>receipt.errors.push(e.message));
  for(const concept of ['comparison','one','two','three']){
   for(const [device,width,height] of [['desktop',1440,1050],['tablet',834,1112],['phone',390,844],['narrow',320,740]]){
    await page.setViewportSize({width,height});
    await page.goto(origin+(concept==='comparison'?'':`/concepts/${concept}.html`));
    await capture(page,`${concept}-${device}`);
   }
  }
  for(const concept of ['one','two','three']){
   await page.setViewportSize({width:1440,height:1050});await page.goto(`${origin}/concepts/${concept}.html`);
   if(concept==='two'){
    await page.locator('[data-item]').first().click();
    await page.locator('#approveBtn').click();await page.locator('#approveConfirm').click();
    assert((await page.locator('#detailBody').textContent()).includes('Searching'));
    await page.locator('#detailClose').click();
    await nav(page,concept,'settings');await page.locator('#testDownloader').click();
    assert((await page.locator('#settingsNote').textContent()).includes('Save'));
    await page.locator('#saveDownloader').click();await page.locator('#testDownloader').click();
    assert((await page.locator('#settingsNote').textContent()).includes('simulated'));
   }else{
    await nav(page,concept,'wishlist');await page.locator('[data-approve]').first().click();
    await page.locator('#approveForm').getByRole('button',{name:/Approve fixture/}).click();
    assert((await page.locator('#main').textContent()).includes('Approved'));
    await nav(page,concept,'downloads');
    await page.getByRole('button',{name:'Mark failed',exact:true}).first().click();
    assert(await page.locator('dialog[open]').count());await page.keyboard.press('Escape');
    await nav(page,concept,'settings');await page.getByRole('button',{name:'Save, then test',exact:true}).click();
    assert((await page.locator('#main').textContent()).includes('saved'));
   }
   await capture(page,`${concept}-desktop-settings`);
   for(const view of ['wishlist',concept==='two'?'flow':'downloads']){await nav(page,concept,view);await capture(page,`${concept}-desktop-${view}`)}
   receipt.checks.push(concept+': approval, confirmation and saved-settings interaction assertions');
   await page.setViewportSize({width:390,height:844});await page.goto(`${origin}/concepts/${concept}.html`);
   if(concept==='one'){
    await page.locator('.mobile-nav [data-view="library"]').click();await page.locator('[data-media]').first().click();
    await capture(page,concept+'-phone-inspector');await page.keyboard.press('Escape');
    assert(!await page.locator('dialog[open]').count());
    await page.locator('.mobile-nav [data-view="home"]').click();await page.locator('[data-approve]').first().click();
    await page.locator('#approveForm').getByRole('button',{name:/Approve fixture/}).click();
    await page.waitForTimeout(50);
    assert(await page.evaluate(()=>document.activeElement.getClientRects().length>0&&!document.activeElement.closest('[inert]')&&document.activeElement!==document.body),'one: visible mobile approval focus');
   }else if(concept==='two'){
    await page.locator('.phone-flow [data-item]').first().click();await capture(page,concept+'-phone-inspector');
    await page.locator('#detailClose').click();assert(await page.evaluate(()=>!document.activeElement.closest('[inert]')));
   }else{
    await page.locator('[data-select]').first().click();await capture(page,concept+'-phone-inspector');
    await page.locator('#inspectorBack').click();assert(await page.evaluate(()=>document.activeElement.isConnected&&!document.activeElement.closest('[inert]')));
   }
   receipt.checks.push(concept+': phone inspector closure/focus checked');
  }
  await context.close();
 }catch(e){receipt.errors.push(e.stack)}finally{
  await browser?.close();clearTimeout(deadline);
  receipt.pass=!receipt.errors.length&&!receipt.violations.length&&!receipt.blockedRequests.length;
  await writeFile(path.join(output,'receipt.json'),JSON.stringify(receipt,null,2));
  process.stdout.write(JSON.stringify({pass:receipt.pass,screenshots:receipt.screenshots.length,checks:receipt.checks.length,errors:receipt.errors.length,violations:receipt.violations.length,output}));
  if(!receipt.pass)process.exitCode=1;
 }
})();
