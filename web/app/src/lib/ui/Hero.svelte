<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { viewTransitionName } from '../motion.ts';

	// Title page hero: full-bleed backdrop (blurred poster when there is none),
	// legibility gradient, title or logo art, meta line, badges, status,
	// a description clamped to 3 lines with Show more, and an action row.
	let {
		title,
		backdrop,
		poster,
		logo,
		meta = [],
		badges = [],
		status,
		statusLabel,
		description,
		breadcrumb = [],
		vtName,
		crumbLabel = 'Breadcrumb',
		actions
	}: {
		title: string;
		backdrop?: string | null;
		poster?: string | null;
		logo?: string | null;
		meta?: string[];
		badges?: string[];
		status?: StatusKind;
		statusLabel?: string;
		description?: string;
		breadcrumb?: { href: string; label: string }[];
		vtName?: string;
		crumbLabel?: string;
		actions?: Snippet;
	} = $props();

	let more = $state(false);
	let text = $state<HTMLElement>();
	let clamped = $state(false);
	$effect(() => {
		void description;
		if (!text) return;
		const check = () => text && (clamped = more || text.scrollHeight > text.clientHeight + 1);
		check();
		const ro = new ResizeObserver(check);
		ro.observe(text);
		return () => ro.disconnect();
	});
	const art = $derived(backdrop || poster || null);
</script>

<div class="ui-hero">
	<div class="bg" class:blur={!backdrop && !!poster} use:viewTransitionName={vtName}>
		{#if art}<img src={art} alt="" decoding="async" />{/if}
	</div>
	<div class="shade" aria-hidden="true"></div>
	<div class="content">
		{#if breadcrumb.length}
			<nav class="crumbs" aria-label={crumbLabel}>
				{#each breadcrumb as c, i (i)}
					{#if i > 0}<span aria-hidden="true">›</span>{/if}
					<a href={c.href} aria-current={i === breadcrumb.length - 1 ? 'page' : undefined}>{c.label}</a>
				{/each}
			</nav>
		{/if}
		{#if logo}<h1 class="logo"><img src={logo} alt={title} /></h1>{:else}<h1>{title}</h1>{/if}
		{#if meta.length}<p class="meta">{meta.join(' · ')}</p>{/if}
		{#if badges.length || status}
			<div class="badges">
				{#if status}<StatusPill kind={status} label={statusLabel} on="art" />{/if}
				{#each badges as b (b)}<Badge label={b} on="art" />{/each}
			</div>
		{/if}
		{#if description}
			<p class="desc" class:open={more} bind:this={text}>{description}</p>
			{#if clamped}<button class="more" type="button" aria-expanded={more} onclick={() => (more = !more)}>{more ? 'Show less' : 'Show more'}</button>{/if}
		{/if}
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
	</div>
</div>

<style>
	.ui-hero { position: relative; isolation: isolate; min-height: clamp(360px, 40vw, 600px); display: flex; align-items: flex-end; overflow: hidden; border-radius: var(--r-panel); background: #111; color: #fff; }
	.bg { position: absolute; inset: 0; z-index: -2; }
	.bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	.bg.blur img { filter: blur(40px) saturate(140%); transform: scale(1.2); }
	.shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, rgb(0 0 0 / 86%) 0%, rgb(0 0 0 / 55%) 38%, rgb(0 0 0 / 0%) 72%), linear-gradient(to right, rgb(0 0 0 / 55%) 0%, rgb(0 0 0 / 0%) 60%); }
	.content { width: min(640px, 100%); padding: clamp(20px, 4vw, 48px); display: flex; flex-direction: column; gap: 10px; }
	.crumbs { display: flex; flex-wrap: wrap; gap: 6px; font: var(--t-subhead); color: rgb(255 255 255 / 82%); }
	.crumbs a:hover { color: #fff; text-decoration: underline; }
	.crumbs a[aria-current='page'] { color: #fff; }
	h1 { font: var(--t-large-title); font-size: clamp(30px, 4.4vw, 48px); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	h1.logo img { max-width: min(420px, 80%); max-height: 140px; object-fit: contain; object-position: left bottom; }
	.meta { font: var(--t-callout); color: rgb(255 255 255 / 86%); font-variant-numeric: tabular-nums; }
	.badges { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
	.desc { font: var(--t-body); color: rgb(255 255 255 / 90%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 60ch; }
	.desc.open { -webkit-line-clamp: unset; line-clamp: unset; display: block; }
	.more { align-self: flex-start; min-height: 32px; padding: 0 2px; color: #fff; font: var(--t-subhead); font-weight: 600; text-decoration: underline; text-underline-offset: 3px; }
	.more:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; border-radius: 4px; }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 6px; }
	@media (max-width: 767px) { .ui-hero { border-radius: 0; margin-inline: calc(var(--gutter) * -1); min-height: 480px; } }
</style>
