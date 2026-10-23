<script lang="ts">
	// The sign-in page's edge frame and panel portals. A solid frame sits
	// inset on every browser edge (as on coderluii.dev). From 5 s in, a hole
	// tears open in the sign-in panel's glass near one corner, inside its
	// padding; the page's media symbols (film frame, music note, play;
	// lib/pixelTrail.ts) rise out of it, run along the padding band to a hole
	// at the next corner (counter-clockwise from top right, one leg every
	// 5 s) and dive in; both holes seal. Phones use the bottom sheet the same
	// way. One canvas, time based; the clock stops while the tab is hidden or
	// the user is signing in; off with reduced motion.
	import { onMount } from 'svelte';
	import { MASKS } from '#lib/pixelTrail.ts';
	import { interacting } from '#lib/signinArt.ts';
	import { legAt, holeAt, symbolAt, easeInOutSine, clamp01, COUNT } from '#lib/signinPortals.ts';

	let canvas: HTMLCanvasElement | undefined = $state();
	// The page forces dark on <main>; the frame follows the app's real theme.
	let theme = $state<'light' | 'dark'>('dark');

	// Note, film, note, play, note, film, play: the music note leads and repeats.
	const ORDER = [1, 0, 1, 2, 1, 0, 2];

	onMount(() => {
		const html = document.documentElement;
		const sys = window.matchMedia('(prefers-color-scheme: dark)');
		const readTheme = () => (theme = html.dataset.theme === 'light' ? 'light' : html.dataset.theme === 'dark' ? 'dark' : sys.matches ? 'dark' : 'light');
		readTheme();
		const mo = new MutationObserver(readTheme);
		mo.observe(html, { attributes: true, attributeFilter: ['data-theme'] });
		sys.addEventListener('change', readTheme);
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		const cv = canvas!;
		const ctx = cv.getContext('2d');
		if (!ctx) return;
		let w = 0, h = 0, dpr = 1;
		let accent = '#ff453a';
		let sprites: HTMLCanvasElement[] = [];
		let soft: HTMLCanvasElement[] = [];
		let PX = 2; // one symbol pixel, whole device pixels so symbols stay crisp

		function sprite(mask: readonly string[], blur = 0): HTMLCanvasElement {
			const pad = 6;
			const c = document.createElement('canvas');
			const size = (8 * PX + pad * 2) * dpr;
			c.width = c.height = size;
			const g = c.getContext('2d')!;
			g.scale(dpr, dpr);
			if (blur) g.filter = `blur(${blur}px)`;
			g.shadowColor = 'rgb(0 0 0 / 0.55)';
			g.shadowBlur = 4;
			g.fillStyle = '#f5f5f7';
			for (let r = 0; r < 8; r++) for (let col = 0; col < 8; col++) if (mask[r][col] === '1') g.fillRect(pad + col * PX, pad + r * PX, PX, PX);
			return c;
		}
		function measure() {
			dpr = Math.min(2, window.devicePixelRatio || 1);
			w = window.innerWidth;
			h = window.innerHeight;
			cv.width = Math.round(w * dpr);
			cv.height = Math.round(h * dpr);
			const a = getComputedStyle(cv).getPropertyValue('--accent').trim();
			if (/^#[0-9a-f]{6}$/i.test(a)) accent = a;
			measurePanel();
			PX = Math.max(1, Math.round((box.band * 0.6) / 8));
			sprites = MASKS.map((m) => sprite(m));
			soft = MASKS.map((m) => sprite(m, 2.5));
		}

		// The path runs inside the panel, along the middle of its padding band,
		// rounded like the panel's own corners. Corners counter-clockwise from
		// top right (TR, TL, BL, BR); a hole sits mid-arc at each corner.
		let box = { x0: 0, y0: 0, x1: 0, y1: 0, r: [0, 0, 0, 0], band: 0 };
		function measurePanel() {
			const el = document.querySelector<HTMLElement>('[data-signin-panel]');
			if (!el) return;
			const b = el.getBoundingClientRect();
			const cs = getComputedStyle(el);
			const z = el.offsetWidth ? b.width / el.offsetWidth : 1;
			const px = (v: string) => (parseFloat(v) || 0) * z;
			// Clipped to the frame (the phone sheet runs under it at the sides and bottom).
			const edge = document.querySelector('.edge');
			const f = edge ? parseFloat(getComputedStyle(edge).borderTopWidth) || 0 : 0;
			const left = Math.max(b.left, f), right = Math.min(b.right, w - f), bottom = Math.min(b.bottom, h - f);
			const band = Math.min(px(cs.paddingTop), px(cs.paddingLeft) - (left - b.left), px(cs.paddingRight) - (b.right - right), px(cs.paddingBottom) - (b.bottom - bottom));
			const i = band / 2;
			const rad = [cs.borderTopRightRadius, cs.borderTopLeftRadius, cs.borderBottomLeftRadius, cs.borderBottomRightRadius].map((v) => Math.max(4, px(v) - i));
			box = { x0: left + i, y0: b.top + i, x1: right - i, y1: bottom - i, r: rad, band };
		}
		const MID = (k: number) => -Math.PI / 4 - (k * Math.PI) / 2;
		function centre(k: number): [number, number] {
			const { x0, y0, x1, y1, r } = box;
			k %= 4;
			const q = r[k];
			return k === 0 ? [x1 - q, y0 + q] : k === 1 ? [x0 + q, y0 + q] : k === 2 ? [x0 + q, y1 - q] : [x1 - q, y1 - q];
		}
		function cornerPoint(k: number): [number, number] {
			const [cx, cy] = centre(k), q = box.r[k % 4];
			return [cx + q * Math.cos(MID(k)), cy + q * Math.sin(MID(k))];
		}
		// Leg k (corner k to k+1) at arclength fraction u: half arc, edge, half arc.
		function legPoint(k: number, u: number): [number, number] {
			const ra = box.r[k % 4], rb = box.r[(k + 1) % 4];
			const qa = (ra * Math.PI) / 4, qb = (rb * Math.PI) / 4;
			const [ax, ay] = centre(k), [bx, by] = centre(k + 1);
			const sx = ax + ra * Math.cos(MID(k) - Math.PI / 4), sy = ay + ra * Math.sin(MID(k) - Math.PI / 4);
			const ex = bx + rb * Math.cos(MID(k + 1) + Math.PI / 4), ey = by + rb * Math.sin(MID(k + 1) + Math.PI / 4);
			const line = Math.hypot(ex - sx, ey - sy);
			const d = u * (qa + line + qb);
			if (d < qa) { const a = MID(k) - d / ra; return [ax + ra * Math.cos(a), ay + ra * Math.sin(a)]; }
			if (d < qa + line) { const f = (d - qa) / line; return [sx + (ex - sx) * f, sy + (ey - sy) * f]; }
			const a = MID(k + 1) + Math.PI / 4 - (d - qa - line) / rb;
			return [bx + rb * Math.cos(a), by + rb * Math.sin(a)];
		}

		// A cut in the glass: dark depth, shadow under the far (top) rim, a thin
		// highlight where light catches the near (bottom) rim, a slightly torn
		// edge while it tears open or seals.
		function holePath(r: number, tear: number, seed: number) {
			ctx!.beginPath();
			for (let j = 0; j <= 28; j++) {
				const a = (j / 28) * Math.PI * 2;
				const rr = r * (1 + tear * (0.09 * Math.sin(a * 5 + seed) + 0.05 * Math.sin(a * 9 - seed * 2)));
				const x = rr * Math.cos(a), y = rr * Math.sin(a);
				if (j) ctx!.lineTo(x, y);
				else ctx!.moveTo(x, y);
			}
			ctx!.closePath();
		}
		function drawHole(x: number, y: number, s: number, squash: number, seed: number) {
			if (s <= 0.001) return;
			const r = Math.max(5, box.band * 0.4) * s;
			if (r < 1) return;
			const tear = Math.min(1, Math.abs(squash) * 6 + (1 - Math.min(1, s)) * 1.2);
			const c = ctx!;
			c.save();
			c.translate(x, y);
			c.scale(1 + squash, 1 - squash);
			holePath(r, tear, seed);
			const depth = c.createRadialGradient(0, r * 0.35, 0, 0, 0, r);
			depth.addColorStop(0, '#000');
			depth.addColorStop(0.7, '#050506');
			depth.addColorStop(1, '#141417');
			c.fillStyle = depth;
			c.fill();
			c.save();
			c.clip();
			// Inner shadow on the far rim.
			c.lineWidth = r * 0.9;
			c.strokeStyle = 'rgb(0 0 0 / 0.7)';
			c.beginPath();
			c.arc(0, r * 0.55, r * 1.25, Math.PI * 1.15, Math.PI * 1.85);
			c.stroke();
			// A faint accent glow deep inside.
			const glow = c.createRadialGradient(0, r * 0.2, 0, 0, r * 0.2, r * 0.8);
			glow.addColorStop(0, accent + '44');
			glow.addColorStop(1, accent + '00');
			c.fillStyle = glow;
			c.fillRect(-r, -r, r * 2, r * 2);
			c.restore();
			// Light catching the near rim.
			c.lineWidth = 1;
			c.strokeStyle = 'rgb(255 255 255 / 0.55)';
			c.beginPath();
			c.arc(0, 0, r - 0.5, Math.PI * 0.15, Math.PI * 0.85);
			c.stroke();
			c.lineWidth = 0.75;
			c.strokeStyle = 'rgb(0 0 0 / 0.5)';
			c.beginPath();
			c.arc(0, 0, r - 0.4, Math.PI * 1.15, Math.PI * 1.85);
			c.stroke();
			c.restore();
		}

		function draw(t: number) {
			ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
			ctx!.clearRect(0, 0, w, h);
			const leg = legAt(t, 4);
			if (!leg || !box.r) return;
			const k = leg.index;
			const [ax, ay] = cornerPoint(k);
			const [bx, by] = cornerPoint(k + 1);
			const a = holeAt(leg.local, 'from');
			const b = holeAt(leg.local, 'to');
			drawHole(ax, ay, a.s, a.squash, k);
			drawHole(bx, by, b.s, b.squash, k + 1.7);
			const pcx = (box.x0 + box.x1) / 2, pcy = (box.y0 + box.y1) / 2;
			const half = 4 * PX + 6;
			for (let i = 0; i < COUNT; i++) {
				const p = symbolAt(leg.local, i);
				if (p === null) continue;
				const [px, py] = legPoint(k, easeInOutSine(p));
				const tt = t * 1000 + i * 290;
				const ox = px - pcx, oy = py - pcy, ol = Math.hypot(ox, oy) || 1;
				// Rising out of the hole and diving into the next: scale and focus.
				const grow = Math.min(clamp01(p / 0.1), clamp01((1 - p) / 0.1));
				const bob = Math.sin(tt / 170) * 1.2 * grow;
				const x = px + (ox / ol) * bob, y = py + (oy / ol) * bob;
				const sc = (0.15 + 0.85 * grow);
				const rot = Math.sin(tt / 230) * 0.2 + (1 - grow) * 1.4 * (p > 0.5 ? 1 : -1);
				const m = ORDER[i % ORDER.length];
				ctx!.setTransform(dpr * sc * Math.cos(rot), dpr * sc * Math.sin(rot), -dpr * sc * Math.sin(rot), dpr * sc * Math.cos(rot), dpr * x, dpr * y);
				const vis = 0.92 * clamp01(grow * 1.6);
				ctx!.globalAlpha = vis * (1 - grow);
				ctx!.drawImage(soft[m], -half, -half, half * 2, half * 2);
				ctx!.globalAlpha = vis * grow;
				ctx!.drawImage(sprites[m], -half, -half, half * 2, half * 2);
			}
			ctx!.globalAlpha = 1;
		}

		// A clock that only moves while the page is visible and the user is idle.
		let clock = 0, last = 0, raf = 0, lastInput = -1e9, engaged = false;
		const isField = (el: Element | null) => !!el && el.matches('input, textarea, select, [contenteditable="true"]');
		function loop(now: number) {
			raf = 0;
			if (document.hidden || motion.matches) return;
			const dt = last ? Math.min(0.05, (now - last) / 1000) : 0;
			last = now;
			if (!interacting(now, lastInput, isField(document.activeElement), engaged)) clock += dt;
			draw(clock);
			raf = requestAnimationFrame(loop);
		}
		function start() {
			if (raf || document.hidden || motion.matches) return;
			last = 0;
			raf = requestAnimationFrame(loop);
		}
		function stop() {
			if (raf) cancelAnimationFrame(raf);
			raf = 0;
		}
		function onVisible() {
			if (document.hidden) stop();
			else start();
		}
		function onMotion() {
			if (motion.matches) {
				stop();
				ctx!.setTransform(1, 0, 0, 1, 0, 0);
				ctx!.clearRect(0, 0, cv.width, cv.height);
			} else start();
		}
		function onInteract() {
			engaged = true;
			lastInput = performance.now();
		}
		function onBlur() {
			if (engaged) lastInput = performance.now();
		}
		let rt: ReturnType<typeof setTimeout> | undefined;
		function onResize() {
			clearTimeout(rt);
			rt = setTimeout(measure, 150);
		}

		measure();
		const panel = document.querySelector('[data-signin-panel]');
		const ro = new ResizeObserver(() => measurePanel());
		if (panel) ro.observe(panel);
		start();
		document.addEventListener('visibilitychange', onVisible);
		for (const e of ['keydown', 'input', 'paste'] as const) document.addEventListener(e, onInteract, true);
		document.addEventListener('focusout', onBlur, true);
		motion.addEventListener('change', onMotion);
		window.addEventListener('resize', onResize);
		return () => {
			stop();
			ro.disconnect();
			mo.disconnect();
			sys.removeEventListener('change', readTheme);
			clearTimeout(rt);
			document.removeEventListener('visibilitychange', onVisible);
			for (const e of ['keydown', 'input', 'paste'] as const) document.removeEventListener(e, onInteract, true);
			document.removeEventListener('focusout', onBlur, true);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="edge" data-theme={theme} aria-hidden="true"></div>
<canvas class="portals" bind:this={canvas} aria-hidden="true"></canvas>

<style>
	/* The coderluii.dev frame: 8 px, 10 at 1280, 12 at 1440, 18 at 1920; square corners. */
	.edge, .portals { --frame: 8px; position: fixed; inset: 0; pointer-events: none; z-index: 2; }
	/* The page background token of the app's theme: --bg (#ffffff light, #0b0b0c dark). */
	.edge { border: var(--frame) solid var(--bg); }
	.portals { width: 100%; height: 100%; }
	@media (min-width: 1280px) { .edge, .portals { --frame: 10px; } }
	@media (min-width: 1440px) { .edge, .portals { --frame: 12px; } }
	@media (min-width: 1920px) { .edge, .portals { --frame: 18px; } }
	@media (prefers-reduced-motion: reduce) { .portals { display: none; } }
</style>
