<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { press, viewTransitionName } from '../motion.ts';

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
		action,
		statusTestid
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
		action?: Snippet;
		/** Test id on the status pill. */
		statusTestid?: string;
	} = $props();
	let failed = $state(false);
	const pct = $derived(progress === undefined ? undefined : Math.max(0, Math.min(100, progress)));
</script>

<div class="ui-wide" style:width>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName}>
			{#if image && !failed}<img src={image} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />{:else}<span class="ph" aria-hidden="true"></span>{/if}
			{#if badges.length || status}
				<span class="over">
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

<style>
	.ui-wide { position: relative; min-width: 0; }
	.hit { display: flex; flex-direction: column; gap: 2px; color: var(--text); border-radius: var(--r-card); }
	.hit:focus-visible { outline: none; }
	.hit:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; aspect-ratio: var(--wide-ratio); margin-bottom: 8px; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { width: 100%; height: 100%; object-fit: cover; transition: transform var(--dur-slow) var(--ease-out); }
	a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	a.hit:hover img { transform: scale(1.03); }
	.ph { position: absolute; inset: 0; background: linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.over { position: absolute; left: 8px; top: 8px; right: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 35%); }
	.line i { display: block; height: 100%; background: var(--accent); transition: width var(--dur-slow) var(--ease-out); }
	.line.done i { background: var(--ok); }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s, .m { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.act { position: absolute; right: 8px; top: 8px; }
	@media (prefers-reduced-motion: reduce) { a.hit:hover img { transform: none; } }
</style>
