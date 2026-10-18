<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '../Icon.svelte';

	// A grouped settings block: title, one-line description, content, optional footer (Save).
	let { title, description, icon, children, footer }: { title: string; description?: string; icon?: IconName; children: Snippet; footer?: Snippet } = $props();
	const id = $props.id();
</script>

<section class="ui-settings">
	<header>
		{#if icon}<span class="ic"><Icon name={icon} size={18} /></span>{/if}
		<div>
			<h2 id="{id}-t">{title}</h2>
			{#if description}<p>{description}</p>{/if}
		</div>
	</header>
	<div class="body">{@render children()}</div>
	{#if footer}<footer>{@render footer()}</footer>{/if}
</section>

<style>
	.ui-settings { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	header { display: flex; gap: 12px; align-items: flex-start; padding: 18px 20px 6px; }
	.ic { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border-radius: 8px; background: var(--accent-soft); color: var(--accent-text); }
	h2 { font: var(--t-headline); font-size: 17px; }
	p { margin-top: 2px; color: var(--text-muted); font: var(--t-callout); }
	.body { padding: 10px 20px 18px; display: flex; flex-direction: column; gap: 12px; }
	footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; padding: 12px 20px; background: var(--surface-2); border-top: 1px solid var(--line); }
</style>
