// Publish only screenshots from a passing local preview receipt.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const repo=path.resolve(__dirname,'../../..');
const evidence=path.resolve(process.argv[2]);
assert(evidence.startsWith(path.join(repo,'.omx/artifacts/visual-ralph/intercom/music-download')+path.sep));
const receipt=JSON.parse(fs.readFileSync(path.join(evidence,'receipt.json')));assert.equal(receipt.status,'passed');
const dest=path.join(repo,'web/app/static/preview-audit/music-download-20261017');fs.mkdirSync(dest,{recursive:true});
for(const name of receipt.screenshots){assert.equal(path.basename(name),name);fs.copyFileSync(path.join(evidence,name),path.join(dest,name));}
fs.writeFileSync(path.join(dest,'index.html'),`<!doctype html><meta name="viewport" content="width=device-width"><title>Music lifecycle verification</title><style>body{font:16px system-ui;margin:32px;background:#f6f6f6;color:#222}img{max-width:100%;height:auto;border:1px solid #ccc}section{margin:32px 0}a{color:#145ac0}</style><h1>Music lifecycle verification</h1><p>Preview only · ${receipt.commit} · ${receipt.finished}</p><p>Download 35: Done, 3 of 3 tracks. Evening Signals: In library. Request: Ready to listen. Synthetic disposable audio.</p>${receipt.screenshots.map(n=>`<section><h2>${n}</h2><a href="${n}"><img loading="lazy" src="${n}" alt="${n}"></a></section>`).join('')}`);
console.log(dest);
