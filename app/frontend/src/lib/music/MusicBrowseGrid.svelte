<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import MusicTileGrid from './MusicTileGrid.svelte';

	// Explore > Music's Browse and New & Popular > Music's Explore more: a heading, two rows of the shared Music tile, and Browse all for the full list. A tile brings one album cover or a few to try.
	type In = { key: string; name: string; href: string; hue?: number; cover?: string; covers?: string[] };
	let { tiles, allHref, title, rows = 2 }: { tiles: In[]; allHref: string; title?: string; rows?: number } = $props();

	const uid = $props.id();
	const grid = $derived(tiles.map((t) => ({ key: t.key, name: t.name, href: t.href, covers: t.covers ?? (t.cover ? [t.cover] : []) })));
</script>

<section class="mbg" data-testid="music-browse-grid" aria-labelledby="{uid}-t">
	<header>
		<h2 id="{uid}-t">{title ?? t('music.browse')}</h2>
		<a class="all" href={allHref}>{t('music.browseAll')}<Icon name="chevronRight" size={15} /></a>
	</header>
	<MusicTileGrid tiles={grid} {rows} />
</section>

<style>
	.mbg { min-width: 0; }
	/* With every tile left out, no heading shows over an empty grid. */
	.mbg:not(:has(li:not([hidden]))) { display: none; }
	header { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.all { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; flex: none; color: var(--accent-text); font: var(--t-callout); font-weight: 500; border-radius: 6px; }
	.all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.all:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
