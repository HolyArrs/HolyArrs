<script lang="ts">
	import IconButton from './IconButton.svelte';
	import SearchField from './SearchField.svelte';
	import { topBar } from '#lib/scroll-bar.svelte.ts';
	import { topCrumbs } from '#lib/top-crumbs.svelte.ts';

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

<header class="ui-topbar" class:away={topBar.hidden}>
	<div class="lead">
		{#if showToggle}<IconButton bind:node={toggle} icon="sidebar" label="Show sidebar" aria-expanded={false} aria-controls={controls} onclick={ontoggle} />{/if}
		{#if history}
			<IconButton icon="chevronLeft" label="Back" onclick={() => window.history.back()} />
			<IconButton icon="chevronRight" label="Forward" onclick={() => window.history.forward()} />
		{/if}
		{#if topCrumbs.items.length}
			<nav class="crumbs" aria-label="Breadcrumb" data-testid="top-crumbs">
				{#each topCrumbs.items as c, i (i)}
					{#if i}<span class="sep" aria-hidden="true">›</span>{/if}
					{#if c.href}<a href={c.href}>{c.label}</a>{:else}<span class="here" aria-current="page">{c.label}</span>{/if}
				{/each}
			</nav>
		{/if}
	</div>
	<div class="centre"><SearchField {searchKey} /></div>
	<div class="trail"></div>
</header>

<style>
	.ui-topbar {
		position: sticky; top: 0; z-index: 30; height: var(--topbar); display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 12px;
		padding: 0 var(--gutter); background: color-mix(in srgb, var(--bg) 82%, transparent); border-bottom: 1px solid var(--line);
	}
	/* The blur sits on a layer behind the bar, not on the bar itself: a
	   backdrop-filter on the bar would make it a backdrop root, so the search
	   dropdown's own glass could not see the page below and the page showed
	   through it. */
	.ui-topbar::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px); }
	:global(html.over-hero) .ui-topbar::before { -webkit-backdrop-filter: none; backdrop-filter: none; }
	/* Glides with the pills (--bar-glide, app.css): transform only, no fade. */
	.ui-topbar { will-change: transform; }
	.ui-topbar.away { transform: translateY(-100%); pointer-events: none; }
	/* Over a page hero the bar is see-through (no glass, no line); the glass fades in once the hero has scrolled by. */
	.ui-topbar { transition: transform var(--bar-glide), background-color 400ms ease, border-color 400ms ease; }
	:global(html.over-hero) .ui-topbar { background-color: transparent; border-bottom-color: transparent; -webkit-backdrop-filter: none; backdrop-filter: none; }
	@media (prefers-reduced-motion: reduce) { .ui-topbar { transition: none; } }
	.lead, .trail { display: flex; align-items: center; gap: 4px; min-width: 0; }
	/* Where you are, in quiet text after the arrows; hidden where the bar is narrow. */
	.crumbs { display: flex; align-items: center; min-width: 0; margin-left: 12px; font: 500 13px/1 var(--font-ui); color: var(--text-muted); white-space: nowrap; }
	.crumbs a { color: var(--text-muted); text-decoration: none; transition: color 150ms ease; }
	.crumbs a:hover { color: var(--text); }
	.crumbs .sep { margin: 0 6px; opacity: .5; }
	.crumbs .here { overflow: hidden; text-overflow: ellipsis; max-width: 28vw; color: color-mix(in srgb, var(--text) 78%, transparent); }
	@media (max-width: 1099px) { .crumbs { display: none; } }
	@media (max-width: 767px) { .ui-topbar { display: none; } }
</style>
