// The sign-in page's pixel language, ported from the website's hero effect: the same 8x8 media symbols (film frame, music note, play), the same cross-shaped spray along the pointer path, the same 0.3 s energy decay with a 0.04 floor, and alpha capped at 0.65. Here it lives inside one button at a time instead of the hero, and the same grid drives the page's block transitions.

export type Grid = { cols: number; rows: number; cell: number; energy: Float32Array };
export type Side = 'left' | 'right' | 'top' | 'bottom';

export type TrailOpts = { selector?: string; color?: string; busy?: boolean; fail?: number };

export const MASKS = [
	['11111111', '10100101', '10000001', '10011001', '10011001', '10000001', '10100101', '11111111'],
	['00111110', '00111110', '00100010', '00100010', '00100010', '11100110', '11101110', '01001100'],
	['00100000', '00110000', '00111000', '00111100', '00111100', '00111000', '00110000', '00100000']
] as const;

export const DECAY_S = 0.3; // Energy time constant, as on the site.
export const FLOOR = 0.04; // Energy below this is off.
export const MAX_ALPHA = 0.65;

// The site's grid: columns by viewport width, square cells.
export function columnsForWidth(width: number): number {
	return width >= 1280 ? 36 : width >= 992 ? 30 : width >= 768 ? 22 : 12;
}

export function siteCell(width: number): number {
	return width / columnsForWidth(width);
}

// A button-sized grid: cells just big enough for one 3 px symbol (24 px), stretched so whole columns fill the width.
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

const calmQuery = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

// Timings for the button effects (ms).
export const SWEEP_MS = 520;

export const BURST_MS = 350;
export const BUSY_MS = 1100;
export const FLICKER_MS = 320;

// Which edge the pointer came in through (vertical distances scaled to the width).
export function entrySide(w: number, h: number, x: number, y: number): Side {
	const k = w / Math.max(1, h);
	const d: Record<Side, number> = { left: x, right: w - x, top: y * k, bottom: (h - y) * k };
	return (Object.keys(d) as Side[]).reduce((a, b) => (d[b] < d[a] ? b : a));
}

// The hover sweep: a two-cell band of symbols crossing the whole button from `side`, k 0 to 1. Lit cells then fade with the normal decay.
export function sweep(g: Grid, side: Side, k: number) {
	const across = side === 'left' || side === 'right' ? g.cols : g.rows;
	const front = k * (across + 2) - 1;
	for (let r = 0; r < g.rows; r++)
		for (let c = 0; c < g.cols; c++) {
			const p = side === 'left' ? c : side === 'right' ? g.cols - 1 - c : side === 'top' ? r : g.rows - 1 - r;
			const d = front - p;
			if (d >= 0 && d < 2) {
				const i = r * g.cols + c;
				g.energy[i] = Math.max(g.energy[i], d < 1 ? 1 : 0.5);
			}
		}
}

// The ambient shimmer while hovered or focused: a slow diagonal swell, low.
export function ambient(g: Grid, t: number) {
	for (let r = 0; r < g.rows; r++)
		for (let c = 0; c < g.cols; c++) {
			const s = Math.sin(c * 0.55 + r * 0.8 - t * 0.0021);
			if (s > 0.6) {
				const i = r * g.cols + c;
				g.energy[i] = Math.max(g.energy[i], 0.08 + (s - 0.6) * 0.45);
			}
		}
}

// The press burst: a ring of symbols from (x, y) out past the farthest corner, k 0 to 1.
export function burst(g: Grid, x: number, y: number, k: number) {
	const w = g.cols * g.cell;
	const h = g.rows * g.cell;
	const far = Math.max(Math.hypot(x, y), Math.hypot(w - x, y), Math.hypot(x, h - y), Math.hypot(w - x, h - y));
	const rad = k * (far + g.cell);
	for (let r = 0; r < g.rows; r++)
		for (let c = 0; c < g.cols; c++) {
			const d = Math.hypot((c + 0.5) * g.cell - x, (r + 0.5) * g.cell - y);
			if (Math.abs(d - rad) < g.cell * 0.9) {
				const i = r * g.cols + c;
				g.energy[i] = Math.max(g.energy[i], 1 - k * 0.35);
			}
		}
}

