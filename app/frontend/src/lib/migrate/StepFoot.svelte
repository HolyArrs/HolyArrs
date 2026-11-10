<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Snippet } from 'svelte';
	import Button from '#lib/ui/Button.svelte';

	let {
		back,
		backLabel,
		backDisabled = false,
		label,
		onclick,
		disabled = false,
		busy = false,
		reason = '',
		variant = 'primary',
		more
	}: {
		back?: () => void;
		backLabel?: string;
		backDisabled?: boolean;
		label?: string;
		onclick?: () => void;
		disabled?: boolean;
		busy?: boolean;
		reason?: string;
		variant?: 'primary' | 'destructive';
		more?: Snippet;
	} = $props();
</script>

<div class="step-foot" data-testid="step-foot">
	{#if back}<Button variant="plain" icon="chevronLeft" onclick={back} disabled={backDisabled}>{backLabel ?? t('migrate.back')}</Button>{/if}
	{#if more}<div class="more">{@render more()}</div>{/if}
	<div class="go">
		<p class="reason hint" id="move-reason" aria-live="polite">{reason}</p>
		{#if label}
			<Button {variant} id="move-primary" {onclick} {disabled} {busy} aria-describedby={reason ? 'move-reason' : undefined}>{label}</Button>
		{/if}
	</div>
</div>

<style>
	.step-foot { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; margin-top: 4px; }
	.more { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
	.go { display: flex; align-items: center; justify-content: flex-end; gap: 12px; margin-left: auto; min-width: 0; }
	.reason { margin: 0; min-width: 0; text-align: right; }
	.reason:empty { display: none; }
	@media (min-width: 768px) {
		.step-foot :global(.ui-link-btn.md) { min-height: 32px; padding-inline: 12px; font-size: 14px; }
	}
	@media (max-width: 767px) {
		.step-foot {
			position: sticky;
			bottom: calc(var(--tabbar, 60px) + env(safe-area-inset-bottom, 0px) + 8px);
			z-index: 20;
			padding: 10px 12px;
			border-radius: var(--r-panel);
			background: var(--surface-1);
			box-shadow: 0 0 0 1px var(--line-strong), var(--shadow-lift);
		}
		div.step-foot :global(:is(.ui-btn, .ui-link-btn)) { min-height: 44px; }
		.step-foot:has(> .more) { position: static; }
		.go { flex-direction: column; align-items: flex-end; gap: 6px; flex: 1 1 auto; }
		.reason { text-align: right; }
	}
</style>
