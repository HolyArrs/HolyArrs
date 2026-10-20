<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { sized } from '../tmdb-image.ts';
	import { shelfNear, whenLoaded } from '../lazy-art.ts';
	import { press, viewTransitionName } from '../motion.ts';
	import HoverPreview from './HoverPreview.svelte';
	import { dwell, finePointer, HOVER_DELAY } from '../trailer.ts';

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
		preview
	}: {
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
		preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number } | null;
	} = $props();
	// The expanded card, as on PosterCard.
	let root = $state<HTMLElement>();
	let open = $state(false);
	const opener = dwell(HOVER_DELAY, () => (open = true));
	const closer = dwell(140, () => (open = false));
	function enter() {
		if (!preview || !finePointer()) return;
		closer.cancel();
		if (!open) opener.start();
	}
	function leave() {
		opener.cancel();
		if (open) closer.start();
	}
	function shut() {
		opener.cancel();
		closer.cancel();
		open = false;
	}
	$effect(() => () => shut());
	let failed = $state(false);
	// The name tile shows until the picture has loaded; in a row the picture
	// waits until the row is near the screen.
	let loaded = $state(false);
	const near = shelfNear();
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const pct = $derived(progress === undefined ? undefined : Math.max(0, Math.min(100, progress)));
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ui-wide" style:width bind:this={root} onpointerenter={enter} onpointerleave={leave} onfocusin={(e) => (e.target as HTMLElement)?.matches?.(':focus-visible') && enter()} onfocusout={(e) => !root?.contains(e.relatedTarget as Node) && leave()} onpointerdown={() => opener.cancel()}>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" class:low={badgesAt === 'bottom' && (badges.length || status)} use:viewTransitionName={vtName}>
			{#if !image || failed || !loaded}<span class="ph" aria-hidden="true"><span class="ph-ic"><Icon name={kindIcon} size={20} /></span><span class="ph-t">{title}</span></span>{/if}
			{#if image && !failed && near()}<img src={sized(image, 'backdrop')} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={() => (failed = true)} />{/if}
			{#if badges.length || status}
				<span class="over" class:bottom={badgesAt === 'bottom'}>
					{#if status}<StatusPill kind={status} label={statusLabel} percent={pct} on="art" testid={statusTestid} />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			{#if pct !== undefined}
				<span class="line" role="progressbar" aria-label="{title} progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} class:done={pct >= 100}><i style="width: {pct}%"></i></span>
			{/if}
		</span>
		<span class="t">{title}</span>
		{#if subtitle}<span class="s">{subtitle}</span>{/if}
		{#if meta}<span class="m">{meta}</span>{/if}
	</svelte:element>
	{#if action}<span class="act">{@render action()}</span>{/if}
</div>

{#if open && preview && root}
	<HoverPreview anchor={root} kind={preview.kind} tmdb={preview.tmdb} year={preview.year} {title} {href} {image} onenter={() => closer.cancel()} onleave={leave} onclose={shut} />
{/if}

<style>
	.ui-wide { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; container: art / inline-size; aspect-ratio: var(--wide-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
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
	.over { position: absolute; left: 8px; top: 8px; right: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.low .ph { padding-bottom: 40px; }
	.over.bottom { top: auto; bottom: 8px; }
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 35%); }
	.line i { display: block; height: 100%; background: var(--accent); transition: width var(--dur-slow) var(--ease-out); }
	.line.done i { background: var(--ok); }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s, .m { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.act { position: absolute; right: 8px; top: 8px; }
</style>
