<script lang="ts">
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import ReportsList from '#lib/reports/ReportsList.svelte';
	import { session } from '#lib/session.svelte.ts';

	// Reported problems (#95): the owner and admins see and close every
	// report; everyone else sees the ones they made.
	const manage = $derived(session.can('manage_library'));
</script>

<svelte:head><title>Reported problems · HolyArrs</title></svelte:head>

<div class="page">
	<PageHeader
		title={manage ? 'Reported problems' : 'Your reported problems'}
		subtitle={manage
			? 'Problems people found with titles in the Library. Mark one fixed to let them know.'
			: 'Problems you reported with titles in the Library, and what was done about them.'}
	/>
	<ReportsList />
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 8px; padding-bottom: 48px; min-width: 0; }
</style>
