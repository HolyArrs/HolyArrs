// Retained preview only: one approved album, three synthetic files, no SQL writes.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const http = require('node:http');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { createRequire } = require('node:module');
const repo = path.resolve(__dirname, '../../..');
const { chromium, expect } = createRequire(path.join(repo, 'web/app/package.json'))('@playwright/test');
const base = 'http://10.10.25.25:18746';
const runtime = path.join(os.tmpdir(), 'holyarrs-intercom-preview-18747');
const artistRoot = path.join(os.tmpdir(), 'holyarrs-intercom-preview-media-18747/music/Amber Satellites');
const releaseName = 'Amber Satellites - Evening Signals (2024) [FLAC]';
const hash = [...releaseName].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 11).toString(16).padStart(8, '0').repeat(5);
const out = path.join(repo, '.omx/artifacts/visual-ralph/intercom/music-upgrade', crypto.randomUUID());
const resume=process.env.HOLYARRS_MUSIC_DOWNLOAD_RESUME;
if(resume && resume!=='e9e9e741-19dd-49e7-8f08-2770d9c55415')throw Error('Only the retained partial music test may continue');
const prior=resume?JSON.parse(fs.readFileSync(path.join(path.dirname(out),resume,'receipt.json'),'utf8')):null;
assert(!resume,'This new upgrade check must not reuse a download continuation');
fs.mkdirSync(out, { recursive: true });
const receipt = { started: new Date().toISOString(), commit: execFileSync('git', ['rev-parse', 'HEAD'], {cwd:repo,encoding:'utf8'}).trim(), hash, writes: [], checks: [], screenshots: [] };
function save() { fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify(receipt, null, 2)); }
function guard() { return JSON.parse(execFileSync('python', [path.join(__dirname, 'music-download-guard.py')], { encoding:'utf8' })); }
async function get(p) { const r = await fetch(base + '/api/v1' + p); assert.equal(r.status,200,p); return r.json(); }
async function fake(p) { try{const r = await fetch('http://127.0.0.1:28768' + p,{signal:AbortSignal.timeout(10000)}); assert.equal(r.status,200); return r.json();}catch(e){throw Error('Fake control: '+e.message+' '+(e.cause?.code||''));} }
async function poll(fn, label, timeout = 65000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) { const value = await fn(); if (value) return value; await new Promise(r => setTimeout(r,1000)); }
  throw Error('Timed out: ' + label);
}
function safeAncestors(p) {
  for (let q = p; q !== path.dirname(q); q = path.dirname(q)) {
    if (fs.existsSync(q)) assert(!fs.lstatSync(q).isSymbolicLink(), 'Linked storage ancestor');
  }
}
let browser, proxy, originalDownloader, restoreDownloader, session, temporaryDownloader;
const temporaryConfig={name:'Disposable music size proxy',kind:'qbittorrent',url:'http://127.0.0.1:28775',username:'',secret:'',category:'holyarrs',priority:1,path_map:[],allow_local_network:true};
const sessionName='music-download-'+crypto.randomBytes(4).toString('hex');
function sessionAction(action,extra={}) { return JSON.parse(execFileSync('python',[path.join(__dirname,'music-download-session.py')],{input:JSON.stringify({action,name:sessionName,...extra}),encoding:'utf8'})); }
(async () => {
  // Command lines stay private; only listener identities are recorded.
  const ps = `$ErrorActionPreference='Stop'; $r=@(Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in @(18730,18745,18746,18747,28768) }); foreach($port in @(18746,18747,28768)) { $x=@($r | Where-Object LocalPort -eq $port); if($x.Count -ne 1){throw 'Ambiguous listener'}; $p=Get-CimInstance Win32_Process -Filter ('ProcessId='+$x[0].OwningProcess); if($port -eq 18746 -and -not $p.CommandLine.Contains('vite.intercom-preview.config.ts')){throw 'Unexpected frontend'}; if($port -eq 18747 -and (-not $p.CommandLine.Contains('${runtime.replace(/'/g,"''")}\\config') -or $x[0].LocalAddress -ne '127.0.0.1')){throw 'Unexpected backend'}; if($port -eq 28768 -and $x[0].LocalAddress -ne '127.0.0.1'){throw 'Nonlocal fake'} }; $r | Select-Object LocalAddress,LocalPort,OwningProcess | ConvertTo-Json -Compress`;
  receipt.listeners = JSON.parse(execFileSync('powershell.exe',['-NoProfile','-NonInteractive','-Command',ps],{encoding:'utf8'}));
  receipt.original = guard();
  const downloadRoot=path.join(runtime,'downloads'); const folder=path.join(downloadRoot,releaseName+' lossless-check');
  safeAncestors(folder);safeAncestors(artistRoot);if(!resume)assert(!fs.existsSync(folder),'Refuse existing test folder');
  assert(fs.existsSync(artistRoot),'Expected disposable MP3 album');
  assert.equal((await get('/asks/34')).state, 'available');
  assert.equal((await get('/albums/144')).album.in_library, 3);
  assert.deepEqual((await get('/albums/144/tracks')).tracks.flatMap(t=>t.slots.map(s=>s.id)),[1183,1184,1185]);
  assert((await get('/albums/144/tracks')).tracks.every(t=>t.slots.every(s=>s.state==='better_wanted'&&[7,8,9].includes(s.file_id))));
  const fakeBefore = await fake('/api/v2/torrents/info');
  if(!resume)assert(fakeBefore.some(t=>t.hash===hash), 'Expected retained rejected FLAC fixture');
  else {receipt.parent=resume;assert.equal(prior.downloadId,35);assert.equal(prior.hash,hash);}
  receipt.fakeBefore = fakeBefore;
  browser = await chromium.launch({headless:true});
  const context = await browser.newContext({viewport:{width:1440,height:1000},deviceScaleFactor:1});
  const token=crypto.randomBytes(32).toString('base64url'),csrf=crypto.randomBytes(32).toString('base64url');
  session=sessionAction('create',{token,csrf}); receipt.disposableAdmin=session.id;
  await context.addCookies([{name:'holyarrs_intercom_preview_session',value:token,url:base,httpOnly:true,sameSite:'Lax'}]);
  receipt.before=guard();
  let allowedRelease = 0;
  await context.route('**/*', async route => {
    const req=route.request(), u=new URL(req.url()), p=u.pathname;
    if (['GET','HEAD','OPTIONS'].includes(req.method())) return route.continue();
    let body={}; try {body=req.postDataJSON() || {};} catch {}
    const configWrite=(req.method()==='PUT' && p==='/api/v1/downloaders/1' && originalDownloader &&
      [JSON.stringify(originalDownloader),JSON.stringify({...originalDownloader,priority:2})].includes(JSON.stringify(body))) ||
      (req.method()==='POST'&&p==='/api/v1/downloaders'&&JSON.stringify(body)===JSON.stringify(temporaryConfig)) ||
      (req.method()==='DELETE'&&temporaryDownloader&&p===`/api/v1/downloaders/${temporaryDownloader}`);
    const allowed=u.origin===base && (configWrite || (req.method()==='POST' && (
      (p==='/api/v1/titles/166/search' && body.album_id===144 && body.choose===false && Object.keys(body).length===2) ||
      (allowedRelease && p===`/api/v1/releases/${allowedRelease}/choose` && body.force===true && Object.keys(body).length===1))));
    if(!allowed) {receipt.writes.push({method:req.method(),path:p,blocked:true});save();return route.abort('blockedbyclient');}
    const response=await route.fetch(); receipt.writes.push({method:req.method(),path:p,status:response.status()});save();await route.fulfill({response});
  });
  const page=await context.newPage();
  await page.goto(base+'/library/166/albums/144');
  await expect(page.getByTestId('track-state').first()).toHaveText('Better copy sought');
  await page.screenshot({path:path.join(out,'before-better-copy-1440.png'),fullPage:true});receipt.screenshots.push('before-better-copy-1440.png');
  // Existing fake reports every file as 1000 bytes. A loopback proxy corrects
  // only this new torrent's file sizes, forwarding every other call unchanged.
  proxy=http.createServer(async(req,res)=>{
    try {
      const chunks=[];for await(const chunk of req)chunks.push(chunk);const bytes=Buffer.concat(chunks);
      const upstream=await fetch('http://127.0.0.1:28768'+req.url,{method:req.method,headers:{'content-type':req.headers['content-type']||'application/x-www-form-urlencoded'},body:bytes.length?bytes:undefined});
      let body=Buffer.from(await upstream.arrayBuffer());const u=new URL(req.url,'http://127.0.0.1');
      if(u.pathname==='/api/v2/torrents/files' && (u.searchParams.get('hash')===hash || new URLSearchParams(bytes.toString()).get('hash')===hash)) {
        const files=JSON.parse(body);for(const f of files) {const p=path.resolve(downloadRoot,f.name);assert(p.startsWith(folder+path.sep));f.size=fs.statSync(p).size;}body=Buffer.from(JSON.stringify(files));
      }
      res.writeHead(upstream.status,{'content-type':upstream.headers.get('content-type')||'application/json'});res.end(body);
    }catch{res.writeHead(502);res.end('Disposable proxy refused');}
  });
  await new Promise((resolve,reject)=>{proxy.once('error',reject);proxy.listen(28775,'127.0.0.1',resolve);});
  const d=(await get('/downloaders')).downloaders.find(d=>d.id===1);assert.equal(d.url,'http://127.0.0.1:28768');
  originalDownloader=Object.fromEntries(['name','kind','url','username','secret','category','priority','path_map','allow_local_network'].map(k=>[k,d[k]]));
  async function configCall(method,p,body){return page.evaluate(async ({method,p,body,csrf})=>{const {api}=await import('/src/lib/api.ts');api.setCsrf(csrf);return api.request(p,{method,body});},{method,p,body,csrf});}
  // The active existing Downloader keeps its URL/category and credentials.
  temporaryDownloader=resume?2:(await configCall('POST','/downloaders',temporaryConfig)).id;
  receipt.temporaryDownloader=temporaryDownloader;
  restoreDownloader=async()=>{await configCall('PUT','/downloaders/1',originalDownloader);await configCall('DELETE',`/downloaders/${temporaryDownloader}`);};
  if(!resume)await configCall('PUT','/downloaders/1',{...originalDownloader,priority:2});
  let release,dl;
  if(!resume){
  await page.getByTestId('album-choose').click();
  release=await poll(async()=> (await get('/titles/166/releases?album_id=144&limit=25&offset=0')).releases.find(r=>r.name===releaseName), 'scoped album search');
  assert.deepEqual(release.slot_ids,[1183,1184,1185]); allowedRelease=release.id; receipt.releaseId=release.id;save();
  const row=page.getByTestId('release').filter({hasText:releaseName});
  await row.getByRole('button',{name:'Choose '+releaseName,exact:true}).click();
  const dialog=page.getByRole('dialog'); await expect(dialog).toContainText('posted');
  await dialog.getByRole('button',{name:'Choose anyway',exact:true}).click();
  dl=await poll(async()=> (await get('/downloads')).downloads.find(d=>d.ReleaseID===release.id&&d.ExternalID===hash), 'real download creation');
  }else{dl=(await get('/downloads')).downloads.find(d=>d.ID===35&&d.ExternalID===hash);assert(dl);release={id:prior.releaseId};receipt.releaseId=release.id;}
  assert.equal(dl.DownloaderID,temporaryDownloader,'Must use the disposable size-aware connection');
  receipt.downloadId=dl.ID;
  await poll(async()=> (await get('/albums/144/tracks')).tracks.every(t=>t.slots.every(s=>['downloading','better_wanted'].includes(s.state))), 'three downloading slots');
  receipt.checks.push('Real choose created a scoped lossless replacement download');
  async function capture(label, url, locator) {
    for(const [width,height] of [[360,800],[834,1112],[1440,1000]]) {
      await page.setViewportSize({width,height}); await page.goto(base+url); await expect(locator(page)).toBeVisible({timeout:15000});
      await page.evaluate(()=>document.fonts.ready); await page.waitForTimeout(500);
      assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1), label+' horizontal overflow');
      const file=`${label}-${width}.png`;await page.screenshot({path:path.join(out,file),fullPage:true});receipt.screenshots.push(file);save();
    }
  }
  await capture('downloading','/downloads',p=>p.getByTestId('download-row').filter({hasText:'Evening Signals'}).first());
  if(!resume){fs.mkdirSync(folder); receipt.synthetic=[];
  for(let i=1;i<=3;i++) {
    const name=`0${i} - Song 1.${i}.flac`;
    execFileSync('ffmpeg',['-hide_banner','-loglevel','error','-n','-f','lavfi','-i',`sine=frequency=${220*i}:duration=${180+(i-1)*15}`,'-c:a','flac','-compression_level','0','-metadata','artist=Amber Satellites','-metadata','album=Evening Signals','-metadata',`title=Song 1.${i}`,'-metadata',`track=${i}`,path.join(folder,name)],{timeout:30000});
    const content=fs.readFileSync(path.join(folder,name));assert(content.length>512*1024);
    receipt.synthetic.push({name,size:content.length,sha256:crypto.createHash('sha256').update(content).digest('hex')});
  }
  }else{receipt.synthetic=prior.synthetic;for(const f of receipt.synthetic)assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(folder,f.name))).digest('hex'),f.sha256);}
  receipt.filesCreated=true;save();
  const files=receipt.synthetic.map(f=>path.basename(folder)+'/'+f.name).join('|');
  await fake('/set?'+new URLSearchParams({hash,progress:'1',state:'stalledUP',save_path:downloadRoot,files}));
  receipt.fakeCompletion={hash,progress:1,files:receipt.synthetic.map(f=>f.name)};save();
  receipt.completed=await poll(async()=> {const d=(await get('/downloads')).downloads.find(d=>d.ID===dl.ID);return d&&['completed','imported','finished'].includes(d.State)?d:null;}, 'finished download',90000);
  await poll(async()=> {const a=(await get('/albums/144')).album;return a.in_library===3 && a.better_wanted===0;}, 'album lossless and good enough',90000);
  receipt.request=await poll(async()=> {const a=await get('/asks/34');return a.state==='available'?a:null;}, 'request available',90000);
  receipt.album=await get('/albums/144'); receipt.tracks=await get('/albums/144/tracks');
  assert(receipt.tracks.tracks.every(t=>t.slots.every(s=>s.state==='in_library'&&s.file_id>9)),'All selected tracks must have a Library file');
  receipt.checks.push('Completed download, all three imported tracks and request Available');
  await capture('completed','/downloads',p=>p.getByTestId('download-row').filter({hasText:'Evening Signals'}).first());
  await capture('album-available','/library/166/albums/144',p=>p.getByRole('heading',{name:'Evening Signals',exact:true}));
  await capture('library','/library?kind=artist&q=Amber%20Satellites',p=>p.locator('main').getByText('Amber Satellites',{exact:true}).first());
  await restoreDownloader();restoreDownloader=null;await new Promise(r=>proxy.close(r));proxy=null;
  receipt.after=guard();
  for(const [table,value]of Object.entries(receipt.before.tables))if(table!=='downloaders')assert.equal(receipt.after.tables[table],value,'Credentials/settings changed: '+table);
  for(const table of ['asks','titles','slots']) assert.deepEqual(receipt.after[table],receipt.before[table], 'Existing '+table+' changed');
  for(const table of ['file_rows','downloads','imports']) for(const [id,value] of Object.entries(receipt.before[table])) if(!(table==='file_rows'&&['7','8','9'].includes(id))) assert.equal(receipt.after[table][id],value,'Existing '+table+' changed');
  assert.equal(Object.keys(receipt.after.file_rows).length-Object.keys(receipt.before.file_rows).length,3);
  for(const [id,value] of Object.entries(receipt.before.files)) if(!id.startsWith(artistRoot+path.sep)) assert.equal(receipt.after.files[id],value,'Existing media changed');
  const newFiles=Object.keys(receipt.after.files).filter(f=>!(f in receipt.before.files));
  assert(newFiles.every(f=>f.startsWith(folder+path.sep)||f.startsWith(artistRoot+path.sep)),'Unexpected media write');
  const imported=newFiles.filter(f=>f.startsWith(artistRoot+path.sep)); const lossless=imported.filter(f=>f.endsWith('.flac'));assert.equal(lossless.length,3);
  assert.deepEqual(lossless.map(f=>receipt.after.files[f]).sort(),receipt.synthetic.map(f=>f.sha256).sort(),'Imported bytes differ');
  receipt.imported=lossless;
  for(const p of Object.keys(receipt.before.files).filter(p=>p.startsWith(artistRoot+path.sep)))assert(Object.values(receipt.after.files).includes(receipt.before.files[p]),'Original disposable MP3 bytes retained');
  assert.equal(receipt.after.original_downloader,receipt.original.original_downloader,'Original downloader configuration restored');
  const fakeAfter=await fake('/api/v2/torrents/info');
  for(const t of fakeBefore)if(t.hash!==hash) assert.deepEqual(fakeAfter.find(x=>x.hash===t.hash),t,'Existing fake torrent changed');
  receipt.checks.push('Original users, credentials, settings, requests, titles, slots, files and fake torrents preserved');
  receipt.status='passed';
})().catch(e=>{receipt.status='failed';receipt.error=e.message;process.exitCode=1;}).finally(async()=>{if(restoreDownloader)try{await restoreDownloader();receipt.configRestored=true;}catch{receipt.configRestoreFailed=true;}if(proxy)await new Promise(r=>proxy.close(r));if(browser)await browser.close();if(session){receipt.sessionRevoked=sessionAction('revoke');}receipt.finished=new Date().toISOString();save();console.log(JSON.stringify({status:receipt.status,error:receipt.error,evidence:out,checks:receipt.checks}));});
