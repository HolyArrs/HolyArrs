<script lang="ts">
	// Quality badges from what the server read from a Release name, so every list shows the judge's reading.
	import type { ReleaseView } from '#lib/core/types.ts';
	import { readTags } from './releases.ts';

	let { release }: { release: Pick<ReleaseView, 'read'> } = $props();
	const tags = $derived(readTags(release));
</script>

{#each tags as tag (tag)}<span class="rtag" data-testid="release-tag">{tag}</span>{/each}

<style>
	.rtag { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 28%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; }
</style>
