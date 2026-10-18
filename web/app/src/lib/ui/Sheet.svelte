<script lang="ts">
	import type { Snippet } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { reducedMotion } from '../motion.ts';

	// A modal sheet on the native <dialog>: centred on wide screens, from the
	// bottom on phones. Escape, the close button and a backdrop click close it;
	// the browser returns focus to whatever opened it.
	let {
		open = $bindable(false),
		title,
		description,
		size = 'md',
		placement = 'auto',
		hideTitle = false,
		closeLabel = 'Close',
		children,
		footer,
		onclose,
		dialog = $bindable()
	}: {
		open?: boolean;
		title: string;
		description?: string;
		size?: 'sm' | 'md' | 'lg';
		placement?: 'auto' | 'center' | 'bottom' | 'top' | 'side';
		hideTitle?: boolean;
		closeLabel?: string;
		children: Snippet;
		footer?: Snippet;
		onclose?: () => void;
		dialog?: HTMLDialogElement;
	} = $props();

	const id = $props.id();
	let closing = $state(false);

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			closing = false;
			dialog.showModal();
		} else if (!open && dialog.open && !closing) {
			shut();
		}
	});

	function shut() {
		if (!dialog?.open) return;
		if (reducedMotion()) return finish();
		closing = true;
		setTimeout(finish, 160);
	}
	function finish() {
		closing = false;
		dialog?.close();
	}
	function closed() {
		closing = false;
		if (open) open = false;
		onclose?.();
	}
	function cancel(e: Event) {
		e.preventDefault();
		open = false;
	}
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
	footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; padding: 14px 22px calc(18px + env(safe-area-inset-bottom)); border-top: 1px solid var(--line); }
	.ui-sheet::backdrop { background: var(--scrim); }
	.ui-sheet[open] { animation: sheet-in var(--dur) var(--ease-spring) both; }
	.ui-sheet[open]::backdrop { animation: fade-in var(--dur-fast) ease both; }
	.ui-sheet.closing { animation: sheet-out var(--dur-fast) var(--ease-in) both; }
	.ui-sheet.closing::backdrop { animation: fade-out var(--dur-fast) ease both; }
	.at-top { margin-top: min(12vh, 120px); }
	@keyframes sheet-in { from { opacity: 0; transform: scale(.96); } }
	@keyframes sheet-out { to { opacity: 0; transform: scale(.97); } }
	@keyframes fade-in { from { opacity: 0; } }
	@keyframes fade-out { to { opacity: 0; } }
	@keyframes rise-in { from { transform: translateY(100%); } }
	@keyframes sink-out { to { transform: translateY(100%); } }
	@media (max-width: 767px) {
		.at-auto, .at-bottom {
			width: 100vw; max-height: 92dvh; margin: auto 0 0; border-radius: var(--r-modal) var(--r-modal) 0 0;
		}
		.at-auto[open], .at-bottom[open] { animation: rise-in var(--dur) var(--ease-out) both; }
		.at-auto.closing, .at-bottom.closing { animation: sink-out var(--dur-fast) var(--ease-in) both; }
		.at-auto .inner::before, .at-bottom .inner::before { content: ''; align-self: center; width: 36px; height: 5px; margin-top: 8px; border-radius: 3px; background: var(--line-strong); }
		.at-top { margin-top: 12px; }
	}
	.at-side { margin: 0 auto 0 0; height: 100dvh; max-height: 100dvh; width: min(300px, 86vw); border-radius: 0 var(--r-panel) var(--r-panel) 0; }
	.at-side[open] { animation: slide-in var(--dur) var(--ease-out) both; }
	.at-side.closing { animation: slide-out var(--dur-fast) var(--ease-in) both; }
	@keyframes slide-in { from { transform: translateX(-100%); } }
	@keyframes slide-out { to { transform: translateX(-100%); } }
</style>
