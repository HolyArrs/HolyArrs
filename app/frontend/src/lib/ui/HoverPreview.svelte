<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Small floating preview card shown on hover, with an optional trailer.
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { metaLine } from './fresh.ts';
	import { GROW_WIDTH } from './grow.ts';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { api } from '../core/api.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { popOut } from '../core/transitions.ts';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '../core/trailer/autoplay.svelte.ts';
	import { placeCard } from '../core/trailer/trailer.ts';

	let {
		anchor,
		kind,
		tmdb,
		title,
		href,
		year,
		image,
		badge,
		action,
		onenter,
		onleave,
		onclose
	}: {
		anchor: HTMLElement;
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		href?: string;
		year?: number;
		image?: string | null;
		badge?: Snippet;
		action?: Snippet;
		onenter?: () => void;
		onleave?: () => void;
		onclose: () => void;
	} = $props();

	let W = $state(340);
	let from = $state(0.6);
	let card = $state<HTMLElement>();
	let pos = $state({ left: -9999, top: -9999 });
	let origin = $state('50% 50%');
	let meta = $state<RichMeta | null>(null);
	let sources = $state<TrailerSource[]>([]);
	let playing = $state(false);
	const ctl = new AbortController();

	const art = $derived(sized(meta?.backdrop ?? null, 'backdrop') || sized(image, 'poster'));
	const line = $derived([...metaLine(kind, [kind === 'movie' && meta?.runtime ? `${Math.floor(meta.runtime / 60) ? `${Math.floor(meta.runtime / 60)}h ` : ''}${meta.runtime % 60}m`.trim() : undefined, meta?.year ?? year]), ...(meta?.genres ?? []).slice(0, 2)].join(' · '));

	onMount(() => {
		place();
		const close = () => onclose();
		// The card is fixed over the page, so any scroll or resize closes it.
		window.addEventListener('scroll', close, { passive: true, capture: true });
		window.addEventListener('resize', close);
		void (async () => {
			meta = await richMetadata(api.get, kind, tmdb);
			// Details change the card height, so place it again.
			requestAnimationFrame(place);
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			sources = await trailerSources(kind, tmdb, meta?.videos, ctl.signal);
		})();
		return () => {
			window.removeEventListener('scroll', close, { capture: true });
			window.removeEventListener('resize', close);
		};
	});
	onDestroy(() => ctl.abort());

	// Sizes and positions the card beside the anchor; the origin lets it scale out from the anchor's centre.
	function place() {
		if (!card) return;
		const a = anchor.getBoundingClientRect();
		W = Math.min(GROW_WIDTH, document.documentElement.clientWidth - 16);
		from = Math.min(1, a.width / W);
		const h = card.offsetHeight;
		const row = (anchor.closest('.row') ?? anchor.closest('main'))?.getBoundingClientRect();
		pos = placeCard(a, W, h, { width: document.documentElement.clientWidth, height: window.innerHeight }, 8, row ? { left: row.left, right: row.right } : undefined);
		origin = `${Math.round(a.left + a.width / 2 - pos.left)}px ${Math.round(a.top + a.height / 2 - pos.top)}px`;
	}

	// Moves the card to body so page transforms and overflow cannot clip it.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div
	class="ui-hover"
	bind:this={card}
	use:portal
	out:popOut
	role="group"
	aria-label={t('ui.hoverPreview.preview', { title })}
	style:left="{pos.left}px"
	style:top="{pos.top}px"
	style:width="{W}px"
	style:transform-origin={origin}
	style:--from={from}
	onpointerenter={onenter}
	onpointerleave={onleave}
>
	<a class="media" href={href} tabindex="-1" aria-hidden="true">
		{#if art}<img src={art} alt="" decoding="async" class:poster={!meta?.backdrop} />{/if}
		{#if sources.length}
			<TrailerPlayer {sources} {title} repeat onplaying={() => (playing = true)} onfail={() => (playing = false)} />
		{/if}
		<span class="fade" class:playing aria-hidden="true"></span>
	</a>
	{#if badge}<div class="badge">{@render badge()}</div>{/if}
	<div class="body">
		<div class="row">
			<div class="words">
				{#if href}<a class="t" {href}>{title}</a>{:else}<span class="t">{title}</span>{/if}
				{#if line}<span class="s">{line}</span>{/if}
			</div>
			{#if action}<span class="act">{@render action()}</span>{/if}
		</div>
	</div>
</div>

<style>
	/* Fixed over the page; the open animation starts at the anchor size. */
	.ui-hover {
		position: fixed; z-index: 60;
		border-radius: var(--r-panel); overflow: hidden;
		background: var(--surface-1); color: var(--text);
		box-shadow: var(--shadow-modal), 0 0 0 1px var(--line);
		animation: hover-open var(--dur) var(--ease-spring, var(--ease-out)) both;
		will-change: transform, opacity;
	}
	@keyframes hover-open { from { opacity: 0.4; transform: scale(var(--from, .62)); } }
	.media { position: relative; display: block; aspect-ratio: 16 / 9; background: var(--surface-2); overflow: hidden; }
	.media img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.media img.poster { object-position: center 20%; }
	.fade { position: absolute; inset: auto 0 0; height: 40%; background: linear-gradient(to top, color-mix(in srgb, var(--surface-1) 70%, transparent), transparent); pointer-events: none; transition: opacity var(--dur-slow) var(--ease-out); }
	.fade.playing { opacity: 0; }
	.body { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px 14px; }
	.row { display: flex; align-items: flex-start; gap: 10px; }
	.words { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.t { font: var(--t-headline); letter-spacing: -0.01em; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	a.t:hover { text-decoration: underline; text-underline-offset: 3px; }
	a.t:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.s { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.act { flex: none; }
	.act :global(.ui-ibtn.glass) { background: var(--surface-2); color: var(--text); box-shadow: inset 0 0 0 1px var(--line); }
	.act :global(.ui-ibtn.glass:hover) { background: var(--surface-3); }
	/* Same 16:9 box as the media so the badge sits on the art. */
	.badge { position: absolute; top: 0; left: 0; right: 0; aspect-ratio: 16 / 9; z-index: 3; pointer-events: none; }
	@media (prefers-reduced-motion: reduce) { .ui-hover { animation: none; } }
</style>