// The busy shimmer: a soft band looping left to right, phase 0 to 1.
export function busyBand(g: Grid, phase: number) {
	const front = phase * (g.cols + 4) - 2;
	for (let r = 0; r < g.rows; r++)
		for (let c = 0; c < g.cols; c++) {
			const d = Math.abs(c - front);
			if (d < 2) {
				const i = r * g.cols + c;
				g.energy[i] = Math.max(g.energy[i], 0.5 * (1 - d / 2) + 0.06);
			}
		}
}

// The failure flicker: a seeded scatter of cells, a new one per step.
export function flicker(g: Grid, step: number) {
	const order = blockOrder(g.energy.length, 11 + step * 7);
	for (let i = 0; i < g.energy.length; i++) if (order[i] < 0.35) g.energy[i] = Math.max(g.energy[i], 0.75);
}

// Svelte action: the pixel layer inside one button, clipped to its shape. `node` may wrap the button; the first match of `selector` hosts the canvas. Hover or keyboard focus: a sweep from the entry side, then an ambient shimmer; the pointer trail adds on top. Press (pointer, Enter, Space or a form submit): a burst from the press point with a 0.97 press. `busy`: a looping band. `fail` (a counter): a shake and a flicker. Frames run only while hovered, focused, busy or cells are lit: 0 fps idle. Reduced motion: nothing runs (the page keeps a static press and the spinner).
export function pixelTrail(node: HTMLElement, opts: TrailOpts = {}) {
	const host = (opts.selector ? node.querySelector<HTMLElement>(opts.selector) : null) ?? node;
	let color = opts.color ?? '#ffffff';
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
	let hovered = false;
	let focused = false;
	let busy = false;
	let busyT0 = 0;
	let failN = opts.fail ?? 0;
	let sw: { side: Side; t0: number } | undefined;
	let bu: { x: number; y: number; t0: number } | undefined;
	let fl: { t0: number; step: number } | undefined;
	let lastPress = 0;
	const fine = matchMedia('(hover: hover) and (pointer: fine)');
	const disabled = () => (host as HTMLButtonElement).disabled;

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
	// offsetWidth ignores the hover lift's transform scale; map client px to layout px.
	function local(e: { clientX: number; clientY: number }) {
		const r = host.getBoundingClientRect();
		const sx = host.offsetWidth / r.width || 1;
		return { x: (e.clientX - r.left) * sx, y: (e.clientY - r.top) * sx };
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
		let on = decay(g, dt);
		if (sw) {
			const k = (now - sw.t0) / SWEEP_MS;
			if (k >= 1) sw = undefined;
			else {
				sweep(g, sw.side, Math.max(0, k));
				on = true;
			}
		}
		if (bu) {
			const k = (now - bu.t0) / BURST_MS;
			if (k >= 1) bu = undefined;
			else {
				burst(g, bu.x, bu.y, Math.max(0, k));
				on = true;
			}
		}
		if (fl) {
			const step = Math.floor((now - fl.t0) / (FLICKER_MS / 4));
			if (step >= 4) fl = undefined;
			else {
				if (step !== fl.step) {
					fl.step = step;
					if (step % 2 === 0) flicker(g, step);
				}
				on = true;
			}
		}
		if (busy) {
			busyBand(g, ((now - busyT0) % BUSY_MS) / BUSY_MS);
			on = true;
		} else if ((hovered || focused) && !sw && !disabled()) {
			ambient(g, now);
			on = true;
		}
		draw();
		if (on && !document.hidden) frame = requestAnimationFrame(loop);
		else lastT = 0;
	}
	function queue() {
		if (!frame) frame = requestAnimationFrame(loop);
	}
	function startSweep(side: Side) {
		size();
		if (!g) return;
		sw = { side, t0: performance.now() };
		queue();
	}
	function enter(e: PointerEvent) {
		if (calmQuery() || !fine.matches || e.pointerType !== 'mouse' || disabled()) return;
		hovered = true;
		size();
		const p = local(e);
		startSweep(entrySide(host.offsetWidth, host.offsetHeight, p.x, p.y));
	}
	function move(e: PointerEvent) {
		if (calmQuery() || !fine.matches || (e.pointerType && e.pointerType !== 'mouse' && e.pointerType !== 'pen')) return;
		if (disabled()) return;
		size();
		if (!g) return;
		hovered = true;
		const { x, y } = local(e);
		stampPath(g, lastP, x, y);
		lastP = { x, y };
		queue();
	}
	function leave() {
		lastP = undefined; // The lit cells fade on their own.
		hovered = false;
	}
	function pressAt(x: number, y: number) {
		if (calmQuery()) return;
		size();
		if (!g) return;
		lastPress = performance.now();
		bu = { x, y, t0: lastPress };
		host.animate([{ transform: 'scale(1)' }, { transform: 'scale(0.97)', offset: 0.35 }, { transform: 'scale(1)' }], { duration: BURST_MS, easing: 'cubic-bezier(.2,.7,.3,1)', composite: 'add' });
		queue();
	}
	function down(e: PointerEvent) {
		if (e.button !== 0 || disabled()) return;
		const p = local(e);
		pressAt(p.x, p.y);
	}
	function key(e: KeyboardEvent) {
		if (e.repeat || (e.key !== 'Enter' && e.key !== ' ') || disabled()) return;
		pressAt(host.offsetWidth / 2, host.offsetHeight / 2);
	}
	// Enter inside a field submits the form without touching the button.
	const form = host instanceof HTMLButtonElement && host.type === 'submit' ? host.form : null;
	function submitted() {
		if (performance.now() - lastPress < 400) return;
		pressAt(host.offsetWidth / 2, host.offsetHeight / 2);
	}
	function focus() {
		if (calmQuery() || !host.matches(':focus-visible')) return;
		focused = true;
		startSweep('left');
	}
	function blur() {
		focused = false;
	}
	function fail() {
		if (calmQuery()) return;
		size();
		if (!g) return;
		fl = { t0: performance.now(), step: -1 };
		host.animate(
			[{ transform: 'translateX(0)' }, { transform: 'translateX(-6px)' }, { transform: 'translateX(5px)' }, { transform: 'translateX(-3px)' }, { transform: 'translateX(2px)' }, { transform: 'translateX(0)' }],
			{ duration: 360, easing: 'ease-out', composite: 'add' }
		);
		queue();
	}
	function setBusy(b: boolean) {
		if (b === busy) return;
		busy = b;
		if (b && !calmQuery()) {
			size();
			busyT0 = performance.now();
			queue();
		}
	}
	host.addEventListener('pointerenter', enter);
	host.addEventListener('pointermove', move);
	host.addEventListener('pointerleave', leave);
	host.addEventListener('pointerdown', down);
	host.addEventListener('keydown', key);
	host.addEventListener('focus', focus);
	host.addEventListener('blur', blur);
	form?.addEventListener('submit', submitted);
	setBusy(!!opts.busy);
	return {
		update(o: TrailOpts = {}) {
			color = o.color ?? '#ffffff';
			setBusy(!!o.busy);
			const n = o.fail ?? 0;
			// The page may pass 0 while another button owns failures; only a new high counts.
			if (n > failN) fail();
			failN = Math.max(failN, n);
		},
		destroy() {
			cancelAnimationFrame(frame);
			host.removeEventListener('pointerenter', enter);
			host.removeEventListener('pointermove', move);
			host.removeEventListener('pointerleave', leave);
			host.removeEventListener('pointerdown', down);
			host.removeEventListener('keydown', key);
			host.removeEventListener('focus', focus);
			host.removeEventListener('blur', blur);
			form?.removeEventListener('submit', submitted);
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

// An SVG mask with the blocks whose order is below k shown (k 0 to 1).
export function blockMask(cols: number, rows: number, order: Float32Array, k: number): string {
	let rects = '';
	for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) if (order[r * cols + c] < k) rects += `<rect x="${c}" y="${r}" width="1.02" height="1.02"/>`;
	return `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${cols} ${rows}" preserveAspectRatio="none" shape-rendering="crispEdges">${rects}</svg>`)}")`;
}

export function clearMask(el: HTMLElement) {
	for (const p of ['mask-image', '-webkit-mask-image', 'mask-size', '-webkit-mask-size', 'mask-repeat', '-webkit-mask-repeat']) el.style.removeProperty(p);
}

// Svelte transition: content appears in site-cell blocks (24 px, the button cell) in a fixed random order, in its own colours.
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

function setMask(el: HTMLElement, v: string) {
	el.style.setProperty('mask-image', v);
	el.style.setProperty('-webkit-mask-image', v);
	el.style.setProperty('mask-size', '100% 100%');
	el.style.setProperty('-webkit-mask-size', '100% 100%');
	el.style.setProperty('mask-repeat', 'no-repeat');
	el.style.setProperty('-webkit-mask-repeat', 'no-repeat');
}
