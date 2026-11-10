<!-- Modal dialog that adapts to a centered box, bottom sheet, drawer or side panel. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Snippet } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { reducedMotion } from '../core/motion.ts';

	let {
		open = $bindable(false),
		title,
		description,
		size = 'md',
		placement = 'auto',
		hideTitle = false,
		closeLabel = t('ui.sheet.close'),
		children,
		footer,
		onclose,
		dialog = $bindable()
	}: {
		open?: boolean;
		title: string;
		description?: string;
		size?: 'sm' | 'md' | 'lg';
		placement?: 'auto' | 'center' | 'bottom' | 'top' | 'side' | 'right' | 'full' | 'drawer';
		hideTitle?: boolean;
		closeLabel?: string;
		children: Snippet;
		footer?: Snippet;
		onclose?: () => void;
		dialog?: HTMLDialogElement;
	} = $props();

	const id = $props.id();
	let closing = $state(false);
	// Fallback timer so the dialog closes even if the exit animation never ends.
	let shutTimer: ReturnType<typeof setTimeout> | undefined;

	// Keeps the native dialog in step with `open`, including a reopen mid-close.
	$effect(() => {
		if (!dialog) return;
		if (open && closing) {
			clearTimeout(shutTimer);
			closing = false;
		} else if (open && !dialog.open) {
			closing = false;
			dialog.showModal();
		} else if (!open && dialog.open && !closing) {
			shut();
		}
	});

	// Plays the exit animation first, unless motion is reduced.
	function shut() {
		if (!dialog?.open) return;
		if (reducedMotion()) return finish();
		closing = true;
		clearTimeout(shutTimer);
		shutTimer = setTimeout(finish, 160);
	}
	function finish() {
		clearTimeout(shutTimer);
		closing = false;
		if (open) return;
		dialog?.close();
	}
	function closed() {
		closing = false;
		if (open) open = false;
		onclose?.();
	}
	// Escape closes through `open` so the exit animation runs.
	function cancel(e: Event) {
		e.preventDefault();
		open = false;
	}
	// A click on the dialog box itself means the backdrop was hit.
	function backdrop(e: MouseEvent) {
		if (e.target === dialog) open = false;
	}
</script>

<dialog
	bind:this={dialog}
	class="ui-sheet {size} at-{placement}"
	class:closing
	aria-labelledby="{id}-t"
	aria-describedby={description ? `${id}-d` : undefined}
	onclose={closed}
	oncancel={cancel}
	onclick={backdrop}
