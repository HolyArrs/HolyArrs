<script lang="ts">
	// Hero banner: art, optional trailer that autoplays when in view, and a quiet text mode.
	import { onMount, type Snippet } from 'svelte';
	import HeroMotion from './HeroMotion.svelte';
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { heroQuiet, wordsShown, type HeroPhase } from './hero-quiet.ts';
	import { autoplayHere, loadUISettings, prefetchTrailer, trailerSources, type TrailerSource } from '../core/trailer/autoplay.svelte.ts';
	import { dwell, HERO_DELAY, type Video } from '../core/trailer/trailer.ts';
	import { fromHandoff, openTheater, resumedPlaying, takeBack, theater, type TheaterHandoff } from '../core/trailer/theater.svelte.ts';
	import { THEATER_CHROME_MS, THEATER_SHRINK_MS, viewTransitionName } from '../core/motion.ts';
	import { minimalLook } from '../core/look.svelte.ts';

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
		once = false,
		onpictureerror,
		thumb,
		thumbSquare = false,
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
		kind?: 'movie' | 'show';
		tmdb?: string | null;
		title: string;
		videos?: Video[];
		pictures?: string[];
		vtName?: string;
		position?: string;
		settled?: boolean;
		reveal?: boolean;
		once?: boolean;
		onpictureerror?: () => void;
		/** Small picture for the minimal look: left out means none, null means the initials tile. */
		thumb?: string | null;
		thumbSquare?: boolean;
		art?: Snippet;
		top?: Snippet;
		heading: Snippet;
		meta?: Snippet;
		fold?: Snippet;
		actions?: Snippet;
		after?: Snippet;
	} = $props();

	let box = $state<HTMLElement>();
	let inView = $state(false);
	let armed = $state(false);
	let sources = $state<TrailerSource[]>([]);
	let playing = $state(false);
	let finished = $state(false);
	let autoPlayed = $state(false);
	let phase = $state<HeroPhase>('still');
	let painted = $state(false);
	let artLoaded = $state(false);
	let artFailed = $state(false);
	let resumeAt = $state(0);
	let resumeRun = $state(0);
	// Plain flag: only read by heroPlaying, never drives markup.
	let resuming = false;

	const words = heroQuiet((p) => (phase = p));
	const wait = dwell(HERO_DELAY, () => (armed = true));

	const quiet = $derived(!wordsShown(phase));
	const artReady = $derived(artLoaded || !!art || !pictures.length);
	const minimal = $derived(minimalLook());
	let thumbFailed = $state(false);
	$effect(() => {
		void thumb;
		thumbFailed = false;
	});
	const trailer = $derived(!minimal && !!kind && !!tmdb && painted && artReady);

	$effect(() => () => words.dispose());

	// Coming back from the theater: resume the trailer where it left off, adding the time the exit animation took.
	$effect(() => {
		if (!theater.back) return;
		const b = takeBack(kind, tmdb);
		if (!b || b.ended || b.from === 'panel') return;
		resumeAt = b.time + (THEATER_CHROME_MS + THEATER_SHRINK_MS) / 1000;
		resumeRun += 1;
		resuming = true;
		wait.cancel();
		armed = true;
	});

	// A new title starts from a clean state.
	$effect(() => {
		void kind;
		void tmdb;
		sources = [];
		armed = false;
		playing = false;
		finished = false;
		autoPlayed = false;
		words.send('reset');
	});

	// Arm the trailer only while it is visible, settled and not already done.
	$effect(() => {
		if (finished && inView) return;
		if (once && autoPlayed && inView && armed) return;
		if (trailer && settled && inView && !finished && !(once && autoPlayed)) {
			wait.start();
			prefetchTrailer(kind!, tmdb!, false);
		} else {
			wait.cancel();
			armed = false;
		}
		return () => wait.cancel();
	});

	// Load trailer sources once armed; abort if the title changes first.
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

	$effect(() => {
		void pictures[0];
		artLoaded = false;
		artFailed = false;
	});

	// Two frames wait so the first paint is done before a trailer can start. 0.6 means mostly on screen.
	onMount(() => {
		requestAnimationFrame(() => requestAnimationFrame(() => (painted = true)));
		const io = new IntersectionObserver(([e]) => (inView = e.isIntersecting && e.intersectionRatio >= 0.6), { threshold: [0, 0.6, 1] });
		if (box) io.observe(box);
		const vis = () => document.hidden && (armed = false);
		document.addEventListener('visibilitychange', vis);
		return () => {
			io.disconnect();
			document.removeEventListener('visibilitychange', vis);
		};
	});

	function expand(o: TheaterHandoff) {
		if (!kind || !tmdb) return;
		playing = false;
		openTheater({ kind, tmdb, title, ...fromHandoff(o), from: 'hero', returnTo: box?.querySelector<HTMLElement>('.actions a[href], .actions button') ?? null });
	}

	function heroPlaying() {
		playing = true;
		autoPlayed = true;
		words.send('play');
		if (resuming) {
			resuming = false;
			resumedPlaying();
		}
	}
