<script lang="ts">
	// The copies HolyArrs turned down (by hand, or on its own after a failed
	// download), reached from the Downloads toolbar. Undo lets a copy be picked again.
	import ReleaseList from '#lib/ReleaseList.svelte';
</script>

<svelte:head><title>Turned down · HolyArrs</title></svelte:head>

<div class="pad"><ReleaseList list="turned-down" from="downloads" /></div>

<style>
	.pad { padding: var(--s-6) var(--gutter); min-width: 0; }
</style>
