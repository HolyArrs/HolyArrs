<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { press } from '../motion.ts';
	import { coverFor } from '../music/music-pages.ts';
	import { tmdbSized } from '../library-art.ts';

	// For work screens: leading art or icon, title, subtitle, trailing status
	// and actions. Only the main area is a link, so trailing buttons stay separate.
	let {
		title,
		subtitle,
		meta,
		href,
		image,
		icon,
		art = 'poster',
		leading,
		trailing,
		onclick
	}: {
		title: string;
		subtitle?: string;
		meta?: string;
		href?: string;
		image?: string | null;
		icon?: IconName;
		art?: 'poster' | 'wide' | 'square' | 'circle';
		leading?: Snippet;
		trailing?: Snippet;
		onclick?: () => void;
	} = $props();
</script>

<div class="ui-row">
	{#snippet body()}
		{#if leading}{@render leading()}
		{:else if image}<span class="art {art}"><img src={tmdbSized(coverFor(image, 80).src, art === 'wide' ? 'backdrop' : art === 'circle' ? 'profile' : 'poster', 80)} alt="" loading="lazy" decoding="async" /></span>
		{:else if icon}<span class="ic"><Icon name={icon} size={18} /></span>{/if}
		<span class="text">
			<span class="t">{title}</span>
			{#if subtitle}<span class="s">{subtitle}</span>{/if}
			{#if meta}<span class="m">{meta}</span>{/if}
		</span>
	{/snippet}
	{#if href}<a class="main hit" {href} use:press={'row'}>{@render body()}</a>
	{:else if onclick}<button class="main hit" type="button" {onclick} use:press={'row'}>{@render body()}</button>
	{:else}<div class="main">{@render body()}</div>{/if}
	{#if trailing}<div class="trail">{@render trailing()}</div>{/if}
</div>

<style>
	.ui-row { display: flex; align-items: center; gap: 12px; min-height: 56px; padding: 6px 8px 6px 6px; border-radius: var(--r-btn); }
	.ui-row:has(.hit:hover) { background: var(--surface-2); }
	.main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; padding: 4px; border-radius: 8px; color: var(--text); text-align: left; }
	.main:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art { flex: none; overflow: hidden; background: var(--surface-2); border-radius: 6px; }
	.art img { width: 100%; height: 100%; object-fit: cover; }
	.poster { width: 36px; height: 54px; }
	.wide { width: 80px; height: 45px; }
	.square { width: 44px; height: 44px; }
	.circle { width: 40px; height: 40px; border-radius: 50%; }
	.ic { flex: none; width: 36px; height: 36px; display: grid; place-items: center; border-radius: 9px; background: var(--surface-2); color: var(--text-muted); }
	.text { display: flex; flex-direction: column; min-width: 0; }
	.t { font: var(--t-headline); font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s, .m { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.m { font-variant-numeric: tabular-nums; }
	.trail { display: flex; align-items: center; gap: 8px; flex: none; }
	@media (max-width: 520px) { .ui-row { flex-wrap: wrap; } .trail { margin-left: 52px; } }
</style>
