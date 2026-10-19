<script lang="ts">
	// Quality and rating marks: 4K, HDR, Dolby Vision, Atmos, CC, PG-13.
	// Neutral and outlined; `on="art"` for use over artwork.
	// Over art it is the shared glass pill (ArtPill), so day tags, status and type all match.
	import ArtPill from './ArtPill.svelte';
	let { label, on = 'surface', title }: { label: string; on?: 'surface' | 'art'; title?: string } = $props();
</script>

{#if on === 'art'}<ArtPill text={label} label={title} />{:else}<span class="ui-badge {on}" {title}>{label}</span>{/if}

<style>
	.ui-badge { display: inline-flex; align-items: center; height: 20px; padding: 0 6px; border-radius: 5px; border: 1px solid var(--line-strong); color: var(--text-muted); font: var(--t-caption); letter-spacing: .01em; white-space: nowrap; font-variant-numeric: tabular-nums; }
</style>
