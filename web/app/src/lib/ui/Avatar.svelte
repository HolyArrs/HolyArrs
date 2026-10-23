<script lang="ts">
	// A person's round picture, or their initials when they have none (or it fails to load).
	let { name, src = null, size = 32, label }: { name: string; src?: string | null; size?: number; label?: string } = $props();
	const initials = $derived(name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join('') || '?');
	let failed = $state(false);
	// A new picture gets a fresh try.
	$effect(() => {
		src;
		failed = false;
	});
</script>

<span class="ui-avatar" style="--av: {size}px" role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : 'true'}>
	{#if src && !failed}
		<img {src} alt="" decoding="async" onerror={() => (failed = true)} />
	{:else}
		{initials}
	{/if}
</span>

<style>
	.ui-avatar { width: var(--av); height: var(--av); flex: none; display: grid; place-items: center; overflow: hidden; border-radius: 50%; background: linear-gradient(150deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #000)); color: var(--text-on-red); font: 600 calc(var(--av) * 0.4)/1 var(--font-ui); user-select: none; }
	img { width: 100%; height: 100%; object-fit: cover; display: block; }
</style>
