<script lang="ts">
	// Poster image that shows a placeholder until the art loads or if it fails.
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import { to } from '../core/paths.ts';
	import { proxied } from '../core/tmdb-image.ts';

	let { src, name }: { src?: string | null; name: string } = $props();

	let broken = $state(false);
	let loaded = $state(false);
	let width = $state(0);

	const usable = $derived(!!src && !broken && (src.startsWith('https://') || src.startsWith('/')));
	// Small posters ask the server for a 250px copy to save bandwidth.
	const shown = $derived.by(() => {
		const url = src ?? '';
		const viaCache = proxied(url);
		if (viaCache !== url) return viaCache;
		const small = sized.test(url) && width <= 125 && width * (globalThis.devicePixelRatio || 1) <= 250;
		return to(small ? `${url}${url.includes('?') ? '&' : '?'}size=250` : url);
	});

	// Only our own art routes can resize, and only when no size is set yet.
	const sized = /^\/api\/v1\/(?:artwork\/[0-9a-f]+\.(?:jpg|jpeg|png|webp)|music-trending\/art\/(?:artist|album)\/[^/?]+|albums\/\d+\/cover)(?:\?(?!.*size=).*)?$/;
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
	/* Faint inner edge so dark art does not blend into the page. */
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
