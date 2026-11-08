<script lang="ts">
	import { assignTileColours } from './tile-palette.ts';
	import { coverSrc } from './browse-tiles.ts';

	// The one Music Browse tile, on Explore, New & Popular and Search: a flat bright colour, the name top left in bold white, and one real album cover, square and turned 25 degrees, at the bottom right and partly cut off.
	type Tile = { key: string; name: string; href: string; covers: string[] };
	let { tiles, rows, ghosts = 0 }: { tiles: Tile[]; rows?: number; ghosts?: number } = $props();

	const coversOf = (t: Tile) => t.covers.filter(Boolean);
	// Which cover each tile is on (an index into its covers), and the tiles whose covers all failed.
	let step = $state<Record<string, number>>({});
	let dead = $state<Record<string, boolean>>({});
	const coverOf = (t: Tile) => coversOf(t)[step[t.key] ?? 0] ?? '';
	function failed(t: Tile) {
		const next = (step[t.key] ?? 0) + 1;
		if (next < coversOf(t).length) step[t.key] = next;
		else dead[t.key] = true;
	}

	const live = $derived(tiles.filter((t) => coversOf(t).length > 0 && !dead[t.key]));

	// The grid's columns as laid out now, so "two rows" means two rows at any width.
	let grid = $state<HTMLElement>();
	let cols = $state(0);
	$effect(() => {
		if (!grid || !rows) return;
		const el = grid;
		const measure = () => (cols = Math.max(1, getComputedStyle(el).gridTemplateColumns.split(' ').filter(Boolean).length));
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => ro.disconnect();
	});
	const limit = $derived(rows ? (cols || 6) * rows : Infinity);
	const shown = $derived(live.slice(0, limit));
	// Colours go over the tiles shown, so a tile's colour never depends on one after it.
	const colour = $derived.by(() => {
		const hexes = assignTileColours(shown.map((t) => t.key));
		return new Map(shown.map((t, i) => [t.key, hexes[i].hex]));
	});
</script>

{#if ghosts > 0 && !live.length}
	<ul class="mtg" aria-hidden="true" data-testid="music-tile-ghosts">
		{#each Array(ghosts) as _, i (i)}<li class="ghost sk"></li>{/each}
	</ul>
{:else}
	<ul class="mtg" bind:this={grid} data-testid="music-tile-grid">
		{#each live as t (t.key)}
			{@const hex = colour.get(t.key)}
			<li hidden={!hex}>
				<a class="tile" href={t.href} style:--tile={hex} data-testid="music-browse-tile">
					<strong>{t.name}</strong>
					{#key coverOf(t)}<img class="cover" src={coverSrc(coverOf(t))} alt="" loading="eager" decoding="async" draggable="false" onerror={() => failed(t)} />{/key}
				</a>
			</li>
		{/each}
	</ul>
{/if}

<style>
	/* About 190 to 240 px wide: 5 across in a 1440 window with the sidebar open, 6 with it folded, 7 on a wide screen. */
	.mtg { --mtg-min: 190px; --mtg-gap: 16px; display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--mtg-min), 1fr)); gap: var(--mtg-gap); margin: 0; padding: 0; list-style: none; }
	@media (max-width: 559px) { .mtg { grid-template-columns: repeat(2, minmax(0, 1fr)); --mtg-gap: 12px; } }
	li { min-width: 0; container-type: inline-size; }
	li[hidden] { display: none; }
	.ghost { aspect-ratio: 16 / 9; border-radius: 10px; }
	.tile {
		position: relative; display: block; aspect-ratio: 16 / 9; overflow: clip; border-radius: 10px; color: #fff; text-decoration: none;
		background: var(--tile); isolation: isolate;
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out);
	}
	/* The name sizes from the tile's own width, so every tile on every page reads the same. */
	strong {
		position: absolute; left: 7%; top: 9%; right: 38%; z-index: 1; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 3; line-clamp: 3; overflow: hidden;
		font: 700 clamp(14px, 8.2cqi, 21px) / 1.15 var(--font-ui); letter-spacing: -.01em; overflow-wrap: break-word; hyphens: auto;
	}
	.cover {
		position: absolute; right: -7%; bottom: -9%; width: 50%; aspect-ratio: 1; object-fit: cover; border-radius: 4px;
		background: rgb(0 0 0 / .25); transform: rotate(25deg); transform-origin: center; box-shadow: 0 6px 16px rgb(0 0 0 / .4);
		transition: transform var(--dur) var(--ease-out);
	}
	.tile:is(:hover, :focus-visible) { transform: translateY(-2px); box-shadow: 0 10px 22px -12px rgb(0 0 0 / .6); }
	.tile:is(:hover, :focus-visible) .cover { transform: rotate(22deg) scale(1.04); }
	.tile:active { transform: scale(.98); }
	.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (prefers-reduced-motion: reduce) {
		.tile, .cover { transition: none; }
		.tile:is(:hover, :focus-visible), .tile:active { transform: none; }
		.tile:is(:hover, :focus-visible) .cover { transform: rotate(25deg); }
	}
</style>
