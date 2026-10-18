<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import { page } from '$app/state';
	import { decisions } from '#lib/decisions.svelte.ts';
	import OpsSegments from './OpsSegments.svelte';

	// Now / Needs a decision / History: the three Downloads pages, shown once
	// under each page's header.
	const path = $derived(appPath(page.url.pathname));
	const at = $derived(path.startsWith('/downloads/decide') ? 'decide' : path.startsWith('/downloads/history') ? 'history' : 'now');
</script>

<div class="tabs">
	<OpsSegments
		nav
		label="Downloads"
		value={at}
		options={[
			{ value: 'now', label: 'Now', href: to('/downloads') },
			{ value: 'decide', label: 'Needs a decision', href: to('/downloads/decide'), count: decisions.count, countTestid: 'decide-count' },
			{ value: 'history', label: 'History', href: to('/downloads/history') }
		]}
	/>
</div>

<style>
	.tabs { margin: -8px 0 24px; max-width: 100%; }
	.tabs :global(.ops-seg) { width: min(560px, 100%); }
</style>
