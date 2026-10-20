<script lang="ts">
	import { metaLine } from '../fresh.ts';
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { api } from '../api.ts';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import Icon from '../Icon.svelte';
	import { sized } from '../tmdb-image.ts';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '../autoplay.svelte.ts';
	import { openTheater } from '../theater.svelte.ts';
	import { flipTransform, growPlacement, GROW_MS, TRAILER_AFTER, type Growth } from '../grow.ts';

	// The wide card grown in place, Netflix style: the card itself (same
	// picture, same logo, same badges) scales up about 1.5x over its
	// neighbours, then an info panel shows under the picture and the
	// trailer fades in inside it. A FLIP from the card's exact rect, in a
	// portal at the end of <body> so rows never clip it; leaving shrinks it
	// back onto the card before it goes.
	let {
		anchor,
		kind,
		tmdb,
		title,
		href,
		year,
		image,
		logo,
		lifted = false,
		leaving = false,
		badge,
		action,
		onenter,
		onleave,
		onclose,
		ongone,
		onmore
	}: {
		/** The card's picture box: the grown card starts exactly on it. */
		anchor: HTMLElement;
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		href?: string;
		year?: number;
		/** The card's own picture URL (already loaded), so the grown card shows the same image. */
		image?: string | null;
		logo?: string | null;
		/** The card has a freshness strip: the name sits higher, as on the card. */
		lifted?: boolean;
		/** Set by the card when the pointer left: shrink back, then ongone. */
		leaving?: boolean;
		badge?: Snippet;
		action?: Snippet;
		onenter?: () => void;
		onleave?: () => void;
		/** Esc, scroll or resize: close now. */
		onclose: () => void;
		/** The shrink finished; the card shows again. */
		ongone: () => void;
		/** The ⌄ opens the More info panel here (Explore) instead of following href. */
		onmore?: (from: DOMRect) => void;
	} = $props();

	let card = $state<HTMLElement>();
	let g = $state<Growth | null>(null);
	let meta = $state<RichMeta | null>(null);
	let sources = $state<TrailerSource[]>([]);
	let trailerOk = $state(false);
	let playing = $state(false);
	let shown = $state(false);
	let logoFailed = $state(false);
	let anim: Animation | null = null;
	// The card's own width: the badge layer is drawn at the card's size and grows with it.
	// svelte-ignore state_referenced_locally
	const baseW = anchor.getBoundingClientRect().width || 1;
	const ctl = new AbortController();
	const reduce = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;

	function measure(): Growth | null {
		if (!card) return null;
		const a = anchor.getBoundingClientRect();
		const panel = card.querySelector<HTMLElement>('.panel')?.offsetHeight ?? 0;
		const row = (anchor.closest('.row') ?? anchor.closest('main'))?.getBoundingClientRect();
		return growPlacement({ left: a.left, top: a.top, width: a.width, height: a.height }, panel, { width: document.documentElement.clientWidth, height: window.innerHeight }, row ? { left: row.left, right: row.right } : undefined);
	}

	/** The clip that shows only the picture (the start and end of the grow). */
	function clipTo(at: Growth) {
		const panel = Math.max(0, (card?.offsetHeight ?? 0) - at.mediaHeight);
		return `inset(0 0 ${panel}px 0 round var(--r-card))`;
	}

	function grow() {
		const next = measure();
		if (!next || !card) return;
		g = next;
		shown = true;
		if (reduce()) return;
		anim?.cancel();
		anim = card.animate(
			[
				// Only the picture at first: the panel below is clipped away, then opens as the card grows.
				{ transform: flipTransform(next.from), clipPath: clipTo(next) },
				{ transform: 'none', clipPath: 'inset(0 0 0 0 round var(--r-card))' }
			],
			{ duration: GROW_MS, easing: 'cubic-bezier(.3, .1, .25, 1)', fill: 'both' }
		);
	}

	function shrink() {
		if (!card || !g || reduce()) return ongone();
		const a = anchor.getBoundingClientRect();
		const back = { x: a.left - g.left, y: a.top - g.top, scale: a.width / g.width };
		const now = anim && anim.playState === 'running' ? getComputedStyle(card).transform : 'none';
		anim?.cancel();
		anim = card.animate([{ transform: now, clipPath: 'inset(0 0 0 0 round var(--r-card))' }, { transform: flipTransform(back), clipPath: clipTo(g) }], { duration: GROW_MS * 0.8, easing: 'cubic-bezier(.4, 0, .6, 1)', fill: 'both' });
		anim.onfinish = () => ongone();
	}

	$effect(() => {
		if (leaving) shrink();
	});

	// The portal: moved to <body> so overflow and transforms of rows do not clip it.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	onMount(() => {
		grow();
		const close = () => onclose();
		window.addEventListener('scroll', close, { passive: true, capture: true });
		window.addEventListener('resize', close);
		const later = setTimeout(() => (trailerOk = true), GROW_MS + TRAILER_AFTER);
		void (async () => {
			meta = await richMetadata(api.get, kind, tmdb);
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			sources = await trailerSources(kind, tmdb, meta?.videos, ctl.signal);
		})();
		return () => {
			clearTimeout(later);
			window.removeEventListener('scroll', close, { capture: true });
			window.removeEventListener('resize', close);
		};
	});
	onDestroy(() => {
		ctl.abort();
		anim?.cancel();
	});

	const shownLogo = $derived(logoFailed ? null : logo ? logo : meta?.logo ? sized(meta.logo, 'poster') : null);
	function runtime(m?: number) {
		if (!m) return '';
		const h = Math.floor(m / 60);
		return h ? `${h}h ${m % 60}m` : `${m}m`;
	}
	const facts = $derived(metaLine(kind, [kind === 'movie' ? runtime(meta?.runtime) : undefined, meta?.year ?? year]));
	const tags = $derived((meta?.genres ?? []).slice(0, 3));
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="ui-grow"
	class:shown
	bind:this={card}
	use:portal
	role="group"
	aria-label="{title} preview"
	data-testid="grow-card"
	style:left="{g?.left ?? -9999}px"
	style:top="{g?.top ?? -9999}px"
	style:width="{g?.width ?? anchor.getBoundingClientRect().width * 1.5}px"
	onpointerenter={onenter}
	onpointerleave={onleave}
