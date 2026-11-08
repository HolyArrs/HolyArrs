<script lang="ts">
	// The sign-in page's edge frame and panel portals. A solid frame sits inset on every browser edge. From 11 s in, a hole tears open in the sign-in panel's glass near one corner, inside its padding. The page's media symbols (film frame, music note, play; pixel-trail.ts) rise out of it, run along the padding band to a hole at the next corner (counter-clockwise from top right, one leg every 5 s) and dive in; both holes seal. Phones use the floating card the same way. One canvas, time based; the clock stops while the tab is hidden or the user is signing in; off with reduced motion.
	import { MASKS } from './pixel-trail.ts';
	import { IDLE_MS } from './signin-art.ts';
	import { legAt, holeAt, symbolAt, easeInOutSine, clamp01, COUNT, pausesPortals } from './signin-portals.ts';
	import { onMount } from 'svelte';

	let canvas: HTMLCanvasElement | undefined = $state();
	let edgeEl: HTMLDivElement | undefined = $state();
	// Dev comparison toggles (query only): ?frame=glass|solid, ?panel=glass|blur.
	let frame = $state<'glass' | 'solid'>('glass');
	// The page forces dark on <main>; the frame follows the app's real theme.
	let theme = $state<'light' | 'dark'>('dark');

	// Note, film, note, play, note, film, play: the music note leads and repeats.
	const ORDER = [1, 0, 1, 2, 1, 0, 2];

	onMount(() => {
		const html = document.documentElement;
		const sys = window.matchMedia('(prefers-color-scheme: dark)');
		const readTheme = () => (theme = html.dataset.theme === 'light' ? 'light' : html.dataset.theme === 'dark' ? 'dark' : sys.matches ? 'dark' : 'light');
		readTheme();
		const q = new URLSearchParams(location.search);
		if (q.get('frame') === 'solid') frame = 'solid';
		const panelMat = q.get('panel') === 'blur' ? 'blur' : 'glass';
		document.querySelector<HTMLElement>('[data-signin-panel]')?.setAttribute('data-mat', panelMat);
		// A theme change repaints the symbols in that theme's ink (remake runs after setup).
		const onTheme = () => {
			readTheme();
			remake();
		};
		const mo = new MutationObserver(onTheme);
		mo.observe(html, { attributes: true, attributeFilter: ['data-theme'] });
		sys.addEventListener('change', onTheme);
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		const cv = canvas!;
		const ctx = cv.getContext('2d');
		if (!ctx) return;
		let w = 0, h = 0, dpr = 1;
		let accent = '#ff453a';
		let sprites: HTMLCanvasElement[] = [];
		let soft: HTMLCanvasElement[] = [];
		let PX = 2; // One symbol pixel, whole device pixels so symbols stay crisp.

		function sprite(mask: readonly string[], blur = 0): HTMLCanvasElement {
			const pad = 6;
			const c = document.createElement('canvas');
			const size = (8 * PX + pad * 2) * dpr;
			c.width = c.height = size;
			const g = c.getContext('2d')!;
			g.scale(dpr, dpr);
			if (blur) g.filter = `blur(${blur}px)`;
			// Light glass: dark ink with a soft white glow, not white with a black shadow.
			const light = theme === 'light';
			g.shadowColor = light ? 'rgb(255 255 255 / 0.7)' : 'rgb(0 0 0 / 0.55)';
			g.shadowBlur = 4;
			g.fillStyle = light ? 'rgb(29 29 31 / 0.72)' : '#f5f5f7';
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
			shapeEdge();
			PX = Math.max(1, Math.round((box.band * 0.6) / 8));
			remake();
		}
		function remake() {
			sprites = MASKS.map((m) => sprite(m));
			soft = MASKS.map((m) => sprite(m, 2.5));
		}

		// The frame's inner window has rounded corners: an even-odd path, the viewport minus a rounded rectangle inset by the frame width.
		function shapeEdge() {
			if (!edgeEl) return;
			const cs = getComputedStyle(edgeEl);
			const f = parseFloat(cs.getPropertyValue('--frame')) || 8;
			const r = parseFloat(cs.getPropertyValue('--frame-r')) || 20;
			const W = window.innerWidth, H = window.innerHeight;
			const x0 = f, y0 = f, x1 = W - f, y1 = H - f;
			const d = `M0 0H${W}V${H}H0Z M${x0 + r} ${y0}H${x1 - r}A${r} ${r} 0 0 1 ${x1} ${y0 + r}V${y1 - r}A${r} ${r} 0 0 1 ${x1 - r} ${y1}H${x0 + r}A${r} ${r} 0 0 1 ${x0} ${y1 - r}V${y0 + r}A${r} ${r} 0 0 1 ${x0 + r} ${y0}Z`;
			edgeEl.style.clipPath = `path(evenodd, '${d}')`;
		}

		// The path runs inside the panel, along the middle of its padding band, rounded like the panel's own corners. Corners counter-clockwise from top right (TR, TL, BL, BR); a hole sits mid-arc at each corner. The panel is never touched: masking its backdrop-filter layer made the blur re-render and the panel flicker at each trip's ends. Each hole is drawn on this canvas, above the panel, as the matching patch of the wallpaper (same picture, zoom, crossfade and scrim), plus its rim.
		let box = { x0: 0, y0: 0, x1: 0, y1: 0, r: [0, 0, 0, 0], band: 0, out: [0, 0, 0, 0], ro: [0, 0, 0, 0] };
		function measurePanel() {
			const el = document.querySelector<HTMLElement>('[data-signin-panel]');
			if (!el) return;
			const b = el.getBoundingClientRect();
			const cs = getComputedStyle(el);
			const z = el.offsetWidth ? b.width / el.offsetWidth : 1;
			const px = (v: string) => (parseFloat(v) || 0) * z;
			// Clipped to the frame (none on phones, where the edge is hidden).
			const edge = document.querySelector<HTMLElement>('.edge');
			const f = edge && getComputedStyle(edge).display !== 'none' ? parseFloat(getComputedStyle(edge).borderTopWidth) || 0 : 0;
			const left = Math.max(b.left, f), right = Math.min(b.right, w - f), bottom = Math.min(b.bottom, h - f);
			const band = Math.min(px(cs.paddingTop), px(cs.paddingLeft) - (left - b.left), px(cs.paddingRight) - (b.right - right), px(cs.paddingBottom) - (b.bottom - bottom));
			const i = band / 2;
			const rad = [cs.borderTopRightRadius, cs.borderTopLeftRadius, cs.borderBottomLeftRadius, cs.borderBottomRightRadius].map((v) => Math.max(4, px(v) - i));
			const ro = [cs.borderTopRightRadius, cs.borderTopLeftRadius, cs.borderBottomLeftRadius, cs.borderBottomRightRadius].map((v) => px(v));
			box = { x0: left + i, y0: b.top + i, x1: right - i, y1: bottom - i, r: rad, band, out: [left, b.top, right, bottom], ro };
		}
		const MID = (k: number) => -Math.PI / 4 - (k * Math.PI) / 2;
		function centre(k: number): [number, number] {
			const { x0, y0, x1, y1, r } = box;
			k %= 4;
			const q = r[k];
			return k === 0 ? [x1 - q, y0 + q] : k === 1 ? [x0 + q, y0 + q] : k === 2 ? [x0 + q, y1 - q] : [x1 - q, y1 - q];
		}
		// Where a hole opens: on the corner's diagonal, deep enough that at least 9 px of clear glass remains between its rim and both panel edges and the rounded corner (and the content, which starts one padding in).
		function holeCentre(k: number): [number, number] {
			const [ox0, oy0, ox1, oy1] = box.out, q = box.ro[k % 4], R = holeR();
			const c = Math.max(R + 9, q - (q - R - 9) / Math.SQRT2);
			return k % 4 === 0 ? [ox1 - c, oy0 + c] : k % 4 === 1 ? [ox0 + c, oy0 + c] : k % 4 === 2 ? [ox0 + c, oy1 - c] : [ox1 - c, oy1 - c];
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

		// A cut in the glass: the panel is never touched; the canvas paints the wallpaper inside and adds the rim: an inner shadow under the far (top) edge and a bright highlight where light catches the near (bottom) cut edge.
		function holeR() {
			return w <= 600 ? 9 : w >= 1440 ? 12 : 11;
		}
		// The wallpaper under (x, y): each layer's picture where its zoom has it now, at its crossfade opacity, then the scrim's darkness at that spot.
		function paintWallpaper(c: CanvasRenderingContext2D, x: number, y: number, r: number) {
			c.fillStyle = '#050507';
			c.fillRect(x - r, y - r, r * 2, r * 2);
			for (const layer of document.querySelectorAll<HTMLElement>('.art .layer')) {
				const a = parseFloat(getComputedStyle(layer).opacity);
				if (!(a > 0)) continue;
				const pic = layer.querySelector<HTMLImageElement>('img.pic');
				const mz = layer.querySelector<HTMLCanvasElement>('canvas.mz');
				c.globalAlpha = a;
				if (mz && getComputedStyle(mz).display !== 'none' && parseFloat(getComputedStyle(mz).opacity) > 0) {
					const b = mz.getBoundingClientRect();
					try { c.drawImage(mz, b.left, b.top, b.width, b.height); } catch { /* not ready */ }
				}
				if (pic && pic.complete && pic.naturalWidth && parseFloat(getComputedStyle(pic).opacity) > 0) {
					const b = pic.getBoundingClientRect();
					const k = Math.max(b.width / pic.naturalWidth, b.height / pic.naturalHeight);
					const dw = pic.naturalWidth * k, dh = pic.naturalHeight * k;
					const [px, py] = (getComputedStyle(pic).objectPosition || '50% 50%').split(/\s+/).map((v) => (Number.isNaN(parseFloat(v)) ? 50 : parseFloat(v)) / 100);
					c.drawImage(pic, b.left + (b.width - dw) * px, b.top + (b.height - dh) * py, dw, dh);
				}
			}
			c.globalAlpha = 1;
			c.fillStyle = `rgb(5 5 7 / ${scrimAt(x, y).toFixed(3)})`;
			c.fillRect(x - r, y - r, r * 2, r * 2);
		}
		// SigninBackdrop's .shade gradients, evaluated at one point (a hole is small enough that the scrim is flat across it).
		function scrimAt(x: number, y: number) {
			const u = x / w, v = y / h;
			const ramp = (d: number, stops: [number, number][]) => {
				if (d <= stops[0][0]) return stops[0][1];
				for (let i = 1; i < stops.length; i++) if (d <= stops[i][0]) {
					const [d0, a0] = stops[i - 1], [d1, a1] = stops[i];
					return a0 + ((a1 - a0) * (d - d0)) / (d1 - d0);
				}
				return stops[stops.length - 1][1];
			};
			const over = (a: number, b: number) => a + b - a * b;
			if (w <= 600) return over(ramp(v, [[0, 0.55], [0.22, 0]]), ramp(1 - v, [[0, 0.9], [0.45, 0.55], [0.75, 0.1], [1, 0.1]]));
			const pool = ramp(Math.hypot((u - 0.5) / 0.46, (v - 0.5) / 0.62), [[0, 0.55], [0.6, 0.3], [1, 0]]);
			const vig = ramp(Math.hypot((u - 0.5) / 1.2, (v - 0.45) / 1.0), [[0.45, 0], [1, 0.6]]);
			return over(over(over(pool, vig), ramp(1 - v, [[0, 0.7], [0.32, 0]])), ramp(v, [[0, 0.35], [0.22, 0]]));
		}
		function drawHole(x: number, y: number, s: number, squash: number) {
			if (s <= 0.001) return;
			const r = holeR() * s;
			// arc() throws IndexSizeError on a negative radius (r - 0.6 below).
			if (!(r >= 1)) return;
			const c = ctx!;
			c.save();
			c.translate(x, y);
			c.scale(1 + squash, 1 - squash);
			c.beginPath();
			c.arc(0, 0, r, 0, Math.PI * 2);
			c.save();
			c.clip();
			c.scale(1 / (1 + squash), 1 / (1 - squash));
			c.translate(-x, -y);
			paintWallpaper(c, x, y, r * 1.3);
			c.translate(x, y);
			c.scale(1 + squash, 1 - squash);
			// Inner shadow on the far rim: the glass's thickness.
			const sh = c.createLinearGradient(0, -r, 0, -r * 0.3);
			sh.addColorStop(0, 'rgb(0 0 0 / 0.55)');
			sh.addColorStop(1, 'rgb(0 0 0 / 0)');
			c.fillStyle = sh;
			c.fillRect(-r, -r, r * 2, r * 2);
			c.lineWidth = Math.max(1.5, r * 0.18);
			c.strokeStyle = 'rgb(0 0 0 / 0.35)';
			c.beginPath();
			c.arc(0, 0, r, Math.PI * 1.05, Math.PI * 1.95);
			c.stroke();
			c.restore();
			// Light catching the cut edge.
			c.lineWidth = 1.25;
			c.strokeStyle = 'rgb(255 255 255 / 0.7)';
			c.beginPath();
			c.arc(0, 0, r - 0.6, Math.PI * 0.12, Math.PI * 0.88);
			c.stroke();
			c.lineWidth = 0.75;
			c.strokeStyle = 'rgb(255 255 255 / 0.18)';
			c.beginPath();
			c.arc(0, 0, r - 0.4, Math.PI * 1.1, Math.PI * 1.9);
			c.stroke();
			c.restore();
		}

		function draw(t: number) {
			ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
			ctx!.clearRect(0, 0, w, h);
			const leg = legAt(t, 4);
			if (!leg || !box.r) return;
			const k = leg.index;
			const [ax, ay] = holeCentre(k);
			const [bx, by] = holeCentre(k + 1);
			const a = holeAt(leg.local, 'from');
			const b = holeAt(leg.local, 'to');
			drawHole(ax, ay, a.s, a.squash);
			drawHole(bx, by, b.s, b.squash);
			const pcx = (box.x0 + box.x1) / 2, pcy = (box.y0 + box.y1) / 2;
			const half = 4 * PX + 6;
			for (let i = 0; i < COUNT; i++) {
				const p = symbolAt(leg.local, i);
				if (p === null) continue;
				// Travel fills 0.08 to 0.82 of the symbol's time. It rises out of the first hole before, and after it sits over the far hole's centre and sinks into its depth (shrinks, blurs, fades).
				const u = easeInOutSine(clamp01((p - 0.08) / 0.74));
				const [lx, ly] = legPoint(k, u);
				// Ease from the hole centre onto the band path and back off it.
				const sm = (v: number) => v * v * (3 - 2 * v);
				const wt = sm(clamp01(u / 0.14)) * sm(clamp01((1 - u) / 0.14));
				const [hx, hy] = u < 0.5 ? [ax, ay] : [bx, by];
				const px = hx + (lx - hx) * wt, py = hy + (ly - hy) * wt;
				const tt = t * 1000 + i * 290;
				const ox = px - pcx, oy = py - pcy, ol = Math.hypot(ox, oy) || 1;
				const grow = Math.min(clamp01(p / 0.08), 1 - easeInOutSine(clamp01((p - 0.82) / 0.18)));
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
		let clock = 0, last = 0, raf = 0, lastInput = -1e9;
		function loop(now: number) {
			raf = 0;
			if (document.hidden || motion.matches) return;
			const dt = last ? Math.min(0.05, (now - last) / 1000) : 0;
			last = now;
			// Pause only while the user types; it resumes IDLE_MS after the last key.
			if (now - lastInput >= IDLE_MS) clock += dt;
			try {
				draw(clock);
			} catch {
				// A bad frame must never stop the loop.
				ctx!.setTransform(1, 0, 0, 1, 0, 0);
				ctx!.globalAlpha = 1;
			}
			raf = requestAnimationFrame(loop);
		}
		// Phones skip the portals: the card is small and the run would crowd the fields.
		const phone = () => window.innerWidth <= 600;
		function start() {
			if (raf || document.hidden || motion.matches || phone()) return;
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
		function onKey(e: KeyboardEvent) {
			const t = e.target as HTMLElement | null;
			const inField = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable);
			if (pausesPortals(e, inField)) lastInput = performance.now();
		}
		let rt: ReturnType<typeof setTimeout> | undefined;
		function onResize() {
			clearTimeout(rt);
			rt = setTimeout(() => {
				measure();
				if (phone()) {
					stop();
					ctx!.setTransform(1, 0, 0, 1, 0, 0);
					ctx!.clearRect(0, 0, cv.width, cv.height);
				} else start();
			}, 150);
		}

		measure();
		const panel = document.querySelector('[data-signin-panel]');
		const ro = new ResizeObserver(() => measurePanel());
		const edgeObs = new ResizeObserver(() => shapeEdge());
		if (edgeEl) edgeObs.observe(edgeEl);
		if (panel) ro.observe(panel);
		// The phone card glides above the keyboard: follow it for the lift.
		let follow = 0;
		const vv = window.visualViewport;
		const track = () => {
			const until = performance.now() + 420;
			cancelAnimationFrame(follow);
			const step = () => { measurePanel(); if (performance.now() < until) follow = requestAnimationFrame(step); };
			follow = requestAnimationFrame(step);
		};
		vv?.addEventListener('resize', track);
		vv?.addEventListener('scroll', track);
		start();
		document.addEventListener('visibilitychange', onVisible);
		document.addEventListener('keydown', onKey, true);
		motion.addEventListener('change', onMotion);
		window.addEventListener('resize', onResize);
		return () => {
			stop();
			ro.disconnect();
			cancelAnimationFrame(follow);
			vv?.removeEventListener('resize', track);
			vv?.removeEventListener('scroll', track);
			edgeObs.disconnect();
			mo.disconnect();
			sys.removeEventListener('change', onTheme);
			clearTimeout(rt);
			document.removeEventListener('visibilitychange', onVisible);
			document.removeEventListener('keydown', onKey, true);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="edge" class:glass={frame === 'glass'} data-theme={theme} bind:this={edgeEl} aria-hidden="true"></div>
<canvas class="portals" bind:this={canvas} aria-hidden="true"></canvas>

<style>
	/* Frame width: 14 px phone, 18 at 601, 22 at 1280, 24 at 1440, 32 at 1920, 40 at 2560. */
	.edge, .portals { --frame: 14px; --frame-r: 22px; position: fixed; inset: 0; pointer-events: none; z-index: 2; }
	/* Clipped in script to the viewport minus a rounded window (radius in the panel's family). Solid: the page background token --bg. Glass: the panel's own material, with the same blur, saturation and tint. */
	.edge { background: var(--bg); }
	.edge.glass {
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
	}
	/* Light mode: the sign-in panel's light tinted glass. */
	.edge.glass[data-theme='light'] {
		background: rgb(255 253 250 / 0.66);
		-webkit-backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.edge.glass { background: rgb(18 18 22 / 0.92); }
		.edge.glass[data-theme='light'] { background: rgb(248 246 242 / 0.94); }
	}
	/* Phones: full-bleed art, no frame (the portals still run in the card). */
	@media (max-width: 600px) { .edge { display: none; } }
	@media (min-width: 601px) { .edge, .portals { --frame: 18px; --frame-r: 26px; } }
	/* Its own compositor layer, so redrawing it never touches the panel's blur. */
	.portals { width: 100%; height: 100%; will-change: transform; transform: translateZ(0); }
	@media (min-width: 1280px) { .edge, .portals { --frame: 22px; --frame-r: 30px; } }
	@media (min-width: 1440px) { .edge, .portals { --frame: 24px; --frame-r: 34px; } }
	@media (min-width: 1920px) { .edge, .portals { --frame: 32px; --frame-r: 38px; } }
	@media (min-width: 2560px) { .edge, .portals { --frame: 40px; --frame-r: 44px; } }
	@media (prefers-reduced-motion: reduce) { .portals { display: none; } }
</style>
