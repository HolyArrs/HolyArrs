<script lang="ts">
	import { to } from '../paths.ts';

	// The wide picture behind a download: the Title's backdrop, else its poster
	// blurred to fill, else a quiet tile. Always darkened by the caller's scrim.
	// `tall` is an app path (a poster), so it goes through to().
	let { wide, tall, eager = false }: { wide?: string; tall?: string; eager?: boolean } = $props();
	let broken = $state(false);
	let loaded = $state(false);
	const src = $derived(!broken && wide ? wide : '');
</script>

<span class="dl-art" class:loaded aria-hidden="true">
	{#if src}
		<!-- `wide` comes from sized(), which already carries the URL base. -->
		<img src={src} alt="" loading={eager ? 'eager' : 'lazy'} decoding="async" referrerpolicy="no-referrer" onload={() => (loaded = true)} onerror={() => (broken = true)} />
	{:else if tall}
		<img class="blur" src={to(tall)} alt="" loading={eager ? 'eager' : 'lazy'} decoding="async" referrerpolicy="no-referrer" onload={() => (loaded = true)} />
	{/if}
</span>

<style>
	.dl-art { position: absolute; inset: 0; overflow: hidden; background: linear-gradient(135deg, color-mix(in srgb, var(--surface-3) 70%, #000), #111); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.loaded img { opacity: 1; }
	.blur { filter: blur(24px) saturate(140%); transform: scale(1.3); }
	@media (prefers-reduced-motion: reduce) { img { transition: none; } }
</style>
