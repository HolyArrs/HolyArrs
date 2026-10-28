<script lang="ts">
	import type { TitleView } from '#lib/types.ts';
	import { bannerSrc } from '#lib/library-art.ts';

	// A 16:9 picture of a title for the Banners layouts: its backdrop (or the
	// first poster), with the logo (or the name) laid over it when `label` is on.
	let { title, width, label = false }: { title: TitleView; width: number; label?: boolean } = $props();
	const raw = $derived(title.backdrop || title.artwork[0] || '');
	const src = $derived(bannerSrc(raw, width));
	const logoSrc = $derived(bannerSrc(title.logo ?? '', width * 0.6));
	let done = $state('');
	const ready = $derived(!raw || done === src);
</script>

<span class="tile" class:ready class:music={title.kind === 'artist'} aria-hidden="true">
	{#if src}<img {src} alt="" loading="lazy" decoding="async" onload={() => (done = src)} onerror={() => (done = src)} />{/if}
	{#if label || !raw}
		<span class="veil" class:soft={!raw}></span>
		{#if label && title.logo}<img class="logo" src={logoSrc} alt="" loading="lazy" decoding="async" />{:else}<span class="name">{title.name}</span>{/if}
	{/if}
</span>

<style>
	.tile { position: relative; display: block; width: 100%; aspect-ratio: 16 / 9; overflow: hidden; border-radius: inherit; background: var(--surface-3); isolation: isolate; }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 30%; opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	.ready img { opacity: 1; }
	.music img:not(.logo) { object-position: center 35%; }
	.veil { position: absolute; inset: 0; background: linear-gradient(0deg, rgb(0 0 0 / 70%) 0%, rgb(0 0 0 / 20%) 60%, rgb(0 0 0 / 8%) 100%); }
	.veil.soft { background: none; }
	.logo { inset: 16% 8%; width: 84%; height: 68%; object-fit: contain; object-position: center; filter: drop-shadow(0 1px 6px rgb(0 0 0 / 60%)); }
	.name { position: absolute; left: 8%; right: 8%; top: 50%; translate: 0 -50%; text-align: center; color: #fff; font: 700 clamp(13px, 1.2vw, 20px)/1.15 var(--font-ui); letter-spacing: -0.01em; text-shadow: 0 1px 6px rgb(0 0 0 / 70%); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.veil.soft + .name { color: var(--text-muted); text-shadow: none; font-size: 11px; }
	@media (prefers-reduced-motion: reduce) { img { transition: none; } }
</style>
