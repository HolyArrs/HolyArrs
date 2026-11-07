<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	// One page-level "More options" disclosure: the rare and technical
	// settings of a settings page, closed until opened. A search hit inside
	// opens it (the settings layout opens the nearest details).
	let { children, label = 'More options', id }: { children: Snippet; label?: string; id?: string } = $props();
</script>

<details class="more-options" {id} data-testid="more-options">
	<summary><Icon name="chevronRight" size={15} /><span>{label}</span></summary>
	<div class="more-body">{@render children()}</div>
</details>

<style>
	.more-options { margin-top: 16px; }
	summary {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 44px;
		padding: 0 6px;
		cursor: pointer;
		list-style: none;
		color: var(--text-muted);
		font: var(--t-headline);
		font-size: 15px;
	}
	summary::-webkit-details-marker { display: none; }
	summary:hover { color: var(--text); }
	summary :global(.icon) { transition: transform var(--dur-fast) var(--ease-out); }
	.more-options[open] summary :global(.icon) { transform: rotate(90deg); }
	.more-body { display: flex; flex-direction: column; gap: 16px; padding-top: 4px; }
	@media (prefers-reduced-motion: reduce) { summary :global(.icon) { transition: none; } }
</style>
