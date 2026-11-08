<script lang="ts">
	// The Settings background, shared: this Library's posters, blurred, dimmed and still, behind a page. Nothing shows until posters are known.
	import { homeArt, startArt } from '../settings/(home)/art.svelte.ts';
	import { wall } from '../settings/(home)/pick.ts';
	import { onMount } from 'svelte';
	import { session } from '#lib/core/session.svelte.ts';

	const tiles = $derived(wall(homeArt.posters));
	onMount(() => {
		startArt(session.s.id, session.can('users'), { name: session.s.name ?? '', avatar: session.s.avatar ?? null });
	});
</script>

{#if tiles.length}
	<div class="pw" aria-hidden="true" data-poster-wall>
		<div class="tiles">{#each tiles as src, i (i)}<img {src} alt="" decoding="async" />{/each}</div>
		<div class="veil"></div>
	</div>
{/if}

<style>
	.pw { position: fixed; inset: 0 0 0 var(--space, 0px); z-index: -1; overflow: hidden; pointer-events: none; }
	.tiles { position: absolute; inset: -60px; display: grid; grid-template-columns: repeat(8, 1fr); gap: 14px; transform: rotate(-8deg) scale(1.15); filter: blur(22px) saturate(1.15); }
	img { width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 10px; display: block; }
	.veil { position: absolute; inset: 0; background: linear-gradient(180deg, color-mix(in srgb, var(--bg) 58%, transparent), color-mix(in srgb, var(--bg) 70%, transparent)), radial-gradient(130% 90% at 50% 10%, transparent, color-mix(in srgb, var(--bg) 70%, transparent)); }
	@media (max-width: 767px) { .pw { left: 0; } .tiles { grid-template-columns: repeat(4, 1fr); } }
</style>
