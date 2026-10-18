const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.omx/artifacts/visual-ralph/intercom');
const source='45144ef19d962c8cd785e124d352c7f00f42821ea359558b919415073e6f2928';
const suites={responsive:['responsive-pass/consolidated-final-v2-20261017',744],interaction:['interaction-pass/consolidated-final-20261017',243],touch:['interaction-pass/touch',8],environment:['audit20261017/env-v3',164],comparison:['audit20261017/comparison',8],motion:['typography-motion/audit20261017-final',6],realSave:['real-save/3e873997-aeb3-4a60-ab2d-74da62aeed12',7]};
function read(rel){try{return JSON.parse(fs.readFileSync(path.join(base,rel),'utf8'));}catch{return null;}}
const nonempty=x=>Array.isArray(x)?x.length>0:!!x;
function bad(c){return !!(c.failure||c.error||c.passed===false||c.overflow||['errors','clipped','outside','overlaps','violations'].some(k=>nonempty(c[k]))||c.geometry?.overflow);}
function hashes(d){return typeof d.before==='string'?[d.before,d.after]:d.sourceBefore?[d.sourceBefore.frontend,d.sourceAfter?.frontend]:[d.before?.sourceHash,d.after?.sourceHash];}
function currentHash(){const h=crypto.createHash('sha256');function walk(d){for(const n of fs.readdirSync(d).sort()){const f=path.join(d,n);if(fs.statSync(f).isDirectory())walk(f);else h.update(path.relative(root,f)).update(fs.readFileSync(f));}}walk(path.join(root,'web/app/src'));return h.digest('hex');}
function build(){
 const results={},data={},currentSourceHash=currentHash();
 for(const [name,[folder,expected]]of Object.entries(suites)){
  const d=read(folder+'/receipt.json');data[name]=d;
  if(!d){results[name]={receipt:'.omx/artifacts/visual-ralph/intercom/'+folder+'/receipt.json',status:'missing',expected};continue;}
  const checks=d.checks||[],h=hashes(d),failures=checks.filter(bad).map(c=>({route:c.route,name:c.name,width:c.width,failure:c.failure||'Recorded finding'})),gates=[];
  if(checks.length!==expected)gates.push(`Expected ${expected}, observed ${checks.length}`);
  if(h.some(v=>v!==source))gates.push('Missing or mismatched source identity');
  if(name==='comparison'&&d.captureComplete!==true)gates.push('Capture gate incomplete; this is not a visual-match assertion');
  if(name==='realSave'&&(d.passed!==true||d.cleanup?.verified!==true||d.cleanup?.originalRowsPreserved!==true||d.cleanup?.originalMediaPreserved!==true))gates.push('Real-save or preservation gate failed');
  if(name==='responsive'){
   const m=read(folder+'/manifest.json'),key=c=>`${c.route}|${c.state||''}|${c.theme}|${c.width}`;
   const expectedKeys=m?.widths?.flatMap(width=>m.selected.map(c=>key({...c,width,theme:m.theme})));
   if(!expectedKeys||JSON.stringify(checks.map(key).sort())!==JSON.stringify(expectedKeys.sort()))gates.push('Responsive manifest case mismatch');
  }
  if(name==='touch'){
   const m=read(folder+'/manifest.json'),key=c=>`${c.route}|${c.name}|${c.width}`;
   const expectedKeys=m?.widths?.flatMap(width=>m.scenarios.map(c=>key({...c,width})));
   if(!expectedKeys||JSON.stringify(checks.map(key).sort())!==JSON.stringify(expectedKeys.sort()))gates.push('Touch manifest case mismatch');
  }
  if(name==='environment'){
   if(checks.some(c=>!Number.isInteger(c.status)||c.status<200||c.status>=400||c.geometry?.innerWidth!==c.width||c.actualDpr!==(c.dpr||1)))gates.push('Environment HTTP/viewport/DPR gate failed');
   if(checks.some(c=>nonempty(c.geometry?.outside)||nonempty(c.geometry?.overlaps)))gates.push('Unresolved environment geometry diagnostics need explicit review');
  }
  if(['responsive','interaction','touch','environment','comparison'].includes(name)){
   const directory=path.resolve(base,folder);
   for(const c of checks){const f=typeof c.screenshot==='string'?path.resolve(directory,c.screenshot):'';if(!f||path.isAbsolute(c.screenshot)||!f.startsWith(directory+path.sep)||!fs.existsSync(f))gates.push('Missing or unsafe screenshot: '+String(c.screenshot));}
  }
  if(name==='interaction'){
   const manifest=read(folder+'/manifest.json'),expectedManifest=read(folder+'/expected.json'),key=c=>`${c.width}|${c.group}|${c.name}`;
   if(manifest?.scenarios?.length!==83)gates.push('Expected 83 declared interaction scenarios');
   if(!expectedManifest||JSON.stringify(checks.map(key).sort())!==JSON.stringify(expectedManifest.checks.map(key).sort()))gates.push('Interaction expected/actual coverage mismatch');
  }
  results[name]={receipt:'.omx/artifacts/visual-ralph/intercom/'+folder+'/receipt.json',status:failures.length||gates.length?'incomplete':'checks-clear',expected,observed:checks.length,failures,gates,diagnostics:name==='environment'?checks.filter(c=>nonempty(c.geometry?.outside)||nonempty(c.geometry?.overlaps)).map(c=>({width:c.width,route:c.route,outside:c.geometry.outside,overlaps:c.geometry.overlaps})):[],sourceHashes:h,commits:[d.before?.commit||d.sourceBefore?.commit||d.commit,d.after?.commit||d.sourceAfter?.commit||d.commit],widths:[...new Set(checks.map(c=>c.width).filter(Boolean))],firstCapture:checks.find(c=>c.capturedAt)?.capturedAt||checks.find(c=>c.timestamp)?.timestamp||d.started,lastCapture:checks.at(-1)?.capturedAt||checks.at(-1)?.timestamp||d.finished};
 }
 const fixedNavigationOcclusions=(data.environment?.checks||[]).filter(c=>c.geometry?.fixedNavigationOcclusion?.length).map(c=>({width:c.width,route:c.route,count:c.geometry.fixedNavigationOcclusion.length,classification:"Fixed navigation occludes scrollable page content at this scroll position; final-row clearance checked in responsive bottom captures."}));
 const routeCoverage=Object.values((data.responsive?.checks||[]).reduce((a,c)=>{const k=c.template==='/library/1'?'/library/[id]':c.template;a[k]??={template:k,cases:0,widths:[],states:[],fixtures:0,findings:0};a[k].cases++;a[k].widths.push(c.width);a[k].states.push(c.state||'base');if(c.fixture)a[k].fixtures++;if(bad(c))a[k].findings++;return a;},{})).map(r=>({...r,widths:[...new Set(r.widths)],states:[...new Set(r.states)]}));
 if(routeCoverage.length!==48&&results.responsive.status==='checks-clear'){results.responsive.status='incomplete';results.responsive.gates.push('Expected 48 route templates');}
 const zoomDir=path.join(base,'audit20261017/zoom'),zoomFiles=fs.existsSync(zoomDir)?fs.readdirSync(zoomDir,{withFileTypes:true}).filter(e=>e.isDirectory()).map(e=>path.join('audit20261017/zoom',e.name,'receipt.json')).sort().reverse():[];
 const zoom=zoomFiles.map(f=>({file:f,data:read(f)})).find(x=>x.data);
 const zoomResult=zoom?{receipt:'.omx/artifacts/visual-ralph/intercom/'+zoom.file.split(path.sep).join('/'),status:zoom.data.status,sourceHashes:hashes(zoom.data),certification:zoom.data.certification,limitations:zoom.data.limitations,routeChecks:zoom.data.checks?.length}:{status:'missing'};
 if(zoom&&hashes(zoom.data).some(h=>h!==source))zoomResult.status='source-mismatch';
 const limitations=['Intercom source CSS viewport/DPR/zoom, authenticated font and timed easing/duration are unavailable. Geist is an explicit substitute.','Native mobile/tablet/TV references and physical 4K hardware are unverified.','Interaction writes are browser-intercepted; separate real-save coverage exercises only the disposable backend.','Fresh music real-save needs a fourth fake artist identity; all three existing identities are occupied by preserved samples. Music sample states and fixture interactions are covered.','Settings-only shell exposes Music but API GET requires manage_library; this permission contract remains a backend integration dependency. Role fixtures certify shell behavior only.','External downloader/indexer/metadata/subtitle/notification delivery remains fake-provider or fixture bounded. No production build or combined backend release is certified.','Native zoom, if proven, covers measured Chromium zoom and document overflow on four sampled routes; component clipping and complete keyboard behavior at native zoom are excluded.'];
 const report={generatedAt:new Date().toISOString(),requiredSourceHash:source,currentSourceHash,status:currentSourceHash===source&&Object.values(results).every(r=>r.status==='checks-clear')&&['verified','unavailable'].includes(zoomResult.status)?'covered-checks-clear-with-limitations':'incomplete',suites:results,zoom:zoomResult,fixedNavigationOcclusions,routeCoverage,interactions:(data.interaction?.checks||[]).map(c=>({name:c.name,group:c.group,route:c.route,width:c.width,steps:c.steps,fixtures:c.fixtures,visitedRoutes:c.visitedRoutes,status:bad(c)?'finding':'clear'})),limitations,excluded:'All prior partial, failed and source-mismatched batches are excluded from final certification.'};
 fs.writeFileSync(path.join(__dirname,'coverage-executed-2026-10-17.json'),JSON.stringify(report,null,2)+'\n');
 const md=['# Executed Intercom preview audit coverage','',`Status: **${report.status}**`,`Frontend SHA-256: ${source}`,'','| Suite | Expected | Observed | Result |','| --- | ---: | ---: | --- |',...Object.entries(results).map(([k,r])=>`| ${k} | ${r.expected} | ${r.observed??0} | ${r.status} |`),'',`Native zoom: ${zoomResult.status}; exact scope in JSON.`,'','## Routes','','| Template | Cases | CSS widths | States | Fixture cases | Findings |','| --- | ---: | --- | --- | ---: | ---: |',...routeCoverage.map(r=>`| ${r.template} | ${r.cases} | ${r.widths.join(', ')} | ${r.states.join(', ')} | ${r.fixtures} | ${r.findings} |`),'','## Limits','',...limitations.map(x=>'- '+x),'',report.excluded];
 fs.writeFileSync(path.join(__dirname,'coverage-executed-2026-10-17.md'),md.join('\n')+'\n');return report;
}
if(require.main===module){const r=build();console.log(JSON.stringify({status:r.status,suites:r.suites,zoom:r.zoom},null,2));}
module.exports={build};
