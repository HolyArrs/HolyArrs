<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { sized } from '../tmdb-image.ts';
	import { shelfNear, whenLoaded } from '../lazy-art.ts';
	import { press, viewTransitionName } from '../motion.ts';
	import HoverPreview from './HoverPreview.svelte';
	import CardBadges, { type CardOwned } from './CardBadges.svelte';
	import { freshLabel, shownFresh, typeWord, type Fresh } from '../fresh.ts';
	import { prefetchTrailer, watchTrailer } from '../autoplay.svelte.ts';
	import { dwell, finePointer, HOVER_DELAY } from '../trailer.ts';
	import GrowCard from './GrowCard.svelte';
	import { richMetadata } from './TitleHero.svelte';
	import { api } from '../api.ts';
	import { GROW_DELAY, logoQueue } from '../grow.ts';
	import { cardTemplate, type CardArtPick } from '../card-art.ts';

	// 16:9 card for episodes and downloads: progress line along the art's
	// bottom edge, overlay badges, title and a meta line below.
	let {
		title,
		href,
		image,
		subtitle,
		meta,
		badges = [],
		status,
		statusLabel,
		progress,
		vtName,
		width,
		kind,
		action,
		statusTestid,
		badgesAt = 'top',
		preview,
		fresh,
		owned,
		typed = false,
		row,
		cover = false,
		logo,
		growAction,
		onmore,
		big = false,
		art: pick
	}: {
		/** Title cards: the server's picture pick (card-art.ts). */
		art?: CardArtPick;
		/** The server's one freshness strip (bottom centre of the art). */
		fresh?: Fresh | null;
		/** Put the type (Movie, Show) first on the metadata line. */
		typed?: boolean;
		/** The row type, so the card never repeats the row's meaning. */
		row?: string;
		/** Solid round library status, top-right, with its aria-label and tooltip. */
		owned?: CardOwned | null;
		/** Solid type pill top-left (Explore's All). */
		title: string;
		href?: string;
		image?: string | null;
		subtitle?: string;
		meta?: string;
		badges?: string[];
		status?: StatusKind;
		statusLabel?: string;
		/** 0–100; draws the line and announces the value. */
		progress?: number;
		vtName?: string;
		width?: string;
		/** What it is, for the artwork fallback's icon. */
		kind?: 'movie' | 'show' | 'artist' | 'episode';
		action?: Snippet;
		/** Test id on the status pill. */
		statusTestid?: string;
		/** Where the badges sit on the art. */
		badgesAt?: 'top' | 'bottom';
		/** Desktop: after a short hover or keyboard focus, open the expanded card (trailer, genres). */
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number } | null;
		/**
		 * Title cards (Explore and Home rows of movies and shows): the logo or
		 * name sits over the picture, nothing below, and a hover grows the
		 * card itself in place (GrowCard) instead of opening a hover card.
		 */
		cover?: boolean;
		/** The title logo (transparent PNG); else one is looked up as the card comes into view. */
		logo?: string | null;
		/** The grown card's one-tap Ask/Add. */
		growAction?: Snippet;
		/** Explore: the grown card's ⌄ opens the More info panel (from the given rect). */
		onmore?: (from: DOMRect) => void;
		/** Big 4 shelves: a larger card (the row sets the width; the name grows with it). */
		big?: boolean;
	} = $props();
	// The expanded card: a hover card (episodes, downloads) or the card grown in place (cover).
	let root = $state<HTMLElement>();
	let artEl = $state<HTMLElement>();
	let hit = $state<HTMLElement>();
	/** Opened by keyboard focus (not a hover): the grown card takes the focus. */
	let byKey = $state(false);
	let open = $state(false);
	/** The grown card is shrinking back onto this card. */
	let leaving = $state(false);
	// A card is a title card or not for its whole life: the delays are read once.
	// svelte-ignore state_referenced_locally
	const opener = dwell(cover ? GROW_DELAY : HOVER_DELAY, () => {
		if (dragging()) return;
		leaving = false;
		open = true;
	});
	// svelte-ignore state_referenced_locally
	const closer = dwell(cover ? 60 : 140, () => (cover ? (leaving = true) : (open = false)));
	const dragging = () => !!root?.closest('.dragging');
	function enter() {
		if (!preview || !finePointer() || dragging()) return;
		closer.cancel();
		if (open && leaving) leaving = false;
		if (!open) {
			opener.start();
			prefetchTrailer(preview.kind, preview.tmdb);
		}
	}
	function leave() {
		opener.cancel();
		if (open) closer.start();
	}
	function shut() {
		opener.cancel();
		closer.cancel();
		open = false;
		leaving = false;
	}
	$effect(() => () => shut());
	/** Tab out of the grown card: on to the next card in the row, else the next tab stop after this card; Shift+Tab back to this card. */
	function tabOut(back: boolean) {
		shut();
		if (back || !root) return hit?.focus();
		const r = root;
		const next = r.nextElementSibling?.querySelector<HTMLElement>('a[href], button:not([disabled])');
		if (next) return next.focus();
		const after = [...document.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, select, textarea, [tabindex]')].find(
			(el) => el.tabIndex >= 0 && !r.contains(el) && !el.closest('.ui-grow') && el.getClientRects().length > 0 && !!(r.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING)
		);
		(after ?? hit)?.focus();
	}
	// In view for a while: the trailer address is fetched ahead of any hover.
	$effect(() => {
		if (root && preview) return watchTrailer(root, preview.kind, preview.tmdb);
	});
	// Cover cards: the logo, looked up gently (a few at a time) once the card is on screen.
	let foundLogo = $state<string | null>(null);
	$effect(() => {
		if (!cover || logo || pick || !preview || !root || typeof IntersectionObserver === 'undefined') return;
		const p = preview;
		let done = false;
		const io = new IntersectionObserver(([e]) => {
			if (!e.isIntersecting || done) return;
			done = true;
			io.disconnect();
			void logoQueue(() => richMetadata(api.get, p.kind, p.tmdb)).then((m) => (foundLogo = m?.logo ?? null));
		}, { rootMargin: '0px 200px' });
		io.observe(root);
		return () => io.disconnect();
	});
	let logoFailed = $state(false);
	const shownLogo = $derived(logoFailed ? null : sized(logo ?? foundLogo, 'poster') || null);
	let failed = $state(false);
	/** The server's picked picture failed to load (the template falls back). */
	let pickFailed = $state(false);
	// Title cards follow the card template: titled art, else logo, else name (all on the art).
	const tpl = $derived(cover ? cardTemplate(pick, { backdrop: image, logo: logo ?? foundLogo }, { art: pickFailed, logo: logoFailed }) : null);
	const src = $derived(tpl ? tpl.image : (image ?? null));
	const art = $derived(src ? sized(src, 'backdrop') : null);
	const coverLogo = $derived(tpl?.variant === 'logo' && tpl.logo ? sized(tpl.logo, 'poster') || null : null);
	// The grown card lays the logo (titled art keeps it too) or the name over the trailer.
	const growLogo = $derived(tpl ? (coverLogo ?? (tpl.variant === 'titled' && pick?.logo && !logoFailed ? sized(pick.logo, 'poster') || null : null)) : shownLogo);
	function artError() {
		if (cover && pick?.art && src === pick.art && !pickFailed) {
			pickFailed = true;
			loaded = false;
		} else failed = true;
	}
	// The name tile shows until the picture has loaded; in a row the picture
	// waits until the row is near the screen.
	let loaded = $state(false);
	const near = shelfNear();
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const pct = $derived(progress === undefined ? undefined : Math.max(0, Math.min(100, progress)));
	// The progress line owns the bottom edge; the strip steps aside for it.
	// The type leads the metadata line ("Show · 2024"); never twice.
	const typedWord = $derived(typed ? typeWord(kind) : '');
	const sub = $derived(typedWord && subtitle === typedWord ? undefined : subtitle);
	const strip = $derived(pct === undefined ? shownFresh(fresh, { row }) : null);
	const marked = $derived(!!(strip || owned));
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-wide" class:cover class:big class:grown={cover && open} style:width bind:this={root} onpointerenter={() => ((byKey = false), enter())} onpointerleave={leave} onfocusin={(e) => (e.target as HTMLElement)?.matches?.(':focus-visible') && ((byKey = true), enter())} onfocusout={(e) => !root?.contains(e.relatedTarget as Node) && leave()} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} role={href ? undefined : 'group'} bind:this={hit} use:press={'card'} aria-label={cover ? [title, typedWord, sub, strip && freshLabel(strip)].filter(Boolean).join(', ') : undefined} title={cover ? title : undefined}>
		<span class="art" class:low={badgesAt === 'bottom' && (badges.length || status)} use:viewTransitionName={vtName} bind:this={artEl}>
			{#if !src || failed || !loaded}<span class="ph" aria-hidden="true">{#if !cover}<span class="ph-ic"><Icon name={kindIcon} size={20} /></span><span class="ph-t">{title}</span>{/if}</span>{/if}
			{#if src && !failed && near()}{#key art}<img src={art} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={artError} />{/key}{/if}
			{#if tpl && (tpl.variant !== 'titled' || !loaded)}
				<span class="scrim" aria-hidden="true"></span>
				<span class="name" class:lifted={!!strip && strip.kind !== 'top10'} aria-hidden="true">
					{#if coverLogo}<img class="logo" src={coverLogo} alt="" onerror={() => (logoFailed = true)} />{:else}<span class="word">{title}</span>{/if}
				</span>
			{/if}
			{#if badges.length || status}
				<span class="over" class:bottom={badgesAt === 'bottom'} class:pushed={badgesAt === 'top' && (!!owned || strip?.kind === 'top10')} class:lifted={badgesAt === 'bottom' && !!strip && strip.kind !== 'top10'}>
					{#if status}<StatusPill kind={status} label={statusLabel} percent={pct} on="art" testid={statusTestid} />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			{#if marked}<CardBadges fresh={strip} owned={owned ?? null} />{/if}
			{#if pct !== undefined}
				<span class="line" role="progressbar" aria-label="{title} progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} class:done={pct >= 100}><i style="width: {pct}%"></i></span>
			{/if}
		</span>
		{#if !cover}
			<span class="t" title={title}>{title}</span>
			{#if typedWord || sub}<span class="s">{#if typedWord}<span class="ty">{typedWord}</span>{#if sub}<span class="dot" aria-hidden="true">·</span>{/if}{/if}{sub ?? ''}</span>{/if}
			{#if meta}<span class="m">{meta}</span>{/if}
		{:else}
			<!-- Title cards: the title is on the art and the name sits under it too, then the muted meta line. -->
			<span class="t under" title={title} aria-hidden="true">{title}</span>
			{#if typedWord || sub}<span class="s" aria-hidden="true">{#if typedWord}{typedWord}{#if sub}<span class="dot">·</span>{/if}{/if}{sub ?? ''}</span>{/if}
		{/if}
	</svelte:element>
	{#if action}<span class="act" class:below={!!owned}>{@render action()}</span>{/if}
</div>

{#snippet wideBadges()}
	<CardBadges fresh={strip} owned={owned ?? null} />
{/snippet}

{#if open && preview && cover && artEl}
	<GrowCard anchor={artEl} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} image={loaded && !failed ? art : null} logo={growLogo} lifted={!!strip && strip.kind !== 'top10'} {leaving} badge={marked ? wideBadges : undefined} action={growAction} followId={preview.titleId} onmore={onmore ? (r) => { shut(); onmore(r); } : undefined} onenter={() => { closer.cancel(); leaving = false; }} onleave={leave} onclose={(back) => { shut(); if (back) hit?.focus(); }} focusIn={byKey} ontab={tabOut} returnTo={hit} ongone={() => leaving && shut()} />
{:else if open && preview && root}
	<HoverPreview anchor={root} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} {image} badge={marked ? wideBadges : undefined} onenter={() => closer.cancel()} onleave={leave} onclose={shut} />
{/if}

<style>
	.ui-wide { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--wide-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img:not(.logo) { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) ease, transform var(--dur-slow) var(--ease-out); }
	img.in:not(.logo) { opacity: 1; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img, a.hit:hover .ph { transform: scale(1.03); }
	}
	.ph { transition: transform var(--dur-slow) var(--ease-out); }
	.ph { position: absolute; inset: 0; overflow: hidden; min-width: 0; display: flex; flex-direction: column; justify-content: flex-end; gap: 10px; padding: 14px; color: var(--text); background: radial-gradient(120% 90% at 0% 0%, var(--accent-soft), transparent 60%), linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.ph-ic { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); color: var(--text-muted); box-shadow: var(--shadow-card); }
	.ph-t { max-width: 100%; min-width: 0; font: var(--t-headline); letter-spacing: -0.01em; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.over { position: absolute; left: 8px; top: 8px; right: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.low .ph { padding-bottom: 40px; }
	.over.bottom { top: auto; bottom: 8px; }
	.over.pushed { top: 42px; }
	.over.lifted { bottom: 30px; }
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 35%); }
	.line i { display: block; height: 100%; background: var(--accent); transition: width var(--dur-slow) var(--ease-out); }
	.line.done i { background: var(--ok); }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s .dot { opacity: 0.7; margin: 0 0.3em; }
	.t.under { display: block; padding-top: 8px; }
	.s, .m { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.act { position: absolute; right: 8px; top: 8px; }
	/* Under the status circle, never on it. */
	.act.below { top: 42px; }
	/* Title cards: the logo or name over a bottom scrim, nothing below; the grown card takes the card's place. */
	.cover .art { margin-bottom: 0; }
	.big .art { border-radius: var(--r-panel); }
	.cover.grown .art { visibility: hidden; }
	.scrim { position: absolute; inset: auto 0 0; height: 60%; background: linear-gradient(to top, rgb(0 0 0 / 0.66), rgb(0 0 0 / 0.18) 55%, transparent); pointer-events: none; }
	/* One fixed box bottom-left for the logo or the name, the same on every card:
	   6% in from the left, 40% of the width, about a quarter of the height. Wide
	   logos meet the width, tall logos meet the height; both sit bottom-left. */
	.name { position: absolute; left: 6%; bottom: 9%; z-index: 1; width: 40%; height: 13cqw; display: flex; align-items: flex-end; justify-content: flex-start; pointer-events: none; }
	.name.lifted { bottom: 20%; }
	.word { font: 700 clamp(13px, 5.2cqw, 24px)/1.05 var(--font-display); letter-spacing: -0.02em; color: #fff; text-shadow: 0 2px 10px rgb(0 0 0 / 0.55); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	/* Status mark, strip and Top 10 tag: 22px under 280px of card, 26px above (whole pixels). */
	.cover .art :global(.mark) { width: 22px; height: 22px; }
	.cover .art :global(.mark svg) { width: 13px; height: 13px; }
	.cover .art :global(.mark.downloading svg) { width: 18px; height: 18px; }
	.cover .art :global(.fresh) { height: 14px; font-size: 10px; }
	.cover .art :global(.top10) { width: 18px; height: 22px; font-size: 5px; }
	.cover .art :global(.top10 b) { font-size: 10px; }
	@container art (min-width: 280px) {
		.cover .art :global(.mark) { width: 26px; height: 26px; }
		.cover .art :global(.mark svg) { width: 15px; height: 15px; }
		.cover .art :global(.mark.downloading svg) { width: 21px; height: 21px; }
		.cover .art :global(.fresh) { height: 16px; font-size: 10px; }
		.cover .art :global(.top10) { width: 21px; height: 26px; font-size: 6px; }
		.cover .art :global(.top10 b) { font-size: 12px; }
	}
	.logo { display: block; max-width: 100%; max-height: 100%; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.5)); }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.cover a.hit:hover .art { transform: none; }
		.cover a.hit:hover img { transform: none; }
	}
</style>
