// Times the sign-in reveal: first frame, panel, name field, and every frame of
// the wallpaper's mosaic reveal. Usage: node scripts/measure-signin-reveal.mjs <width> <height> <outdir>
import { chromium } from 'playwright';
const [w = '1440', h = '900', out = '.', mode = 'shots', at = '0,300,800,1500,2500'] = process.argv.slice(2);
const url = 'http://127.0.0.1:18729/login';
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: +w, height: +h } });
await ctx.addInitScript(() => {
	const f = (window.__frames = []);
	const tick = (t) => {
		f.push(t);
		if (t < 4000) requestAnimationFrame(tick);
	};
	requestAnimationFrame(tick);
	// Was the mosaic thumbnail painted before any app script ran?
	document.addEventListener('DOMContentLoaded', () => (window.__seeded = !!document.getElementById('holyarrs-signin-seed')));
});
const page = await ctx.newPage();
const probe = () =>
	page.evaluate(() => {
		const p = document.querySelector('.panel');
		const inp = document.getElementById('name');
		const kb = document.querySelector('.art .kb');
		return {
			t: performance.now(),
			seeded: window.__seeded ?? null,
			panel: !!p && p.getBoundingClientRect().height > 0,
			input: !!inp && document.activeElement === inp,
			mosaic: !!kb?.classList.contains('mosaic'),
			sharp: !!kb?.classList.contains('sharp')
		};
	});
for (const run of ['cold', 'warm']) {
	await page.goto(url, { waitUntil: 'commit' });
	const marks = { panel: null, input: null, mosaic: null, sharp: null, seededFirstFrame: null };
	const stamps = at.split(',').map(Number);
	let shotN = 0;
	const t0 = Date.now();
	while (Date.now() - t0 < 6000) {
		let s;
		try { s = await probe(); } catch { continue; }
		if (marks.seededFirstFrame === null && s.seeded !== null) marks.seededFirstFrame = s.seeded;
		for (const k of ['panel', 'input', 'mosaic', 'sharp']) if (marks[k] === null && s[k]) marks[k] = Math.round(s.t);
		if (mode === 'shots' && shotN < stamps.length && s.t >= stamps[shotN]) {
			await page.screenshot({ path: `${out}/reveal-${w}-${run}-${String(stamps[shotN]).padStart(4, '0')}ms.png` });
			shotN++;
		}
		if ((mode !== 'shots' || shotN >= stamps.length) && marks.sharp !== null) break;
		await page.waitForTimeout(10);
	}
	await page.waitForTimeout(800);
	const frames = await page.evaluate(() => window.__frames);
	const gaps = frames.slice(1).map((t, i) => t - frames[i]);
	const inReveal = gaps.filter((_, i) => frames[i] >= (marks.mosaic ?? 0) && frames[i] <= (marks.sharp ?? 4000));
	const over = (a, ms) => a.filter((g) => g > ms).length;
	console.log(JSON.stringify({ size: `${w}x${h}`, run, ...marks, frames: inReveal.length, maxGapMs: Math.round(Math.max(0, ...inReveal) * 10) / 10, over17: over(inReveal, 17.5), over20: over(inReveal, 20), over34: over(inReveal, 34), slowAt: gaps.map((g, i) => [Math.round(frames[i]), Math.round(g)]).filter(([t, g]) => g > 20 && t > (marks.mosaic ?? 0) - 50 && t < (marks.sharp ?? 4000)) }));
	// The thumbnail for the next visit is made 3.2 s after the first picture shows.
	await page.waitForTimeout(3000);
}
await browser.close();
