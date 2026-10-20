<script lang="ts">
	import { onMount } from 'svelte';

	// The Search entry: a cover of page-coloured squares over the content
	// (from the top bar's lower edge down) that dissolves square by square,
	// sweeping straight down with a little jitter per square. One canvas,
	// drawn at CSS pixels; it removes itself when done (ondone).
	let { duration = 560, cell = 26, ondone }: { duration?: number; cell?: number; ondone?: () => void } = $props();
	let canvas = $state<HTMLCanvasElement>();
	let box = $state({ top: 0, left: 0, width: 0, height: 0 });
	let bg = $state('');

	onMount(() => {
		const main = document.getElementById('main');
		const bar = document.querySelector('.ui-topbar');
		const r = main?.getBoundingClientRect();
		const top = Math.max(0, bar?.getBoundingClientRect().bottom ?? 0);
		const left = Math.max(0, r?.left ?? 0);
		box = { top, left, width: Math.max(1, (r?.right ?? innerWidth) - left), height: Math.max(1, innerHeight - top) };
		bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim() || getComputedStyle(document.body).backgroundColor || '#000';
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
		const fade = 0.22;
		const sweep = 1 - fade - 0.12;
		const starts = new Float32Array(cols * rows);
		for (let y = 0; y < rows; y++) for (let x = 0; x < cols; x++) starts[y * cols + x] = (y / rows) * sweep + Math.random() * 0.12;
		let raf = 0;
		const t0 = performance.now();
		const frame = (now: number) => {
			const p = Math.min(1, (now - t0) / duration);
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
			if (p < 1) raf = requestAnimationFrame(frame);
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
