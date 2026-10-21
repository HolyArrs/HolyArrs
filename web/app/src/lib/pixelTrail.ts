// The sign-in page's pixel language, ported from the website's hero effect
// (site\repo\public\hero-effects.js): the same 8x8 media symbols (film
// frame, music note, play), the same cross-shaped spray along the pointer
// path, the same 0.3 s energy decay with a 0.04 floor, and alpha capped at
// 0.65. Here it lives inside one button at a time instead of the hero, and
// the same grid drives the page's block transitions.

export const MASKS = [
	['11111111', '10100101', '10000001', '10011001', '10011001', '10000001', '10100101', '11111111'],
	['00111110', '00111110', '00100010', '00100010', '00100010', '11100110', '11101110', '01001100'],
	['00100000', '00110000', '00111000', '00111100', '00111100', '00111000', '00110000', '00100000']
] as const;

export const DECAY_S = 0.3; // energy time constant, as on the site
export const FLOOR = 0.04; // energy below this is off
export const MAX_ALPHA = 0.65;

// The site's grid: columns by viewport width, square cells.
export function columnsForWidth(width: number): number {
	return width >= 1280 ? 36 : width >= 992 ? 30 : width >= 768 ? 22 : 12;
}
export function siteCell(width: number): number {
	return width / columnsForWidth(width);
}
// The site's symbol pixel: a tenth of a cell, 3 to 5 px.
export function pixelFor(cell: number): number {
	return Math.max(3, Math.min(5, Math.round(cell / 10)));
}

export type Grid = { cols: number; rows: number; cell: number; energy: Float32Array };

// A button-sized grid: cells just big enough for one 3 px symbol (24 px),
// stretched so whole columns fill the width.
export function buttonGrid(width: number, height: number): Grid {
	const cols = Math.max(1, Math.round(width / 26));
	const cell = width / cols;
	const rows = Math.max(1, Math.ceil(height / cell));
	return { cols, rows, cell, energy: new Float32Array(cols * rows) };
}

export function light(g: Grid, x: number, y: number, v: number) {
	const c = Math.floor(x / g.cell);
	const r = Math.floor(y / g.cell);
	if (c < 0 || r < 0 || c >= g.cols || r >= g.rows) return;
	const i = r * g.cols + c;
	g.energy[i] = Math.max(g.energy[i], v);
}

// The site's spray: full on the cell under the pointer, 0.4 on its four neighbours.
export function spray(g: Grid, x: number, y: number) {
	light(g, x, y, 1);
	light(g, x - g.cell, y, 0.4);
	light(g, x + g.cell, y, 0.4);
	light(g, x, y - g.cell, 0.4);
	light(g, x, y + g.cell, 0.4);
}

// Fills the path from the last point in half-cell steps (at most 30).
export function stampPath(g: Grid, from: { x: number; y: number } | undefined, x: number, y: number) {
	const p = from ?? { x, y };
	const d = Math.hypot(x - p.x, y - p.y);
	const steps = Math.min(30, Math.ceil(d / (g.cell * 0.5)) || 1);
	for (let s = 1; s <= steps; s++) {
		const k = s / steps;
		spray(g, p.x + (x - p.x) * k, p.y + (y - p.y) * k);
	}
}

// One decay step; returns whether any cell is still lit.
export function decay(g: Grid, dt: number): boolean {
	const f = Math.exp(-dt / DECAY_S);
	let on = false;
	for (let i = 0; i < g.energy.length; i++) {
		const v = g.energy[i] * f;
		g.energy[i] = v < FLOOR ? 0 : v;
		if (g.energy[i]) on = true;
	}
	return on;
}

