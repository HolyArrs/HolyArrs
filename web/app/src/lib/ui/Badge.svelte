<script lang="ts">
	// Quality and rating marks: 4K, HDR, Dolby Vision, Atmos, CC, PG-13.
	// Neutral and outlined; `on="art"` for use over artwork.
	let { label, on = 'surface', title }: { label: string; on?: 'surface' | 'art'; title?: string } = $props();
</script>

<span class="ui-badge {on}" {title}>{label}</span>

<style>
	.ui-badge { display: inline-flex; align-items: center; height: 20px; padding: 0 6px; border-radius: 5px; border: 1px solid var(--line-strong); color: var(--text-muted); font: var(--t-caption); letter-spacing: .01em; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.art { border-color: rgb(255 255 255 / 55%); color: var(--on-art); background: rgb(0 0 0 / 28%); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px); }
</style>
