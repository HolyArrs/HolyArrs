<script lang="ts">
	import { to } from './paths.ts';
	import ArtPlaceholder from './ui/ArtPlaceholder.svelte';
	// A poster, or the title's initials on a quiet tile when there is no picture.
	let { src, name }: { src?: string | null; name: string } = $props();
	let broken = $state(false);
	const usable = $derived(!!src && !broken && (src.startsWith('https://') || src.startsWith('/')));
	let loaded = $state(false);
</script>

<div class="poster">
	{#if !usable || !loaded}<ArtPlaceholder name={usable ? '' : name} shape="poster" loading={usable} />{/if}
	{#if usable}
		<img src={to(src ?? '')} alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onload={() => (loaded = true)} onerror={() => (broken = true)} />
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
