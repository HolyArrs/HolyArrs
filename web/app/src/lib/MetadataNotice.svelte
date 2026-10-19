<script lang="ts">
	import { onMount } from 'svelte';
	import { checkMetadata, metadataHealth, METADATA_DOWN } from '#lib/metadata-health.svelte.ts';

	onMount(() => {
		void checkMetadata();
	});
</script>

{#if !metadataHealth.up}
	<p class="notice warn metadata-down" role="status">{METADATA_DOWN}</p>
{/if}