>
	<a class="media" href={href} tabindex="-1" aria-hidden="true">
		{#if image}<img src={image} alt="" decoding="async" />{/if}
		{#if trailerOk && sources.length}
			<TrailerPlayer {sources} {title} repeat onplaying={() => (playing = true)} onfail={() => (playing = false)} onexpand={(o) => { openTheater({ kind, tmdb, title, source: o.source, time: o.time, rect: { left: o.rect.left, top: o.rect.top, width: o.rect.width, height: o.rect.height }, from: 'hover' }); onclose(); }} />
		{/if}
		<span class="scrim" class:playing aria-hidden="true"></span>
		<span class="name" class:playing class:lifted>
			{#if shownLogo}<img class="logo" src={shownLogo} alt="" onerror={() => (logoFailed = true)} />{:else}<span class="word">{title}</span>{/if}
		</span>
		{#if badge && g}<span class="badge" class:playing style:width="{baseW}px" style:height="{baseW * 9 / 16}px" style:transform="scale({g.width / baseW})">{@render badge()}</span>{/if}
	</a>
	<div class="panel">
		<div class="buttons">
			{#if action}{@render action()}{/if}
			{#if onmore}<button type="button" class="more" aria-label="More info about {title}" title="More info" aria-haspopup="dialog" onclick={() => card && onmore(card.getBoundingClientRect())}><Icon name="chevronDown" size={20} /></button>
			{:else if href}<a class="more" {href} aria-label="More about {title}" title="More info"><Icon name="chevronDown" size={20} /></a>{/if}
		</div>
		<!-- Both lines always take their room, so the panel's height is known before the facts arrive. -->
		<p class="facts">
			{#if meta?.certification}<span class="rating">{meta.certification}</span>{/if}
			{#each facts as f (f)}<span>{f}</span>{/each}
		</p>
		<p class="tags">{#each tags as t, i (t)}{#if i}<i aria-hidden="true">•</i>{/if}<span>{t}</span>{/each}</p>
	</div>
</div>

<style>
	.ui-grow {
		--grow-shadow: 0 18px 48px rgb(0 0 0 / 0.42), 0 4px 14px rgb(0 0 0 / 0.28);
		position: fixed; z-index: 60;
		border-radius: var(--r-card); overflow: hidden;
		background: var(--surface-1); color: var(--text);
		box-shadow: var(--grow-shadow), 0 0 0 1px var(--line);
		transform-origin: 0 0;
		visibility: hidden;
		will-change: transform;
	}
	.ui-grow.shown { visibility: visible; }
	.media { position: relative; display: block; container-type: inline-size; aspect-ratio: var(--wide-ratio, 16 / 9); background: var(--surface-2); overflow: hidden; }
	.media > img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.scrim { position: absolute; inset: auto 0 0; height: 55%; background: linear-gradient(to top, rgb(0 0 0 / 0.62), transparent); pointer-events: none; transition: opacity 600ms ease; }
	.name { position: absolute; left: 6%; right: 30%; bottom: 13%; z-index: 2; display: flex; align-items: flex-end; pointer-events: none; transition: transform 600ms var(--ease-out), opacity 600ms ease; transform-origin: left bottom; }
	.name.lifted { bottom: 26%; }
	.name.playing { opacity: 0; transform: scale(0.9); transition-delay: 1.2s; }
	.badge { transition: opacity 600ms ease 1.2s; }
	.badge.playing { opacity: 0; }
	.scrim.playing { opacity: 0.5; }
	.logo { max-width: 100%; max-height: 22cqw; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.5)); }
	.word { color: #fff; font-family: var(--font-display, inherit); font-size: max(13px, 7.2cqw); font-weight: 700; letter-spacing: -0.02em; line-height: 1.05; text-shadow: 0 1px 10px rgb(0 0 0 / 0.55); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.badge { position: absolute; left: 0; top: 0; z-index: 3; container: art / inline-size; pointer-events: none; transform-origin: 0 0; }
	.panel { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px 14px; opacity: 0; transform: translateY(-6px); }
	.shown .panel { animation: panel-in 240ms var(--ease-out) 90ms both; }
	@keyframes panel-in { to { opacity: 1; transform: none; } }
	.buttons { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.buttons :global(.ui-ibtn.glass) { background: var(--text); color: var(--surface-1); box-shadow: none; }
	.buttons :global(.ui-ibtn.glass:hover) { background: color-mix(in srgb, var(--text) 85%, var(--surface-1)); }
	.more { flex: none; width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; color: var(--text); box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--text) 40%, transparent); transition: box-shadow var(--dur-fast) ease, background-color var(--dur-fast) ease; }
	.more:hover { box-shadow: inset 0 0 0 2px var(--text); background: var(--surface-2); }
	.more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.facts, .tags { min-height: 18px; }
	.facts { display: flex; align-items: center; gap: 8px; font: var(--t-footnote); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.rating { padding: 0 5px; border: 1px solid color-mix(in srgb, var(--text) 45%, transparent); border-radius: 3px; color: var(--text); font-size: 11px; line-height: 16px; }
	.tags { display: flex; flex-wrap: nowrap; overflow: hidden; white-space: nowrap; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--text); }
	.tags i { color: var(--text-muted); font-style: normal; font-size: 9px; }
	@media (prefers-reduced-motion: reduce) {
		.shown .panel { animation: none; opacity: 1; transform: none; }
		.name, .scrim { transition: opacity 200ms linear; }
		.name.playing { transform: none; }
	}
</style>
