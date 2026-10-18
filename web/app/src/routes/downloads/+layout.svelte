<script lang="ts">
	import type { Snippet } from 'svelte';

	// Each Downloads page shows its own header and the Now / Needs a decision /
	// History link tabs (lib/ui/OpsDownloadsTabs.svelte).
	let { children }: { children: Snippet } = $props();
</script>

{@render children()}
