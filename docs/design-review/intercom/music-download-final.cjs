// Read-only verification of the retained real music import and responsive UI.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const {execFileSync}=require('node:child_process'),{createRequire}=require('node:module');
const repo=path.resolve(__dirname,'../../..'),base='http://10.10.25.25:18746';
const {chromium,expect}=createRequire(path.join(repo,'web/app/package.json'))('@playwright/test');
const parent=path.join(repo,'.omx/artifacts/visual-ralph/intercom/music-download');
const first=JSON.parse(fs.readFileSync(path.join(parent,'36af3d87-1125-4ea6-8885-19346b5e7c46/receipt.json')));
const download=JSON.parse(fs.readFileSync(path.join(parent,'e9e9e741-19dd-49e7-8f08-2770d9c55415/receipt.json')));
const completion=JSON.parse(fs.readFileSync(path.join(parent,'e6b7741d-a80a-4793-91fd-ea7f55d2087b/receipt.json')));
const out=path.join(parent,'final-'+crypto.randomUUID());fs.mkdirSync(out);
const receipt={commit:execFileSync('git',['rev-parse','HEAD'],{cwd:repo,encoding:'utf8'}).trim(),started:new Date().toISOString(),checks:[],screenshots:[],browserWrites:[]};
async function get(p){const r=await fetch(base+'/api/v1'+p);assert.equal(r.status,200);return r.json();}
let browser;
(async()=>{
  const after=JSON.parse(execFileSync('python',[path.join(__dirname,'music-download-guard.py')],{encoding:'utf8'}));
  for(const [t,h]of Object.entries(first.before.tables))if(!['users','downloaders'].includes(t))assert.equal(after.tables[t],h,t+' preserved');
  for(const t of ['asks','titles','slots','file_rows'])for(const[id,h]of Object.entries(first.before[t]))assert.equal(after[t][id],h,'Original '+t+' '+id);
  for(const[p,h]of Object.entries(first.before.files))assert.equal(after.files[p],h,'Original file changed');
  for(const t of ['downloads','imports'])for(const[id,h]of Object.entries(download.original[t]))assert.equal(after[t][id],h,'Existing '+t+' '+id);
  const dbProof=JSON.parse(execFileSync('python',['-c',String.raw`
import sqlite3,pathlib,tempfile,hashlib,json
c=sqlite3.connect((pathlib.Path(tempfile.gettempdir())/'holyarrs-intercom-preview-18747/config/holyarrs.db').as_uri()+'?mode=ro',uri=True);c.row_factory=sqlite3.Row
def digest(v):return hashlib.sha256(json.dumps(v,sort_keys=True,default=str).encode()).hexdigest()
users=[dict(r)for r in c.execute('SELECT * FROM users WHERE id<=10')]
print(json.dumps({'originalUsers':digest(sorted(users,key=lambda r:json.dumps(r,sort_keys=True,default=str))), 'testAdmins': [dict(r)for r in c.execute("SELECT id,state,(SELECT count(*) FROM sessions s WHERE s.user_id=u.id) sessions FROM users u WHERE name LIKE 'music-download-%'")], 'slots':[dict(r)for r in c.execute('SELECT id,state,file_id FROM slots WHERE id IN(1183,1184,1185)')], 'files':[dict(r)for r in c.execute('SELECT id,version_id,relative_path,state,size FROM files WHERE id>6')], 'import':[dict(r)for r in c.execute('SELECT id,download_id,state FROM imports WHERE id=14')]}))
`],{encoding:'utf8'}));
  assert.equal(dbProof.originalUsers,first.before.tables.users,'All original users and password hashes preserved');
  assert.deepEqual(dbProof.testAdmins.map(u=>u.id).sort((a,b)=>a-b),[11,12,13,14,15]);
  assert(dbProof.testAdmins.every(u=>u.state==='disabled'&&u.sessions===0),'Temporary access revoked');
  assert.equal(after.original_downloader,download.original.tables.downloaders,'Original downloader configuration restored');
  assert.equal(dbProof.slots.length,3);assert(dbProof.slots.every(s=>['in_library','better_wanted'].includes(s.state)&&s.file_id));
  assert.equal(new Set(dbProof.slots.map(s=>s.file_id)).size,3);
  assert.equal(dbProof.files.length,3);assert(dbProof.files.every(f=>f.version_id===166&&f.state==='present'));
  assert.deepEqual(dbProof.slots.map(s=>s.file_id).sort(),dbProof.files.map(f=>f.id).sort());
  assert.equal(dbProof.import[0].state,'done');receipt.database=dbProof;
  const media=path.join(os.tmpdir(),'holyarrs-intercom-preview-media-18747/music/Amber Satellites');
  const imported=Object.keys(after.files).filter(p=>p.startsWith(media+path.sep));assert.equal(imported.length,3);
  assert.deepEqual(imported.map(p=>after.files[p]).sort(),download.synthetic.map(f=>f.sha256).sort());receipt.imported=imported;
  for(const f of dbProof.files){const p=path.join(os.tmpdir(),'holyarrs-intercom-preview-media-18747/music',f.relative_path);assert(imported.includes(p));assert.equal(fs.statSync(p).size,f.size);}
  const downloadRoot=path.join(os.tmpdir(),'holyarrs-intercom-preview-18747/downloads');
  for(const p of Object.keys(after.files).filter(p=>!(p in first.before.files)))assert(p.startsWith(media+path.sep)||p.startsWith(path.join(downloadRoot,'Amber Satellites - Evening Signals (2024) [FLAC]')+path.sep)||p.startsWith(path.join(downloadRoot,'Amber Satellites - Evening Signals (2024) [MP3 320]')+path.sep),'Unexpected new file');
  const d=(await get('/downloaders')).downloaders;assert.equal(d.length,1);assert.equal(d[0].id,1);assert.equal(d[0].url,'http://127.0.0.1:28768');assert.equal(d[0].priority,1);
  const torrents=await(await fetch('http://127.0.0.1:28768/api/v2/torrents/info')).json();
  for(const t of first.fakeBefore)assert.deepEqual(torrents.find(x=>x.hash===t.hash),t,'Original fake torrent changed');
  receipt.checks.push('Original users/password hashes, settings, keys, requests, titles, slots, files and fake torrents preserved; temporary access revoked and connection restored');
  receipt.album=await get('/albums/144');assert.equal(receipt.album.album.in_library,3);assert.equal(receipt.album.album.state,'complete');
  receipt.request=await get('/asks/34');assert.equal(receipt.request.state,'available');assert.equal(receipt.request.state_label,'Ready to listen');
  receipt.download=(await get('/downloads/view?limit=100')).downloads.find(d=>d.id===35);assert.equal(receipt.download.phase,'done');assert.equal(receipt.download.text,'3 of 3 tracks in the library.');
  receipt.checks.push('Real download35 finished, import14 done, three distinct files and slots available, request34 Ready to listen');
  browser=await chromium.launch({headless:true});const ctx=await browser.newContext({deviceScaleFactor:1});
  await ctx.route('**/*',route=>{if(['GET','HEAD','OPTIONS'].includes(route.request().method()))return route.continue();receipt.browserWrites.push(new URL(route.request().url()).pathname);return route.abort();});
  const page=await ctx.newPage();
  for(const[width,height]of[[360,800],[834,1112],[1440,1000]]){
    await page.setViewportSize({width,height});
    for(const[name,url]of[['completed','/downloads'],['album','/library/166/albums/144'],['library','/library?kind=artist&q=Amber%20Satellites'],['library-list','/library?kind=artist&q=Amber%20Satellites'],['request','/requests']]){
      await page.goto(base+url);await page.evaluate(()=>document.fonts.ready);
      if(name==='completed'){const row=page.getByTestId('download-row').filter({hasText:'[MP3 320]'}).filter({hasText:'Evening Signals'});await expect(row.getByTestId('row-state')).toHaveText('Done');await expect(row.getByTestId('row-text')).toHaveText('3 of 3 tracks in the library.');}
      if(name==='album'){await expect(page.getByTestId('album-state')).toHaveText('In library');await expect(page.getByTestId('track-state')).toHaveCount(3);await expect(page.getByTestId('track-state').first()).toHaveText('Better copy sought');}
      if(name==='library'||name==='library-list'){
        await page.getByRole('button',{name:name==='library'?'Posters':'List',exact:true}).click();
        const title=page.getByTestId(name==='library'?'library-card':'library-row').filter({hasText:'Amber Satellites'});
        await expect(title).toContainText('3 tracks in library');
      }
      if(name==='request'){await page.getByRole('button',{name:/^Everyone's/}).click();const ask=page.getByTestId('ask').filter({hasText:'Amber Satellites'});await expect(ask.getByTestId('ask-state')).toHaveText('Ready to listen');}
      await page.waitForTimeout(300);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),name+' overflow '+width);
      const file=`${name}-${width}.png`;await page.screenshot({path:path.join(out,file),fullPage:true});receipt.screenshots.push(file);
      if(name==='completed'||name==='request'||name==='library'){
        const item=name==='completed'?page.getByTestId('download-row').filter({hasText:'[MP3 320]'}).filter({hasText:'Evening Signals'}):name==='request'?page.getByTestId('ask').filter({hasText:'Amber Satellites'}):page.getByTestId('library-card').filter({hasText:'Amber Satellites'});
        const detail=`${name}-detail-${width}.png`;await item.screenshot({path:path.join(out,detail)});receipt.screenshots.push(detail);
      }
    }
  }
  assert.equal(receipt.browserWrites.length,0);receipt.checks.push('State-asserted screenshots and no horizontal overflow at 360, 834 and 1440 CSS pixels; DPR1, default zoom');receipt.status='passed';
})().catch(e=>{receipt.status='failed';receipt.error=e.message;process.exitCode=1;}).finally(async()=>{if(browser)await browser.close();receipt.finished=new Date().toISOString();fs.writeFileSync(path.join(out,'receipt.json'),JSON.stringify(receipt,null,2));console.log(JSON.stringify({status:receipt.status,error:receipt.error,evidence:out,checks:receipt.checks}));});
