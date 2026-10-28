<script lang="ts">
	import { bannerArt, type BannerEntry } from '#lib/calendar.ts';
	import { sized } from '#lib/tmdb-image.ts';

	// A List row's wide banner (TV-banner style): the title's textless art
	// with its logo over it, filling the row's right side and fading into
	// the row on its left. A picture that fails moves on to the next one;
	// with none left (or no logo) the banner keeps what still loads.
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
			<img class="bg" class:in={shown} {src} alt="" loading="lazy" decoding="async" onload={() => (shown = true)} onerror={() => ((shown = false), at++)} />
		{/key}
	{/if}
	{#if logo}<img class="logo" class:in={shown || !src} src={logo} alt="" loading="lazy" decoding="async" onerror={() => (logoOk = false)} />{/if}
</span>

<style>
	.cal-banner { position: absolute; inset: 0; overflow: hidden; background: var(--surface-2); pointer-events: none;
		-webkit-mask-image: linear-gradient(to right, transparent 0%, rgb(0 0 0 / .18) 22%, rgb(0 0 0 / .55) 42%, #000 62%); mask-image: linear-gradient(to right, transparent 0%, rgb(0 0 0 / .18) 22%, rgb(0 0 0 / .55) 42%, #000 62%); }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 30%; opacity: 0; transform: translateX(10px) scale(1.02);
		transition: opacity var(--dur-slow) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	.album .bg { object-position: center 25%; }
	.bg.in { opacity: 1; transform: none; }
	/* The logo sits right of centre so the left fade never eats it; a soft shadow keeps it clear on bright art. */
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
