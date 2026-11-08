<script lang="ts">
	import { MusicTileGrid } from '../music/index.ts';
	import { to } from '../core/paths.ts';

	// Search > Music Browse: titled sections of the shared Music tile. A tile with no album cover is left out, and one whose covers fail to load drops out of its grid.
	type Tile = { key: string; name: string; href: string; hue?: number; covers: string[] };
	type Section = { id: string; title: string; tiles: Tile[] };
	let { sections }: { sections: Section[] } = $props();
</script>

<div class="mbs" data-testid="music-browse-sections">
	{#each sections as s (s.id)}
		<section aria-labelledby={`mbs-${s.id}`}>
			<h2 id={`mbs-${s.id}`}>{s.title}</h2>
			<MusicTileGrid tiles={s.tiles.map((t) => ({ key: t.key, name: t.name, href: to(t.href), covers: t.covers }))} />
		</section>
	{/each}
</div>

<style>
	.mbs { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	/* A section whose tiles are all left out shows no heading. */
	section:not(:has(li:not([hidden]))) { display: none; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); margin: 0 0 12px; }
</style>
