<!-- Small colored square holding a settings icon. -->
<script lang="ts">
	import Icon, { type IconName } from './Icon.svelte';
	import type { Tint } from '../core/settings.ts';

	let { icon, tint = 'gray', size = 'md' }: { icon: IconName; tint?: Tint; size?: 'xs' | 'sm' | 'md' | 'lg' } = $props();
	// Icon scales with the tile so the padding looks even.
	const px = $derived(size === 'xs' ? 12 : size === 'sm' ? 14 : size === 'lg' ? 19 : 16);
</script>

<span class="tile {tint} {size}" aria-hidden="true"><Icon name={icon} size={px} stroke={2.1} /></span>

<style>
	.tile {
		--tile: #8e8e93;
		flex: none;
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: 7px;
		color: #fff;
		background: linear-gradient(180deg, color-mix(in srgb, var(--tile) 82%, #fff) 0%, var(--tile) 100%);
		box-shadow: inset 0 0 0 0.5px rgb(0 0 0 / 10%);
	}
	.tile.xs { width: 20px; height: 20px; border-radius: 5px; }
	.tile.sm { width: 24px; height: 24px; border-radius: 6px; }
	.tile.lg { width: 34px; height: 34px; border-radius: 9px; }
	/* Tint colors set the tile base; the gradient lightens the top. */
	.gray { --tile: #8e8e93; }
	.graphite { --tile: #5f6168; }
	.blue { --tile: #0a7aff; }
	.indigo { --tile: #5856d6; }
	.purple { --tile: #a550d8; }
	.pink { --tile: #f2335b; }
	.red { --tile: #f0372d; }
	.orange { --tile: #f58b00; }
	.amber { --tile: #e9a400; }
	.green { --tile: #2fb350; }
	.teal { --tile: #1ea9be; }
</style>
