<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import { press, viewTransitionName } from '../motion.ts';

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
		action?: Snippet;
	} = $props();
	const initials = $derived(title.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join(''));
	let failed = $state(false);
</script>

<div class="ui-poster" style:width>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName}>
			{#if image && !failed}
				<img src={image} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
			{:else}
				<span class="ph" aria-hidden="true">{initials}</span>
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
	img { width: 100%; height: 100%; object-fit: cover; transition: transform var(--dur-slow) var(--ease-out); }
	a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	a.hit:hover img { transform: scale(1.03); }
	.ph { position: absolute; inset: 0; display: grid; place-items: center; color: var(--text-muted); font: var(--t-title1); background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.over { position: absolute; left: 8px; right: 8px; top: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.prog { position: absolute; left: 8px; right: 8px; bottom: 8px; }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.act-layer { position: absolute; inset: 0 0 auto; aspect-ratio: var(--poster-ratio); pointer-events: none; }
	.act { position: absolute; right: 8px; bottom: 8px; pointer-events: auto; }
	@media (prefers-reduced-motion: reduce) { a.hit:hover img { transform: none; } }
</style>
