<script lang="ts">
	// A "Browse by genre" card: a 16:9 backdrop from a popular title in the
	// genre, a dark gradient and the genre name bottom-left. No badges, no trailer.
	let { name, href, image, onclick }: { name: string; href: string; image: string; onclick?: (e: MouseEvent) => void } = $props();
	let loaded = $state(false);
	let img = $state<HTMLImageElement>();
	// A picture already in the cache loads before the handler is attached.
	$effect(() => {
		if (img?.complete && img.naturalWidth > 0) loaded = true;
	});
</script>

<a class="ui-genre" {href} {onclick} data-testid="genre-card">
	<span class="art">
		<img bind:this={img} src={image} alt="" loading="lazy" decoding="async" class:in={loaded} onload={() => (loaded = true)} />
		<span class="shade" aria-hidden="true"></span>
		<span class="name">{name}</span>
	</span>
</a>

<style>
	.ui-genre { display: block; min-width: 0; border-radius: var(--r-card); color: #fff; }
	.ui-genre:focus-visible { outline: none; }
	.ui-genre:focus-visible .art { box-shadow: var(--focus-ring); }
	.art { position: relative; display: block; aspect-ratio: var(--wide-ratio, 16 / 9); border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) ease, transform var(--dur-slow) var(--ease-out); }
	img.in { opacity: 1; }
	.shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.82) 0%, rgb(0 0 0 / 0.45) 38%, rgb(0 0 0 / 0.08) 70%, transparent 100%), linear-gradient(to right, rgb(0 0 0 / 0.35), transparent 60%); }
	.name { position: absolute; left: 16px; right: 16px; bottom: 12px; font: var(--weight-semibold) clamp(18px, 1.6vw + 6px, 26px) / var(--leading-tight) var(--font-display); letter-spacing: -0.015em; text-shadow: 0 1px 12px rgb(0 0 0 / 0.5); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.ui-genre:hover .art { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
		.ui-genre:hover img { transform: scale(1.04); }
	}
</style>