export function paint(ctx: CanvasRenderingContext2D, g: Grid, color: string, scale = 1) {
	const px = 3 * scale;
	const half = 4 * px;
	ctx.fillStyle = color;
	for (let r = 0; r < g.rows; r++) {
		for (let c = 0; c < g.cols; c++) {
			const e = g.energy[r * g.cols + c];
			if (!e) continue;
			const left = Math.round((c + 0.5) * g.cell * scale) - half;
			const top = Math.round((r + 0.5) * g.cell * scale) - half;
			const mask = MASKS[(c + r) % MASKS.length];
			ctx.globalAlpha = Math.min(MAX_ALPHA, e * MAX_ALPHA);
			for (let y = 0; y < 8; y++) for (let x = 0; x < 8; x++) if (mask[y][x] === '1') ctx.fillRect(left + x * px, top + y * px, px, px);
		}
	}
	ctx.globalAlpha = 1;
}

// The static focus look: a dim band of symbols across the middle row.
export function focusPattern(g: Grid) {
	g.energy.fill(0);
	const mid = Math.floor((g.rows - 1) / 2);
	for (let c = 0; c < g.cols; c++) g.energy[mid * g.cols + c] = c % 2 ? 0.18 : 0.28;
}

const calmQuery = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

// Svelte action: a pixel trail inside one button, clipped to its shape.
// `node` may wrap the button; the first match of `selector` hosts the canvas.
// Idle at 0 fps: frames run only while cells are lit.
export function pixelTrail(node: HTMLElement, opts: { selector?: string; color?: string } = {}) {
	const host = (opts.selector ? node.querySelector<HTMLElement>(opts.selector) : null) ?? node;
	const color = opts.color ?? '#ffffff';
	const canvas = document.createElement('canvas');
	canvas.className = 'px-trail';
	canvas.setAttribute('aria-hidden', 'true');
	Object.assign(canvas.style, { position: 'absolute', inset: '0', width: '100%', height: '100%', pointerEvents: 'none', borderRadius: 'inherit', zIndex: '0' });
	host.prepend(canvas);
	let g: Grid | undefined;
	let frame = 0;
	let lastT = 0;
	let lastP: { x: number; y: number } | undefined;
	let dpr = 1;
	const fine = matchMedia('(hover: hover) and (pointer: fine)');

	function size() {
		const r = host.getBoundingClientRect();
		const w = host.offsetWidth || r.width;
		const h = host.offsetHeight || r.height;
		if (!w || !h) return;
		dpr = Math.min(2, window.devicePixelRatio || 1);
		if (!g || canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
			canvas.width = Math.round(w * dpr);
			canvas.height = Math.round(h * dpr);
			g = buttonGrid(w, h);
		}
	}
	function draw() {
		const ctx = canvas.getContext('2d');
		if (!ctx || !g) return;
		ctx.clearRect(0, 0, canvas.width, canvas.height);
		paint(ctx, g, color, dpr);
	}
	function loop(now: number) {
		frame = 0;
		if (!g) return;
		const dt = lastT ? Math.min(0.05, (now - lastT) / 1000) : 1 / 60;
		lastT = now;
		const on = decay(g, dt);
		draw();
		if (on && !document.hidden) frame = requestAnimationFrame(loop);
		else lastT = 0;
	}
	function queue() {
		if (!frame) frame = requestAnimationFrame(loop);
	}
	function move(e: PointerEvent) {
		if (calmQuery() || !fine.matches || (e.pointerType && e.pointerType !== 'mouse' && e.pointerType !== 'pen')) return;
		if ((host as HTMLButtonElement).disabled) return;
		size();
		if (!g) return;
		// offsetWidth ignores the hover lift's transform scale; map client px to layout px.
		const r = host.getBoundingClientRect();
		const sx = host.offsetWidth / r.width || 1;
		const x = (e.clientX - r.left) * sx;
		const y = (e.clientY - r.top) * sx;
		stampPath(g, lastP, x, y);
		lastP = { x, y };
		queue();
	}
	function leave() {
		lastP = undefined; // the lit cells fade on their own
	}
	function focus() {
		if (!host.matches(':focus-visible')) return;
		size();
		if (!g) return;
		cancelAnimationFrame(frame);
		frame = 0;
		focusPattern(g);
		draw();
	}
	function blur() {
		if (g) g.energy.fill(0);
		draw();
	}
	host.addEventListener('pointermove', move);
	host.addEventListener('pointerleave', leave);
	host.addEventListener('focus', focus);
	host.addEventListener('blur', blur);
	return {
		destroy() {
			cancelAnimationFrame(frame);
			host.removeEventListener('pointermove', move);
			host.removeEventListener('pointerleave', leave);
			host.removeEventListener('focus', focus);
			host.removeEventListener('blur', blur);
			canvas.remove();
		}
	};
}

