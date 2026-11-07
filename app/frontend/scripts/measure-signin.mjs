// Measures the sign-in page: panel visible + name typeable, wallpaper sharp. Usage: node scripts/measure-signin.mjs <label> <outdir>
import { chromium } from 'playwright';
const [label = 'run', out = '.'] = process.argv.slice(2);
const url = 'http://127.0.0.1:18729/login';
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
const probe = () => page.evaluate(() => {
	const hl = document.getElementById('hl');
	const loader = !!hl && !hl.hidden && getComputedStyle(hl).display !== 'none' && getComputedStyle(hl).opacity !== '0';
	const p = document.querySelector('.panel');
	const op = p ? parseFloat(getComputedStyle(p).opacity) : 0;
	const inp = document.getElementById('name');
	const typeable = !!inp && !inp.disabled && op > 0.95 && !loader;
	const pic = document.querySelector('.art .layer.top');
	const canvasOn = !!document.querySelector('.art canvas.on');
	const sharp = !!pic && parseFloat(getComputedStyle(pic).opacity) > 0.95 && !canvasOn && parseFloat(getComputedStyle(document.querySelector('.art')).opacity) > 0.95;
	return { t: performance.now(), loader, typeable, sharp, focused: document.activeElement?.id };
});
for (const run of ['cold', 'warm']) {
	const t0 = Date.now();
	await page.goto(url, { waitUntil: 'commit' });
	let panelAt = null, sharpAt = null, focused = null, shotN = 0;
	const stamps = [0, 150, 400, 900, 1600, 2600];
	while (Date.now() - t0 < 7000 && (panelAt === null || sharpAt === null)) {
		let s; try { s = await probe(); } catch { continue; }
		if (panelAt === null && s.typeable) { panelAt = Math.round(s.t); focused = s.focused; }
		if (sharpAt === null && s.sharp) sharpAt = Math.round(s.t);
		if (shotN < stamps.length && s.t >= stamps[shotN]) { try { await page.screenshot({ path: `${out}/${label}-${run}-${String(stamps[shotN]).padStart(4, "0")}ms.png` }); } catch {} shotN++; }
		await page.waitForTimeout(16);
	}
	console.log(JSON.stringify({ label, run, panelTypeableMs: panelAt, wallpaperSharpMs: sharpAt, focused }));
}
await browser.close();
