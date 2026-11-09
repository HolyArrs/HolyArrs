<script lang="ts">
	import { onMount } from 'svelte';
	import { checkMetadata } from './health.svelte.ts';

	// Renders nothing; the outage itself shows as a toast.
	onMount(() => {
		void checkMetadata();
	});
</script>
