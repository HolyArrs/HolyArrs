<script lang="ts">
	import Icon from '../Icon.svelte';
	import IconButton from './IconButton.svelte';

	// The 56px translucent bar above the content (tablet and desktop):
	// sidebar toggle when collapsed, back/forward on detail pages, the search
	// field that opens the command palette, live connection and downloads.
	let {
		showToggle,
		controls,
		ontoggle,
		history = false,
		onsearch,
		searchKey,
		live,
		downloads,
		toggle = $bindable()
	}: {
		showToggle: boolean;
		controls: string;
		ontoggle: () => void;
		history?: boolean;
		onsearch: () => void;
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
	<button class="search" type="button" onclick={onsearch} aria-haspopup="dialog" aria-keyshortcuts={searchKey === '⌘K' ? 'Meta+K' : 'Control+K'}>
		<Icon name="search" size={16} /><span class="ph">Search…</span><kbd>{searchKey}</kbd>
	</button>
	<div class="trail">
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
		position: sticky; top: 0; z-index: 30; height: var(--topbar); display: grid; grid-template-columns: 1fr minmax(200px, 420px) 1fr; align-items: center; gap: 12px;
		padding: 0 var(--gutter); background: color-mix(in srgb, var(--bg) 82%, transparent);
		backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line);
	}
	.lead, .trail { display: flex; align-items: center; gap: 4px; min-width: 0; }
	.trail { justify-content: flex-end; gap: 10px; }
	.search { display: flex; align-items: center; gap: 8px; height: 36px; padding: 0 8px 0 12px; border-radius: 10px; background: var(--surface-2); color: var(--text-muted); font: var(--t-callout); text-align: left; transition: background var(--dur-fast); }
	.search:hover { background: var(--surface-3); }
	.search:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ph { flex: 1; color: var(--text-muted); }
	kbd { padding: 2px 6px; border-radius: 5px; background: var(--surface-1); color: var(--text-muted); font: var(--t-footnote); box-shadow: 0 0 0 1px var(--line); }
	.live { width: 28px; height: 28px; display: grid; place-items: center; }
	.live i { width: 8px; height: 8px; border-radius: 50%; background: var(--text-faint); }
	.live.live i { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 22%, transparent); }
	.live.connecting i, .live.reconnecting i { background: var(--wait); }
	@media (max-width: 1023px) { .ui-topbar { grid-template-columns: auto 1fr auto; } }
	@media (max-width: 767px) { .ui-topbar { display: none; } }
</style>
