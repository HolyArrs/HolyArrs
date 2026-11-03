// Phone tap sizes on the 18729 preview (#79): every page at 390 wide on a touch phone, every visible
// control must be a 44px-tall target (its own box, a wrapping label, a segmented track or a ::after hit
// area), pages must not scroll sideways and must have an h1. Run through the e2e lock so the login
// file is handed over and never printed:
//   e2e-run.ps1 'node scripts/phone-tap-survey.mjs' -Who <tag> -As owner
// Optional OUT=<file.json> writes the full result. Exits 1 when a control outside EXCEPTIONS is short.
import { chromium, devices } from 'playwright';
import { readFileSync, writeFileSync } from 'node:fs';

const B = process.env.HOLYARRS_PREVIEW_URL ?? 'http://localhost:18729';
// Allowed on purpose, with the reason the #79 claim gives.
const EXCEPTIONS = [
	// 27 letters cannot each be 44px on a phone; each is a 24px target (WCAG 2.5.8).
	{ k: /^button\.rail-l$/, why: 'A–Z rail' },
	// The whole row card opens the same title and is far taller than 44px.
	{ k: /^a$/, path: /^\/library/, why: 'Library row title link (the row is the target)' }
];
const t = readFileSync(process.env.HOLYARRS_SHOWCASE_LOGIN, 'utf8');
const name = (t.match(/(?:name|user(?:name)?)\s*[:=]\s*(\S+)/i) || [])[1];
const pw = (t.match(/password\s*[:=]\s*(\S+)/i) || [])[1];
const browser = await chromium.launch();
const ctx = await browser.newContext({ ...devices['Pixel 7'], viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();
await page.goto(B + '/login');
const local = page.getByRole('button', { name: 'Use a HolyArrs account' });
if (await local.isVisible().catch(() => false)) await local.click();
await page.getByLabel('Name').fill(name);
await page.getByLabel('Password', { exact: true }).fill(pw);
await page.getByRole('button', { name: 'Sign in', exact: true }).click();
await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20000 });

const survey = () => page.evaluate(() => {
	const cw = document.documentElement.clientWidth, sw = document.documentElement.scrollWidth;
	const cls = (el) => `${el.tagName.toLowerCase()}${[...el.classList].filter((x) => !x.startsWith('svelte-')).map((x) => '.' + x).join('')}`;
	const pseudo = (el) => Math.max(...['::after', '::before'].map((p) => { const s = getComputedStyle(el, p); return s.content !== 'none' && s.position === 'absolute' ? parseFloat(s.height) || 0 : 0; }));
	const sel = 'a[href], button, input:not([type=hidden]), select, textarea, summary, [role=button], [role=tab], [role=radio], [role=checkbox], [role=switch], [role=menuitem], [role=option]';
	const out = [];
	for (const el of document.querySelectorAll(sel)) {
		const r = el.getBoundingClientRect();
		if (!r.width || !r.height || r.bottom < 0 || r.right < 0 || r.left > cw) continue;
		const cs = getComputedStyle(el);
		// Hidden, or not a target at all (pointer-events: none, e.g. a disabled stand-in icon).
		if (cs.visibility === 'hidden' || cs.pointerEvents === 'none' || el.classList.contains('sr-only') || el.closest('[inert], [aria-hidden=true]')) continue;
		// Links in running text are exempt (WCAG 2.5.8); a linked heading is a control.
		if (el.tagName === 'A' && cs.display === 'inline' && !el.parentElement?.matches('h1, h2, h3, h4, h5, h6')) continue;
		if (r.width <= 2 && r.height <= 2) continue; // visually hidden inputs
		const host = el.closest('label') || el.closest('.ui-seg') || el;
		const h = Math.max(r.height, host.getBoundingClientRect().height, pseudo(el));
		if (h >= 44) continue;
		const text = (el.getAttribute('aria-label') || el.textContent || el.getAttribute('placeholder') || '').trim().replace(/\s+/g, ' ').slice(0, 24);
		out.push({ k: cls(el), text, w: Math.round(r.width), h: Math.round(h) });
	}
	return { path: location.pathname + location.search, sideways: sw > cw ? sw : false, h1: !!document.querySelector('h1'), short: out };
});
async function links(u, re, n) {
	await page.goto(B + u);
	await page.waitForTimeout(3000);
	const hs = await page.evaluate((src) => [...new Set([...document.querySelectorAll('a[href]')].map((a) => a.getAttribute('href')).filter((h) => h && new RegExp(src).test(h)))], re.source);
	return hs.slice(0, n);
}

const pages = ['/', '/explore', '/new', '/search', '/library', '/library?kind=movie', '/library?kind=show', '/library?kind=music', '/library/collections', '/library/held', '/library/turned-down', '/library/missing-subtitles', '/library/copies', '/calendar', '/people', '/requests', '/downloads', '/downloads/history', '/downloads/turned-down', '/status', '/account', '/settings', '/settings/account', '/settings/this-app', '/settings/appearance', '/settings/parts', '/settings/library-folders', '/settings/naming', '/settings/pick-rules', '/settings/search', '/settings/sources', '/settings/downloaders', '/settings/subtitles', '/settings/users', '/settings/sign-in', '/settings/alerts', '/settings/media-servers', '/settings/schedules', '/settings/backups', '/settings/health', '/settings/logs', '/settings/api', '/settings/music', '/settings/wishlist', '/settings/outside-lists', '/settings/title-defaults', '/settings/move', '/settings/danger'];
// One of each detail page, picked from the lists.
const details = [
	...(await links('/library?kind=movie', /^\/library\/\d+$/, 1)), ...(await links('/library?kind=show', /^\/library\/\d+$/, 1)),
	...(await links('/explore', /^\/title\/\w+\/\d+/, 1)), ...(await links('/library?kind=music', /^\/(library\/\d+|music\/artist\/)/, 1)),
	...(await links('/explore?kind=music', /^\/music\/(artist|album)\//, 1)), ...(await links('/people', /^\/people\/\d+/, 1)),
	...(await links('/library/collections', /^\/library\/collections\/\d+/, 1))
];
const results = [];
let bad = 0;
for (const u of [...pages, ...new Set(details)]) {
	try {
		await page.goto(B + u);
		// Wait for the page title (slow data loads), then let it settle.
		await page.locator('h1').first().waitFor({ timeout: 8000 }).catch(() => {});
		await page.waitForTimeout(1000);
		const s = await survey();
		s.allowed = s.short.filter((o) => EXCEPTIONS.some((e) => e.k.test(o.k) && (!e.path || e.path.test(s.path))));
		s.short = s.short.filter((o) => !s.allowed.includes(o));
		results.push(s);
		const fail = s.sideways || !s.h1 || s.short.length;
		if (fail) bad++;
		const groups = {};
		for (const o of s.short) (groups[o.k] ||= []).push(`${o.text} ${o.w}x${o.h}`);
		console.log(fail ? 'FAIL' : 'ok  ', s.path, s.sideways ? `sideways ${s.sideways}px` : '', s.h1 ? '' : 'no h1', Object.entries(groups).map(([k, v]) => `${k}×${v.length} [${v.slice(0, 3).join(' | ')}]`).join(' ;; '));
	} catch (e) {
		bad++;
		console.log('FAIL', u, 'error', String(e).slice(0, 120));
	}
}
if (process.env.OUT) writeFileSync(process.env.OUT, JSON.stringify(results, null, 1));
console.log(`pages ${results.length}, failing ${bad}`);
await browser.close();
process.exit(bad ? 1 : 0);
