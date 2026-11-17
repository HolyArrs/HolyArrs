<script lang="ts" generics="K extends string">
	// Grid of app tiles split into headed groups. Native radios keep arrow keys and screen readers working.
	type Tile = { value: K; name: string; badge: string; hue: number; hint?: string };
	type Group = { id: string; title: string; tiles: Tile[] };

	let { name, groups, value, onpick }: { name: string; groups: Group[]; value: K | undefined; onpick: (v: K) => void } = $props();
</script>

<div class="app-tiles">
	{#each groups as g (g.id)}
		<div class="tile-group">
			<h4 class="tile-head" id="{name}-{g.id}">{g.title}</h4>
			<div class="tile-grid" role="radiogroup" aria-labelledby="{name}-{g.id}">
				{#each g.tiles as tile (tile.value)}
					<label class="app-tile" class:on={value === tile.value} data-kind={tile.value}>
						<input class="hit" type="radio" {name} value={tile.value} checked={value === tile.value} onchange={() => onpick(tile.value)} />
						<span class="badge" style="--h: {tile.hue}" aria-hidden="true">{tile.badge}</span>
						<span class="words">
							<b>{tile.name}</b>
							{#if tile.hint}<span class="hint">{tile.hint}</span>{/if}
						</span>
					</label>
				{/each}
			</div>
		</div>
	{/each}
</div>

<style>
	.app-tiles { display: grid; gap: var(--s-4); }
	.tile-group { display: grid; gap: var(--s-2); }
	.tile-head { margin: 0; font-size: 12px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-3, var(--text-2)); }
	.tile-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: var(--s-2); }
	.app-tile { position: relative; display: flex; align-items: center; gap: var(--s-2); min-height: 56px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--surface-2); cursor: pointer; transition: border-color 0.15s, background 0.15s; }
	.app-tile:hover { border-color: color-mix(in srgb, var(--accent) 45%, var(--line)); }
	.app-tile.on { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); background: color-mix(in srgb, var(--accent) 7%, var(--surface-2)); }
	.app-tile.on:has(input:focus-visible) { box-shadow: inset 0 0 0 1px var(--accent), var(--focus-ring); }
	.app-tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.badge { flex: none; width: 32px; height: 32px; border-radius: 8px; display: grid; place-items: center; font-size: 12px; font-weight: 700; color: #fff; background: hsl(var(--h) 55% 42%); }
	.words { display: grid; min-width: 0; line-height: 1.25; }
	.words b { overflow-wrap: break-word; }
	.app-tile.on .words b { color: var(--accent-text); }
	.hint { font-size: 12px; color: var(--text-2); }
	.hit { position: absolute; inset: 0; z-index: 1; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	@media (max-width: 420px) {
		.tile-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.app-tile { padding: 8px 10px; gap: 8px; }
		.badge { width: 28px; height: 28px; font-size: 11px; }
		.words b { font-size: 14px; }
	}
</style>
