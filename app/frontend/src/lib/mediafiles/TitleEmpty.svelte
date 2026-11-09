<script lang="ts">
	// Empty state card with an icon, message and optional actions.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';

	let {
		icon = 'info',
		title,
		children,
		actions
	}: { icon?: IconName; title: string; children?: Snippet; actions?: Snippet } = $props();
</script>

<div class="empty rise">
	<div class="mark"><Icon name={icon} size={26} /></div>
	<h2 class="h2">{title}</h2>
	{#if children}<div class="text">{@render children()}</div>{/if}
	{#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
	.empty {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--s-3);
		padding: var(--s-8);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.mark {
		width: 48px;
		height: 48px;
		display: grid;
		place-items: center;
		border-radius: var(--r-btn);
		background: var(--surface-3);
		color: var(--text-muted);
	}
	.text {
		color: var(--text-muted);
		max-width: 60ch;
	}
	/* body text comes from a snippet, so its tags are styled from here */
	.text :global(p + p) {
		margin-top: var(--s-2);
	}
	.text :global(code) {
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		color: var(--text);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-3);
		margin-top: var(--s-2);
	}
	/* tighter padding on phones */
	@media (max-width: 767px) {
		.empty {
			padding: var(--s-5);
		}
	}
</style>
