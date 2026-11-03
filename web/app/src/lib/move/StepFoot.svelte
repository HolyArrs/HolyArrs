<script lang="ts">
	import type { Snippet } from 'svelte';
	import Button from '../ui/Button.svelte';

	// A step's footer: Back on the left, the step's one primary on the right,
	// with the reason it waits. Sticky above the tab bar on phones.
	let {
		back,
		backLabel = 'Back',
		backDisabled = false,
		label,
		onclick,
		disabled = false,
		busy = false,
		reason = '',
		variant = 'primary',
		more
	}: {
		/** Back's action; no Back button without it. */
		back?: () => void;
		backLabel?: string;
		backDisabled?: boolean;
		/** The primary's words; no primary without them. */
		label?: string;
		onclick?: () => void;
		disabled?: boolean;
		busy?: boolean;
		reason?: string;
		variant?: 'primary' | 'destructive';
		/** More buttons between Back and the primary (Undo, the report). */
		more?: Snippet;
	} = $props();
</script>

<div class="step-foot" data-testid="step-foot">
	{#if back}<Button variant="plain" icon="chevronLeft" onclick={back} disabled={backDisabled}>{backLabel}</Button>{/if}
	{#if more}<div class="more">{@render more()}</div>{/if}
	<div class="go">
		<!-- Always drawn, so a reason that appears later is read out. -->
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
	/* The report link matches the Buttons next to it (.settings-content .ui-btn.md). */
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
		/* div. outranks the settings button sizes, so every footer button is 44px. */
		div.step-foot :global(:is(.ui-btn, .ui-link-btn)) { min-height: 44px; }
		/* Done's three buttons wrap; there is no reason to keep in view, so it scrolls with the page. */
		.step-foot:has(> .more) { position: static; }
		/* The reason sits above the button on phones. */
		.go { flex-direction: column; align-items: flex-end; gap: 6px; flex: 1 1 auto; }
		.reason { text-align: right; }
	}
</style>
