// Publish only screenshots from a passing local preview receipt.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const repo=path.resolve(__dirname,'../../..');
const evidence=path.resolve(process.argv[2]);
assert(evidence.startsWith(path.join(repo,'.omx/artifacts/visual-ralph/intercom/music-upgrade')+path.sep));
const receipt=JSON.parse(fs.readFileSync(path.join(evidence,'receipt.json')));assert.equal(receipt.status,'passed');
const dest=path.join(repo,'web/app/static/preview-audit/music-upgrade-20261017');fs.mkdirSync(dest,{recursive:true});
for(const name of receipt.screenshots){assert.equal(path.basename(name),name);fs.copyFileSync(path.join(evidence,name),path.join(dest,name));}
fs.writeFileSync(path.join(dest,'index.html'),`<!doctype html><meta name="viewport" content="width=device-width"><title>Lossless replacement verification</title><style>body{font:16px system-ui;margin:32px;background:#f6f6f6;color:#222}img{max-width:100%;height:auto;border:1px solid #ccc}section{margin:32px 0}a{color:#145ac0}</style><h1>Lossless replacement verification</h1><p>Preview only · ${receipt.commit} · ${receipt.finished}</p><p>Download 36: Done. Three FLAC tracks in library, zero Better copy sought. Request: Ready to listen. Old disposable MP3 copies retained in holding.</p>${receipt.screenshots.map(n=>`<section><h2>${n}</h2><a href="${n}"><img loading="lazy" src="${n}" alt="${n}"></a></section>`).join('')}`);
console.log(dest);
