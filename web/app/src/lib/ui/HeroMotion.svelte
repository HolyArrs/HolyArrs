<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { viewTransitionName } from '../motion.ts';
	import { sharpen } from '../tmdb-image.ts';

	// Layer 1 of every hero: the backdrop at once (from the artwork cache),
	// a slow Ken Burns zoom-and-pan, and a crossfade through the title's
	// other backdrops. Transforms and opacity only (GPU), never layout.
	// Reduce motion: the first picture, still.
	let {
		pictures,
		vtName,
		position = '70% 30%',
		filter,
		every = 9000,
		paused = false,
		onerror
	}: {
		/** Already-sized addresses; the first shows straight away. */
		pictures: string[];
		vtName?: string;
		position?: string;
		filter?: string;
		/** Milliseconds each picture stays. */
		every?: number;
		/** A trailer is playing over it: stop changing pictures. */
		paused?: boolean;
		/** The first picture failed. */
		onerror?: () => void;
	} = $props();

	let box = $state<HTMLElement>();
	let active = $state(0);
	// Later pictures load only once the hero has been on screen a while.
	let more = $state(false);
	let ready = $state<boolean[]>([]);
	let visible = $state(true);
	let still = $state(false);
	const shown = $derived(more ? pictures : pictures.slice(0, 1));
	// The small picture paints at once; the one sized to the rendered width
	// (w1280 or original on wide screens) replaces it once it has loaded.
	let wide = $state(0);
	let sharp = $state<Record<string, string>>({});
	$effect(() => {
		if (!wide) return;
		const gone: HTMLImageElement[] = [];
		for (const p of shown) {
			const s = sharpen(p, wide);
			if (s === p || untrack(() => sharp[p]) === s) continue;
			const im = new Image();
			im.decoding = 'async';
			im.onload = () => im.decode().catch(() => {}).then(() => (sharp[p] = s));
			im.src = s;
			gone.push(im);
		}
		return () => gone.forEach((im) => (im.onload = null));
	});

	onMount(() => {
		still = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
		const ro = new ResizeObserver(([e]) => {
			const w = Math.round(e.contentRect.width);
			if (w > wide) wide = w;
		});
		if (box) ro.observe(box);
		const io = new IntersectionObserver(([e]) => (visible = e.isIntersecting), { threshold: 0.2 });
		if (box) io.observe(box);
		const t = setTimeout(() => (more = true), Math.min(every, 4000));
		return () => {
			io.disconnect();
			ro.disconnect();
			clearTimeout(t);
		};
	});

	$effect(() => {
		if (still || paused || !visible || shown.length < 2) return;
		const t = setInterval(() => {
			if (document.hidden) return;
			// Only move to a picture that has loaded.
			for (let step = 1; step < shown.length; step++) {
				const n = (active + step) % shown.length;
				if (n === 0 || ready[n]) {
					active = n;
					return;
				}
			}
		}, every);
		return () => clearInterval(t);
	});

	function loaded(i: number) {
		ready[i] = true;
	}
	function failed(i: number) {
		if (i === 0) onerror?.();
		ready[i] = false;
	}
</script>

<div class="ui-kb" bind:this={box} class:still>
	{#each shown as p, i (p)}
		<div class="frame" class:on={i === active} style:--kb-delay="{-(i * 7)}s" style:--kb-dir={i % 2 ? 'alternate-reverse' : 'alternate'}>
			<img
				src={sharp[p] ?? p}
				alt=""
				decoding="async"
				fetchpriority={i === 0 ? 'high' : 'low'}
				loading={i === 0 ? 'eager' : 'lazy'}
				style:object-position={position}
				style:filter
				use:viewTransitionName={i === 0 ? vtName : undefined}
				onload={() => loaded(i)}
				onerror={() => failed(i)}
			/>
		</div>
	{/each}
</div>

<style>
	.ui-kb { position: absolute; inset: 0; overflow: hidden; contain: strict; }
	.frame { position: absolute; inset: 0; opacity: 0; transition: opacity 1600ms var(--ease-in-out); will-change: opacity; }
	.frame.on { opacity: 1; }
	.frame:first-child { animation: kb-in var(--dur-page, 420ms) var(--ease-out) backwards; }
	img {
		position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover;
		transform-origin: 60% 40%;
		animation: kb 26s var(--ease-in-out) var(--kb-delay, 0s) infinite var(--kb-dir, alternate) both;
		will-change: transform;
		backface-visibility: hidden;
	}
	@keyframes kb {
		from { transform: scale(1.02) translate3d(0, 0, 0); }
		to { transform: scale(1.1) translate3d(-2.2%, -1.4%, 0); }
	}
	@keyframes kb-in { from { opacity: 0; } }
	.still img, .still .frame { animation: none; transition: none; }
	@media (prefers-reduced-motion: reduce) {
		img, .frame, .frame:first-child { animation: none; transition: none; }
	}
</style>
