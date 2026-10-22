<script lang="ts">
	// A music genre tile: 16:9, radius 10, the genre's hue as a flat colour;
	// when the server sends tile art (Phase 2), a small grey picture sits on
	// the hue at .55 opacity (no blend modes). The name sits bottom-left.
	// overflow: clip, not hidden: a scroll container gives the grid a 0 px
	// row (its aspect-ratio height is not counted), so the tiles collapsed.
	let { name, href, hue, art = '' }: { name: string; href: string; hue: number; art?: string } = $props();
	const colour = $derived(`hsl(${(((hue % 24) + 24) % 24) * 15} 52% 36%)`);
</script>

<a class="ui-gtile" {href} style:--hue={colour} data-testid="genre-tile">
	{#if art}<img src={art} alt="" loading="lazy" decoding="async" />{/if}
	<span class="name">{name}</span>
</a>

<style>
	.ui-gtile { position: relative; display: block; width: 100%; min-width: 0; aspect-ratio: 16 / 9; align-self: start; border-radius: 10px; overflow: clip; background: var(--hue); color: #fff; text-decoration: none; }
	.ui-gtile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0.55; }
	.name { position: absolute; left: 12px; bottom: 10px; right: 12px; font-size: 17px; font-weight: 700; line-height: 1.2; text-shadow: 0 1px 2px rgb(0 0 0 / 0.35); }
</style>
