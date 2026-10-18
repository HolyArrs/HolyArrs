<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import { page } from '$app/state';
	import { decisions } from '#lib/decisions.svelte.ts';

	// Now / Needs a decision / History: the three Downloads pages, as link
	// tabs under each page's header (aria-current marks the open page).
	const path = $derived(appPath(page.url.pathname));
	const at = $derived(path.startsWith('/downloads/decide') ? 'decide' : path.startsWith('/downloads/history') ? 'history' : 'now');
</script>

<nav class="tabs" aria-label="Downloads">
	<a href={to('/downloads')} aria-current={at === 'now' ? 'page' : undefined}>Now</a>
	<a href={to('/downloads/decide')} aria-current={at === 'decide' ? 'page' : undefined}>
		<span class="long">Needs a decision</span><span class="short" aria-hidden="true">Decide</span>
		{#if decisions.count}<b class="n" data-testid="decide-count">{decisions.count}</b>{/if}
	</a>
	<a href={to('/downloads/history')} aria-current={at === 'history' ? 'page' : undefined}>History</a>
</nav>

<style>
	.tabs { display: flex; gap: 4px; margin: -8px 0 24px; max-width: 100%; border-bottom: 1px solid var(--line); }
	a { position: relative; display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 12px; border-radius: 8px 8px 0 0; color: var(--text-muted); font: var(--t-subhead); font-size: 15px; white-space: nowrap; transition: color var(--dur-fast); }
	a::after { content: ''; position: absolute; left: 12px; right: 12px; bottom: -1px; height: 2px; border-radius: 2px; background: transparent; transition: background var(--dur-fast); }
	a:hover { color: var(--text); }
	a[aria-current='page'] { color: var(--text); font-weight: 600; }
	a[aria-current='page']::after { background: var(--accent); }
	a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.short { display: none; }
	.n { min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red); font: var(--t-caption); line-height: 20px; text-align: center; font-variant-numeric: tabular-nums; }
	@media (max-width: 480px) {
		.tabs { gap: 0; }
		a { flex: 1 1 0; justify-content: center; padding: 0 8px; }
		a::after { left: 8px; right: 8px; }
		.long { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
		.short { display: inline; }
	}
	@media (prefers-reduced-motion: reduce) { a, a::after { transition: none; } }
</style>
