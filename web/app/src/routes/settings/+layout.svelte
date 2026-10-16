<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import Icon from '#lib/Icon.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	const sections = $derived(visible(all, session.can));

	let { children }: { children: Snippet } = $props();
	const atIndex = $derived(appPath(page.url.pathname) === '/settings');
</script>

<div class="settings" class:at-index={atIndex}>
	<nav class="subnav" aria-label="Settings sections">
		<p class="label xs muted">Settings</p>
		{#each sections as s (s.href)}
			<a href={to(s.href)} aria-current={appPath(page.url.pathname) === s.href ? 'page' : undefined}>
				<Icon name={s.icon} size={18} />{s.label}
			</a>
		{/each}
	</nav>
	<div class="body">
		{#if !atIndex}
			<a class="back small muted" href={to(`/settings`)}><Icon name="back" size={16} /> Settings</a>
		{/if}
		{@render children()}
	</div>
</div>

<style>
	.settings {
		display: grid;
		grid-template-columns: 220px minmax(0, 1fr);
		gap: var(--s-8);
		align-items: start;
	}
	.subnav {
		position: sticky;
		top: calc(var(--topbar) + var(--s-6));
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.label {
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 0 var(--s-3) var(--s-2);
	}
	.subnav a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 40px;
		padding: 0 var(--s-3);
		border-radius: var(--r-btn);
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		transition:
			background var(--dur-fast),
			color var(--dur-fast);
	}
	.subnav a:hover {
		background: var(--surface-3);
		color: var(--text);
	}
	.subnav a[aria-current='page'] {
		background: var(--ha-red-soft);
		color: var(--text);
	}
	.subnav a[aria-current='page'] :global(.icon) {
		color: var(--ha-red);
	}
	.body {
		max-width: 860px;
		min-width: 0;
	}
	.back {
		display: none;
		align-items: center;
		gap: 6px;
		min-height: 40px;
		margin-bottom: var(--s-2);
	}
	@media (max-width: 900px) {
		.settings {
			grid-template-columns: minmax(0, 1fr);
		}
		.subnav {
			display: none;
		}
		.back {
			display: inline-flex;
		}
	}
</style>
