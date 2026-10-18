<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
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
		kind,
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
		/** What it is, for the artwork fallback's icon. */
		kind?: 'movie' | 'show' | 'artist' | 'episode';
		action?: Snippet;
		/** Test id on the status pill. */
		statusTestid?: string;
	} = $props();
	let failed = $state(false);
	const kindIcon = $derived<IconName>(kind === 'show' || kind === 'episode' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const pct = $derived(progress === undefined ? undefined : Math.max(0, Math.min(100, progress)));
</script>

<div class="ui-wide" style:width>
	<svelte:element this={href ? 'a' : 'div'} class="hit" href={href} use:press={'card'}>
		<span class="art" use:viewTransitionName={vtName}>
			{#if image && !failed}<img src={image} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />{:else}<span class="ph" aria-hidden="true"><span class="ph-ic"><Icon name={kindIcon} size={20} /></span><span class="ph-t">{title}</span></span>{/if}
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
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		a.hit:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		a.hit:hover img, a.hit:hover .ph { transform: scale(1.03); }
	}
	.ph { transition: transform var(--dur-slow) var(--ease-out); }
	.ph { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: flex-end; gap: 10px; padding: 14px; color: var(--text); background: radial-gradient(120% 90% at 0% 0%, var(--accent-soft), transparent 60%), linear-gradient(160deg, var(--surface-2), var(--surface-3)); }
	.ph-ic { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); color: var(--text-muted); box-shadow: var(--shadow-card); }
	.ph-t { font: var(--t-headline); letter-spacing: -0.01em; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.over { position: absolute; left: 8px; top: 8px; right: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(0 0 0 / 35%); }
	.line i { display: block; height: 100%; background: var(--accent); transition: width var(--dur-slow) var(--ease-out); }
	.line.done i { background: var(--ok); }
	.t { font: var(--t-subhead); font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s, .m { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.act { position: absolute; right: 8px; top: 8px; }
</style>
