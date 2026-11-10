<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Poster tile with art, badges, status and an optional hover preview card.
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import CardBadges from './CardBadges.svelte';
	import GrowCard from './GrowCard.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import { shownFresh, typeWord, type Fresh } from './fresh.ts';
	import { shelfNear, whenLoaded } from './lazy-art.ts';
	import { GROW_DELAY } from './grow.ts';
	import { press, viewTransitionName } from '../core/motion.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { pictureOf } from '../core/pictures.ts';
	import { prefetchTrailer, watchTrailer } from '../core/trailer/autoplay.svelte.ts';
	import { dwell, finePointer, scrollHold } from '../core/trailer/trailer.ts';

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
		fallback,
		needsArt = false
	}: {
		needsArt?: boolean;
		fallback?: string | null;
		color?: string | null;
		eager?: boolean;
		high?: boolean;
		title: string;
		href?: string;
		image?: string | null;
		subtitle?: string;
		badges?: string[];
		status?: StatusKind;
		statusLabel?: string;
		progress?: number;
		vtName?: string;
		width?: string;
		kind?: 'movie' | 'show' | 'artist' | 'episode';
		owned?: { state: 'available' | 'partial' | 'downloading' | 'requested' | 'declined'; label: string } | null;
		actionOnHover?: boolean;
		shape?: 'poster' | 'square' | 'round';
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number } | null;
		fresh?: Fresh | null;
		typed?: boolean;
		row?: string;
		music?: boolean;
		action?: Snippet;
		growAction?: Snippet;
		followTmdb?: boolean;
	} = $props();

	const uid = $props.id();

	let root = $state<HTMLElement>();
	let artEl = $state<HTMLElement>();
	let hit = $state<HTMLElement>();
	let byKey = $state(false);
	let open = $state(false);
	let leaving = $state(false);
	let hot = $state(false);
	let failed = $state(false);
	let loaded = $state(false);
	let cw = $state(0);
	let second = $state(false);
	// Open waits for a pause; close waits a beat so moving between card and tile does not flicker.
	const opener = dwell(GROW_DELAY, () => {
		if (scrollHold()) return;
		leaving = false;
		open = true;
	});
	const closer = dwell(60, () => (leaving = true));
	const near = shelfNear();

	const lazyAction = $derived(actionOnHover && finePointer());
	const showAction = $derived(!!action && !(preview && finePointer()) && (!lazyAction || hot));
	const typedWord = $derived(typed ? typeWord(kind) : '');
	const sub = $derived(typedWord && subtitle === typedWord ? undefined : subtitle);
	const more = $derived([typedWord, sub, owned?.label, status ? statusLabel : ''].filter(Boolean).join(', '));
	const strip = $derived(progress === undefined ? shownFresh(fresh, { music, row }) : null);
	const pic = $derived(second ? (fallback ?? null) : sized(image, 'poster', cw));
	const shownPic = $derived(pic);
	const asking = $derived(!!image && !failed && near() && cw > 0);
	const tint = $derived(color ?? pictureOf(pic)?.color ?? null);
	const gone = $derived(needsArt && (!image || failed));

	$effect(() => () => shut());
	$effect(() => {
		if (root && preview) return watchTrailer(root, preview.kind, preview.tmdb);
	});

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
	// Tab from the preview goes to the next tile in the row, else the next focusable thing after the row.
	function tabOut(back: boolean) {
		shut();
		if (back || !root) return void (hit?.focus(), opener.cancel());
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
	// Only keyboard focus opens the preview; a mouse click should not.
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
	// Music tiles try the fallback art once before giving up.
	function broken() {
		if (music && fallback && !second) second = true;
		else failed = true;
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-poster" class:gone class:square={shape === 'square'} class:round={shape === 'round'} style:width bind:this={root} bind:clientWidth={cw} onpointerenter={() => ((byKey = false), enter())} onpointermove={(e) => (e.movementX || e.movementY) && !open && !opener.pending && ((byKey = false), enter())} onpointerleave={leave} onfocusin={focusIn} onfocusout={focusOut} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} aria-labelledby={href ? `${uid}-t` : undefined} aria-describedby={href && more ? `${uid}-d` : undefined} bind:this={hit} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName} bind:this={artEl} style:background-color={tint}>
			{#if !image || failed || !loaded}
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
			{#if progress !== undefined}<span class="prog"><ProgressBar value={progress} label={t('ui.posterCard.progress', { title })} size="sm" on="art" /></span>{/if}
		</span>
		<span class="t" id="{uid}-t" title={title}>{title}</span>
		{#if href && more}<span class="sr-only" id="{uid}-d">{more}</span>{/if}
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
	/* Hides the tile and its list item when art is required and missing. */
	.ui-poster.gone, :global(li:has(> .ui-poster.gone)) { display: none; }
	.ui-poster { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.square, .round { --poster-ratio: 1; }
	.round .art { border-radius: 50%; }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--poster-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	img.in { opacity: 1; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img { transform: scale(1.03); }
	}
	.over { position: absolute; left: 8px; right: 8px; top: 8px; display: flex; flex-wrap: nowrap; align-items: center; gap: 4px; }
	.round .over { justify-content: center; top: 6px; }
	.prog { position: absolute; left: 8px; right: 8px; bottom: 8px; }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s .dot { opacity: 0.7; margin: 0 0.3em; }
	.s { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* Overlay matches the art box so the action button sits on the poster corner. */
	.act-layer { position: absolute; inset: 0 0 auto; aspect-ratio: var(--poster-ratio); pointer-events: none; }
	.act { position: absolute; right: 8px; bottom: 8px; pointer-events: auto; }
	/* Lifts the button above the freshness strip. */
	.act-layer.over-strip .act { bottom: 24px; }
	@media (hover: hover) and (pointer: fine) {
		.act-layer.hover-only .act { opacity: 0; transform: scale(0.9); transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
		.ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 1; transform: none; }
		@starting-style { .ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 0; transform: scale(0.9); } }
	}
</style>
