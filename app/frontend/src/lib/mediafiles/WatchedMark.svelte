<!-- The signed-in person's Watched mark on a title's art: a check badge or a filling ring, or a thin bar on a small thumbnail. The words go on the title's own label. -->
<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import type { WatchedMark } from './watched.ts';

	let { mark, look = 'badge' }: { mark: WatchedMark | null | undefined; look?: 'badge' | 'bar' } = $props();

	// A sliver stays visible when what aired is not known, so partly watched never reads as nothing.
	const share = $derived(!mark ? 0 : mark.state === 'watched' ? 100 : mark.of > 0 ? Math.round((Math.min(mark.seen, mark.of) / mark.of) * 100) : 10);
</script>

{#if mark}
	{#if look === 'bar'}
		<span class="wbar" aria-hidden="true"><i style:width="{share}%"></i></span>
	{:else if mark.state === 'watched'}
		<span class="wm done" aria-hidden="true"><Icon name="check" size={15} stroke={3} /></span>
	{:else}
		<span class="wm part" aria-hidden="true">
			<svg width="20" height="20" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9.5" fill="none" stroke="currentColor" stroke-opacity=".28" stroke-width="2.75" /><circle cx="12" cy="12" r="9.5" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" transform="rotate(-90 12 12)" pathLength="100" stroke-dasharray="{share} 100" /></svg>
		</span>
	{/if}
{/if}

<style>
	.wm { position: absolute; right: 8px; bottom: 10px; z-index: 1; width: 26px; height: 26px; display: grid; place-items: center; border-radius: 99px; pointer-events: none; transition: opacity var(--dur-fast) var(--ease-out); }
	.done { color: #fff; background: var(--ok-on-art); box-shadow: 0 1px 4px rgb(0 0 0 / 35%); }
	.part { color: var(--ok-on-art); background: rgb(20 20 22 / 72%); -webkit-backdrop-filter: blur(8px) saturate(160%); backdrop-filter: blur(8px) saturate(160%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 14%); }
	.wbar { position: absolute; left: 3px; right: 3px; bottom: 3px; z-index: 1; height: 3px; border-radius: 2px; background: rgb(20 20 22 / 72%); box-shadow: 0 0 0 1px rgb(20 20 22 / 40%); pointer-events: none; }
	.wbar i { display: block; height: 100%; border-radius: inherit; background: #fff; }
	:global([dir='rtl']) .wm { right: auto; left: 8px; }
	@media (prefers-reduced-transparency: reduce) { .part { backdrop-filter: none; -webkit-backdrop-filter: none; background: rgb(24 24 26 / 94%); } }
	@media (prefers-reduced-motion: reduce) { .wm { transition: none; } }
</style>
