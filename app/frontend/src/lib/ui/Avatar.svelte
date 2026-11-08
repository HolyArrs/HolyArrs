<script lang="ts">
	// A round user picture that falls back to a generated placeholder.
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import { coverFor } from '../core/pictures.ts';
	let { name, src = null, size = 32, label }: { name: string; src?: string | null; size?: number; label?: string } = $props();
	let failed = $state(false);
	// A new source gets a fresh try after an earlier load failure.
	$effect(() => {
		src;
		failed = false;
	});
</script>

<span class="ui-avatar" style="--av: {size}px" role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : 'true'}>
	{#if src && !failed}
		<img src={coverFor(src, size).src} alt="" decoding="async" onerror={() => (failed = true)} />
	{:else}
		<ArtPlaceholder {name} shape="circle" />
	{/if}
</span>

<style>
	/* Accent gradient shows behind the image while it loads. */
	.ui-avatar { position: relative; width: var(--av); height: var(--av); flex: none; display: grid; place-items: center; overflow: hidden; border-radius: 50%; background: linear-gradient(150deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #000)); color: var(--text-on-red); font: 600 calc(var(--av) * 0.4)/1 var(--font-ui); user-select: none; }
	img { width: 100%; height: 100%; object-fit: cover; display: block; }
</style>
