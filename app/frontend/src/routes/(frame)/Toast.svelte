<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import type { IconName } from '#lib/ui/Icon.svelte';
	import { AlertSymbol } from '#lib/alerts/index.ts';

	// One notification: a dark glass pill with a filled symbol whose shape and colour carry the tone, a short title and an optional second line. `compact` shrinks it to a small pill (tap to open again). × or a swipe up closes it.
	type Tone = 'ok' | 'fail' | 'info' | 'warn';
	let {
		text,
		title,
		tone = 'info',
		compact = false,
		action,
		ondismiss,
		onexpand,
		onhold,
		onlet
	}: {
		text: string;
		title?: string;
		tone?: Tone;
		/** Kept for callers; the symbol follows the tone. */
		icon?: IconName;
		/** Kept for callers; not shown. */
		time?: string;
		compact?: boolean;
		action?: { label: string; run: () => void };
		ondismiss?: () => void;
		onexpand?: () => void;
		/** Hover or focus: hold the timer. */
		onhold?: () => void;
		onlet?: () => void;
	} = $props();

	// Swipe up to dismiss (the pill sits at the top of the screen).
	let dy = $state(0);
	let startY: number | null = null;
	function down(e: PointerEvent) {
		if (!ondismiss || e.button !== 0 || (e.target as Element).closest('button')) return;
		startY = e.clientY;
		(e.currentTarget as Element).setPointerCapture?.(e.pointerId);
	}
	function move(e: PointerEvent) {
		if (startY === null) return;
		dy = Math.min(0, e.clientY - startY);
	}
	function up() {
		if (startY === null) return;
		startY = null;
		if (dy < -24) ondismiss?.();
		dy = 0;
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'Escape' && ondismiss) {
			e.stopPropagation();
			ondismiss();
		}
	}
	// Held while the pointer is over it or focus is inside it; let go only when neither.
	let hovering = false;
	let focused = false;
	function hold(kind: 'hover' | 'focus') {
		if (kind === 'hover') hovering = true;
		else focused = true;
		onhold?.();
	}
	function release(kind: 'hover' | 'focus') {
		if (kind === 'hover') hovering = false;
		else focused = false;
		if (!hovering && !focused) onlet?.();
	}
	function out(e: FocusEvent) {
		if (!(e.currentTarget as Element).contains(e.relatedTarget as Node | null)) release('focus');
	}
</script>

{#snippet symbol(size: number)}<AlertSymbol {tone} {size} />{/snippet}

{#if compact}
	<button type="button" class="ui-toast pill {tone}" onclick={onexpand} aria-label={t('frame.toast.details', { title: title ?? text })}>
		{@render symbol(20)}
		<span class="pill-t">{title ?? text}</span>
	</button>
{:else}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="ui-toast card {tone}"
		style:transform={dy ? `translateY(${dy}px)` : undefined}
		style:opacity={dy ? Math.max(0.3, 1 + dy / 90) : undefined}
		class:drag={dy !== 0}
		onpointerdown={down}
		onpointermove={move}
		onpointerup={up}
		onpointercancel={up}
		onkeydown={key}
		onpointerenter={() => hold('hover')}
		onpointerleave={() => release('hover')}
		onfocusin={() => hold('focus')}
		onfocusout={out}
	>
		{@render symbol(30)}
		<span class="body">
			<strong class="title">{title ?? text}</strong>
			{#if title}<span class="text">{text}</span>{/if}
		</span>
		{#if action}<button class="act" type="button" data-testid="toast-action" onclick={action.run}>{action.label}</button>{/if}
		{#if ondismiss}<button class="x" type="button" aria-label={t('frame.toast.dismiss')} onclick={ondismiss}><Icon name="x" size={10} stroke={2.6} /></button>{/if}
	</div>
{/if}

<style>
	/* Always dark glass so it reads the same over any page or artwork. */
	.ui-toast {
		position: relative;
		pointer-events: auto;
		color: #f5f5f7;
		background: rgb(28 28 32 / 82%);
		-webkit-backdrop-filter: blur(40px) saturate(180%);
		backdrop-filter: blur(40px) saturate(180%);
		box-shadow: 0 18px 50px rgb(0 0 0 / 42%), 0 2px 6px rgb(0 0 0 / 22%), inset 0 1px 0 rgb(255 255 255 / 9%), inset 0 0 0 0.5px rgb(255 255 255 / 7%);
		border-radius: 999px;
	}
	.card { display: flex; align-items: center; gap: 12px; min-height: 58px; padding: 9px 18px 9px 14px; touch-action: pan-x; transition: transform var(--dur) var(--ease-spring), opacity var(--dur) var(--ease-out); }
	.card.drag { transition: none; }
	.body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.title { font: var(--t-callout); font-size: 14.5px; font-weight: 600; letter-spacing: -0.01em; line-height: 1.25; overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.text { font: var(--t-subhead); font-size: 12.5px; color: rgb(245 245 247 / 62%); overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.pill { align-self: flex-end; display: inline-flex; align-items: center; gap: 8px; height: 36px; max-width: 100%; padding: 0 14px 0 8px; font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill-t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pill:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.act { flex: none; min-height: 32px; padding: 0 14px; border-radius: 999px; color: #f5f5f7; background: rgb(255 255 255 / 14%); font: var(--t-subhead); font-size: 13px; font-weight: 600; }
	.act:hover { background: rgb(255 255 255 / 22%); }
	/* × on the top-right corner: on hover or focus with a mouse, always on touch. */
	.x { position: absolute; top: -6px; inset-inline-end: -6px; width: 22px; height: 22px; display: grid; place-items: center; border-radius: 50%; color: #f5f5f7; background: rgb(58 58 64 / 90%); -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); box-shadow: 0 2px 8px rgb(0 0 0 / 40%), inset 0 0 0 0.5px rgb(255 255 255 / 14%); opacity: 0; transform: scale(0.85); transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.card:hover .x, .card:focus-within .x { opacity: 1; transform: none; }
	.act:focus-visible, .x:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) {
		.x { opacity: 1; transform: none; }
		/* A bigger hit area around the small dot. */
		.x::after { content: ''; position: absolute; inset: -11px; }
		.act { min-height: 40px; }
	}
	@media (max-width: 767px) {
		.card { min-height: 54px; padding: 8px 16px 8px 12px; }
		.card :global(.ui-alert-sym) { width: 28px; height: 28px; }
	}
	@media (prefers-reduced-motion: reduce) { .card, .x { transition: opacity var(--dur-fast) var(--ease-out); } }
</style>