// Block reveal order: a seeded shuffle so each transition reads the same.
export function blockOrder(n: number, seed = 7): Float32Array {
	const out = new Float32Array(n);
	let s = seed >>> 0 || 1;
	for (let i = 0; i < n; i++) {
		s ^= s << 13;
		s ^= s >>> 17;
		s ^= s << 5;
		out[i] = (s >>> 0) / 4294967296;
	}
	return out;
}

// An SVG mask with the blocks whose order is below k shown (k 0..1).
export function blockMask(cols: number, rows: number, order: Float32Array, k: number): string {
	let rects = '';
	for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) if (order[r * cols + c] < k) rects += `<rect x="${c}" y="${r}" width="1.02" height="1.02"/>`;
	return `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${cols} ${rows}" preserveAspectRatio="none" shape-rendering="crispEdges">${rects}</svg>`)}")`;
}

function setMask(el: HTMLElement, v: string) {
	el.style.setProperty('mask-image', v);
	el.style.setProperty('-webkit-mask-image', v);
	el.style.setProperty('mask-size', '100% 100%');
	el.style.setProperty('-webkit-mask-size', '100% 100%');
	el.style.setProperty('mask-repeat', 'no-repeat');
	el.style.setProperty('-webkit-mask-repeat', 'no-repeat');
}
export function clearMask(el: HTMLElement) {
	for (const p of ['mask-image', '-webkit-mask-image', 'mask-size', '-webkit-mask-size', 'mask-repeat', '-webkit-mask-repeat']) el.style.removeProperty(p);
}

// Svelte transition: content appears in site-cell blocks (24 px, the
// button cell) in a fixed random order, in its own colours.
export function pixelIn(node: HTMLElement, { duration = 300, cell = 24 }: { duration?: number; cell?: number } = {}) {
	if (calmQuery() || !duration) return { duration: 0 };
	const cols = Math.max(1, Math.ceil(node.offsetWidth / cell));
	const rows = Math.max(1, Math.ceil(node.offsetHeight / cell));
	const order = blockOrder(cols * rows, cols * 31 + rows);
	let last = -1;
	return {
		duration,
		tick(t: number) {
			// Step in tenths, like the site's frame-quantised decay.
			const q = Math.round(t * 10) / 10;
			if (q === last) return;
			last = q;
			if (q >= 1) clearMask(node);
			else setMask(node, blockMask(cols, rows, order, q));
		}
	};
}

// Dissolves an element in (`dir` 1) or out (-1) in blocks of `cell` px.
export function blockDissolve(el: HTMLElement, { duration = 320, cell = 24, dir = -1 as 1 | -1, delay = 0 } = {}): Promise<void> {
	return new Promise((done) => {
		const w = el.offsetWidth || window.innerWidth;
		const h = el.offsetHeight || window.innerHeight;
		const cols = Math.max(1, Math.ceil(w / cell));
		const rows = Math.max(1, Math.ceil(h / cell));
		const order = blockOrder(cols * rows, cols * 17 + rows);
		let last = -1;
		const t0 = performance.now() + delay;
		if (dir === 1) setMask(el, blockMask(cols, rows, order, 0));
		const frame = (now: number) => {
			const t = Math.max(0, Math.min(1, (now - t0) / duration));
			const q = Math.round(t * 12) / 12;
			if (q !== last) {
				last = q;
				setMask(el, blockMask(cols, rows, order, dir === 1 ? q : 1 - q));
			}
			if (t < 1) requestAnimationFrame(frame);
			else done();
		};
		requestAnimationFrame(frame);
	});
}
