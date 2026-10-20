<script lang="ts">
	import { metaLine } from '../fresh.ts';
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { api } from '../api.ts';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import Icon from '../Icon.svelte';
	import { followHint, followLabel, setFollow } from '../follow.ts';
	import { sized } from '../tmdb-image.ts';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '../autoplay.svelte.ts';
	import { fromHandoff, openTheater } from '../theater.svelte.ts';
	import { flipTransform, growPlacement, GROW_MS, GROW_SCALE, GROW_WIDTH, SHRINK_MS, TRAILER_AFTER, type Growth, type Rect } from '../grow.ts';

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
		onmore,
		focusIn = false,
		ontab,
		returnTo,
		followId
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
		/** Esc, scroll or resize: close now (back: focus was inside, return it to the card). */
		onclose: (back?: boolean) => void;
		/** Opened from the keyboard: focus moves to the first button. */
		focusIn?: boolean;
		/** Tab past the last button (or Shift+Tab before the first): back is true for Shift+Tab. */
		ontab?: (back: boolean) => void;
		/** Where focus goes when the full-screen trailer opened from here closes. */
		returnTo?: HTMLElement;
		/** The shrink finished; the card shows again. */
		ongone: () => void;
		/** The ⌄ opens the More info panel here (Explore) instead of following href. */
		onmore?: (from: DOMRect) => void;
		/** A show in the Library: the bell follows it. */
		followId?: number;
	} = $props();

	let following = $state(false);
	let followBusy = $state(false);
	$effect(() => {
		const id = followId;
		if (!id || kind !== 'show') return;
		api.get<{ following: boolean }>(`/titles/${id}/follow`).then((r) => (following = r.following), () => {});
	});
	async function flipFollow() {
		if (!followId) return;
		followBusy = true;
		try {
			following = await setFollow(api, followId, !following);
		} catch {
			/* the bell stays as it was */
		} finally {
			followBusy = false;
		}
	}

	let card = $state<HTMLElement>();
	let g = $state<Growth | null>(null);
	let meta = $state<RichMeta | null>(null);
	let sources = $state<TrailerSource[]>([]);
	let trailerOk = $state(false);
	let playing = $state(false);
	let shown = $state(false);
	let logoFailed = $state(false);
	let anim: Animation | null = null;
	const ctl = new AbortController();
	const reduce = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;

	/**
	 * The rect the grow starts from: the card's own picture when it is 16:9;
	 * a poster or square tile stands in for a 16:9 frame centred on it, as
	 * tall as the tile, so every card opens the same wide preview (Netflix).
	 */
	function from(): Rect {
		const a = anchor.getBoundingClientRect();
		if (a.height <= 0 || Math.abs(a.width / a.height - 16 / 9) < 0.05) return a;
		const width = (a.height * 16) / 9 / GROW_SCALE;
		const height = (width * 9) / 16;
		return { left: a.left + a.width / 2 - width / 2, top: a.top + a.height / 2 - height / 2, width, height };
	}

	function measure(): Growth | null {
		if (!card) return null;
		const a = from();
		const panel = card.querySelector<HTMLElement>('.panel')?.offsetHeight ?? 0;
		const row = (anchor.closest('.row') ?? anchor.closest('main'))?.getBoundingClientRect();
		return growPlacement({ left: a.left, top: a.top, width: a.width, height: a.height }, panel, { width: document.documentElement.clientWidth, height: window.innerHeight }, row ? { left: row.left, right: row.right } : undefined, { min: GROW_WIDTH, max: GROW_WIDTH });
	}

	/** The clip that shows only the picture (the start and end of the grow). */
	function clipTo(at: Growth) {
		const panel = Math.max(0, (card?.offsetHeight ?? 0) - at.mediaHeight);
		return `inset(0 0 ${panel}px 0 round var(--r-card))`;
	}

	/** will-change only while it moves, so the card at rest is drawn crisp at its real size. */
	function settle(a: Animation, done?: () => void) {
		a.onfinish = () => {
			if (anim === a) {
				a.cancel();
				anim = null;
				if (card) card.style.willChange = '';
			}
			done?.();
		};
	}

	function grow() {
		const next = measure();
		if (!next || !card) return;
		g = next;
		shown = true;
		if (reduce()) return;
		anim?.cancel();
		card.style.willChange = 'transform';
		anim = card.animate(
			[
				// Only the picture at first: the panel below is clipped away, then opens as the card grows.
				{ transform: flipTransform(next.from), clipPath: clipTo(next) },
				{ transform: 'none', clipPath: 'inset(0 0 0 0 round var(--r-card))' }
			],
			{ duration: GROW_MS, easing: 'cubic-bezier(.2, .8, .2, 1)', fill: 'both' }
		);
		settle(anim);
	}

	/** The full-screen player took over from this card's spot: the card goes at once, under it, instead of shrinking back. */
	function hideNow() {
		anim?.cancel();
		anim = null;
		if (card) card.style.visibility = 'hidden';
	}

	function shrink() {
		if (card?.style.visibility === 'hidden') return ongone();
		if (!card || !g || reduce()) return ongone();
		const a = from();
		const back = { x: a.left - g.left, y: a.top - g.top, scale: a.width / g.width };
		const now = anim && anim.playState === 'running' ? getComputedStyle(card).transform : 'none';
		anim?.cancel();
		card.style.willChange = 'transform';
		anim = card.animate([{ transform: now, clipPath: 'inset(0 0 0 0 round var(--r-card))' }, { transform: flipTransform(back), clipPath: clipTo(g) }], { duration: SHRINK_MS, easing: 'cubic-bezier(.4, 0, .6, 1)', fill: 'both' });
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

	const focusables = () => (card ? [...card.querySelectorAll<HTMLElement>('a[href]:not([tabindex="-1"]), button:not([disabled])')] : []);
	function keydown(e: KeyboardEvent) {
		if (e.key !== 'Tab' || !ontab) return;
		const list = focusables();
		const i = list.indexOf(document.activeElement as HTMLElement);
		if (e.shiftKey ? i <= 0 : i === list.length - 1) {
			e.preventDefault();
			ontab(e.shiftKey);
		}
	}
	function focusout(e: FocusEvent) {
		const to = e.relatedTarget as Node | null;
		if (to && (card?.contains(to) || anchor.closest('.ui-wide, .ui-poster')?.contains(to))) return;
		onleave?.();
	}

	onMount(() => {
		grow();
		// The panel's buttons exist from the start (it is clipped, not absent): focus the first one.
		if (focusIn) requestAnimationFrame(() => card?.querySelector<HTMLElement>('.buttons a[href], .buttons button:not([disabled])')?.focus({ preventScroll: true }));
		const close = () => onclose();
		window.addEventListener('scroll', close, { passive: true, capture: true });
		window.addEventListener('resize', close);
		// The player is put in at once, hidden until a picture plays (it fades
		// in over the end of the grow); the address was prefetched in view or
		// at hover start, so the trailer does not wait on the details.
		const later = setTimeout(() => (trailerOk = true), TRAILER_AFTER);
		const metaP = richMetadata(api.get, kind, tmdb).then((m) => (meta = m));
		void (async () => {
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			const first = await trailerSources(kind, tmdb, undefined, ctl.signal);
			if (ctl.signal.aborted) return;
			if (first.some((x) => x.kind !== 'embed')) return void (sources = first);
			// No direct stream: the embed needs the title's videos from the details.
			const m = await metaP;
			if (!ctl.signal.aborted) sources = await trailerSources(kind, tmdb, m?.videos, ctl.signal);
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

	// The sharp picture for the grown card's real size (w1280), over the card's own w300 or w780 until it loads.
	let bigIn = $state(false);
	// Poster cards pass no picture: the title's backdrop fills the 16:9 frame.
	const pic = $derived(image ?? (meta?.backdrop ? sized(meta.backdrop, 'backdrop') || null : null));
	const big = $derived(pic && /\/artwork\/tmdb\/w(?:300|780)\//.test(pic) ? pic.replace(/\/w(?:300|780)\//, '/w1280/') : null);
	const shownLogo = $derived(logoFailed ? null : logo ? logo : meta?.logo ? sized(meta.logo, 'poster') : null);
	function runtime(m?: number) {
		if (!m) return '';
		const h = Math.floor(m / 60);
		return h ? `${h}h ${m % 60}m` : `${m}m`;
	}
	// Netflix's line: Movie · R · 1h 49m · HD/4K (the quality only when the Library has it).
	// TMDB's score, small, after the age rating.
	const stars = $derived(meta?.score ? `★ ${(Math.round(meta.score * 10) / 10).toFixed(1)}` : undefined);
	const facts = $derived(metaLine(kind, [meta?.certification, stars, kind === 'movie' ? runtime(meta?.runtime) : undefined, meta?.quality]));
	// Mood words from the server (TMDB keywords mapped to our moods), else genres.
	const tags = $derived((meta?.moods?.length ? meta.moods : (meta?.genres ?? [])).slice(0, 3));
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose(!!card?.contains(document.activeElement))} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
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
	style:width="{g?.width ?? GROW_WIDTH}px"
	onpointerenter={onenter}
	onpointerleave={onleave}
	onfocusin={onenter}
	onfocusout={focusout}
	onkeydown={keydown}
>
	<a class="media" href={href} tabindex="-1">
		{#if pic}<img src={pic} alt="" decoding="async" />{/if}
		{#if big}<img src={big} alt="" decoding="async" class="big" class:in={bigIn} onload={() => (bigIn = true)} />{/if}
		{#if trailerOk && sources.length}
			<TrailerPlayer {sources} {title} repeat onplaying={() => (playing = true)} onfail={() => (playing = false)} onended={() => (playing = false)} onexpand={(o) => { openTheater({ kind, tmdb, title, ...fromHandoff(o), from: 'hover', returnTo }); hideNow(); onclose(); }} />
		{/if}
		<span class="scrim" class:playing aria-hidden="true"></span>
		<!-- The logo, else the name, stays over the trailer (card template). -->
		{#if shownLogo}<span class="name" class:lifted aria-hidden="true"><img class="logo" src={shownLogo} alt="" onerror={() => (logoFailed = true)} /></span>
		{:else}<span class="name" class:lifted aria-hidden="true"><span class="word">{title}</span></span>{/if}
		{#if badge && g}<span class="badge" class:playing>{@render badge()}</span>{/if}
	</a>
	<div class="panel">
		<div class="buttons">
			{#if action}{@render action()}{/if}
			{#if followId && kind === 'show'}<button type="button" class="more bell" class:on={following} aria-pressed={following} aria-label="{followLabel(following)} {title}" title={followHint('show', following)} disabled={followBusy} onclick={flipFollow} data-testid="grow-follow"><Icon name="bell" size={18} /></button>{/if}
			{#if onmore}<button type="button" class="more" aria-label="More info about {title}" title="More info" aria-haspopup="dialog" onclick={() => card && onmore(card.getBoundingClientRect())}><Icon name="chevronDown" size={20} /></button>
			{:else if href}<a class="more" {href} aria-label="More about {title}" title="More info"><Icon name="chevronDown" size={20} /></a>{/if}
		</div>
		<p class="title" title={title}>{title}</p>
		<!-- Both lines always take their room, so the panel's height is known before the facts arrive. -->
		<p class="facts">
			{#each facts as f, i (f)}{#if i}<i aria-hidden="true">·</i>{/if}<span class:rating={f === meta?.certification} class:q={f === meta?.quality} class:stars={f === stars}>{f}</span>{/each}
		</p>
		<p class="tags">{#each tags as t, i (t)}{#if i}<i aria-hidden="true">·</i>{/if}<span>{t}</span>{/each}</p>
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
	}
	.ui-grow.shown { visibility: visible; }
	.media { position: relative; display: block; container-type: inline-size; aspect-ratio: var(--wide-ratio, 16 / 9); background: var(--surface-2); overflow: hidden; }
	.media > img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.media > img.big { opacity: 0; transition: opacity 200ms ease; }
	.media > img.big.in { opacity: 1; }
	.scrim { position: absolute; inset: auto 0 0; height: 55%; background: linear-gradient(to top, rgb(0 0 0 / 0.62), transparent); pointer-events: none; transition: opacity 600ms ease; }
	.name { position: absolute; left: 6%; right: 30%; bottom: 13%; z-index: 2; display: flex; align-items: flex-end; pointer-events: none; transition: opacity 300ms ease-out; }
	.name.lifted { bottom: 26%; }
	.badge.playing { opacity: 0; }
	.word { font: 700 clamp(18px, 7cqw, 40px)/1.05 var(--font-display); letter-spacing: -0.02em; color: #fff; text-shadow: 0 2px 10px rgb(0 0 0 / 0.55); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.badge { transition: opacity 300ms ease-out; }
	.scrim.playing { opacity: 0.5; }
	.logo { max-width: 100%; max-height: 22cqw; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.5)); }
	/* The badges at their native size on the grown picture (no scale). */
	.badge { position: absolute; inset: 0; z-index: 3; container: art / inline-size; pointer-events: none; }
	.panel { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px 14px; opacity: 0; transform: translateY(-6px); }
	.shown .panel { animation: panel-in 160ms cubic-bezier(.2, .8, .2, 1) 120ms both; }
	@keyframes panel-in { to { opacity: 1; transform: none; } }
	.buttons { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.buttons :global(.ui-ibtn.glass) { background: var(--text); color: var(--surface-1); box-shadow: none; }
	.buttons :global(.ui-ibtn.glass:hover) { background: color-mix(in srgb, var(--text) 85%, var(--surface-1)); }
	.more { flex: none; width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; color: var(--text); box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--text) 40%, transparent); transition: box-shadow var(--dur-fast) ease, background-color var(--dur-fast) ease; }
	.more:hover { box-shadow: inset 0 0 0 2px var(--text); background: var(--surface-2); }
	.more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.more.bell.on { background: var(--text); color: var(--bg); box-shadow: none; }
	.title { margin: 2px 0 -4px; font: var(--t-headline); letter-spacing: -0.01em; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.facts, .tags { min-height: 18px; }
	.facts { display: flex; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.rating { padding: 0 5px; border: 1px solid color-mix(in srgb, var(--text) 45%, transparent); border-radius: 3px; color: var(--text); font-size: 11px; line-height: 16px; }
	.stars { color: var(--text); font-weight: 600; font-variant-numeric: tabular-nums; }
	.tags { display: flex; flex-wrap: nowrap; overflow: hidden; white-space: nowrap; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--text); }
	.tags i { color: var(--text-muted); font-style: normal; font-size: 9px; }
	@media (prefers-reduced-motion: reduce) {
		.shown .panel { animation: none; opacity: 1; transform: none; }
		.name, .scrim { transition: opacity 200ms linear; }
			}
</style>
