<script lang="ts">
	import { to } from '../paths.ts';
	import ArtPlaceholder from './ArtPlaceholder.svelte';

	// Search > Music Browse (music_lists_v1 row templates): titled sections of
	// colour tiles, the name top left and one album cover turned in the
	// bottom-right corner. Each tile tries its covers in order; when none
	// loads it shows a drawn record sleeve, so no tile ever shows a broken image.
	type Tile = { key: string; name: string; href: string; hue: number; covers: string[] };
	type Section = { id: string; title: string; tiles: Tile[] };
	let { sections }: { sections: Section[] } = $props();

	// Which cover each tile shows (index into covers; past the end = the
	// placeholder). A cover that fails moves to the next one at once.
	let step = $state<Record<string, number>>({});
	const coverOf = (t: Tile) => t.covers[step[t.key] ?? 0] ?? '';
	const next = (t: Tile) => (step[t.key] = (step[t.key] ?? 0) + 1);
	const tint = (hue: number) => `hsl(${(((hue % 24) + 24) % 24) * 15} 55% 30%)`;
</script>

<div class="mbs" data-testid="music-browse-sections">
	{#each sections as s (s.id)}
		<section aria-labelledby={`mbs-${s.id}`}>
			<h2 id={`mbs-${s.id}`}>{s.title}</h2>
			<div class="grid">
				{#each s.tiles as t (t.key + s.id)}
					<a class="tile" href={to(t.href)} style:--g={tint(t.hue)} data-testid="music-browse-tile">
						<strong>{t.name}</strong>
						{#if coverOf(t)}
								{#key coverOf(t)}<img class="cover" src={to(coverOf(t))} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => next(t)} />{/key}
						{:else}
							<span class="cover ph"><ArtPlaceholder name={t.name} shape="square" /></span>
						{/if}
					</a>
				{/each}
			</div>
		</section>
	{/each}
</div>

<style>
	.mbs { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); margin: 0 0 12px; }
	.grid { display: grid; gap: var(--shelf-gap, 12px); grid-template-columns: repeat(4, minmax(0, 1fr)); }
	.tile {
		position: relative; display: block; aspect-ratio: 16 / 9; overflow: hidden; border-radius: 10px; color: #fff;
		background: var(--g); transition: transform .3s cubic-bezier(.16, 1, .3, 1), box-shadow .3s;
	}
	strong { position: absolute; left: 12px; top: 10px; right: 38%; font: 700 clamp(15px, 1.3vw, 19px) / 1.15 var(--font-ui); letter-spacing: -.005em; overflow-wrap: anywhere; z-index: 1; }
	.cover {
		position: absolute; right: -9%; bottom: -12%; width: 40%; aspect-ratio: 1; object-fit: cover; border-radius: 4px;
		transform: rotate(25deg); box-shadow: 0 6px 16px rgba(0, 0, 0, .45);
	}
	.cover.ph { overflow: hidden; }
	.tile:is(:hover, :focus-visible) { transform: translateY(-2px); box-shadow: 0 10px 22px -12px rgba(0, 0, 0, .6); }
	.tile:active { transform: scale(.98); }
	.tile:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	@media (max-width: 1023px) { .grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
	@media (max-width: 559px) {
		.grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
		strong { left: 10px; top: 8px; font-size: 15px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.tile { transition: none; }
		.tile:is(:hover, :focus-visible), .tile:active { transform: none; }
	}
</style>
