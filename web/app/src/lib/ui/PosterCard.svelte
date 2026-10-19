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
		typePill,
		owned,
		actionOnHover = false,
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
		/** Small type pill top-left (MOVIE, SHOW, ARTIST). */
		typePill?: 'movie' | 'show' | 'artist';
		/** Round status icon top-right, with its aria-label and tooltip. */
		owned?: { state: 'available' | 'partial' | 'downloading' | 'requested' | 'declined'; label: string } | null;
		/** The round action shows on hover or focus only (always on touch screens). */
		actionOnHover?: boolean;
		action?: Snippet;
	} = $props();
	const typeWords = { movie: 'Movie', show: 'Show', artist: 'Artist' } as const;
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
			{#if owned}
				<span class="own {owned.state}" role="img" aria-label={owned.label} title={owned.label}>
					<svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
						{#if owned.state === 'available'}<path d="M5 12.5l4.5 4.5L19 7.5" />
						{:else if owned.state === 'partial'}<path d="M6 12h12" />
						{:else if owned.state === 'downloading'}<path d="M12 5v12M6.5 11.5L12 17l5.5-5.5" />
						{:else if owned.state === 'requested'}<circle cx="12" cy="12" r="8" stroke-width="2.4" /><path d="M12 8v4.5l3 2" stroke-width="2.4" />
						{:else}<path d="M7 7l10 10M17 7L7 17" />{/if}
					</svg>
				</span>
			{/if}
			{#if badges.length || status || typePill}
				<span class="over" class:own-gap={!!owned}>
					{#if typePill}<span class="type {typePill}">{typeWords[typePill]}</span>{/if}
					{#if status}<StatusPill kind={status} label={statusLabel} on="art" />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</span>
			{/if}
			{#if progress !== undefined}<span class="prog"><ProgressBar value={progress} label="{title} progress" size="sm" on="art" /></span>{/if}
		</span>
		<span class="t">{title}</span>
		{#if subtitle}<span class="s">{subtitle}</span>{/if}
	</svelte:element>
	{#if action}<span class="act-layer" class:hover-only={actionOnHover}><span class="act">{@render action()}</span></span>{/if}
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
	.over.own-gap { right: 42px; }
	/* Type pill: small caps word on a calm tinted glass, one hue per kind. */
	.type { --tone: var(--info); display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: var(--r-pill); font: var(--t-caption); font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--on-art); background: color-mix(in srgb, var(--tone) 72%, rgb(0 0 0 / 0.55)); box-shadow: 0 0 0 0.5px var(--on-art-ring); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.type.show { --tone: color-mix(in srgb, var(--info) 55%, var(--fail)); }
	.type.artist { --tone: color-mix(in srgb, var(--ok) 55%, var(--info)); }
	/* Status icon: a round solid disc, one colour per state, never colour alone (aria-label + tooltip). */
	.own { position: absolute; top: 8px; right: 8px; z-index: 1; width: 26px; height: 26px; display: grid; place-items: center; border-radius: 50%; color: var(--on-art); background: color-mix(in srgb, var(--tone, var(--ok)) 72%, black); box-shadow: 0 0 0 1.5px var(--on-art-ring), 0 2px 6px rgb(0 0 0 / 0.35); }
	.own.available, .own.partial { --tone: var(--ok); }
	.own.downloading { --tone: var(--info); }
	.own.requested { --tone: var(--wait); }
	.own.declined { --tone: var(--fail); }
	@media (hover: hover) and (pointer: fine) {
		.act-layer.hover-only .act { opacity: 0; transform: scale(0.9); transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
		.ui-poster:hover .act-layer.hover-only .act, .ui-poster:focus-within .act-layer.hover-only .act { opacity: 1; transform: none; }
	}
</style>
