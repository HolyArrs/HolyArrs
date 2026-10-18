<script lang="ts">
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { to } from '#lib/paths.ts';
</script>

<svelte:head><title>Explore · HolyArrs</title></svelte:head>

<PageHeader title="Explore" subtitle="Find something new to watch or listen to." />
<EmptyState icon="compass" title="Explore is on its way" text="Rows of titles to browse will show here soon." action={{ label: 'Open the Library', href: to('/library'), icon: 'library' }} />
