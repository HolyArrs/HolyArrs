<script lang="ts">
	// Centered placeholder with icon, text and one optional action.
	import Icon, { type IconName } from './Icon.svelte';
	import Button from './Button.svelte';

	let {
		icon = 'info',
		title,
		text,
		action,
		compact = false
	}: { icon?: IconName; title: string; text?: string; action?: { label: string; href?: string; onclick?: () => void; icon?: IconName }; compact?: boolean } = $props();
</script>

<div class="ui-empty" class:compact>
	<span class="ic"><Icon name={icon} size={compact ? 22 : 28} /></span>
	<h2>{title}</h2>
	{#if text}<p>{text}</p>{/if}
	{#if action}<div class="act"><Button variant="primary" href={action.href} icon={action.icon} onclick={action.onclick}>{action.label}</Button></div>{/if}
</div>

<style>
	.ui-empty { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 8px; padding: 48px 24px; max-width: 460px; margin: 0 auto; }
	/* Compact fits inside small panels. */
	.compact { padding: 24px 16px; }
	.ic { width: 60px; height: 60px; display: grid; place-items: center; margin-bottom: 6px; border-radius: 50%; background: var(--surface-2); color: var(--text-muted); }
	.compact .ic { width: 44px; height: 44px; }
	h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	p { color: var(--text-muted); font: var(--t-body); }
	.act { margin-top: 10px; }
</style>
