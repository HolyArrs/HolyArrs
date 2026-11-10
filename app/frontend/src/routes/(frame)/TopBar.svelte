<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { SearchField, topBar, cameFrom, pageLabel, type CameFrom } from '#lib/discover/index.ts';
	import { afterNavigate } from '$app/navigation';
	import { topCrumbs } from '#lib/core/top-crumbs.svelte.ts';

	// The 56px translucent bar above the content (tablet and desktop): sidebar toggle when collapsed, back and forward on detail pages, and the one search field in the centre.
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

	// Where you came from (read after each move, once the layout has noted it) and this page's name.
	let from = $state<CameFrom | null>(null);
	let titleName = $state('');
	// After stepping back: the scroll to return to, kept until the page has grown tall enough (rows that load late would otherwise leave you higher up than where you were).
	let pendingY: number | null = null;
	afterNavigate(() => {
		from = cameFrom();
		titleName = pageLabel(document.title);
		const y = pendingY;
		pendingY = null;
		if (y === null) return;
		const until = performance.now() + 4000;
		const step = () => {
			if (document.documentElement.scrollHeight >= y + window.innerHeight || performance.now() > until) {
				window.scrollTo(0, y);
				return;
			}
			window.scrollTo(0, y);
			requestAnimationFrame(step);
		};
		requestAnimationFrame(step);
	});
	$effect(() => {
		const mo = new MutationObserver(() => (titleName = pageLabel(document.title)));
		mo.observe(document.head, { childList: true, characterData: true, subtree: true });
		return () => mo.disconnect();
	});
	const first = $derived(from?.label ? { label: from.label, href: from.path, back: true } : topCrumbs.parent ? { ...topCrumbs.parent, back: false } : null);
	const here = $derived(topCrumbs.here || titleName);
	// The first link steps back (scroll and filters come back with it); a page opened fresh goes to its parent.
	function stepBack(e: MouseEvent) {
		if (!first?.back || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		pendingY = from?.y ?? null;
		window.history.back();
	}
</script>

<header class="ui-topbar" class:away={topBar.hidden}>
	<div class="lead">
		{#if showToggle || history}
			<div class="glass nav">
				{#if showToggle}<IconButton bind:node={toggle} icon="sidebar" label={t('frame.sidebar.show')} aria-expanded={false} aria-controls={controls} onclick={ontoggle} />{/if}
				{#if history}
					<IconButton icon="chevronLeft" label={t('frame.top.back')} onclick={() => window.history.back()} />
					<IconButton icon="chevronRight" label={t('frame.top.forward')} onclick={() => window.history.forward()} />
				{/if}
			</div>
		{/if}
		{#if history && (first || here)}
			<nav class="crumbs" aria-label={t('frame.top.breadcrumb')} data-testid="top-crumbs">
				{#if first}<a href={first.href} onclick={stepBack}>{first.label}</a><span class="sep" aria-hidden="true">›</span>{/if}
				{#if here}<span class="here" aria-current="page" title={here}>{here}</span>{/if}
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
	/* The blur sits on a layer behind the bar, not on the bar itself: a backdrop-filter on the bar would make it a backdrop root, so the search dropdown's own glass could not see the page below and the page showed through it. */
	.ui-topbar::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px); }
	:global(html.over-hero) .ui-topbar::before { -webkit-backdrop-filter: none; backdrop-filter: none; }
	/* Glides with the pills (--bar-glide, app.css): transform only, no fade. */
	.ui-topbar { will-change: transform; }
	.ui-topbar.away { transform: translateY(-100%); pointer-events: none; }
	/* Over a page hero the bar is see-through (no glass, no line); the glass fades in once the hero has scrolled by. */
	.ui-topbar { transition: transform var(--bar-glide), background-color var(--dur-slow) var(--ease-out), border-color var(--dur-slow) var(--ease-out); }
	/* Over the poster wall the bar is lighter glass, so the wall shows through like the page. */
	:global(html:has([data-poster-wall])) .ui-topbar { background-color: color-mix(in srgb, var(--bg) 62%, transparent); border-bottom-color: color-mix(in srgb, var(--text) 8%, transparent); }
	:global(html.over-hero) .ui-topbar { background-color: transparent; border-bottom-color: transparent; -webkit-backdrop-filter: none; backdrop-filter: none; }
	@media (prefers-reduced-motion: reduce) { .ui-topbar { transition: none; } }
	.lead, .trail { display: flex; align-items: center; gap: 4px; min-width: 0; }
	/* Where you are, in quiet text after the arrows; hidden where the bar is narrow. */
	/* A glass pill, so the trail stays readable over any hero frame, bright or black, in both themes. */
	/* Same glass as the search field: translucent, blurred, soft shadow. */
	.glass, .crumbs { border-radius: 999px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); box-shadow: 0 8px 24px rgb(0 0 0 / 0.18), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); }
	.nav { display: flex; align-items: center; height: 44px; padding: 0 2px; }
	.nav :global(button) { width: 38px; height: 38px; border-radius: 999px; }
	.crumbs { display: flex; align-items: center; min-width: 0; margin-inline-start: 8px; height: 44px; padding: 0 18px; font: 500 13px/1 var(--font-ui); color: var(--text-muted); white-space: nowrap; }
	/* The place you came from gives way (with an ellipsis) before this page's name collapses. */
	.crumbs a { display: block; line-height: 24px; min-width: 0; max-width: 16vw; overflow: hidden; text-overflow: ellipsis; color: var(--text-muted); text-decoration: none; transition: color var(--dur-fast) var(--ease-out); }
	.crumbs a:hover { color: var(--text); }
	.crumbs .sep { margin: 0 6px; opacity: .5; }
	/* Long page names (other languages) wrap to a second line instead of cutting off; the search stays centred. */
	.crumbs .here { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; white-space: normal; overflow-wrap: break-word; min-width: 0; line-height: 1.15; max-width: 28vw; color: var(--text); font-weight: 600; }
	@media (max-width: 1099px) { .crumbs { display: none; } }
	@media (max-width: 767px) { .ui-topbar { display: none; } }
</style>
