<script lang="ts" module>
	// The Search layer's sweep: a canvas of page-coloured squares over the content, from the top bar's lower edge to the bottom of the viewport. A soft band (FEATHER px tall) moves through it and squares inside the band fade out or in.
	export const ENTER_DELAY = 120;
	export const ENTER_SWEEP = 1600;
	export const EXIT_MS = 900;
	export const FEATHER = 280;
	// A gentle cubic ease-in-out so the band travels at a visible, even pace.
	const sweepEase = (x: number) => (x >= 1 ? 1 : x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2);
	const easeIn = (x: number) => x * x * x;

	/** The band's leading edge (0 top, 1 bottom of the cover) `ms` after the entry starts. */
	export function frontAt(ms: number): number {
		return sweepEase(Math.min(1, Math.max(0, ms - ENTER_DELAY) / ENTER_SWEEP));
	}
	/** When, in ms after the entry starts, the band's edge reaches height fraction `f`. */
	export function passAt(f: number): number {
		const e = Math.min(0.999, Math.max(0, f));
		// Inverse of sweepEase.
		const x = e < 0.5 ? Math.cbrt(e / 4) : 1 - Math.cbrt(2 * (1 - e)) / 2;
		return ENTER_DELAY + x * ENTER_SWEEP;
	}
	// Seeded, so the shimmer is the same every time.
	function seeded(seed: number) {
		return () => {
			seed = (seed + 0x6d2b79f5) | 0;
			let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
			t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
			return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
		};
	}
</script>

<script lang="ts">
	import { onMount } from 'svelte';

	let { exit = false, cell = 36, ondone }: { exit?: boolean; cell?: number; ondone?: () => void } = $props();
	let canvas = $state<HTMLCanvasElement>();
	function measure() {
		if (typeof document === 'undefined') return { top: 0, left: 0, width: 0, height: 0 };
		const r = document.getElementById('main')?.getBoundingClientRect();
		const top = Math.max(0, document.querySelector('.ui-topbar')?.getBoundingClientRect().bottom ?? 0);
		const left = Math.max(0, r?.left ?? 0);
		return { top, left, width: Math.max(1, (r?.right ?? innerWidth) - left), height: Math.max(1, innerHeight - top) };
	}
	const box = measure();
	const bg = typeof document === 'undefined' ? '' : getComputedStyle(document.documentElement).getPropertyValue('--bg').trim() || getComputedStyle(document.body).backgroundColor || '#000';
	// Breaking squares are drawn slightly lighter than the page so the pixels read as pixels.
	const px = typeof document === 'undefined' ? '' : getComputedStyle(document.documentElement).getPropertyValue('--surface-2').trim() || bg;

	onMount(() => {
		const c = canvas;
		const ctx = c?.getContext('2d', { alpha: true });
		if (!c || !ctx) {
			ondone?.();
			return;
		}
		const dpr = Math.min(2, devicePixelRatio || 1);
		c.width = Math.ceil(box.width * dpr);
		c.height = Math.ceil(box.height * dpr);
		ctx.scale(dpr, dpr);
		// Each square's seeded delay as a share of the band, computed once.
		const cols = Math.ceil(box.width / cell);
		const rows = Math.ceil(box.height / cell);
		const rnd = seeded(20261020);
		const jitter = new Float32Array(cols * rows);
		for (let i = 0; i < jitter.length; i++) jitter[i] = rnd() * 0.9;
		const H = box.height;
		// Far enough that the last row's slowest squares settle before the canvas goes.
		const travel = H + FEATHER * 1.6;
		let raf = 0;
		const t0 = performance.now();
		const total = exit ? EXIT_MS : ENTER_DELAY + ENTER_SWEEP;
		const frame = (now: number) => {
			const ms = now - t0;
			// The band's leading edge in px; squares above it (entering) are clear, below are covered.
			const edge = exit ? (1 - easeIn(Math.min(1, ms / EXIT_MS))) * travel - FEATHER : frontAt(ms) * travel - FEATHER;
			ctx.clearRect(0, 0, box.width, H);
			ctx.fillStyle = bg;
			// Fully covered below the band: one fill.
			const solid = Math.max(0, Math.floor((edge + FEATHER) / cell) * cell);
			if (solid < H) ctx.fillRect(0, solid, box.width, H - solid);
			const r0 = Math.max(0, Math.floor(edge / cell));
			const r1 = Math.min(rows, Math.ceil((edge + FEATHER) / cell));
			for (let y = r0; y < r1; y++) {
				if (y * cell >= solid) break;
				for (let x = 0; x < cols; x++) {
					// 0 at the band's top (clear), 1 at its bottom (covered), minus the seeded delay.
					const k = Math.min(1, Math.max(0, ((y + 0.5) * cell - edge) / FEATHER * 1.4 + jitter[y * cols + x] - 0.45));
					if (k <= 0.02) continue;
					// Each square shrinks to nothing, lighter while it goes.
					ctx.globalAlpha = Math.min(1, 0.35 + k);
					ctx.fillStyle = k > 0.96 ? bg : px;
					const s = Math.max(2, cell * k - (k < 0.96 ? 2 : 0));
					const o = (cell - s) / 2;
					ctx.fillRect(x * cell + o, y * cell + o, s, s);
					ctx.fillStyle = bg;
				}
			}
			ctx.globalAlpha = 1;
			if (ms < total) raf = requestAnimationFrame(frame);
			else ondone?.();
		};
		c.style.background = 'transparent';
		raf = requestAnimationFrame(frame);
		return () => cancelAnimationFrame(raf);
	});
</script>

<canvas
	bind:this={canvas}
	class="ui-mosaic"
	aria-hidden="true"
	style="top: {box.top}px; left: {box.left}px; width: {box.width ? box.width + 'px' : '100%'}; height: {box.height ? box.height + 'px' : '100%'}; background: {exit ? 'transparent' : bg || 'var(--bg)'}"
></canvas>

<style>
	.ui-mosaic { position: fixed; top: 0; left: 0; z-index: 25; pointer-events: none; }
</style>
