<script lang="ts">
	import Icon from '../Icon.svelte';

	// One toast. ToastList stacks and animates them (slide up 12px + fade).
	let { text, tone = 'info', action, ondismiss }: { text: string; tone?: 'ok' | 'fail' | 'info'; action?: { label: string; run: () => void }; ondismiss?: () => void } = $props();
	const icon = $derived(tone === 'ok' ? 'check' : tone === 'fail' ? 'alert' : 'info');
</script>

<div class="ui-toast {tone}">
	<span class="ic"><Icon name={icon} size={16} /></span>
	<span class="grow">{text}</span>
	{#if action}<button class="act" type="button" data-testid="toast-action" onclick={action.run}>{action.label}</button>{/if}
	{#if ondismiss}<button class="x" type="button" aria-label="Close message" onclick={ondismiss}><Icon name="x" size={15} /></button>{/if}
</div>

<style>
	.ui-toast { pointer-events: auto; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 6px 6px 6px 12px; border-radius: 14px; background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-lift), 0 0 0 1px var(--line); font: var(--t-callout); font-weight: 500; }
	.ic { width: 24px; height: 24px; flex: none; display: grid; place-items: center; border-radius: 50%; background: var(--surface-3); color: var(--text); }
	.ok .ic { background: var(--ok); color: var(--on-art); }
	.fail .ic { background: var(--fail); color: var(--on-art); }
	.grow { flex: 1; min-width: 0; }
	.x { width: 36px; height: 36px; flex: none; display: grid; place-items: center; border-radius: 50%; color: var(--text-muted); }
	.act { min-height: 36px; padding: 0 12px; border-radius: 999px; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; flex: none; }
	.act:hover { background: var(--surface-3); }
	.act:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.x:hover { background: var(--surface-3); color: var(--text); }
	.x:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .x { width: 44px; height: 44px; } }
</style>
