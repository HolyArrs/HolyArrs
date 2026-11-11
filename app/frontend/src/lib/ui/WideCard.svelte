<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Wide poster card with cover art, hover grow preview and badges.
	import type { Snippet } from 'svelte';
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import Badge from './Badge.svelte';
	import CardBadges, { type CardOwned } from './CardBadges.svelte';
	import GrowCard from './GrowCard.svelte';
	import HoverPreview from './HoverPreview.svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { cardTemplate, type CardArtPick } from './card-art.ts';
	import { freshLabel, shownFresh, typeWord, type Fresh } from './fresh.ts';
	import { GROW_DELAY } from './grow.ts';
	import { shelfNear, whenLoaded } from './lazy-art.ts';
	import { press, viewTransitionName } from '../core/motion.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { prefetchTrailer, watchTrailer } from '../core/trailer/autoplay.svelte.ts';
	import { dwell, finePointer, HOVER_DELAY, scrollHold } from '../core/trailer/trailer.ts';

	let {
		title,
		href,
		image,
		poster,
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
		inset,
		art: pick,
		growContext = null,
		growService = null,
		followTmdb = false,
		needsArt = false
	}: {
		needsArt?: boolean;
		art?: CardArtPick;
		fresh?: Fresh | null;
		typed?: boolean;
		row?: string;
		owned?: CardOwned | null;
		title: string;
		href?: string;
		image?: string | null;
		poster?: string | null;
		subtitle?: string;
		meta?: string;
		badges?: string[];
		status?: StatusKind;
		statusLabel?: string;
		progress?: number;
		vtName?: string;
		width?: string;
		kind?: 'movie' | 'show' | 'artist' | 'episode';
		action?: Snippet;
		statusTestid?: string;
		badgesAt?: 'top' | 'bottom';
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number } | null;
		cover?: boolean;
		logo?: string | null;
		growAction?: Snippet;
		onmore?: (from: DOMRect) => void;
		big?: boolean;
		inset?: string | null;
		growContext?: { text: string; kind: 'house' | 'why' } | null;
		growService?: { name: string; logo?: string } | null;
		followTmdb?: boolean;
	} = $props();

	let root = $state<HTMLElement>();
	let artEl = $state<HTMLElement>();
	let hit = $state<HTMLElement>();
	let byKey = $state(false);
	let open = $state(false);
	let leaving = $state(false);
	let logoFailed = $state(false);
	let failed = $state(false);
	let pickFailed = $state(false);
	let tall = $state(false);
	let cw = $state(0);
	let loaded = $state(false);

	const shownLogo = $derived(logoFailed ? null : sized(logo ?? null, 'poster') || null);
	const tpl = $derived(cover ? cardTemplate(pick, { backdrop: image, logo: logo ?? null }, { art: pickFailed, logo: logoFailed }) : null);
	const src = $derived((tpl ? tpl.image : image) || poster || null);
	// A poster is used as the fill only when the card is small, so a tall image is not stretched wide.
	const posterFill = $derived(!!src && src === poster && src !== image && cw > 0 && cw * (globalThis.devicePixelRatio || 1) <= 500);
	const art = $derived(src ? sized(src, posterFill ? 'poster' : 'backdrop', cw) : null);
	const coverLogo = $derived(tpl?.variant === 'logo' && tpl.logo ? sized(tpl.logo, 'poster') || null : null);
	// Smaller logo file for the card itself; the full size is kept for the grow preview.
	const cardLogo = $derived(tpl?.variant === 'logo' && tpl.logo && cw ? sized(tpl.logo, 'poster', cw * 0.4) || null : null);
	const growLogo = $derived(tpl ? (coverLogo ?? (tpl.variant === 'titled' && pick?.logo && !logoFailed ? sized(pick.logo, 'poster') || null : null)) : shownLogo);
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const pct = $derived(progress === undefined ? undefined : Math.max(0, Math.min(100, progress)));
	const typedWord = $derived(typed ? typeWord(kind) : '');
	const sub = $derived(typedWord && subtitle === typedWord ? undefined : subtitle);
	const strip = $derived(pct === undefined ? shownFresh(fresh, { row }) : null);
	const marked = $derived(!!(strip || owned));
	// Spoken label for the strip, skipped when the subtitle already says the same.
	const stripWords = $derived(strip && !(sub && freshLabel(strip).endsWith(sub)) ? freshLabel(strip) : '');

	// Closes any open preview and timers when the card goes away.
	$effect(() => () => shut());
	$effect(() => {
		if (root && preview) return watchTrailer(root, preview.kind, preview.tmdb);
	});
	$effect(() => {
		if (root) cw = root.clientWidth;
	});

	// A drag on the row must not open previews under the pointer.
	const dragging = () => !!root?.closest('.dragging');
	// svelte-ignore state_referenced_locally
	const opener = dwell(cover ? GROW_DELAY : HOVER_DELAY, () => {
		if (dragging() || scrollHold()) return;
		leaving = false;
		open = true;
	});
	// svelte-ignore state_referenced_locally
	const closer = dwell(cover ? 60 : 140, () => (cover ? (leaving = true) : (open = false)));
	const near = shelfNear();
	function enter() {
		if (!preview || !finePointer() || dragging() || scrollHold()) return;
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
	// Tab from the preview moves to the next control after the card, since the preview sits outside the row.
	function tabOut(back: boolean) {
		shut();
		if (back || !root) return void (hit?.focus(), opener.cancel());
		const r = root;
		const next = r.nextElementSibling?.querySelector<HTMLElement>('a[href], button:not([disabled])');
		if (next) return next.focus();
		const after = [...document.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, select, textarea, [tabindex]')].find(
			(el) => el.tabIndex >= 0 && !r.contains(el) && !el.closest('.ui-grow') && el.getClientRects().length > 0 && !!(r.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING)
		);
		(after ?? hit)?.focus();
	}
	// The picked art falls back to the plain image once before the card gives up.
	function artError() {
		if (cover && pick?.art && src === pick.art && !pickFailed) {
			pickFailed = true;
			loaded = false;
		} else failed = true;
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-wide" class:gone={needsArt && (!src || failed)} class:cover class:big class:grown={cover && open} style:width bind:this={root} onpointerenter={() => ((byKey = false), enter())} onpointermove={(e) => (e.movementX || e.movementY) && !open && !opener.pending && ((byKey = false), enter())} onpointerleave={leave} onfocusin={(e) => (e.target as HTMLElement)?.matches?.(':focus-visible') && ((byKey = true), enter())} onfocusout={(e) => !root?.contains(e.relatedTarget as Node) && leave()} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} role={href ? undefined : 'group'} bind:this={hit} use:press={'card'} aria-label={cover ? [title, typedWord, sub, stripWords].filter(Boolean).join(', ') : undefined} title={cover ? title : undefined}>
		<span class="art" class:low={badgesAt === 'bottom' && (badges.length || status)} use:viewTransitionName={vtName} bind:this={artEl}>
			{#if !src || failed || !loaded}<ArtPlaceholder name={title} shape="wide" loading={!!src && !failed && near() && !loaded} />{/if}
			{#if src && !failed && near()}{#key art}<img src={art} class:in={loaded} class:tall alt="" loading="lazy" decoding="async" use:whenLoaded={(e) => { tall = !!e && e.naturalHeight > e.naturalWidth; loaded = true; }} onerror={artError} />{/key}{/if}
			{#if tpl && (tpl.variant !== 'titled' || !loaded)}
				<span class="scrim" aria-hidden="true"></span>
				{#if !inset}<span class="name" class:lifted={!!strip} aria-hidden="true">
					{#if coverLogo}<img class="logo" src={cardLogo ?? coverLogo} alt="" loading="lazy" decoding="async" onerror={() => (logoFailed = true)} />{:else}<span class="word">{title}</span>{/if}
				</span>{/if}
			{/if}
			{#if badges.length || status}
				<span class="over" class:bottom={badgesAt === 'bottom'} class:pushed={badgesAt === 'top' && !!owned} class:lifted={badgesAt === 'bottom' && !!strip}>
					{#if status}<StatusPill kind={status} label={statusLabel} percent={pct} on="art" testid={statusTestid} />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			{#if inset}<img class="inset" src={inset} alt="" loading="lazy" decoding="async" />{/if}
			{#if marked}<CardBadges fresh={strip} owned={owned ?? null} />{/if}
			{#if pct !== undefined}
				<span class="line" role="progressbar" aria-label={t('ui.wideCard.progress', { title })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} class:done={pct >= 100}><i style="transform: translateX({pct - 100}%)"></i></span>
			{/if}
		</span>
		{#if !cover}
			<span class="t" title={title}>{title}</span>
			{#if typedWord || sub}<span class="s">{#if typedWord}<span class="ty">{typedWord}</span>{#if sub}<span class="dot" aria-hidden="true">·</span>{/if}{/if}{sub ?? ''}</span>{/if}
			{#if meta}<span class="m">{meta}</span>{/if}
		{:else}
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
	<GrowCard anchor={artEl} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} image={loaded && !failed ? art : null} logo={growLogo} lifted={!!strip} {leaving} badge={marked ? wideBadges : undefined} action={growAction} owned={owned ?? null} followId={preview.titleId} {followTmdb} context={growContext} service={growService} onmore={onmore ? (r) => { shut(); onmore(r); } : undefined} onenter={() => { closer.cancel(); leaving = false; }} onleave={leave} onclose={(back) => { shut(); if (back) { hit?.focus(); opener.cancel(); } }} focusIn={byKey} ontab={tabOut} returnTo={hit} ongone={() => leaving && shut()} />
{:else if open && preview && root}
	<HoverPreview anchor={root} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} {image} badge={marked ? wideBadges : undefined} onenter={() => closer.cancel()} onleave={leave} onclose={shut} />
{/if}

<style>
	/* Cards that need art vanish with their list item when none loads. */
	.ui-wide.gone, :global(li:has(> .ui-wide.gone)) { display: none; }
	.ui-wide { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--wide-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img.tall { object-position: center 30%; }
	img:not(.logo) { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	img.in:not(.logo) { opacity: 1; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img { transform: scale(1.03); }
		/* Minimal: the still does not zoom. */
		:global(html[data-look='minimal']) a.hit:hover img { transform: none; }
	}
	.over { position: absolute; left: 8px; top: 8px; right: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.over.bottom { top: auto; bottom: 8px; }
	/* Leaves room for the owned mark that sits at the top. */
	.over.pushed { top: 42px; }
	.over.lifted { bottom: 30px; }
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 35%); overflow: hidden; }
	.line i { display: block; width: 100%; height: 100%; background: var(--accent); transition: transform var(--dur-slow) var(--ease-out); }
	.line.done i { background: var(--ok); }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s .dot { opacity: 0.7; margin: 0 0.3em; }
	.t.under { display: block; padding-top: 8px; }
	.s, .m { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.act { position: absolute; right: 8px; top: 8px; }
	.act.below { top: 42px; }
	.cover .art { margin-bottom: 0; }
	.art img.inset { position: absolute; left: 8%; top: 50%; translate: 0 -50%; right: auto; bottom: auto; width: 34%; height: auto; aspect-ratio: 1; object-fit: cover; opacity: 1; border-radius: 6px; box-shadow: 0 6px 18px rgb(0 0 0 / .45); z-index: 1; }
	.big .art { border-radius: var(--r-panel); }
	/* The grow preview replaces the art, so the original hides under it. */
	.cover.grown .art { visibility: hidden; }
	.scrim { position: absolute; inset: auto 0 0; height: 60%; background: linear-gradient(to top, rgb(0 0 0 / 0.66), rgb(0 0 0 / 0.18) 55%, transparent); pointer-events: none; }
	/* Height in cqw keeps the logo box in step with the card width. */
	.name { position: absolute; left: 6%; bottom: 9%; z-index: 1; width: 40%; height: 13cqw; display: flex; align-items: flex-end; justify-content: flex-start; pointer-events: none; }
	.name.lifted { bottom: 20%; }
	.name:has(.word) { width: 62%; }
	.word { font: 700 clamp(13px, 5.2cqw, 24px)/1.05 var(--font-display); letter-spacing: -0.02em; color: #fff; text-shadow: 0 2px 10px rgb(0 0 0 / 0.55); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: break-word; hyphens: auto; -webkit-hyphens: auto; }
	.cover .art :global(.mark) { width: 22px; height: 22px; }
	.cover .art :global(.mark svg) { width: 13px; height: 13px; }
	.cover .art :global(.mark.downloading svg) { width: 18px; height: 18px; }
	.cover .art :global(.fresh) { height: 14px; font-size: 10px; }
	@container art (min-width: 280px) {
		.cover .art :global(.mark) { width: 26px; height: 26px; }
		.cover .art :global(.mark svg) { width: 15px; height: 15px; }
		.cover .art :global(.mark.downloading svg) { width: 21px; height: 21px; }
		.cover .art :global(.fresh) { height: 16px; font-size: 10px; }
	}
	.logo { display: block; max-width: 100%; max-height: 100%; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.5)); }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.cover a.hit:hover .art { transform: none; }
		.cover a.hit:hover img { transform: none; }
	}
</style>
