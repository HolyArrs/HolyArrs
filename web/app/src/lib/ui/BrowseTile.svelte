<script lang="ts">
	import { fitTile } from '../tmdb-image.ts';
	// A Search Browse tile: a tall 3:4 picture in grey under the category's own
	// colour wash, the name bottom-left in bold white. Hover lifts it a little.
	let { name, href, image, hue, row = 0, plain = false }: { name: string; href: string; image: string; hue: string; row?: number; plain?: boolean } = $props();
	let loaded = $state(false);
	let img = $state<HTMLImageElement>();
	// The picture in the size this tile is shown at on this screen: TMDB
	// profile sizes for people (w185, w500, original), backdrop sizes for
	// genres (w780, w1280, original).
	let tileW = $state(0);
	const src = $derived(fitTile(image, tileW, plain));
	$effect(() => {
		if (img?.complete && img.naturalWidth > 0) loaded = true;
	});
</script>

<a bind:clientWidth={tileW} class="ui-btile" class:plain {href} style="--hue: {hue}; --row: {row}" data-testid="browse-tile">
	<span class="art">
		<img bind:this={img} src={src} alt="" loading="lazy" decoding="async" class:in={loaded} onload={() => (loaded = true)} />
		<span class="wash" aria-hidden="true"></span>
		<span class="shade" aria-hidden="true"></span>
		<span class="name">{name}</span>
	</span>
</a>

<style>
	.ui-btile { display: block; min-width: 0; border-radius: 16px; color: #fff; }
	.ui-btile:focus-visible { outline: none; }
	.ui-btile:focus-visible .art { box-shadow: var(--focus-ring); }
	.art {
		position: relative; display: block; aspect-ratio: 3 / 4; border-radius: 16px; overflow: hidden; isolation: isolate;
		background: color-mix(in srgb, var(--hue) 70%, #000); box-shadow: var(--shadow-card);
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out);
	}
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; filter: grayscale(1) contrast(1.25) brightness(1.1); opacity: 0; transition: opacity var(--dur) ease, transform var(--dur-slow) var(--ease-out); }
	img.in { opacity: 1; }
	/* Cinematic duotone: the grey picture under the category's colour fading into near-black (strong top, deep bottom). */
	.wash { position: absolute; inset: 0; background: linear-gradient(160deg, var(--hue) 0%, color-mix(in srgb, var(--hue) 18%, #050505) 100%); mix-blend-mode: multiply; opacity: 0.95; }
	.shade { position: absolute; inset: 0; background: linear-gradient(to top, color-mix(in srgb, var(--hue) 35%, #000) 0%, color-mix(in srgb, var(--hue) 30%, transparent) 45%, transparent 75%); opacity: 0.9; }
	.name {
		position: absolute; left: 14px; right: 14px; bottom: 14px; font: 700 clamp(16px, 0.7vw + 8px, 22px) / 1.12 var(--font-display); letter-spacing: -0.015em;
		text-shadow: 0 1px 10px rgb(0 0 0 / 0.45); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; hyphens: auto;
	}
	/* People: the photo stays in colour under a very light wash. */
	.plain img { filter: none; }
	.plain .wash { opacity: 0.12; }
	.plain .shade { background: linear-gradient(to top, rgb(0 0 0 / 0.7) 0%, transparent 55%); }
	@media (max-width: 599px) { .name { left: 12px; right: 12px; bottom: 10px; } }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.ui-btile:hover .art { transform: translateY(-4px); box-shadow: var(--shadow-lift); }
		.ui-btile:hover img { transform: scale(1.04); }
	}
	/* Hover or keyboard focus: the picture in its own full colour at once, no tint and no fade. */
	.ui-btile:hover img, .ui-btile:focus-visible img { filter: none; }
	.ui-btile:hover .wash, .ui-btile:focus-visible .wash { opacity: 0; }
	.ui-btile:hover .shade, .ui-btile:focus-visible .shade { background: linear-gradient(to top, rgb(0 0 0 / 0.7) 0%, transparent 55%); }
</style>
