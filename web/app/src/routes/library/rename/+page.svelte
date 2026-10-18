<script lang="ts">
	import { to } from '#lib/paths.ts';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import { session } from '#lib/session.svelte.ts';
</script>

<svelte:head><title>Rename files · HolyArrs</title></svelte:head>

<div class="page">
	<PageHeader
		title="Rename files"
		subtitle="Every file in the Library whose name does not follow the naming pattern, with its name now and the name it gets. Nothing changes until you press Rename."
		back={{ href: to('/library'), label: 'Library' }}
	/>
	{#if session.can('settings')}<p class="hint">The patterns are in <a href={to(`/settings/naming`)}>Settings &gt; Naming and files</a>.</p>{/if}

	<section class="card" aria-labelledby="rename-h">
		<h2 id="rename-h" class="sr-only">Rename preview</h2>
		<RenamePreview />
	</section>
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.hint { margin: -8px 0 0; color: var(--text-muted); font: var(--t-callout); }
	.hint a { color: var(--text); text-decoration: underline; text-underline-offset: 2px; }
	.card { padding: 16px 20px 20px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); min-width: 0; }
</style>
