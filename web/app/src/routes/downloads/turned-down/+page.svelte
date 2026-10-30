<script lang="ts">
	// The copies HolyArrs turned down (by hand, or on its own after a failed
	// download), reached from the Downloads toolbar. Undo lets a copy be picked again.
	// Laid out like the Downloads page (DlTurnedDown); the Library keeps ReleaseList.
	import DlTurnedDown from '#lib/downloads/DlTurnedDown.svelte';
</script>

<svelte:head><title>Turned down · HolyArrs</title></svelte:head>

<DlTurnedDown />
