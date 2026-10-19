<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { press, viewTransitionName } from '../motion.ts';
	import { sized } from '../tmdb-image.ts';
	import { shelfNear, whenLoaded } from '../lazy-art.ts';

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
		action?: Snippet;
	} = $props();
	let failed = $state(false);
	// The name tile shows until the picture has loaded; in a row the picture
	// waits until the row is near the screen.
	let loaded = $state(false);
	const near = shelfNear();
	const pic = $derived(sized(image, 'poster'));
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
</script>

<div class="ui-poster" style:width>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName}>
			{#if !image || failed || !loaded}
				<span class="ph" aria-hidden="true"><span class="ph-ic"><Icon name={kindIcon} size={22} /></span><span class="ph-t">{title}</span></span>
			{/if}
			{#if image && !failed && near()}
				<img src={pic} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={() => (failed = true)} />
			{/if}
			{#if badges.length || status}
				<span class="over">
					{#if status}<StatusPill kind={status} label={statusLabel} on="art" />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			{#if progress !== undefined}<span class="prog"><ProgressBar value={progress} label="{title} progress" size="sm" on="art" /></span>{/if}
		</span>
		<span class="t">{title}</span>
		{#if subtitle}<span class="s">{subtitle}</span>{/if}
	</svelte:element>
	{#if action}<span class="act-layer"><span class="act">{@render action()}</span></span>{/if}
</div>

<style>
	.ui-poster { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; aspect-ratio: var(--poster-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
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
	.over { position: absolute; left: 8px; right: 8px; top: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.prog { position: absolute; left: 8px; right: 8px; bottom: 8px; }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.act-layer { position: absolute; inset: 0 0 auto; aspect-ratio: var(--poster-ratio); pointer-events: none; }
	.act { position: absolute; right: 8px; bottom: 8px; pointer-events: auto; }
</style>
