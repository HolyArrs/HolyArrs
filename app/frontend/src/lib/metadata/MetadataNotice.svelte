<script lang="ts">
	import { onMount } from 'svelte';
	import { checkMetadata } from './health.svelte.ts';

	// Asks for a fresh health answer where metadata matters. The outage itself
	// shows as a notification (ToastList), not as a banner on the page.
	onMount(() => {
		void checkMetadata();
	});
</script>
