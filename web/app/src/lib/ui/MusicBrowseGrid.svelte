<script lang="ts">
	import Icon from '../Icon.svelte';
	import { to } from '../paths.ts';
	import { genreTintToken } from '../music/genre-tint.ts';

	// Explore → Music's Browse grid (flag music_lists_v1): one solid colour tile per
	// genre from our own palette, the name top left, and the genre's top charted album
	// as a small tilted cover in the bottom-right corner, partly cut off. Two rows of
	// four on a desktop, three rows of two on a phone; Browse all opens the full list.
	type Tile = { key: string; name: string; href: string; hue: number; cover: string };
	let { tiles, allHref }: { tiles: Tile[]; allHref: string } = $props();

	const slugOf = (key: string) => key.replace(/^genre:/, '');
	let broken = $state<Record<string, boolean>>({});
</script>

<section class="mbg" data-testid="music-browse-grid" aria-labelledby="mbg-t">
	<header>
		<h2 id="mbg-t">Browse</h2>
		<a class="all" href={allHref}>Browse all<Icon name="chevronRight" size={15} /></a>
	</header>
	<div class="grid">
		{#each tiles.slice(0, 8) as t (t.key)}
			<a class="tile" href={t.href} style:--g="var({genreTintToken(slugOf(t.key), t.hue)})">
				<strong>{t.name}</strong>
				{#if t.cover && !broken[t.key]}
					<img class="cover" src={to(t.cover)} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => (broken[t.key] = true)} />
				{/if}
			</a>
		{/each}
	</div>
</section>

<style>
	.mbg { min-width: 0; }
	header { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.all { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; flex: none; color: var(--accent-text); font: var(--t-callout); font-weight: 500; border-radius: 6px; }
	.all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.grid { display: grid; gap: var(--shelf-gap, 12px); grid-template-columns: repeat(4, minmax(0, 1fr)); }
	.tile {
		position: relative; display: block; aspect-ratio: 16 / 9; overflow: hidden; border-radius: 10px; color: #fff;
		background: var(--g, var(--mg-charcoal));
		transition: transform .3s cubic-bezier(.16, 1, .3, 1), box-shadow .3s;
	}
	strong { position: absolute; left: 12px; top: 10px; right: 38%; font: 700 clamp(15px, 1.3vw, 19px) / 1.15 var(--font-ui); letter-spacing: -.005em; overflow-wrap: anywhere; z-index: 1; }
	/* The cover: about 58% of the tile's height, turned 25°, a third of it past the edge. */
	.cover {
		position: absolute; right: -9%; bottom: -12%; width: 40%; aspect-ratio: 1; object-fit: cover; border-radius: 4px;
		transform: rotate(25deg); box-shadow: 0 6px 16px rgba(0, 0, 0, .45);
	}
	.tile:is(:hover, :focus-visible) { transform: translateY(-2px); box-shadow: 0 10px 22px -12px rgba(0, 0, 0, .6); }
	.tile:active { transform: scale(.98); }
	.tile:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	@media (max-width: 559px) {
		.grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
		.tile:nth-child(n + 7) { display: none; }
		strong { left: 10px; top: 8px; font-size: 15px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.tile { transition: none; }
		.tile:is(:hover, :focus-visible), .tile:active { transform: none; }
	}
</style>
