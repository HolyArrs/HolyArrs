<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { decisions } from '#lib/decisions.svelte.ts';

	let { children }: { children: Snippet } = $props();
	const onDecide = $derived(appPath(page.url.pathname).startsWith('/downloads/decide'));
	const onHistory = $derived(appPath(page.url.pathname).startsWith('/downloads/history'));
</script>

<nav class="subnav" aria-label="Downloads">
	<a href={to(`/downloads`)} aria-current={onDecide || onHistory ? undefined : 'page'}>Downloads</a>
	<a href={to(`/downloads/decide`)} aria-current={onDecide ? 'page' : undefined}>
		Needs a decision
		{#if decisions.count > 0}<b class="count" data-testid="decide-count">{decisions.count}</b>{/if}
	</a>
	<a href={to(`/downloads/history`)} aria-current={onHistory ? 'page' : undefined}>History</a>
</nav>

{@render children()}

<style>
	.subnav {
		display: flex;
		gap: var(--s-2);
		margin-bottom: var(--s-4);
		border-bottom: 1px solid var(--line);
		overflow-x: auto;
	}
	.subnav a {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		padding: 0 var(--s-3);
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		border-bottom: 2px solid transparent;
		white-space: nowrap;
	}
	.subnav a[aria-current='page'] {
		color: var(--text);
		border-bottom-color: var(--info);
	}
	.count {
		min-width: 20px;
		height: 20px;
		padding: 0 6px;
		border-radius: var(--r-sm);
		background: var(--info);
		color: var(--bg);
		font: var(--weight-semibold) var(--text-xs) / 20px var(--font-ui);
		text-align: center;
	}
</style>
