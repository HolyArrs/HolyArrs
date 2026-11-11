<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import ArtPill from '#lib/ui/ArtPill.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import type { Snippet } from 'svelte';
	import { proxied } from '../core/tmdb-image.ts';
	import { minimalLook } from '../core/look.svelte.ts';

	// Explore's banner for the household's favourite genre: slow art, the genre in large type, a short line and a row of picks not in the Library yet. One click opens the genre page. `layout` 1 lays the picks along the bottom, 2 puts them in a panel on the right.
	let {
		name,
		line,
		eyebrow,
		href,
		art = [],
		layout = 1,
		count = 0,
		square = false,
		picks
	}: {
		name: string;
		line: string;
		eyebrow: string;
		href: string;
		/** Wide pictures of the picks; the first is the main art. */
		art?: string[];
		layout?: 1 | 2;
		count?: number;
		/** Square picks (music covers) instead of posters. */
		square?: boolean;
		/** Posters rendered by the page so they match every other row. */
		picks: Snippet;
	} = $props();
	// The minimal look shows no backdrop, so it is never fetched.
	const minimal = $derived(minimalLook());
	const bg = $derived(minimal ? '' : proxied(art[0]));
</script>

<section class="gb l{layout}" class:square class:minimal aria-label={eyebrow} data-testid="genre-banner" data-layout={layout}>
	{#if !minimal}
		<div class="art" aria-hidden="true">
			{#if bg}<img src={bg} alt="" loading="lazy" decoding="async" />{/if}
			<span class="veil"></span>
		</div>
	{/if}
	<div class="words">
		<ArtPill text={eyebrow} icon="sparkle" />
		<h2 class="name"><a {href} class="stretch">{name}</a></h2>
		<p class="line">{line}</p>
		<a class="open" {href} tabindex="-1" aria-hidden="true">{t('discover.genreBanner.seeTheWholeGenre')}<Icon name="chevronRight" size={16} /></a>
	</div>
	<div class="picks">{@render picks()}</div>
</section>

<style>
	.gb { position: relative; isolation: isolate; display: grid; overflow: hidden; border-radius: var(--r-card, 18px); background: var(--surface-2); color: #fff; container-type: inline-size; min-width: 0; }
	.art { position: absolute; inset: 0; z-index: -1; overflow: hidden; background: var(--surface-3, var(--surface-2)); }
	.art img { width: 100%; height: 100%; object-fit: cover; transform-origin: 60% 40%; animation: drift 38s ease-in-out infinite alternate; }
	.veil { position: absolute; inset: 0; background: linear-gradient(90deg, rgb(0 0 0 / .78), rgb(0 0 0 / .35) 55%, rgb(0 0 0 / .1)), linear-gradient(0deg, rgb(0 0 0 / .72), transparent 55%); }
	@keyframes drift { from { transform: scale(1.04) translate3d(0, 0, 0); } to { transform: scale(1.16) translate3d(-2.5%, -1.5%, 0); } }
	.words { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; position: relative; }
	.name { margin: 0; font: 800 clamp(40px, 6.4cqw, 92px)/.95 var(--font-display); letter-spacing: -.03em; text-wrap: balance; text-shadow: 0 2px 30px rgb(0 0 0 / .4); }
	/* The whole card is the link. */
	.stretch { color: inherit; text-decoration: none; }
	.stretch::after { content: ''; position: absolute; inset: 0; z-index: 1; }
	.stretch:focus-visible { outline: none; }
	.stretch:focus-visible::after { box-shadow: inset var(--focus-ring); box-shadow: 0 0 0 3px var(--accent, #fff) inset; border-radius: inherit; }
	.line { margin: 0; max-width: 34ch; font: 500 clamp(15px, 1.4cqw, 19px)/1.4 var(--font-ui); color: rgb(255 255 255 / .86); }
	.open { display: inline-flex; align-items: center; gap: 4px; margin-top: 6px; font: 600 14px/1 var(--font-ui); color: #fff; opacity: .9; }
	.picks { position: relative; z-index: 2; display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; min-width: 0; --pick: 168px; }
	.picks::-webkit-scrollbar { display: none; }
	.picks > :global(*) { flex: none; width: var(--pick); }
	.l1 { min-height: 520px; grid-template-rows: 1fr auto; padding: 40px 36px 28px; gap: 28px; }
	.l2 { min-height: 0; grid-template-columns: minmax(0, 4fr) minmax(0, 7fr); align-items: center; padding: 32px 36px; gap: 32px; }
	.l2 .veil { background: linear-gradient(90deg, rgb(0 0 0 / .86), rgb(0 0 0 / .6) 45%, rgb(0 0 0 / .3)); }
	.l2 .picks { flex-wrap: nowrap; overflow: visible; --pick: calc((100% - 24px) / 4); }
	.l2 .picks > :global(*:nth-child(n + 5)) { display: none; }
	/* The pictures carry the names; captions under them sat on busy art and could not be read. */
	.l2 .picks :global(.t), .l2 .picks :global(.s), .l2 .picks :global(.sk-line) { display: none; }
	/* Placeholders follow, so a pick's height never changes when covers arrive. */
	@container (max-width: 900px) { .l2 { grid-template-columns: 1fr; } .l2 .picks { overflow-x: auto; --pick: 140px; } .l2 .picks > :global(*:nth-child(n + 5)) { display: block; } }
	@media (max-width: 767px) {
		.l1 { min-height: 0; padding: 26px 16px 18px; gap: 20px; }
		.l2 { min-height: 0; padding: 26px 16px 18px; gap: 20px; }
		.picks { --pick: 128px; margin-inline: -16px; padding-inline: 16px; }
		.name { font-size: clamp(36px, 12cqw, 56px); }
	}
	@media (prefers-reduced-motion: reduce) { .art img { animation: none; } }
	/* Minimal: a plain theme panel with no art, so the words take theme colours. */
	.gb.minimal { color: var(--text); }
	.minimal.l1 { min-height: 0; }
	.minimal .name { font-size: clamp(28px, 3.2cqw, 40px); text-shadow: none; }
	.minimal .line { color: var(--text-muted); }
	.minimal .open { color: var(--text); }
	.minimal .words :global(.ui-art-pill) { --tint: var(--text-muted); color: var(--text); background: var(--surface-3); box-shadow: none; -webkit-backdrop-filter: none; backdrop-filter: none; }
</style>
