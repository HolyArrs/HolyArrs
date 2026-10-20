<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { api } from '../api.ts';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import HeroMotion from './HeroMotion.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { autoplayHere, loadUISettings, prefetchTrailer, trailerSources, type TrailerSource } from '../autoplay.svelte.ts';
	import { dwell, heroPictures, HERO_DELAY } from '../trailer.ts';
	import { sized } from '../tmdb-image.ts';
	import { shortSynopsis } from '../synopsis.ts';
	import { openTheater } from '../theater.svelte.ts';
	import { heroQuiet, wordsShown, type HeroPhase } from '../hero-quiet.ts';

	// The Explore hero: one featured title, Layer 1 moving pictures at once,
	// and (Layer 2) a muted trailer that fades in once the page has settled
	// and the hero has stayed in view for HERO_DELAY. Leaving the view
	// removes the player; when the trailer ends the pictures come back.
	let {
		kind,
		tmdb,
		title,
		href,
		backdrop,
		subtitle,
		settled = true,
		badge,
		action,
		onmore
	}: {
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		href?: string;
		/** A backdrop known already (shown at once). */
		backdrop?: string | null;
		subtitle?: string;
		/** The page has finished its first load. */
		settled?: boolean;
		badge?: Snippet;
		action?: Snippet;
		/** More info opens the panel on this page (a plain click); the link stays for new tabs. */
		onmore?: (from: DOMRect) => void;
	} = $props();

	let box = $state<HTMLElement>();
	let meta = $state<RichMeta | null>(null);
	let inView = $state(false);
	let armed = $state(false);
	let sources = $state<TrailerSource[]>([]);
	let playing = $state(false);
	// Netflix (measured 2026-10-20): about 6 s into the trailer the overview
	// folds away and the logo glides down; only the trailer stopping for good
	// (ended, failed, taken over by another player) or a new title brings the
	// words back. Pointer moves and scrolling leave it as it is.
	let phase = $state<HeroPhase>('still');
	const words = heroQuiet((p) => (phase = p));
	const quiet = $derived(!wordsShown(phase));
	$effect(() => () => words.dispose());
	function moreInfo(e: MouseEvent) {
		if (!onmore || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		onmore((box ?? (e.currentTarget as HTMLElement)).getBoundingClientRect());
	}
	function expand(o: { source: TrailerSource; time: number; rect: DOMRect }) {
		playing = false;
		openTheater({ kind, tmdb, title, source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, from: 'hero' });
	}

	const pictures = $derived(heroPictures(meta?.backdrop ?? backdrop, meta?.backdrops).map((u) => sized(u, 'backdrop')));
	const line = $derived([meta?.year, ...(meta?.genres ?? []).slice(0, 3)].filter(Boolean).join(' · ') || subtitle || '');
	const wait = dwell(HERO_DELAY, () => (armed = true));

	$effect(() => {
		const key = `${kind}:${tmdb}`;
		let live = true;
		meta = null;
		sources = [];
		armed = false;
		playing = false;
		words.send('reset');
		void richMetadata(api.get, kind, tmdb).then((m) => live && key === `${kind}:${tmdb}` && (meta = m));
		return () => {
			live = false;
		};
	});

	$effect(() => {
		if (settled && inView) {
			wait.start();
			// The lookup starts now, not after the dwell.
			prefetchTrailer(kind, tmdb);
		}
		else {
			wait.cancel();
			armed = false;
		}
		return () => wait.cancel();
	});

	// Armed and still in view: ask for the sources; out of view: drop the player.
	$effect(() => {
		if (!armed) {
			sources = [];
			playing = false;
			return;
		}
		const ctl = new AbortController();
		void (async () => {
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			const s = await trailerSources(kind, tmdb, meta?.videos, ctl.signal);
			if (!ctl.signal.aborted) sources = s;
		})();
		return () => ctl.abort();
	});

	onMount(() => {
		const io = new IntersectionObserver(([e]) => (inView = e.isIntersecting && e.intersectionRatio >= 0.6), { threshold: [0, 0.6, 1] });
		if (box) io.observe(box);
		const vis = () => document.hidden && (armed = false);
		document.addEventListener('visibilitychange', vis);
		return () => {
			io.disconnect();
			document.removeEventListener('visibilitychange', vis);
		};
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<section class="ui-xhero" class:quiet bind:this={box} aria-label="Featured: {title}" data-testid="explore-hero">
	<div class="art" aria-hidden={!sources.length}>
		{#if pictures.length}
			{#key pictures[0]}<HeroMotion {pictures} position="70% 25%" paused={playing} />{/key}
		{/if}
		{#if sources.length}
			<TrailerPlayer {sources} {title} onexpand={expand} onplaying={() => ((playing = true), words.send('play'))} onended={() => ((playing = false), words.send('stop'))} onfail={() => ((playing = false), words.send('stop'))} />
		{/if}
		<div class="shade" class:playing></div>
	</div>
	<div class="content" class:playing>
		{#if meta?.logo}
			<h2 class="logo"><img src={meta.logo} alt={title} /></h2>
		{:else}
			<h2 class="name">{title}</h2>
		{/if}
		{#if line || badge}
			<div class="meta">
				{#if line}<p>{line}</p>{/if}
				{#if badge}{@render badge()}{/if}
			</div>
		{/if}
		<div class="fold" aria-hidden={quiet || undefined}><div class="fold-in">
		{#if !meta}
			<div class="about" aria-hidden="true"></div>
		{:else if meta.overview}
			<div class="about">
				<!-- Netflix keeps the billboard synopsis to a couple of lines; no Show more. -->
				<p class="desc" title={meta.overview}>{shortSynopsis(meta.overview)}</p>
			</div>
		{/if}
		</div></div>
		<div class="actions">
			{#if href}<a class="info" {href} onclick={moreInfo} aria-haspopup={onmore ? 'dialog' : undefined}>More info</a>{/if}
			{#if action}{@render action()}{/if}
		</div>
	</div>
</section>

<style>
	.ui-xhero {
		--hero-ink: var(--on-art-ink, #0b0b0c);
		/* Netflix's inset billboard (measured): inside the page gutter, 24 px corners,
		   77 % of the viewport tall but never taller than a 2.18:1 card (1913 x 878 at 2048 x 1145). */
		--hero-r: 24px;
		position: relative; isolation: isolate; overflow: hidden;
		margin: 8px 0 0;
		border-radius: var(--hero-r);
		height: max(380px, min(77svh, calc((100vw - var(--gutter) * 2) / 2.18)));
		display: flex; flex-direction: column; justify-content: flex-end;
		background: var(--hero-ink); color: var(--on-art, #fff);
	}
	.art { position: absolute; inset: 0; z-index: -1; background: radial-gradient(110% 90% at 80% 20%, #2a2c33 0%, transparent 60%), linear-gradient(160deg, #1d1f25 0%, #0b0b0c 75%); }
	.shade {
		position: absolute; inset: 0; pointer-events: none;
		background:
			linear-gradient(to right, rgb(0 0 0 / 60%) 0%, rgb(0 0 0 / 0%) 60%),
			linear-gradient(to top, rgb(0 0 0 / 50%) 0%, rgb(0 0 0 / 10%) 35%, rgb(0 0 0 / 0%) 70%);
		transition: opacity var(--dur-slow) var(--ease-out);
	}
	.shade.playing { opacity: .85; }
	/* Netflix keeps the billboard's words in the left ~40%, so the picture stays open. */
	.content { position: relative; width: clamp(320px, 40%, 600px); padding: 0 clamp(20px, 2vw, 40px) clamp(24px, 2.6vw, 50px); display: flex; flex-direction: column; gap: 10px; transition: opacity var(--dur-slow) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	/* About 6 s into the trailer the overview folds (height and opacity, ~1 s ease) and
	   the logo glides down with it; it unfolds the same way. Both directions are one
	   transition, so it reverses mid-way and never sticks. */
	.fold { display: grid; grid-template-rows: 1fr; opacity: 1; transition: grid-template-rows 1100ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)), opacity 650ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)); }
	.fold-in { min-height: 0; overflow: hidden; }
	.quiet .fold { grid-template-rows: 0fr; opacity: 0; pointer-events: none; }
	/* The title box and the overview keep their space while the details load, so nothing moves when they arrive. */
	.content { --title-h: clamp(80px, 10vw, 140px); }
	h2 { margin: 0; min-height: var(--title-h); display: flex; align-items: flex-end; transform-origin: left bottom; transition: transform 1100ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)); }
	/* While the trailer plays the title settles a little smaller, like before. */
	.quiet h2 { transform: scale(.8); }
	.about { min-height: calc(15px * 1.5 * 3); }
	.name { font-family: var(--font-display, var(--font-ui)); font-weight: 800; font-size: clamp(36px, 4.6vw, 64px); line-height: 1; letter-spacing: -0.03em; text-wrap: balance; text-shadow: 0 2px 30px rgb(0 0 0 / 35%); }
	.logo img { display: block; max-width: 100%; max-height: var(--title-h); width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 24px rgb(0 0 0 / 45%)); }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
	.meta p { margin: 0; font: 600 15px/1.35 var(--font-ui); color: rgb(255 255 255 / 92%); font-variant-numeric: tabular-nums; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.desc { margin: 0; font: var(--t-body); color: rgb(255 255 255 / 92%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 100%; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 4px; }
	.info { display: inline-flex; align-items: center; min-height: 44px; padding: 0 20px; border-radius: var(--r-pill); background: rgb(255 255 255 / 16%); border: 1px solid rgb(255 255 255 / 22%); color: #fff; font: var(--t-headline); -webkit-backdrop-filter: blur(18px) saturate(160%); backdrop-filter: blur(18px) saturate(160%); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-instant) var(--ease-out); }
	.info:hover { background: rgb(255 255 255 / 26%); }
	.info:active { transform: scale(.97); }
	.info:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; }
	.ui-xhero :global(.ui-trailer + .shade) { z-index: 1; }
	.ui-xhero :global(.ctl) { z-index: 2; bottom: clamp(24px, 3.4vw, 44px); right: clamp(20px, 4vw, 56px); }
	@media (max-width: 767px) {
		.ui-xhero { --hero-r: 16px; height: auto; min-height: min(560px, 120vw); }
		.content { width: 100%; padding: 0 var(--gutter) 24px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.shade, .content { transition: none; }
		/* Opacity only: the overview keeps its room and simply fades. */
		.fold { transition: opacity 400ms linear; }
		.quiet .fold { grid-template-rows: 1fr; }
		h2, .quiet h2 { transition: none; transform: none; }
	}
</style>
