<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import HeroMotion from './HeroMotion.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { autoplayHere, loadUISettings, prefetchTrailer, trailerSources, type TrailerSource } from '../autoplay.svelte.ts';
	import { dwell, HERO_DELAY, type Video } from '../trailer.ts';
	import { openTheater } from '../theater.svelte.ts';
	import { heroQuiet, wordsShown, type HeroPhase } from '../hero-quiet.ts';

	// The billboard every hero shares (Explore and the four title pages): an
	// inset rounded frame inside the page gutter, the words in the left 40%,
	// Layer 1 moving pictures at once and (Layer 2) a muted trailer once the
	// page has settled and the hero has stayed in view for HERO_DELAY. About
	// 6 s into the trailer the words fold away (hero-quiet.ts); with `reveal`
	// hovering or focusing the hero brings them back. Phones, reduced motion,
	// data saver or the Autoplay trailers setting off: no autoplay
	// (autoplayHere), the pictures stay.
	let {
		label,
		testid,
		kind,
		tmdb,
		title,
		videos,
		pictures = [],
		vtName,
		position = '70% 25%',
		settled = true,
		reveal = false,
		onpictureerror,
		art,
		top,
		heading,
		meta,
		fold,
		actions,
		after
	}: {
		label: string;
		testid?: string;
		/** A movie or show: the trailer plays here. Artists and albums leave it out. */
		kind?: 'movie' | 'show';
		tmdb?: string | null;
		title: string;
		videos?: Video[];
		/** Already-sized backdrops; the first shows at once. */
		pictures?: string[];
		vtName?: string;
		position?: string;
		/** The page has finished its first load. */
		settled?: boolean;
		/** Hover or focus brings folded words back (title pages). */
		reveal?: boolean;
		onpictureerror?: () => void;
		/** Art drawn instead of the pictures (an album cover, a poster). */
		art?: Snippet;
		/** Top-left (the breadcrumb). */
		top?: Snippet;
		heading: Snippet;
		meta?: Snippet;
		/** The words that fold away (the synopsis). */
		fold?: Snippet;
		actions?: Snippet;
		/** Inside the content, under the actions. */
		after?: Snippet;
	} = $props();

	let box = $state<HTMLElement>();
	let inView = $state(false);
	let armed = $state(false);
	let sources = $state<TrailerSource[]>([]);
	let playing = $state(false);
	let phase = $state<HeroPhase>('still');
	const words = heroQuiet((p) => (phase = p));
	const quiet = $derived(!wordsShown(phase));
	$effect(() => () => words.dispose());
	const trailer = $derived(!!kind && !!tmdb);
	const wait = dwell(HERO_DELAY, () => (armed = true));

	function expand(o: { source: TrailerSource; time: number; rect: DOMRect }) {
		if (!kind || !tmdb) return;
		playing = false;
		openTheater({ kind, tmdb, title, source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, from: 'hero', returnTo: box?.querySelector<HTMLElement>('.actions a[href], .actions button') ?? null });
	}

	// A new title: back to the still pictures.
	$effect(() => {
		void kind;
		void tmdb;
		sources = [];
		armed = false;
		playing = false;
		words.send('reset');
	});

	$effect(() => {
		if (trailer && settled && inView) {
			wait.start();
			// The lookup starts now, not after the dwell.
			prefetchTrailer(kind!, tmdb!);
		} else {
			wait.cancel();
			armed = false;
		}
		return () => wait.cancel();
	});

	// Armed and still in view: ask for the sources; out of view: drop the player.
	$effect(() => {
		if (!armed || !kind || !tmdb) {
			sources = [];
			playing = false;
			return;
		}
		const k = kind;
		const t = tmdb;
		const ctl = new AbortController();
		void (async () => {
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			const s = await trailerSources(k, t, videos, ctl.signal);
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

<section class="ui-billboard" class:quiet class:reveal bind:this={box} aria-label={label} data-testid={testid}>
	<div class="art" aria-hidden={!sources.length}>
		{#if art}
			{@render art()}
		{:else if pictures.length}
			{#key pictures[0]}<HeroMotion {pictures} {vtName} {position} paused={playing} onerror={onpictureerror} />{/key}
		{/if}
		{#if sources.length}
			<TrailerPlayer {sources} {title} pausable onexpand={expand} onplaying={() => ((playing = true), words.send('play'))} onended={() => ((playing = false), words.send('stop'))} onfail={() => ((playing = false), words.send('stop'))} />
		{/if}
		<div class="shade" class:playing></div>
	</div>
	{#if top}<div class="top">{@render top()}</div>{/if}
	<div class="content" class:playing>
		<div class="heading">{@render heading()}</div>
		{#if meta}<div class="meta">{@render meta()}</div>{/if}
		{#if fold}
			<div class="fold" aria-hidden={(quiet && !reveal) || undefined}><div class="fold-in">{@render fold()}</div></div>
		{/if}
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
		{#if after}{@render after()}{/if}
	</div>
</section>

<style>
	.ui-billboard {
		--hero-ink: var(--on-art-ink, #0b0b0c);
		/* Netflix's inset billboard (measured): inside the page gutter, 24 px corners,
		   77 % of the viewport tall but never taller than a 2.18:1 card (1913 x 878 at 2048 x 1145). */
		--hero-r: 24px;
		position: relative; isolation: isolate; overflow: hidden;
		margin: 8px 0 0;
		border-radius: var(--hero-r);
		min-height: max(380px, min(77svh, calc((100vw - var(--gutter) * 2) / 2.18)));
		display: flex; flex-direction: column; justify-content: flex-end;
		background: var(--hero-ink); color: var(--on-art, #fff);
	}
	.art { position: absolute; inset: 0; z-index: -1; overflow: hidden; background: radial-gradient(110% 90% at 80% 20%, #2a2c33 0%, transparent 60%), linear-gradient(160deg, #1d1f25 0%, #0b0b0c 75%); }
	.shade {
		position: absolute; inset: 0; pointer-events: none;
		background:
			linear-gradient(to right, rgb(0 0 0 / 60%) 0%, rgb(0 0 0 / 0%) 60%),
			linear-gradient(to top, rgb(0 0 0 / 50%) 0%, rgb(0 0 0 / 10%) 35%, rgb(0 0 0 / 0%) 70%),
			/* Behind the breadcrumb and chips at the top: bright frames kept them under 3:1. */
			linear-gradient(to bottom, rgb(0 0 0 / 45%) 0%, rgb(0 0 0 / 0%) 22%);
		transition: opacity var(--dur-slow) var(--ease-out);
	}
	.shade.playing { opacity: .85; }
	/* Room for the breadcrumb, so a tall logo never runs under it. */
	.ui-billboard:has(> .top) { padding-top: 60px; }
	.ui-billboard:has(> .top) .heading :global(.bb-logo img) { max-height: min(var(--title-h), calc(100% - 8px)); }
	.top :global(:is(a, span, p, button, li)) { text-shadow: 0 1px 3px rgb(0 0 0 / 80%), 0 0 12px rgb(0 0 0 / 50%); }
	.top { position: absolute; top: clamp(14px, 2vw, 24px); left: clamp(20px, 2vw, 40px); right: 16px; z-index: 2; }
	/* Netflix keeps the billboard's words in the left ~40%, so the picture stays open. */
	.content { position: relative; width: clamp(320px, 40%, 600px); padding: 0 clamp(20px, 2vw, 40px) clamp(24px, 2.6vw, 50px); display: flex; flex-direction: column; gap: 10px; transition: opacity var(--dur-slow) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	/* About 6 s into the trailer the overview folds (height and opacity, ~1 s ease) and
	   the logo glides down with it; it unfolds the same way. */
	.fold { display: grid; grid-template-rows: 1fr; opacity: 1; transition: grid-template-rows 1100ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)), opacity 650ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)); }
	.fold-in { min-height: 0; overflow: hidden; }
	.quiet .fold { grid-template-rows: 0fr; opacity: 0; pointer-events: none; }
	.quiet.reveal:hover .fold, .quiet.reveal:focus-within .fold { grid-template-rows: 1fr; opacity: 1; pointer-events: auto; }
	.content { --title-h: clamp(80px, 10vw, 140px); min-height: 0; }
	.ui-billboard:has(> .top) .content { --title-h: clamp(64px, 8vw, 130px); }
	.heading { transform-origin: left bottom; transition: transform 1100ms var(--ease-hero, cubic-bezier(.4, 0, .2, 1)); }
	.quiet .heading { transform: scale(.8); }
	.quiet.reveal:hover .heading, .quiet.reveal:focus-within .heading { transform: none; }
	.heading :global(h1), .heading :global(h2) { margin: 0; min-height: var(--title-h); display: flex; align-items: flex-end; }
	.heading :global(.bb-name) { font-family: var(--font-display, var(--font-ui)); font-weight: 800; font-size: clamp(36px, 4.6vw, 64px); line-height: 1; letter-spacing: -0.03em; text-wrap: balance; overflow-wrap: anywhere; text-shadow: 0 2px 30px rgb(0 0 0 / 35%); }
	.heading :global(.bb-logo img) { display: block; max-width: 100%; max-height: var(--title-h); width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 24px rgb(0 0 0 / 45%)); }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
	.meta :global(p) { margin: 0; font: 600 15px/1.35 var(--font-ui); color: rgb(255 255 255 / 92%); font-variant-numeric: tabular-nums; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.fold :global(.bb-desc) { margin: 0; font: 400 clamp(13px, 1.05vw, 16px)/1.45 var(--font-ui); color: rgb(255 255 255 / 92%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 100%; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.fold :global(.bb-desc.open) { -webkit-line-clamp: unset; line-clamp: unset; display: block; }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 4px; }
	.ui-billboard :global(.ui-trailer + .shade) { z-index: 1; }
	.ui-billboard :global(.ctl) { z-index: 2; bottom: clamp(24px, 3.4vw, 44px); right: clamp(20px, 4vw, 56px); }
	@media (max-width: 767px) {
		.ui-billboard { --hero-r: 16px; height: auto; min-height: min(560px, 120vw); }
		.content { width: 100%; padding: 64px var(--gutter) 24px; }
		.top { left: var(--gutter); }
		.actions :global(.ui-act.primary) { flex: 1 1 100%; }
	}
	@media (prefers-reduced-motion: reduce) {
		.shade, .content { transition: none; }
		.fold { transition: opacity 400ms linear; }
		.quiet .fold { grid-template-rows: 1fr; }
		.heading, .quiet .heading { transition: none; transform: none; }
	}
</style>