</script>

<section class="ui-billboard" class:quiet class:reveal class:minimal bind:this={box} data-hero={minimal ? undefined : ''} aria-label={label} data-testid={testid}>
	{#if minimal}
		{#if thumb !== undefined}
			<div class="thumb" class:sq={thumbSquare} aria-hidden="true">
				{#if thumb && !thumbFailed}<img src={thumb} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={vtName} onerror={() => (thumbFailed = true)} />{:else}<ArtPlaceholder name={title} shape={thumbSquare ? 'square' : 'poster'} />{/if}
			</div>
		{/if}
	{:else}
	<div class="art" aria-hidden={!sources.length}>
		{#if !art && (!pictures.length || artFailed || !artLoaded)}
			<ArtPlaceholder name={!pictures.length || artFailed ? title : ''} shape="wide" loading={!!pictures.length && !artFailed} />
		{/if}
		{#if art}
			{@render art()}
		{:else if pictures.length}
			{#key pictures[0]}<HeroMotion {pictures} {vtName} {position} paused={playing} onerror={() => ((artFailed = true), onpictureerror?.())} onload={() => (artLoaded = true)} />{/key}
		{/if}
		{#if sources.length}
			{#key resumeRun}
			<TrailerPlayer {sources} {title} sharp pausable top start={resumeAt} onexpand={expand} onplaying={heroPlaying} onended={() => ((playing = false), (finished = true), words.send('stop'))} onfail={() => ((playing = false), words.send('stop'))} />
			{/key}
		{/if}
		<div class="shade" class:playing></div>
	</div>
	{/if}
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
		--hero-r: 24px;
		position: relative; isolation: isolate; overflow: hidden;
		margin: 8px 0 0;
		border-radius: var(--hero-r);
		min-height: max(380px, min(77svh, calc((100vw - var(--gutter) * 2) / 2.18)));
		display: flex; flex-direction: column; justify-content: flex-end;
		background: var(--hero-ink); color: var(--on-art, #fff);
	}
	.art { position: absolute; inset: 0; z-index: -1; overflow: hidden; background: radial-gradient(110% 90% at 80% 20%, #2a2c33 0%, transparent 60%), linear-gradient(160deg, #1d1f25 0%, #0b0b0c 75%); }
	/* Layered dark gradients keep light text readable on any art. */
	.shade {
		position: absolute; inset: 0; pointer-events: none;
		background:
			radial-gradient(70% 75% at 0% 100%, rgb(0 0 0 / 72%) 0%, rgb(0 0 0 / 45%) 40%, rgb(0 0 0 / 0%) 75%),
			linear-gradient(to right, rgb(0 0 0 / 60%) 0%, rgb(0 0 0 / 0%) 60%),
			linear-gradient(to top, rgb(0 0 0 / 62%) 0%, rgb(0 0 0 / 18%) 35%, rgb(0 0 0 / 0%) 70%),
			linear-gradient(to bottom, rgb(0 0 0 / 45%) 0%, rgb(0 0 0 / 0%) 22%);
		transition: opacity var(--dur-slow) var(--ease-out);
	}
	.shade.playing { opacity: .85; }
	.ui-billboard:has(:global(.ctl [data-act])) :global([data-off-while-playing]) { display: none; }
	/* Leave room for the top strip above the heading. */
	.ui-billboard:has(> .top) { padding-top: 60px; }
	.ui-billboard:has(> .top) .heading :global(.bb-logo img) { max-height: min(var(--title-h), calc(100% - 8px)); }
	.top :global(:is(a, span, p, button, li)) { text-shadow: 0 1px 3px rgb(0 0 0 / 80%), 0 0 12px rgb(0 0 0 / 50%); }
	.top { position: absolute; top: clamp(14px, 2vw, 24px); left: clamp(20px, 2vw, 40px); right: 16px; z-index: 2; }
	.content { position: relative; width: clamp(320px, max(40%, min(500px, 60%)), 600px); padding: 0 clamp(20px, 2vw, 40px) clamp(24px, 2.6vw, 50px); display: flex; flex-direction: column; gap: 10px; transition: opacity var(--dur-slow) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	.fold { display: grid; min-height: 0; grid-template-rows: minmax(0, 1fr); opacity: 1; transition: grid-template-rows var(--dur-hero) var(--ease-in-out), opacity var(--dur-slow) var(--ease-in-out); }
	.fold-in { min-height: 0; overflow: hidden; }
	/* Quiet mode folds the extra text away until hover or focus. */
	.quiet .fold { grid-template-rows: minmax(0, 0fr); opacity: 0; pointer-events: none; }
	.quiet.reveal:hover .fold, .quiet.reveal:focus-within .fold { grid-template-rows: minmax(0, 1fr); opacity: 1; pointer-events: auto; }
	.content { --title-h: clamp(80px, 10vw, 140px); min-height: 0; }
	.ui-billboard:has(> .top) .content { --title-h: clamp(64px, 8vw, 130px); }
	.heading { transform-origin: left bottom; transition: transform var(--dur-hero) var(--ease-in-out); }
	.quiet .heading { transform: scale(.8); }
	.meta { transform-origin: left center; transition: transform var(--dur-hero) var(--ease-in-out); }
	.quiet .meta { transform: scale(.8); }
	.quiet.reveal:hover .heading, .quiet.reveal:focus-within .heading, .quiet.reveal:hover .meta, .quiet.reveal:focus-within .meta { transform: none; }
	.heading :global(h1), .heading :global(h2) { margin: 0; min-height: var(--title-h); display: flex; align-items: flex-end; }
	.heading :global(.bb-name) { font-family: var(--font-display, var(--font-ui)); font-weight: 800; font-size: clamp(36px, 4.6vw, 64px); line-height: 1; letter-spacing: -0.03em; text-wrap: balance; overflow-wrap: anywhere; text-shadow: 0 2px 30px rgb(0 0 0 / 35%); }
	.heading :global(.bb-logo img) { display: block; max-width: 100%; max-height: var(--title-h); width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 24px rgb(0 0 0 / 45%)); }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
	.meta :global(p) { margin: 0; font: 600 clamp(15px, .9vw, 22px)/1.35 var(--font-ui); color: rgb(255 255 255 / 92%); font-variant-numeric: tabular-nums; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.fold :global(.bb-desc) { margin: 0; font: 400 clamp(13px, 1.05vw, 16px)/1.45 var(--font-ui); color: rgb(255 255 255 / 92%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 100%; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.fold :global(.bb-desc.open) { -webkit-line-clamp: unset; line-clamp: unset; display: block; }
	.actions { --hero-btn: clamp(44px, 2.35vw, 60px); display: flex; flex-wrap: wrap; align-items: center; gap: clamp(10px, .6vw, 14px); margin-top: 4px; }
	.actions :global(.ui-act) { height: var(--hero-btn); min-width: var(--hero-btn); font-size: clamp(16px, 1vw, 22px); }
	.actions :global(.ui-act.round) { width: var(--hero-btn); }
	.actions :global(.ui-actmenu .ui-pop > .trigger) { height: var(--hero-btn); min-width: var(--hero-btn); font-size: clamp(16px, 1vw, 22px); }
	.actions :global(.ui-actmenu.round .ui-pop > .trigger) { width: var(--hero-btn); }
	.actions :global(.ui-ibtn) { width: var(--hero-btn); height: var(--hero-btn); }
	/* Shade sits above the trailer, controls above the shade. */
	.ui-billboard :global(.ui-trailer + .shade) { z-index: 1; }
	.ui-billboard :global(.ctl) { z-index: 4; }
	@media (max-width: 767px) {
		.ui-billboard { --hero-r: 16px; height: auto; min-height: min(560px, 120vw); }
		.content { width: 100%; padding: 64px var(--gutter) 24px; }
		.top { left: var(--gutter); }
		.actions :global(.ui-act.primary) { flex: 1 1 100%; }
	}
	/* Minimal look: a compact flat card with a small poster instead of the big art. */
	.ui-billboard.minimal { --hero-ink: #161618; min-height: 0; overflow: visible; flex-direction: row; align-items: center; justify-content: flex-start; gap: clamp(16px, 2vw, 28px); padding: clamp(16px, 2vw, 28px); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 8%); }
	.thumb { position: relative; flex: none; width: clamp(96px, 9vw, 148px); aspect-ratio: 2 / 3; overflow: hidden; border-radius: 10px; background: rgb(255 255 255 / 6%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 10%); }
	.thumb.sq { aspect-ratio: 1; }
	.thumb img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.minimal .content { --title-h: clamp(32px, 3.6vw, 56px); flex: 1 1 auto; width: auto; min-width: 0; max-width: 640px; padding: 0; }
	/* Smaller buttons and type than the full hero, so the compact card stays compact. */
	.minimal .actions { --hero-btn: 40px; gap: 8px; }
	.minimal .actions :global(:is(.ui-act, .ui-actmenu .ui-pop > .trigger)) { font-size: 15px; }
	.minimal .heading :global(h1), .minimal .heading :global(h2) { min-height: 0; }
	.minimal .heading :global(.bb-name) { font-size: clamp(24px, 2.8vw, 40px); }
	/* A wide title logo may ask for most of the screen; keep it inside the text column. */
	.minimal .heading :global(.bb-logo img) { max-width: min(100%, 420px) !important; min-height: 0; }
	@media (max-width: 767px) {
		.ui-billboard.minimal { --hero-r: 16px; min-height: 0; align-items: flex-start; gap: 14px; padding: 16px; }
		.ui-billboard.minimal { gap: 12px; }
		.minimal .thumb { width: 64px; border-radius: 8px; }
		.minimal .actions { --hero-btn: 38px; gap: 6px; }
		.minimal .actions :global(.ui-act:not(.primary):not(.round)) { padding: 0 14px 0 12px; }
		.minimal .content { width: auto; padding: 0; }
		.minimal .actions :global(.ui-act.primary) { flex: 0 1 auto; }
		.minimal .actions :global(:is(.ui-act, .ui-actmenu .ui-pop > .trigger)) { font-size: 14px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.shade, .content { transition: none; }
		.fold { transition: opacity var(--dur-slow) linear; }
		.quiet .fold { grid-template-rows: minmax(0, 0fr); }
		.heading, .quiet .heading, .meta, .quiet .meta { transition: none; transform: none; }
	}
</style>
