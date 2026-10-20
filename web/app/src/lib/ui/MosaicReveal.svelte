<script lang="ts" module>
	// The sweep's timing, shared with the Search page so its tiles land as the
	// sweep passes them. Progress eases out (fast start, slow finish).
	export const REVEAL_MS = 1200;
	const FADE = 0.2;
	const JITTER = 0.1;
	const SWEEP = 1 - FADE - JITTER;
	const ease = (p: number) => 1 - Math.pow(1 - p, 3);
	/** clearedAt is when (ms from the start) the squares at a height fraction f (0 top, 1 bottom) are gone. */
	export function clearedAt(f: number, duration = REVEAL_MS): number {
		const e = Math.min(1, Math.max(0, f) * SWEEP + JITTER / 2 + FADE / 2);
		return (1 - Math.cbrt(1 - e)) * duration;
	}
</script>

<script lang="ts">
	import { onMount } from 'svelte';

	// The Search entry: a cover of page-coloured squares over the content
	// (from the top bar's lower edge down) that dissolves square by square,
	// sweeping straight down the whole visible page (about 1.2 s, ease-out)
	// with a little jitter per square; it plays under reduced motion too. One canvas,
	// drawn at CSS pixels; it removes itself when done (ondone).
	let { duration = REVEAL_MS, cell = 26, ondone }: { duration?: number; cell?: number; ondone?: () => void } = $props();
	let canvas = $state<HTMLCanvasElement>();
	// Measured as the component starts, so the very first frame covers only the content.
	function measure() {
		if (typeof document === 'undefined') return { top: 0, left: 0, width: 0, height: 0 };
		const r = document.getElementById('main')?.getBoundingClientRect();
		const top = Math.max(0, document.querySelector('.ui-topbar')?.getBoundingClientRect().bottom ?? 0);
		const left = Math.max(0, r?.left ?? 0);
		return { top, left, width: Math.max(1, (r?.right ?? innerWidth) - left), height: Math.max(1, innerHeight - top) };
	}
	const box = measure();
	const bg = typeof document === 'undefined' ? '' : getComputedStyle(document.documentElement).getPropertyValue('--bg').trim() || getComputedStyle(document.body).backgroundColor || '#000';

	onMount(() => {
		const c = canvas;
		const ctx = c?.getContext('2d');
		if (!c || !ctx) {
			ondone?.();
			return;
		}
		c.width = Math.ceil(box.width);
		c.height = Math.ceil(box.height);
		const cols = Math.ceil(box.width / cell);
		const rows = Math.ceil(box.height / cell);
		// Each square's start: its row's place in the sweep plus jitter; then a short fade.
		const fade = FADE;
		const starts = new Float32Array(cols * rows);
		for (let y = 0; y < rows; y++) for (let x = 0; x < cols; x++) starts[y * cols + x] = (y / Math.max(1, rows - 1)) * SWEEP + Math.random() * JITTER;
		let raf = 0;
		const t0 = performance.now();
		const frame = (now: number) => {
			const p = ease(Math.min(1, (now - t0) / duration));
			const done = now - t0 >= duration;
			ctx.clearRect(0, 0, c.width, c.height);
			ctx.fillStyle = bg;
			for (let i = 0; i < starts.length; i++) {
				const a = 1 - Math.min(1, Math.max(0, (p - starts[i]) / fade));
				if (a <= 0) continue;
				ctx.globalAlpha = a;
				// A square shrinks a touch as it goes, so the edge reads as pixels.
				const s = cell * (0.55 + 0.45 * a);
				const o = (cell - s) / 2;
				ctx.fillRect((i % cols) * cell + o, Math.floor(i / cols) * cell + o, s, s);
			}
			ctx.globalAlpha = 1;
			if (!done) raf = requestAnimationFrame(frame);
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
	style="top: {box.top}px; left: {box.left}px; width: {box.width ? box.width + 'px' : '100%'}; height: {box.height ? box.height + 'px' : '100%'}; background: {bg || 'var(--bg)'}"
></canvas>

<style>
	.ui-mosaic { position: fixed; top: 0; left: 0; z-index: 25; pointer-events: none; }
</style>
