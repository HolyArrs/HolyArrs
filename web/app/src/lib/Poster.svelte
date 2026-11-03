<script lang="ts">
	import { to } from './paths.ts';
	import { proxied } from './tmdb-image.ts';
	import ArtPlaceholder from './ui/ArtPlaceholder.svelte';
	// A poster, or the title's initials on a quiet tile when there is no picture.
	let { src, name }: { src?: string | null; name: string } = $props();
	let broken = $state(false);
	const usable = $derived(!!src && !broken && (src.startsWith('https://') || src.startsWith('/')));
	let loaded = $state(false);
	// A small slot (a History row, a Requests thumbnail) takes the 250 px copy
	// instead of the stored 500-1000 px picture, by the cover rule in
	// music-pages.ts coverSize: up to 125 css pixels on screens up to 2x. Library
	// pictures, album covers and artist photos all have one.
	let width = $state(0);
	const sized = /^\/api\/v1\/(?:artwork\/[0-9a-f]+\.(?:jpg|jpeg|png|webp)|music-trending\/art\/(?:artist|album)\/[^/?]+|albums\/\d+\/cover)(?:\?(?!.*size=).*)?$/;
	const shown = $derived.by(() => {
		const url = src ?? '';
		// A TMDB picture comes through HolyArrs' artwork cache, never from TMDB itself.
		const viaCache = proxied(url);
		if (viaCache !== url) return viaCache;
		const small = sized.test(url) && width <= 125 && width * (globalThis.devicePixelRatio || 1) <= 250;
		return to(small ? `${url}${url.includes('?') ? '&' : '?'}size=250` : url);
	});
</script>

<div class="poster" bind:clientWidth={width}>
	{#if !usable || !loaded}<ArtPlaceholder name={usable ? '' : name} shape="poster" loading={usable} />{/if}
	{#if usable && width > 0}
		<img src={shown} alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onload={() => (loaded = true)} onerror={() => (broken = true)} />
	{/if}
</div>

<style>
	.poster {
		position: relative;
		aspect-ratio: var(--poster-ratio);
		border-radius: 6px;
		overflow: hidden;
		background:
			linear-gradient(145deg, color-mix(in srgb, var(--ha-steel) 26%, transparent), transparent 58%),
			var(--surface-2);
		border: 1px solid var(--line);
		display: grid;
		place-items: center;
		isolation: isolate;
	}
	.poster::after {
		content: '';
		position: absolute;
		inset: 0;
		z-index: 1;
		pointer-events: none;
		box-shadow: inset 0 0 0 1px color-mix(in srgb, white 4%, transparent);
	}
	img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		background: var(--surface-2);
	}
</style>
