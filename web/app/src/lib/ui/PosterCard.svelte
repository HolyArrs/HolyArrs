<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import CardBadges from './CardBadges.svelte';
	import { shownFresh, type Fresh } from '../fresh.ts';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { press, viewTransitionName } from '../motion.ts';
	import { sized } from '../tmdb-image.ts';
	import { shelfNear, whenLoaded } from '../lazy-art.ts';
	import HoverPreview from './HoverPreview.svelte';
	import { prefetchTrailer } from '../autoplay.svelte.ts';
	import { dwell, finePointer, HOVER_DELAY } from '../trailer.ts';

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
		typePill,
		owned,
		actionOnHover = false,
		shape = 'poster',
		preview,
		fresh,
		music = false,
		top10Row = false,
		action
	}: {
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
		/** Small type pill top-left: glyph plus Movie, Show or Artist. */
		typePill?: 'movie' | 'show' | 'artist';
		/** Status pill top-right (glyph only on narrow cards), with its aria-label and tooltip. */
		owned?: { state: 'available' | 'partial' | 'downloading' | 'requested' | 'declined'; label: string } | null;
		/** The round action shows on hover or focus only (always on touch screens). */
		actionOnHover?: boolean;
		/** Music: square for albums, round for artists (same width as a poster). */
		shape?: 'poster' | 'square' | 'round';
		/** Desktop: after a short hover or keyboard focus, open the expanded card (trailer, genres). */
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number } | null;
		/** The server's one freshness strip (bottom centre of the art). */
		fresh?: Fresh | null;
		/** Music cards keep Recently Added and New Release only. */
		music?: boolean;
		/** In the Top 10 row, which shows the rank already. */
		top10Row?: boolean;
		action?: Snippet;
	} = $props();

	// The expanded card: pointer: fine only, after HOVER_DELAY; leaving both
	// the poster and the card (a short grace to cross the gap) closes it.
	let root = $state<HTMLElement>();
	let open = $state(false);
	const opener = dwell(HOVER_DELAY, () => (open = true));
	const closer = dwell(140, () => (open = false));
	// A hover-only action (fine pointers) is put in only while the card is hovered or
	// focused: a glass button on every card makes long rows slow to scroll.
	let hot = $state(false);
	const lazyAction = $derived(actionOnHover && finePointer());
	const showAction = $derived(!!action && (!lazyAction || hot));
	function enter() {
		hot = true;
		if (!preview || !finePointer()) return;
		closer.cancel();
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
	}
	function focusIn(e: FocusEvent) {
		hot = true;
		if ((e.target as HTMLElement)?.matches?.(':focus-visible')) enter();
	}
	function focusOut(e: FocusEvent) {
		if (!root?.contains(e.relatedTarget as Node)) {
			if (!root?.matches(':hover')) hot = false;
			leave();
		}
	}
	$effect(() => () => shut());
	const strip = $derived(progress === undefined ? shownFresh(fresh, { music, top10Row }) : null);
	let failed = $state(false);
	// The name tile shows until the picture has loaded; in a row the picture
	// waits until the row is near the screen.
	let loaded = $state(false);
	const near = shelfNear();
	const pic = $derived(sized(image, 'poster'));
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-poster" class:square={shape === 'square'} class:round={shape === 'round'} style:width bind:this={root} onpointerenter={enter} onpointerleave={leave} onfocusin={focusIn} onfocusout={focusOut} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName}>
			{#if !image || failed || !loaded}
				<span class="ph" aria-hidden="true"><span class="ph-ic"><Icon name={kindIcon} size={22} /></span><span class="ph-t">{title}</span></span>
			{/if}
			{#if image && !failed && near()}
				<img src={pic} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={() => (failed = true)} />
			{/if}
			{#if badges.length || status}
				<span class="over" class:pushed={!!typePill}>
					{#if status}<StatusPill kind={status} label={statusLabel} on="art" />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			<CardBadges fresh={strip} {owned} typePill={typePill ?? null} size={shape === 'poster' ? 'md' : 'sm'} />
			{#if progress !== undefined}<span class="prog"><ProgressBar value={progress} label="{title} progress" size="sm" on="art" /></span>{/if}
		</span>
		<span class="t">{title}</span>
		{#if subtitle}<span class="s">{subtitle}</span>{/if}
	</svelte:element>
	{#if action && showAction}<span class="act-layer" class:hover-only={actionOnHover}><span class="act">{@render action()}</span></span>{/if}
</div>

{#snippet ownBadge()}
	<CardBadges fresh={strip} {owned} typePill={typePill ?? null} />
{/snippet}

{#if open && preview && root}
	<HoverPreview anchor={root} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} {image} badge={owned || strip || typePill ? ownBadge : undefined} {action} onenter={() => closer.cancel()} onleave={leave} onclose={shut} />
{/if}

<style>
	.ui-poster { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.square, .round { --poster-ratio: 1; }
	.round .art { border-radius: 50%; }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--poster-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) ease, transform var(--dur-slow) var(--ease-out); }
	img.in { opacity: 1; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img, a.hit:hover .ph { transform: scale(1.03); }
	}
	.ph { transition: transform var(--dur-slow) var(--ease-out); }
	.ph { position: absolute; inset: 0; overflow: hidden; min-width: 0; display: flex; flex-direction: column; justify-content: flex-end; gap: 10px; padding: 14px; color: var(--text); background: radial-gradient(120% 90% at 0% 0%, var(--accent-soft), transparent 60%), linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.ph-ic { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); color: var(--text-muted); box-shadow: var(--shadow-card); }
	.ph-t { max-width: 100%; min-width: 0; font: var(--t-headline); letter-spacing: -0.01em; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.over { position: absolute; left: 8px; right: 8px; top: 8px; display: flex; flex-wrap: nowrap; align-items: center; gap: 4px; }
	.prog { position: absolute; left: 8px; right: 8px; bottom: 8px; }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.act-layer { position: absolute; inset: 0 0 auto; aspect-ratio: var(--poster-ratio); pointer-events: none; }
	.act { position: absolute; right: 8px; bottom: 8px; pointer-events: auto; }
	.over.pushed { top: 34px; }
	@media (hover: hover) and (pointer: fine) {
		.act-layer.hover-only .act { opacity: 0; transform: scale(0.9); transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
		.ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 1; transform: none; }
		@starting-style { .ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 0; transform: scale(0.9); } }
	}
</style>
