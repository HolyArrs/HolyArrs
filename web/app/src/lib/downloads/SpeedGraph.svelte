<script lang="ts">
	// The header strip's speed graph: the last 15 minutes of total download
	// speed (a filled area with a thin top line and a tick at the peak) and
	// upload speed (a thin line), both on one scale. A reading is
	// taken every second, easing from the last one toward the newest poll, and
	// the graph slides left smoothly between readings. It rests while the tab is
	// hidden; with reduced motion it steps once a second without sliding.
	import { onMount } from 'svelte';
	import { reducedMotion } from '../motion.ts';
	import { stepToward } from './sections.ts';
	// The graph shows the server's whole buffer: 15 minutes, a reading a second.
	const SPAN = 900;

	let { speed, up = 0, width = 120, height = 28, history = [] }: { speed: number; up?: number; width?: number; height?: number; history?: { down: number; up: number }[] } = $props();

	let canvas = $state<HTMLCanvasElement>();
	let peak = $state(0);
	const pts: number[] = [];
	const ups: number[] = [];
	let at = 0;
	// The server's readings since the app started: drawn on first paint, then live ones follow.
	// svelte-ignore state_referenced_locally
	for (const h of history.slice(-SPAN)) {
		pts.push(Math.max(0, h.down || 0));
		ups.push(Math.max(0, h.up || 0));
	}
	peak = pts.length ? Math.max(...pts) : 0;

	function color(name: string, fallback: string) {
		return (canvas && getComputedStyle(canvas).getPropertyValue(name).trim()) || fallback;
	}
	function draw(now: number) {
		const c = canvas?.getContext('2d');
		if (!c || !canvas) return;
		const dpr = devicePixelRatio || 1;
		if (canvas.width !== width * dpr) {
			canvas.width = width * dpr;
			canvas.height = height * dpr;
		}
		c.setTransform(dpr, 0, 0, dpr, 0, 0);
		c.clearRect(0, 0, width, height);
		if (pts.length < 2) return;
		const top = Math.max(...pts, ...ups) || 1;
		const step = width / (Math.max(60, pts.length) - 1); // fits the whole history, at least a minute wide
		const shift = reducedMotion() ? 0 : Math.min(1, (now - at) / 1000) * step;
		const x0 = width - (pts.length - 1) * step - shift;
		const y = (v: number) => height - 1 - (v / top) * (height - 4);
		c.beginPath();
		pts.forEach((v, i) => (i ? c.lineTo(x0 + i * step, y(v)) : c.moveTo(x0, y(v))));
		const ok = color('--ok', '#30a46c');
		c.lineWidth = 1.25;
		c.strokeStyle = ok;
		c.lineJoin = 'round';
		c.stroke();
		c.lineTo(x0 + (pts.length - 1) * step, height);
		c.lineTo(x0, height);
		c.closePath();
		c.globalAlpha = 0.22;
		c.fillStyle = ok;
		c.fill();
		c.globalAlpha = 1;
		// Upload: a thin line only.
		c.beginPath();
		ups.forEach((v, i) => (i ? c.lineTo(x0 + i * step, y(v)) : c.moveTo(x0, y(v))));
		c.lineWidth = 1;
		c.strokeStyle = color('--info', '#3e8ef7');
		c.stroke();
		// The peak tick.
		const pi = pts.indexOf(Math.max(...pts));
		if (pts[pi] > 0) {
			c.fillStyle = color('--text', '#888');
			c.fillRect(Math.round(x0 + pi * step) - 0.5, y(pts[pi]) - 3, 1, 3);
		}
	}

	onMount(() => {
		let raf = 0;
		const sample = () => {
			if (document.hidden) return;
			pts.push(stepToward(pts[pts.length - 1] ?? speed, speed));
			ups.push(stepToward(ups[ups.length - 1] ?? up, up));
			if (pts.length > SPAN + 1) pts.shift();
			if (ups.length > SPAN + 1) ups.shift();
			peak = Math.max(...pts);
			at = performance.now();
			if (reducedMotion()) draw(at);
		};
		const loop = (t: number) => {
			draw(t);
			raf = requestAnimationFrame(loop);
		};
		const start = () => {
			cancelAnimationFrame(raf);
			if (!document.hidden && !reducedMotion()) raf = requestAnimationFrame(loop);
		};
		if (pts.length) {
			at = performance.now();
			draw(at);
		} else sample();
		start();
		const tickId = setInterval(sample, 1000);
		document.addEventListener('visibilitychange', start);
		return () => {
			clearInterval(tickId);
			cancelAnimationFrame(raf);
			document.removeEventListener('visibilitychange', start);
		};
	});
</script>

<canvas bind:this={canvas} class="sg" style="width: {width}px; height: {height}px" data-testid="dl-speed-graph" data-peak={Math.round(peak)} data-up={Math.round(ups[ups.length - 1] ?? 0)} aria-hidden="true"></canvas>

<style>
	.sg { display: block; flex: none; }
</style>
