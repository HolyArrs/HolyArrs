// Audits the sign-in reveal in fresh, signed-out contexts.
// Usage: node scripts/audit-signin-reveal.mjs <width> <height> <outdir> [cpu=1] [strip=0] [base=http://10.10.25.25:18729]
// Prints one JSON line: rAF frame gaps (boot, wave, after), long tasks, layout
// shifts, main-thread task time, WebGL draw-call cost, JS heap, console warnings.
// With strip=1 it also saves a frame strip (CDP screencast frames, binned to 100 ms).
import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'node:fs';
const [w = '1440', h = '900', out = '.', cpu = '1', strip = '0', base = 'http://10.10.25.25:18729'] = process.argv.slice(2);
const browser = await chromium.launch({ args: (process.env.GL ?? '--use-angle=swiftshader --enable-unsafe-swiftshader').split(' ') });
// Warm the browser itself (GPU process, compositor) on a blank page in its own context, as a
// browser that is already open would be; the measured context stays fresh and signed out.
{ const wc = await browser.newContext(); const wp = await wc.newPage(); await wp.setContent('<p style="font:16px sans-serif">warm</p><img id=i>'); await wp.evaluate(async () => { const c = document.createElement('canvas'); c.width = 1280; c.height = 720; const g = c.getContext('2d'); for (let i = 0; i < 400; i++) { g.fillStyle = 'hsl(' + i + ',60%,50%)'; g.fillRect(Math.random() * 1280, Math.random() * 720, 90, 90); } const im = document.getElementById('i'); im.src = c.toDataURL('image/jpeg', 0.8); await im.decode(); if (location.hash !== '#x' && (globalThis.WARMGL ?? true)) document.createElement('canvas').getContext('webgl2')?.clear(0); }); await wp.waitForTimeout(600); await wc.close(); }
const ctx = await browser.newContext({ viewport: { width: +w, height: +h } });
await ctx.addInitScript(() => {
	const f = (window.__frames = []);
	const tick = (t) => { f.push(t); if (t < 6000) requestAnimationFrame(tick); };
	requestAnimationFrame(tick);
	window.__lt = []; window.__cls = [];
	try { new PerformanceObserver((l) => l.getEntries().forEach((e) => window.__lt.push([Math.round(e.startTime), Math.round(e.duration)]))).observe({ type: 'longtask', buffered: true }); } catch {}
	try { new PerformanceObserver((l) => l.getEntries().forEach((e) => window.__cls.push([Math.round(e.startTime), +e.value.toFixed(4)]))).observe({ type: 'layout-shift', buffered: true }); } catch {}
	// CPU cost of each wave draw call (submission side).
	window.__draw = [];
	const P = window.WebGL2RenderingContext?.prototype;
	if (P) { const d = P.drawArrays; P.drawArrays = function (...a) { const s = performance.now(); const r = d.apply(this, a); window.__draw.push(performance.now() - s); return r; }; }
	const mark = () => {
		const kb = document.querySelector('.art .kb');
		if (kb?.classList.contains('mosaic') && !window.__mz) window.__mz = performance.now();
		if (kb?.classList.contains('sharp') && !window.__sh) window.__sh = performance.now();
		if (!window.__sh) requestAnimationFrame(mark);
	};
	requestAnimationFrame(mark);
});
const page = await ctx.newPage();
const warns = [];
page.on('console', (m) => { if (m.type() === 'warning' || m.type() === 'error') warns.push(m.text().slice(0, 160)); });
const cdp = await ctx.newCDPSession(page);
await cdp.send('Performance.enable');
if (+cpu > 1) await cdp.send('Emulation.setCPUThrottlingRate', { rate: +cpu });
const shots = [];
if (strip === '1') {
	mkdirSync(out, { recursive: true });
	cdp.on('Page.screencastFrame', (f) => { shots.push(f); cdp.send('Page.screencastFrameAck', { sessionId: f.sessionId }).catch(() => {}); });
	await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 70, everyNthFrame: 1 });
}
const nav0 = Date.now();
await page.goto(`${base}/login`, { waitUntil: 'commit' });
await page.waitForFunction(() => window.__sh || performance.now() > 5500, null, { timeout: 15000, polling: 50 }).catch(() => {});
await page.waitForTimeout(600);
const m = await cdp.send('Performance.getMetrics');
const metric = (n) => m.metrics.find((x) => x.name === n)?.value ?? 0;
const r = await page.evaluate(() => ({ frames: window.__frames, lt: window.__lt, cls: window.__cls, draw: window.__draw, mz: window.__mz, sh: window.__sh, heap: performance.memory?.usedJSHeapSize }));
if (strip === '1') {
	await cdp.send('Page.stopScreencast');
	// Bin screencast frames to 100 ms steps from navigation (wall clock).
	const t0 = shots.length ? shots[0].metadata.timestamp : 0;
	let next = 0;
	for (const f of shots) {
		const ms = (f.metadata.timestamp - t0) * 1000;
		if (ms >= next && next <= 3000) { writeFileSync(`${out}/strip-${w}-${String(next).padStart(4, '0')}ms.jpg`, Buffer.from(f.data, 'base64')); next += 100; }
	}
}
const fr = r.frames, gaps = fr.slice(1).map((t, i) => [fr[i], t - fr[i]]);
const seg = (a, b) => gaps.filter(([t]) => t >= a && t < b).map(([, g]) => g);
const sum = (a) => ({ n: a.length, max: +Math.max(0, ...a).toFixed(1), p95: +(a.slice().sort((x, y) => x - y)[Math.floor(a.length * 0.95)] ?? 0).toFixed(1), over17: a.filter((g) => g > 17.5).length, over34: a.filter((g) => g > 34).length });
const mz = r.mz ?? 0, sh = r.sh ?? 5000;
const d = r.draw.slice().sort((a, b) => a - b);
console.log(JSON.stringify({
	size: `${w}x${h}`, cpu: +cpu, mosaicAt: Math.round(mz), sharpAt: Math.round(sh),
	boot: sum(seg(0, mz)), wave: sum(seg(mz, sh)), after: sum(seg(sh, sh + 600)),
	slow: gaps.filter(([, g]) => g > 17.5).map(([t, g]) => [Math.round(t), Math.round(g)]),
	longTasks: r.lt, cls: +r.cls.reduce((a, [, v]) => a + v, 0).toFixed(4), shifts: r.cls,
	mainThreadMs: { script: Math.round(metric('ScriptDuration') * 1000), layout: Math.round(metric('LayoutDuration') * 1000), style: Math.round(metric('RecalcStyleDuration') * 1000), task: Math.round(metric('TaskDuration') * 1000), layouts: metric('LayoutCount'), recalcs: metric('RecalcStyleCount') },
	drawMs: { n: d.length, p50: +(d[d.length >> 1] ?? 0).toFixed(3), max: +(d.at(-1) ?? 0).toFixed(3) },
	heapMB: +((r.heap ?? 0) / 1048576).toFixed(1), warns, strip: shots.length
}));
await browser.close();

