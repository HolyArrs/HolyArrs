<script lang="ts">
	import { bannerArt, type BannerEntry } from './calendar.ts';
	import { sized } from '#lib/core/tmdb-image.ts';

	// A List row's wide banner: the title's textless art with its logo over it, filling the right side and fading into the row. A picture that fails moves on to the next one.
	let { e }: { e: BannerEntry } = $props();

	const art = $derived(bannerArt(e));
	let at = $state(0);
	let shown = $state(false);
	let logoOk = $state(true);
	const src = $derived(art.images[at] ? sized(art.images[at], 'backdrop', 400) : null);
	const logo = $derived(art.logo && logoOk ? sized(art.logo, 'poster', 240) : null);
</script>

<span class="cal-banner" class:album={e.kind === 'album'} aria-hidden="true" data-testid="cal-banner">
	{#if src}
		{#key src}
			<!-- The picture keeps its 16:9 shape; a blurred, dimmed copy fills the rest of the row so nothing is stretched or cropped to a sliver. -->
			<img class="fill" class:in={shown} {src} alt="" loading="lazy" decoding="async" />
			<img class="bg" class:in={shown} {src} alt="" loading="lazy" decoding="async" onload={() => (shown = true)} onerror={() => ((shown = false), at++)} />
		{/key}
	{/if}
	{#if logo}<img class="logo" class:in={shown || !src} src={logo} alt="" loading="lazy" decoding="async" onerror={() => (logoOk = false)} />{/if}
</span>

<style>
	.cal-banner { position: absolute; inset: 0; overflow: hidden; background: var(--surface-2); pointer-events: none;
		-webkit-mask-image: linear-gradient(to right, transparent 0%, rgb(0 0 0 / .18) 22%, rgb(0 0 0 / .55) 42%, #000 62%); mask-image: linear-gradient(to right, transparent 0%, rgb(0 0 0 / .18) 22%, rgb(0 0 0 / .55) 42%, #000 62%); }
	.fill { position: absolute; inset: -20px; width: calc(100% + 40px); height: calc(100% + 40px); object-fit: cover; filter: blur(22px) brightness(.55) saturate(1.2); opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.fill.in { opacity: 1; }
	.bg { position: absolute; top: 0; right: 0; height: 100%; width: auto; aspect-ratio: 16 / 9; object-fit: cover; opacity: 0; transform: translateX(10px) scale(1.02);
		-webkit-mask-image: linear-gradient(to right, transparent 0, #000 28%); mask-image: linear-gradient(to right, transparent 0, #000 28%);
		transition: opacity var(--dur-slow) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	.album .bg { object-position: center 25%; }
	.bg.in { opacity: 1; transform: none; }
	/* The logo sits right of centre so the left fade never covers it. */
	.logo { position: absolute; right: 6%; top: 50%; max-width: 30%; max-height: 62%; object-fit: contain; transform: translateY(-50%);
		filter: drop-shadow(0 1px 6px rgb(0 0 0 / 0.55)); opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.logo.in { opacity: 1; }
	:global(.item:hover) .bg.in { transform: scale(1.04); }
	@media (prefers-reduced-motion: reduce) {
		.bg, .logo { transition: none; transform: none; }
		.logo { transform: translateY(-50%); }
		:global(.item:hover) .bg.in { transform: none; }
	}
</style>
