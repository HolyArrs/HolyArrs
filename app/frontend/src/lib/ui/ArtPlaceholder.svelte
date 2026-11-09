<script lang="ts">
	// A generated stand-in tile with initials, shown when art is missing.
	import { inkOn, placeholderHue, placeholderText } from '../core/art-placeholder.ts';

	let {
		name,
		shape = 'square',
		loading = false,
		inline = false,
		size,
		color = null
	}: {
		name: string;
		shape?: 'circle' | 'square' | 'poster' | 'wide';
		loading?: boolean;
		inline?: boolean;
		size?: string;
		color?: string | null;
	} = $props();
	const text = $derived(placeholderText(name));
	const hue = $derived(placeholderHue(name));
	// Pick a readable text color for a given tint.
	const ink = $derived(inkOn(color));
</script>

<span class="aph {shape}" class:inline class:loading class:tinted={!!ink} class:ink-dark={ink === 'dark'} class:ink-light={ink === 'light'} style:--h={hue} style:--c={ink ? color : undefined} style:width={inline ? size : undefined} aria-hidden="true" data-testid="art-placeholder">
	{#if text}<span class="t" class:long={text.length > 2}>{text}</span>{/if}
</span>

<style>
	/* Size container lets the letters scale with the box. */
	.aph {
		position: absolute; inset: 0; display: grid; place-items: center; overflow: hidden; container-type: size;
		background: linear-gradient(145deg, hsl(var(--h) 28% 30%), hsl(calc(var(--h) + 25) 30% 18%));
		color: hsl(var(--h) 40% 86%); user-select: none;
	}
	.aph.tinted { background: linear-gradient(145deg, color-mix(in srgb, var(--c) 88%, white), var(--c) 55%, color-mix(in srgb, var(--c) 80%, black)); }
	.aph.ink-light { color: rgb(255 255 255 / .82); }
	.aph.ink-dark { color: rgb(0 0 0 / .58); }
	.aph.inline { position: relative; inset: auto; }
	.aph.circle { border-radius: 50%; }
	.aph.inline.circle, .aph.inline.square { aspect-ratio: 1; }
	.aph.inline.poster { aspect-ratio: 2 / 3; }
	.aph.inline.wide { aspect-ratio: 16 / 9; }
	.t { font: 700 clamp(12px, 30cqmin, 64px) / 1 var(--font-ui); letter-spacing: .02em; opacity: .92; }
	.t.long { font-size: clamp(11px, 20cqmin, 44px); }
	/* Soft sweep shows the art is still loading. */
	.loading::after {
		content: ''; position: absolute; inset: 0;
		background: linear-gradient(100deg, transparent 30%, rgb(255 255 255 / .08) 50%, transparent 70%);
		background-size: 200% 100%; animation: aph-sweep 1.6s ease-in-out infinite;
	}
	@keyframes aph-sweep { from { background-position: 150% 0; } to { background-position: -50% 0; } }
	@media (prefers-reduced-motion: reduce) { .loading::after { animation: none; } }
</style>
