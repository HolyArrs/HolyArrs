<script lang="ts">
	// The Turned down list now lives as a tab on the History page; old links land there.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { to } from '#lib/paths.ts';

	onMount(() => {
		void goto(to('/downloads/history?tab=turned-down'), { replaceState: true });
	});
</script>

<svelte:head><title>Turned down · HolyArrs</title></svelte:head>
