<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { RenamePreview, LibraryToolPage } from '#lib/mediafiles/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';

	let changeCount = $state(0);
</script>

<svelte:head><title>Rename files · HolyArrs</title></svelte:head>

<LibraryToolPage title="Rename files" count={changeCount ? `${changeCount} ${changeCount === 1 ? 'file' : 'files'}` : ''}>
	{#snippet bar()}
		{#if session.can('settings')}
			<span class="tp-gap"></span>
			<span class="tp-cap"><Button href={to('/settings/naming')}>Naming settings</Button></span>
		{/if}
	{/snippet}

	<RenamePreview page bind:changeCount />
</LibraryToolPage>
