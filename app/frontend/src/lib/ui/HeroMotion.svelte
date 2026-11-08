<script lang="ts">
	// Slow zoom-and-fade slideshow used as a hero backdrop.
	import { onMount, untrack } from 'svelte';
	import { viewTransitionName } from '../core/motion.ts';
	import { proxied, sharpen } from '../core/tmdb-image.ts';

	let {
		pictures,
		vtName,
		position = '70% 30%',
		filter,
		every = 9000,
		paused = false,
		onerror,
		onload
	}: {
		pictures: string[];
		vtName?: string;
		position?: string;
		filter?: string;
		every?: number;
		paused?: boolean;
		onerror?: () => void;
		onload?: () => void;
	} = $props();

	let box = $state<HTMLElement>();
	let active = $state(0);
	let more = $state(false);
	let ready = $state<boolean[]>([]);
	let visible = $state(true);
	let still = $state(false);
	let wide = $state(0);
	let sharp = $state<Record<string, string>>({});

	// Only the first picture loads at first; the rest wait so the hero paints fast.
	const shown = $derived(more ? pictures : pictures.slice(0, 1));

	// Fetch a sharper size once the box is known, and swap it in after it decodes.
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

	// Skip frames that have not loaded yet so the slideshow never shows a blank.
	$effect(() => {
		if (still || paused || !visible || shown.length < 2) return;
		const t = setInterval(() => {
			if (document.hidden) return;
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

	onMount(() => {
		still = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
		// Width only ever grows, so a resize never swaps back to a blurrier size.
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

	function loaded(i: number) {
		ready[i] = true;
		if (i === 0) onload?.();
	}
	function failed(i: number) {
		if (i === 0) (onerror?.(), onload?.());
		ready[i] = false;
	}
</script>

<div class="ui-kb" bind:this={box} class:still>
	{#each shown as p, i (p)}
		<div class="frame" class:on={i === active} style:--kb-delay="{-(i * 7)}s" style:--kb-dir={i % 2 ? 'alternate-reverse' : 'alternate'}>
			<img
				src={proxied(sharp[p] ?? p)}
				alt=""
				decoding="async"
				fetchpriority={i === 0 ? 'high' : 'low'}
				loading={i === 0 ? 'eager' : 'lazy'}
				class:in={ready[i]}
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
	/* Long fade so slides blend instead of cut. */
	.frame { position: absolute; inset: 0; opacity: 0; transition: opacity 1600ms var(--ease-in-out); will-change: opacity; }
	.frame.on { opacity: 1; }
	.frame:first-child { animation: kb-in var(--dur-page, 420ms) var(--ease-out) backwards; }
	img {
		position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover;
		transform-origin: 60% 40%;
		animation: kb 26s var(--ease-in-out) var(--kb-delay, 0s) infinite var(--kb-dir, alternate) both;
		will-change: transform;
		backface-visibility: hidden;
		opacity: 0;
		transition: opacity var(--dur-slow, 400ms) var(--ease-out);
	}
	img.in { opacity: 1; }
	@keyframes kb {
		from { transform: scale(1.02) translate3d(0, 0, 0); }
		to { transform: scale(1.1) translate3d(-2.2%, -1.4%, 0); }
	}
	@keyframes kb-in { from { opacity: 0; } }
	/* Reduced motion is also set from script, so the class covers older engines. */
	.still img, .still .frame { animation: none; transition: none; }
	@media (prefers-reduced-motion: reduce) {
		img, .frame, .frame:first-child { animation: none; transition: none; }
	}
</style>
