<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import CardBadges from './CardBadges.svelte';
	import { shownFresh, typeWord, type Fresh } from '../fresh.ts';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import { press, viewTransitionName } from '../motion.ts';
	import { sized } from '../tmdb-image.ts';
	import { pictureOf } from '../pictures.ts';
	import { shelfNear, whenLoaded } from '../lazy-art.ts';
	import GrowCard from './GrowCard.svelte';
	import { prefetchTrailer, watchTrailer } from '../autoplay.svelte.ts';
	import { dwell, finePointer, scrollHold } from '../trailer.ts';
	import { GROW_DELAY } from '../grow.ts';

	// 2:3 poster with title below. Lifts on hover; artwork zooms inside its frame.
	// `action` is an optional round button (Add / Ask) over the art corner.
	let {
		title,
		href,
		image,
		subtitle,
		badges = [],
		status,
		statusLabel,
		progress,
		vtName,
		width,
		kind,
		typed = false,
		row,
		owned,
		actionOnHover = false,
		shape = 'poster',
		preview,
		fresh,
		music = false,
		action,
		growAction,
		followTmdb = false,
		color = null,
		eager = false,
		high = false,
		fallback
	}: {
		/** Music: the next picture when image keeps failing (an album's artist photo). */
		fallback?: string | null;
		/** The picture's main colour (#rrggbb); music cards read it from the picture's record. */
		color?: string | null;
		/** Above the fold (the first row): load at once. */
		eager?: boolean;
		/** Ask for the picture at high priority (the first screen). */
		high?: boolean;
		title: string;
		href?: string;
		image?: string | null;
		subtitle?: string;
		badges?: string[];
		status?: StatusKind;
		statusLabel?: string;
		progress?: number;
		/** view-transition-name, e.g. `title-art-<id>`, so the poster grows into the hero. */
		vtName?: string;
		width?: string;
		/** What it is, for the artwork fallback's icon. */
		kind?: 'movie' | 'show' | 'artist' | 'episode';
		/** Status pill top-right (glyph only on narrow cards), with its aria-label and tooltip. */
		owned?: { state: 'available' | 'partial' | 'downloading' | 'requested' | 'declined'; label: string } | null;
		/** The round action shows on hover or focus only (always on touch screens). */
		actionOnHover?: boolean;
		/** Music: square for albums, round for artists (same width as a poster). */
		shape?: 'poster' | 'square' | 'round';
		/** Desktop: after a short hover or keyboard focus, open the expanded card (trailer, genres). */
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number } | null;
		/** The server's one freshness strip (bottom centre of the art). */
		fresh?: Fresh | null;
		/** Put the type (Movie, Show) first on the metadata line. */
		typed?: boolean;
		/** The row type, so the card never repeats the row's meaning. */
		row?: string;
		/** Music cards keep Recently Added and New Release only. */
		music?: boolean;
		action?: Snippet;
		/** The expanded card's first button when it differs from the poster's own (a ranked poster shows none). */
		growAction?: Snippet;
		/** The grown card's bell follows the title by TMDB id (a show, or a movie not out yet). */
		followTmdb?: boolean;
	} = $props();

	// The expanded card: pointer: fine only, after HOVER_DELAY; leaving both
	// the poster and the card (a short grace to cross the gap) closes it.
	// The same wide preview every row opens (GrowCard), centred over the poster.
	let root = $state<HTMLElement>();
	let artEl = $state<HTMLElement>();
	let hit = $state<HTMLElement>();
	/** Opened by keyboard focus: the preview takes the focus. */
	let byKey = $state(false);
	let open = $state(false);
	let leaving = $state(false);
	const opener = dwell(GROW_DELAY, () => {
		if (scrollHold()) return;
		leaving = false;
		open = true;
	});
	const closer = dwell(60, () => (leaving = true));
	// A hover-only action (fine pointers) is put in only while the card is hovered or
	// focused: a glass button on every card makes long rows slow to scroll.
	let hot = $state(false);
	const lazyAction = $derived(actionOnHover && finePointer());
	const showAction = $derived(!!action && (!lazyAction || hot));
	function enter() {
		hot = true;
		if (!preview || !finePointer() || scrollHold()) return;
		closer.cancel();
		if (open && leaving) leaving = false;
		if (!open) {
			opener.start();
			prefetchTrailer(preview.kind, preview.tmdb);
		}
	}
	function leave() {
		if (!root?.contains(document.activeElement)) hot = false;
		opener.cancel();
		if (open) closer.start();
	}
	function shut() {
		opener.cancel();
		closer.cancel();
		open = false;
		leaving = false;
	}
	/** Tab out of the preview: on to the next card, else back to this one (Shift+Tab). */
	function tabOut(back: boolean) {
		shut();
		// Back on this card: it stays closed (no reopen from the focus).
		if (back || !root) return void (hit?.focus(), opener.cancel());
		// The next card's link in the row (a ranked card sits in a wrapper, so not always the next sibling).
		const hits = [...(root.closest('.row')?.querySelectorAll<HTMLElement>('.ui-poster a.hit, .ui-wide a.hit') ?? [])];
		const i = hit ? hits.indexOf(hit) : -1;
		const r = root;
		const next =
			(i >= 0 ? hits[i + 1] : undefined) ??
			[...document.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, select, textarea, [tabindex]')].find(
				(el) => el.tabIndex >= 0 && !r.closest('.row')?.contains(el) && !el.closest('.ui-grow') && el.getClientRects().length > 0 && !!(r.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING)
			);
		(next ?? hit)?.focus();
	}
	function focusIn(e: FocusEvent) {
		hot = true;
		if ((e.target as HTMLElement)?.matches?.(':focus-visible')) {
			byKey = true;
			enter();
		}
	}
	function focusOut(e: FocusEvent) {
		if (!root?.contains(e.relatedTarget as Node)) {
			if (!root?.matches(':hover')) hot = false;
			leave();
		}
	}
	$effect(() => () => shut());
	// In view for a while: the trailer address is fetched ahead of any hover.
	$effect(() => {
		if (root && preview) return watchTrailer(root, preview.kind, preview.tmdb);
	});
	// The type leads the metadata line ("Show · 2024"); never twice.
	const typedWord = $derived(typed ? typeWord(kind) : '');
	const sub = $derived(typedWord && subtitle === typedWord ? undefined : subtitle);
	const strip = $derived(progress === undefined ? shownFresh(fresh, { music, row }) : null);
	let failed = $state(false);
	// The name tile shows until the picture has loaded; in a row the picture
	// waits until the row is near the screen.
	let loaded = $state(false);
	const near = shelfNear();
	// The card's rendered width picks the picture size; it is watched (not
	// read once), so a card first laid out at width 0 still gets its picture.
	let cw = $state(0);
	// A picture that fails (a 404: the server has not stored it yet) shows
	// the fallback picture at once, then the placeholder; no retry timer.
	let second = $state(false);
	function broken() {
		if (music && fallback && !second) second = true;
		else failed = true;
	}
	const pic = $derived(second ? (fallback ?? null) : sized(image, 'poster', cw));
	const shownPic = $derived(pic);
	// Asked once the card has its width, so the slot size is picked once
	// (never a first default size, then the measured one).
	const asking = $derived(!!image && !failed && near() && cw > 0);
	// The box and colour come before the picture: the record's main colour
	// (music) under the initials, so the picture never fades in over grey.
	const tint = $derived(color ?? pictureOf(pic)?.color ?? null);
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-poster" class:square={shape === 'square'} class:round={shape === 'round'} style:width bind:this={root} bind:clientWidth={cw} onpointerenter={() => ((byKey = false), enter())} onpointermove={(e) => (e.movementX || e.movementY) && !open && !opener.pending && ((byKey = false), enter())} onpointerleave={leave} onfocusin={focusIn} onfocusout={focusOut} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} bind:this={hit} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName} bind:this={artEl} style:background-color={tint}>
			{#if !image || failed || !loaded}
				<!-- The shimmer only while a picture is really on its way; else the still initials. -->
				<ArtPlaceholder name={title} shape={shape === 'round' ? 'circle' : shape} loading={asking && !loaded} color={tint} />
			{/if}
			{#if asking}
				{#key shownPic}
					<img src={shownPic} class:in={loaded} alt="" loading={eager ? 'eager' : 'lazy'} fetchpriority={high ? 'high' : 'auto'} decoding="async" use:whenLoaded={() => (loaded = true)} onerror={broken} />
				{/key}
			{/if}
			{#if badges.length || status}
				<span class="over">
					{#if status}<StatusPill kind={status} label={statusLabel} on="art" compact={shape === 'round'} />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			<CardBadges fresh={strip} {owned} size={shape === 'poster' ? 'md' : 'sm'} />
			{#if progress !== undefined}<span class="prog"><ProgressBar value={progress} label="{title} progress" size="sm" on="art" /></span>{/if}
		</span>
		<span class="t" title={title}>{title}</span>
		{#if typedWord || sub}<span class="s">{#if typedWord}<span class="ty">{typedWord}</span>{#if sub}<span class="dot" aria-hidden="true">·</span>{/if}{/if}{sub ?? ''}</span>{/if}
	</svelte:element>
	{#if action && showAction}<span class="act-layer" class:hover-only={actionOnHover} class:over-strip={!!strip}><span class="act">{@render action()}</span></span>{/if}
</div>

{#snippet ownBadge()}
	<CardBadges fresh={strip} {owned} />
{/snippet}

{#if open && preview && artEl}
	<GrowCard anchor={artEl} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} image={null} {leaving} badge={owned || strip ? ownBadge : undefined} action={growAction ?? action} owned={owned ?? null} followId={preview.titleId} {followTmdb} onenter={() => { closer.cancel(); leaving = false; }} onleave={leave} onclose={(back) => { shut(); if (back) { hit?.focus(); opener.cancel(); } }} focusIn={byKey} ontab={tabOut} returnTo={hit} ongone={() => leaving && shut()} />
{/if}

<style>
	.ui-poster { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.square, .round { --poster-ratio: 1; }
	.round .art { border-radius: 50%; }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--poster-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 240ms ease-out, transform var(--dur-slow) var(--ease-out); }
	img.in { opacity: 1; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img { transform: scale(1.03); }
	}
	/* While the picture loads, the name tile carries the shared Silhouette sweep. */
	.over { position: absolute; left: 8px; right: 8px; top: 8px; display: flex; flex-wrap: nowrap; align-items: center; gap: 4px; }
	/* A circle clips corners: its status sits centred on the top edge, glyph only. */
	.round .over { justify-content: center; top: 6px; }
	.prog { position: absolute; left: 8px; right: 8px; bottom: 8px; }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s .dot { opacity: 0.7; margin: 0 0.3em; }
	.s { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.act-layer { position: absolute; inset: 0 0 auto; aspect-ratio: var(--poster-ratio); pointer-events: none; }
	.act { position: absolute; right: 8px; bottom: 8px; pointer-events: auto; }
	/* The round action sits above the freshness strip, never on it. */
	.act-layer.over-strip .act { bottom: 24px; }
	@media (hover: hover) and (pointer: fine) {
		.act-layer.hover-only .act { opacity: 0; transform: scale(0.9); transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
		.ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 1; transform: none; }
		@starting-style { .ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 0; transform: scale(0.9); } }
	}
</style>
