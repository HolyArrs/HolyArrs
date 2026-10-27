<script lang="ts">
	import type { Snippet } from 'svelte';

	// Each Downloads page shows its own header; History has its own sidebar item.
	let { children }: { children: Snippet } = $props();
</script>

{@render children()}
