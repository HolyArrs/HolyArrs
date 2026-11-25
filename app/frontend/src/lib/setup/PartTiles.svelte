<script lang="ts">
	// The parts as big tiles, each a real checkbox (first run's Start fresh and a later visit to /welcome).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import type { createWelcomeParts } from './welcome-parts.svelte.ts';

	let { partsStep }: { partsStep: ReturnType<typeof createWelcomeParts> } = $props();
</script>

<!-- Big tiles, each a real checkbox (it covers the tile, so a tap anywhere toggles it). -->
<div class="tiles five" role="group" aria-label={t('welcome.parts')}>
	{#each partsStep.choices as c (c.part)}
		<label class="tile" class:on={partsStep.want[c.part]}>
			<input type="checkbox" bind:checked={partsStep.want[c.part]} aria-label={c.label} />
			<span class="tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span>
			<Icon name={c.icon} size={28} />
			<span class="tile-label">{c.label}</span>
			<span class="tile-desc">{c.desc}</span>
		</label>
	{/each}
</div>

<style>
	/* Tiles: an icon and a word; picked tiles take the accent, with a tick. */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
	.tiles.five { grid-template-columns: repeat(5, minmax(0, 1fr)); }
	.tile {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		min-height: 108px;
		padding: 14px 8px;
		border-radius: 16px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text-muted);
		font: inherit;
		text-decoration: none;
		cursor: pointer;
		transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	/* The real control covers the whole tile, invisible, so taps and clicks land on it. */
	.tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.tile-label { font: 600 14px/1.2 var(--font-ui); color: var(--text); text-align: center; }
	.tile-desc { font: 400 12px/1.35 var(--font-ui); color: var(--text-muted); text-align: center; text-wrap: pretty; }
	@media (hover: hover) { .tile:hover { border-color: var(--line-strong); } }
	.tile:active { transform: scale(0.98); }
	.tile.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 7%, var(--surface-1)); color: var(--accent-text); }
	.tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.tick {
		position: absolute;
		top: 9px;
		inset-inline-end: 9px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		display: grid;
		place-items: center;
		border: 1.5px solid var(--line-strong);
		color: transparent;
		transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
	}
	.tile.on .tick { background: var(--accent); border-color: var(--accent); color: #fff; }
	@media (max-width: 520px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.five { grid-template-columns: repeat(3, minmax(0, 1fr)); }
		.tile-desc { display: none; }
		.tile { min-height: 96px; }
	}
	/* One layer inside the glass card: lists are flat rows split by hairlines, choices are outlined, and only text fields keep a filled box. */
	.tile { background: transparent; }
	@media (hover: hover) { .tile:hover { background: color-mix(in srgb, var(--text) 4%, transparent); } }
	/* Choice tiles: outlined, see-through, a faint fill on hover. */
	:global(.glass) .tile { border-color: rgb(var(--ink-rgb) / 0.14); background: transparent; color: rgb(var(--ink-rgb) / 0.75); }
	@media (hover: hover) { :global(.glass) .tile:hover { border-color: rgb(var(--ink-rgb) / 0.24); background: rgb(var(--ink-rgb) / 0.05); } }
	:global(.glass) .tile-label { color: var(--ink); }
	:global(.glass) .tile-desc { color: rgb(var(--ink-rgb) / 0.62); }
	:global(.glass) .tile.on { border-color: rgb(var(--ink-rgb) / 0.9); background: rgb(var(--ink-rgb) / 0.1); color: var(--ink); }
	:global(.glass) .tile { border-radius: var(--g-r); }
	:global(.glass) .tile { border-color: var(--g-line); background: var(--g-bg); }
	@media (hover: hover) { :global(.glass) .tile:hover { border-color: var(--g-line-hover); background: var(--g-bg-hover); } }
	@media (min-width: 1100px) {
		:global(.glass) .tile { min-height: clamp(120px, 22vh, 260px); gap: 14px; }
		:global(.glass) .tile :global(svg:not(.tick svg)) { width: 40px; height: 40px; }
		:global(.glass) .tile-label { font-size: 20px; }
		:global(.glass) .tile-desc { font-size: 15px; padding: 0 6px; }
	}
</style>
