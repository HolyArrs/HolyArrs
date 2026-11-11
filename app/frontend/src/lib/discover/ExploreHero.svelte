<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { richMetadata, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import Billboard from '#lib/ui/Billboard.svelte';
	import { shortSynopsis } from './synopsis.ts';
	import type { Snippet } from 'svelte';
	import { api } from '../core/api.ts';
	import { heroPictures } from '../core/trailer/trailer.ts';
	import { proxied, sized } from '../core/tmdb-image.ts';

	// The Explore hero: one featured title in the shared billboard. More info opens the title's page.
	let {
		kind,
		tmdb,
		title,
		href,
		backdrop,
		poster,
		subtitle,
		settled = true,
		vtName,
		badge,
		action
	}: {
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		href?: string;
		/** A backdrop known already, shown at once. */
		backdrop?: string | null;
		/** The card's poster, shown at once in the minimal look. */
		poster?: string | null;
		subtitle?: string;
		/** The page has finished its first load. */
		settled?: boolean;
		/** The art's view-transition name, so it grows into the title page's hero. */
		vtName?: string;
		badge?: Snippet;
		action?: Snippet;
	} = $props();

	let rich = $state<RichMeta | null>(null);
	$effect(() => {
		const key = `${kind}:${tmdb}`;
		let live = true;
		rich = null;
		void richMetadata(api.get, kind, tmdb).then((m) => live && key === `${kind}:${tmdb}` && (rich = m));
		return () => {
			live = false;
		};
	});
	const pictures = $derived(heroPictures(rich?.backdrop ?? backdrop, rich?.backdrops).map((u) => sized(u, 'backdrop')));
	const line = $derived([rich?.year, ...(rich?.genres ?? []).slice(0, 3)].filter(Boolean).join(' · ') || subtitle || '');
</script>

<Billboard label={t('discover.exploreHero.featured', { title })} testid="explore-hero" {kind} {tmdb} {title} videos={rich?.videos} {pictures} {vtName} {settled} thumb={sized(poster ?? rich?.poster, 'poster', 148) || null}>
	{#snippet heading()}
		{#if rich?.logo}
			<h2 class="bb-logo"><img src={proxied(rich.logo)} alt={title} /></h2>
		{:else}
			<h2 class="bb-name">{title}</h2>
		{/if}
	{/snippet}
	{#snippet meta()}
		{#if line}<p>{line}</p>{/if}
		{#if badge}{@render badge()}{/if}
	{/snippet}
	{#snippet fold()}
		<!-- The title box and overview keep their space while the details load. -->
		<div class="about">
			{#if rich?.overview}<p class="bb-desc" title={rich.overview}>{shortSynopsis(rich.overview, 220)}</p>{/if}
		</div>
	{/snippet}
	{#snippet actions()}
		{#if href}<a class="info" {href} onfocus={(e) => e.currentTarget.scrollIntoView({ block: 'nearest', behavior: 'instant' })}>{t('discover.exploreHero.moreInfo')}</a>{/if}
		{#if action}{@render action()}{/if}
	{/snippet}
</Billboard>

<style>
	.about { min-height: calc(16px * 1.45 * 3); }
	.info { display: inline-flex; align-items: center; min-height: var(--hero-btn, 44px); padding: 0 clamp(20px, 1.4vw, 32px); font-size: clamp(16px, 1vw, 22px) !important; border-radius: var(--r-pill); background: rgb(255 255 255 / 16%); border: 1px solid rgb(255 255 255 / 22%); color: #fff; font: var(--t-headline); -webkit-backdrop-filter: blur(18px) saturate(160%); backdrop-filter: blur(18px) saturate(160%); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-instant) var(--ease-out); }
	.info:hover { background: rgb(255 255 255 / 26%); }
	.info:active { transform: scale(.97); }
	.info:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; }
	:global(.ui-billboard.minimal) .info { font-size: 15px !important; }
	/* Minimal look on phones: the floating type pills overlap the card's top, so the words start below them. */
	@media (max-width: 767px) { :global(html .ui-billboard.minimal[data-testid='explore-hero']) { padding-top: 48px; } }
</style>
