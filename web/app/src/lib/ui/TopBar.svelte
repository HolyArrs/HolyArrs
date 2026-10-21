<script lang="ts">
	import IconButton from './IconButton.svelte';
	import SearchField from './SearchField.svelte';

	// The 56px translucent bar above the content (tablet and desktop):
	// sidebar toggle when collapsed, back/forward on detail pages, the one
	// search field in the centre.
	let {
		showToggle,
		controls,
		ontoggle,
		history = false,
		searchKey,
		toggle = $bindable()
	}: {
		showToggle: boolean;
		controls: string;
		ontoggle: () => void;
		history?: boolean;
		searchKey: string;
		toggle?: HTMLElement;
	} = $props();
</script>

<header class="ui-topbar">
	<div class="lead">
		{#if showToggle}<IconButton bind:node={toggle} icon="sidebar" label="Show sidebar" aria-expanded={false} aria-controls={controls} onclick={ontoggle} />{/if}
		{#if history}
			<IconButton icon="chevronLeft" label="Back" onclick={() => window.history.back()} />
			<IconButton icon="chevronRight" label="Forward" onclick={() => window.history.forward()} />
		{/if}
	</div>
	<div class="centre"><SearchField {searchKey} /></div>
	<div class="trail"></div>
</header>

<style>
	.ui-topbar {
		position: sticky; top: 0; z-index: 30; height: var(--topbar); display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 12px;
		padding: 0 var(--gutter); background: color-mix(in srgb, var(--bg) 82%, transparent);
		backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line);
	}
	.lead, .trail { display: flex; align-items: center; gap: 4px; min-width: 0; }
	@media (max-width: 767px) { .ui-topbar { display: none; } }
</style>
