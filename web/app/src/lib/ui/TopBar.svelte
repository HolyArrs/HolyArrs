<script lang="ts">
	import IconButton from './IconButton.svelte';

	// The 56px translucent bar above the content (tablet and desktop):
	// sidebar toggle when collapsed, back/forward on detail pages, a search
	// icon that opens the Search page (hidden there: the page has the one
	// field), live connection and downloads.
	let {
		showToggle,
		controls,
		ontoggle,
		history = false,
		searchHref,
		searchKey,
		live,
		downloads,
		toggle = $bindable()
	}: {
		showToggle: boolean;
		controls: string;
		ontoggle: () => void;
		history?: boolean;
		/** The Search page; none on the Search page itself. */
		searchHref?: string;
		searchKey: string;
		live?: { state: 'off' | 'connecting' | 'live' | 'reconnecting'; word: string };
		downloads?: { count: number; href: string };
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
	<div class="trail">
		{#if searchHref}<IconButton icon="search" href={searchHref} label="Search ({searchKey})" />{/if}
		{#if live}
			<span class="live {live.state}" title={live.word}><i aria-hidden="true"></i><span class="sr-only">{live.word}</span></span>
		{/if}
		{#if downloads}
			<IconButton icon="download" href={downloads.href} label={downloads.count > 0 ? `Downloads, ${downloads.count} active` : 'Downloads'} badge={downloads.count} />
		{/if}
	</div>
</header>

<style>
	.ui-topbar {
		position: sticky; top: 0; z-index: 30; height: var(--topbar); display: grid; grid-template-columns: 1fr auto; align-items: center; gap: 12px;
		padding: 0 var(--gutter); background: color-mix(in srgb, var(--bg) 82%, transparent);
		backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line);
	}
	.lead, .trail { display: flex; align-items: center; gap: 4px; min-width: 0; }
	.trail { justify-content: flex-end; gap: 10px; }
	.live { width: 28px; height: 28px; display: grid; place-items: center; }
	.live i { width: 8px; height: 8px; border-radius: 50%; background: var(--text-faint); }
	.live.live i { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 22%, transparent); }
	.live.connecting i, .live.reconnecting i { background: var(--wait); }
	@media (max-width: 767px) { .ui-topbar { display: none; } }
</style>