>
	<div class="inner">
		<header class:sr={hideTitle}>
			<div class="titles">
				<h2 id="{id}-t" class:sr-only={hideTitle}>{title}</h2>
				{#if description}<p id="{id}-d" class:sr-only={hideTitle}>{description}</p>{/if}
			</div>
			{#if !hideTitle}<IconButton icon="x" label={closeLabel} variant="tinted" size="sm" tooltip={false} onclick={() => (open = false)} />{/if}
		</header>
		<div class="body">{@render children()}</div>
		{#if footer}<footer>{@render footer()}</footer>{/if}
	</div>
</dialog>

<style>
	.ui-sheet {
		width: min(var(--w), calc(100vw - 32px)); max-width: none; max-height: min(86dvh, 820px);
		margin: auto; padding: 0; border: 0; border-radius: var(--r-modal);
		background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); overflow: hidden;
	}
	.sm { --w: 400px; }
	.md { --w: 560px; }
	.lg { --w: 820px; }
	.inner { display: flex; flex-direction: column; max-height: inherit; }
	header { display: flex; align-items: flex-start; gap: 12px; padding: 18px 18px 8px 22px; }
	header.sr { padding: 0; }
	.titles { flex: 1; min-width: 0; padding-top: 4px; }
	h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	p { margin-top: 4px; color: var(--text-muted); font: var(--t-callout); }
	.body { padding: 8px 22px 22px; overflow: auto; overscroll-behavior: contain; }
	@media (pointer: coarse), (max-width: 600px) { footer :global(.ui-btn) { min-height: 44px; } }
	footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; padding: 14px 22px calc(18px + env(safe-area-inset-bottom)); border-top: 1px solid var(--line); }
	.ui-sheet::backdrop { background: var(--scrim); }
	.ui-sheet[open] { animation: sheet-in var(--dur) var(--ease-spring) both; }
	.ui-sheet[open]::backdrop { animation: fade-in var(--dur-fast) var(--ease-out) both; }
	.ui-sheet.closing { animation: sheet-out var(--dur-fast) var(--ease-in) both; }
	.ui-sheet.closing::backdrop { animation: fade-out var(--dur-fast) var(--ease-out) both; }
	/* Phones turn most placements into bottom sheets with a grab handle. */
	.at-top, .at-full { margin-top: min(12vh, 120px); }
	@keyframes sheet-in { from { opacity: 0; transform: scale(.96); } }
	@keyframes sheet-out { to { opacity: 0; transform: scale(.97); } }
	@keyframes fade-in { from { opacity: 0; } }
	@keyframes fade-out { to { opacity: 0; } }
	@keyframes rise-in { from { transform: translateY(100%); } }
	@keyframes sink-out { to { transform: translateY(100%); } }
	.at-drawer { width: min(1040px, calc(100vw - 32px)); max-height: min(72dvh, 640px); margin: auto auto 0; border-radius: var(--r-modal) var(--r-modal) 0 0; }
	.at-drawer[open] { animation: rise-in var(--dur) var(--ease-out) both; }
	.at-drawer.closing { animation: sink-out var(--dur-fast) var(--ease-in) both; }
	@media (max-width: 767px) {
		.at-drawer { width: 100vw; max-height: 92dvh; }
		.at-drawer .inner::before { content: ''; align-self: center; width: 36px; height: 5px; margin-top: 8px; border-radius: 3px; background: var(--line-strong); }
		.at-auto, .at-bottom {
			width: 100vw; max-height: 92dvh; margin: auto 0 0; border-radius: var(--r-modal) var(--r-modal) 0 0;
		}
		.at-auto[open], .at-bottom[open] { animation: rise-in var(--dur) var(--ease-out) both; }
		.at-auto.closing, .at-bottom.closing { animation: sink-out var(--dur-fast) var(--ease-in) both; }
		.at-auto .inner::before, .at-bottom .inner::before { content: ''; align-self: center; width: 36px; height: 5px; margin-top: 8px; border-radius: 3px; background: var(--line-strong); }
		.at-top { margin-top: 12px; }
		.at-auto .body:last-child, .at-bottom .body:last-child, .at-right .body:last-child { padding-bottom: calc(22px + env(safe-area-inset-bottom)); }
		.at-full { width: 100%; height: 100dvh; max-height: 100dvh; margin: 0; border-radius: 0; padding-top: env(safe-area-inset-top); }
		.at-full .inner { height: 100%; }
		.at-full .body { flex: 1; display: flex; flex-direction: column; }
	}
	.at-side { margin-block: 0; margin-inline: 0 auto; height: 100dvh; max-height: 100dvh; width: min(300px, 86vw); border-radius: 0 var(--r-panel) var(--r-panel) 0; }
	.at-side[open] { animation: slide-in var(--dur) var(--ease-out) both; }
	.at-side.closing { animation: slide-out var(--dur-fast) var(--ease-in) both; }
	@keyframes slide-in { from { transform: translateX(calc(-100% * var(--dir, 1))); } }
	@keyframes slide-out { to { transform: translateX(calc(-100% * var(--dir, 1))); } }
	.at-right { margin-block: 0; margin-inline: auto 0; height: 100dvh; max-height: 100dvh; width: min(var(--w), 100vw); border-radius: var(--r-panel) 0 0 var(--r-panel); }
	.at-right .inner { height: 100%; }
	.at-right[open] { animation: slide-from-right var(--dur) var(--ease-out) both; }
	.at-right.closing { animation: slide-to-right var(--dur-fast) var(--ease-in) both; }
	:global([dir='rtl']) .at-side { border-radius: var(--r-panel) 0 0 var(--r-panel); }
	@media (min-width: 768px) { :global([dir='rtl']) .at-right { border-radius: 0 var(--r-panel) var(--r-panel) 0; } }
	@keyframes slide-from-right { from { transform: translateX(calc(100% * var(--dir, 1))); } }
	@keyframes slide-to-right { to { transform: translateX(calc(100% * var(--dir, 1))); } }
	@media (max-width: 767px) {
		.at-right { width: 100vw; height: auto; max-height: 92dvh; margin: auto 0 0; border-radius: var(--r-modal) var(--r-modal) 0 0; }
		.at-right[open] { animation: rise-in var(--dur) var(--ease-out) both; }
		.at-right.closing { animation: sink-out var(--dur-fast) var(--ease-in) both; }
	}
</style>
