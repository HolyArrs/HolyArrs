const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const repo = path.resolve(__dirname, '../../..');
const runId = process.argv[2];
assert.match(runId || '', /^[a-f0-9-]{36}$/);
const source = path.join(repo, '.omx/artifacts/visual-ralph/intercom/real-save', runId);
const receipt = JSON.parse(fs.readFileSync(path.join(source, 'receipt.json'), 'utf8'));
assert.equal(receipt.passed, true, 'Only a passing real-save run can become review evidence');
assert.ok(receipt.checks.length > 0 && receipt.checks.every(check => check.passed));
assert.ok(receipt.requests.length >= 5);
assert.deepEqual(receipt.sourceBefore, receipt.sourceAfter);
assert.deepEqual(receipt.processBefore, receipt.processAfter);
assert.deepEqual(receipt.blocked, []);
assert.deepEqual(receipt.pageErrors, []);
assert.equal(receipt.database?.verified, true);
assert.equal(receipt.database?.originalRowsPreserved, true);
assert.equal(receipt.database?.originalMediaPreserved, true);
assert.equal(receipt.cleanup?.verified, true);
assert.equal(receipt.cleanup?.disabledMembers, 2);
assert.equal(receipt.cleanup?.originalRowsPreserved, true);
assert.equal(receipt.cleanup?.originalMediaPreserved, true);
assert.ok(receipt.writes.length > 0 && receipt.writes.every(write => write.passedThrough && write.status >= 200 && write.status < 300));
const out = path.join(repo, 'web/app/static/preview-audit/real-save');
const imagesDir = path.join(out, runId);
fs.mkdirSync(imagesDir, { recursive: true });
for (const item of receipt.screenshots) {
  assert.equal(path.basename(item.file), item.file);
  assert.match(item.file, /\.png$/);
  const bytes = fs.readFileSync(path.join(source, item.file));
  assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a');
  fs.writeFileSync(path.join(imagesDir, item.file), bytes);
}
const html = value => String(value ?? '').replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
function publicPersistence(value) {
  const keys = ['verified', 'members', 'requests', 'requesterLinks', 'disabledMembers', 'revokedSessions', 'requestsPreserved', 'originalRowsPreserved', 'originalMediaPreserved'];
  return Object.fromEntries(keys.filter(key => Object.hasOwn(value, key)).map(key => [key, value[key]]));
}
const summary = {
  runId, started: receipt.started, finished: receipt.finished, source: receipt.sourceBefore,
  checks: receipt.checks.map(({ name, passed }) => ({ name, passed })),
  requests: receipt.requests, database: publicPersistence(receipt.database), cleanup: publicPersistence(receipt.cleanup),
  screenshots: receipt.screenshots, realWrites: receipt.writes.length,
  previousAttempts: (receipt.previousAttempts || []).map(attempt => ({
    checks: attempt.checks.map(({ name, passed }) => ({ name, passed })),
    finished: attempt.finished,
    cleanupVerified: attempt.cleanup?.verified === true
  })),
  continuationAuthentication: receipt.continuation ? 'Fixture sessions for the same two disposable members; no password changes' : null,
  backendHash: receipt.processBefore.backendHash
};
fs.writeFileSync(path.join(out, 'results.json'), JSON.stringify(summary, null, 2) + '\n');
const ordered = [...receipt.screenshots.filter(item => item.reviewOnly), ...receipt.screenshots.filter(item => !item.reviewOnly)];
const cards = ordered.map(item => `<figure data-width="${item.viewport.width}"><a href="${runId}/${encodeURIComponent(item.file)}" target="_blank" rel="noopener"><img loading="lazy" src="${runId}/${encodeURIComponent(item.file)}" alt="${html(item.label)} at ${item.viewport.width} pixels"></a><figcaption><strong>${html(item.label)}</strong><span>${item.viewport.width} × ${item.viewport.height} · ${html(item.route)}</span></figcaption></figure>`).join('\n');
const continuation = receipt.continuation ? '<li>The initial run created both members through invitation forms and saved the movie approval. After a test assertion was corrected, this same run continued with fixture sessions for those members. No passwords were changed. Earlier diagnostic screenshots are retained.</li>' : '';
const checks = continuation + summary.checks.map(item => `<li>Passed · ${html(item.name)}</li>`).join('');
fs.writeFileSync(path.join(out, 'index.html'), `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HolyArrs real-save verification</title><style>
*{box-sizing:border-box}body{margin:0;background:#f5f6f7;color:#171b23;font:15px/1.55 system-ui,sans-serif}main{max-width:1600px;padding:32px;margin:auto}h1{font-size:32px;margin:0 0 12px}p{max-width:100ch}a{color:#0964cb}code{overflow-wrap:anywhere}details{padding:16px;background:white;border:1px solid #dce0e6;border-radius:12px;margin:24px 0}summary{cursor:pointer;font-weight:650}.gallery{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,340px),1fr));gap:20px}figure{margin:0;background:#fff;border:1px solid #dce0e6;border-radius:12px;overflow:hidden}img{display:block;width:100%;height:300px;object-fit:contain;object-position:top;background:#e9edf2}figcaption{padding:16px}figcaption span{display:block;color:#626a79;overflow-wrap:anywhere}select{font:inherit;padding:8px;margin-bottom:20px}li{margin:6px 0}@media(max-width:600px){main{padding:20px}h1{font-size:27px}}
</style></head><body><main><h1>Real saves: request → approval → Library</h1><p>${summary.checks.length} passing named checks, ${summary.realWrites} accepted real writes and ${receipt.screenshots.length} screenshots. Movie, TV-season and selected-album requests were saved through the UI, then read back after refresh and independently checked in the disposable database. No successful API response was substituted by this runner.</p><p>The two new test members were disabled after the checks; their requests and approved titles remain visible in the preview. Existing users, credentials, settings and media were compared against a private baseline. The installed app and saved Air preview received no requests from this runner.</p><p>Preview backend: <code>${html(summary.backendHash)}</code>. Frontend: <code>${html(summary.source.frontend)}</code>. This verifies this preview combination; a later integrated build needs its own checks.</p><p><a href="results.json">Sanitized verification results</a> · <a href="/wishlist">View requests</a> · <a href="/library">View Library</a></p><details><summary>Checks and persistence evidence</summary><ul>${checks}</ul><pre style="white-space:pre-wrap;overflow-wrap:anywhere">${html(JSON.stringify({ requests: summary.requests, database: summary.database, cleanup: summary.cleanup }, null, 2))}</pre></details><label for="width">Screenshot width </label><select id="width"><option value="all">All widths</option>${[...new Set(receipt.screenshots.map(item => item.viewport.width))].sort((a,b)=>a-b).map(width=>`<option value="${width}">${width}px</option>`).join('')}</select><section class="gallery">${cards}</section></main><script>document.getElementById('width').addEventListener('change',e=>{document.querySelectorAll('figure').forEach(card=>card.hidden=e.target.value!=='all'&&card.dataset.width!==e.target.value)})</script></body></html>`);
process.stdout.write(JSON.stringify({ runId, checks: summary.checks.length, screenshots: receipt.screenshots.length, gallery: '/preview-audit/real-save/index.html' }) + '\n');
