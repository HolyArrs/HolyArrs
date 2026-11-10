<script lang="ts">
	// Button that opens a floating panel placed to stay inside the viewport.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import { popIn, popOut } from '../core/transitions.ts';
	import { press } from '../core/motion.ts';

	let {
		label,
		icon,
		text,
		trigger,
		variant = 'icon',
		align = 'start',
		placement = 'below',
		open = $bindable(false),
		haspopup = 'dialog',
		panelRole = 'dialog',
		width = 240,
		children,
		onopen,
		ontriggerkey,
		tabindex
	}: {
		label: string;
		icon?: IconName;
		text?: string;
		trigger?: Snippet;
		variant?: 'icon' | 'button' | 'row';
		align?: 'start' | 'end';
		placement?: 'below' | 'above';
		open?: boolean;
		haspopup?: 'dialog' | 'menu' | 'listbox';
		panelRole?: 'dialog' | 'menu' | 'none';
		width?: number;
		children: Snippet<[() => void]>;
		onopen?: (panel: HTMLElement) => void;
		ontriggerkey?: (e: KeyboardEvent) => void;
		tabindex?: number;
	} = $props();

	const id = $props.id();

	let root = $state<HTMLElement>();
	let button = $state<HTMLButtonElement>();
	let panel = $state<HTMLElement>();
	let pos = $state('');
	let flip = $state(false);

	const origin = $derived(`${(placement === 'below') !== flip ? 'top' : 'bottom'} ${align === 'start' ? 'left' : 'right'}`);

	// Outside click and Escape close it; focus moves into the panel on the next tick.
	$effect(() => {
		if (!open) return;
		const outside = (e: PointerEvent) => {
			if (root && !root.contains(e.target as Node)) close(false);
		};
		const esc = (e: KeyboardEvent) => {
			if (e.key === 'Escape') {
				e.preventDefault();
				e.stopPropagation();
				close();
			}
		};
		document.addEventListener('pointerdown', outside, true);
		document.addEventListener('keydown', esc, true);
		queueMicrotask(() => {
			if (!panel) return;
			if (onopen) onopen(panel);
			else if (panelRole === 'dialog') panel.querySelector<HTMLElement>('button:not([disabled]),a[href],input,[tabindex="0"]')?.focus({ preventScroll: true });
		});
		return () => {
			document.removeEventListener('pointerdown', outside, true);
			document.removeEventListener('keydown', esc, true);
		};
	});

	// Capture scroll so scrolling any parent keeps the panel next to its button.
	$effect(() => {
		if (!open || !panel) return;
		place();
		const again = () => place();
		window.addEventListener('scroll', again, true);
		window.addEventListener('resize', again);
		return () => {
			window.removeEventListener('scroll', again, true);
			window.removeEventListener('resize', again);
		};
	});

	export function close(returnFocus = true) {
		if (!open) return;
		open = false;
		if (returnFocus) button?.focus({ preventScroll: true });
	}
	function toggle() {
		open = !open;
	}
	function leave(e: FocusEvent) {
		const to = e.relatedTarget as Node | null;
		if (to && root && !root.contains(to)) close(false);
	}

	// Flips to the roomier side, keeps 8px from the edges, and offsets for a positioned parent.
	function place() {
		if (!button || !panel) return;
		const b = button.getBoundingClientRect();
		const vw = document.documentElement.clientWidth;
		const vh = window.innerHeight;
		const h = panel.offsetHeight;
		const w = Math.min(width, vw - 24);
		const up = placement === 'above' ? b.top - 6 >= h || b.top > vh - b.bottom : vh - b.bottom - 6 < h && b.top > vh - b.bottom;
		flip = up !== (placement === 'above');
		let left = align === 'start' ? b.left : b.right - w;
		left = Math.max(8, Math.min(left, vw - 8 - w));
		const p = panel.offsetParent;
		const o = p && p !== document.body && p !== document.documentElement ? p.getBoundingClientRect() : { left: 0, top: 0, bottom: vh };
		left -= o.left;
		pos = up ? `left: ${left}px; bottom: ${o.bottom - b.top + 6}px;` : `left: ${left}px; top: ${b.bottom - o.top + 6}px;`;
	}
</script>

<div class="ui-pop" bind:this={root} onfocusout={leave}>
	<button
		bind:this={button}
		class="trigger {variant}"
		type="button"
		{tabindex}
		aria-label={variant === 'icon' || trigger ? label : undefined}
		title={variant === 'icon' && !trigger ? label : undefined}
		aria-haspopup={haspopup}
		aria-expanded={open}
		aria-controls={id}
		onclick={toggle}
		onkeydown={ontriggerkey}
		use:press={variant === 'icon' ? 'round' : 'button'}
	>
		{#if trigger}{@render trigger()}{:else}
			{#if icon}<Icon name={icon} size={variant === 'icon' ? 19 : 17} />{/if}
			{#if variant !== 'icon'}<span>{text ?? label}</span><Icon name="chevronDown" size={14} />{/if}
		{/if}
	</button>
	{#if open}
		<div
			bind:this={panel}
			{id}
			class="panel"
			role={panelRole === 'none' ? undefined : panelRole}
			aria-label={label}
			style="width: {width}px; {pos} transform-origin: {origin}"
			in:popIn
			out:popOut
		>
			{@render children(() => close())}
		</div>
	{/if}
</div>

<style>
	.ui-pop { position: relative; display: inline-flex; }
	.trigger { display: inline-flex; align-items: center; gap: 6px; color: var(--text); }
	.trigger:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.icon { width: 36px; height: 36px; justify-content: center; border-radius: 50%; color: var(--text-muted); }
	.icon:hover, .icon[aria-expanded='true'] { background: var(--surface-3); color: var(--text); }
	.button { flex: none; white-space: nowrap; min-height: 36px; padding: 0 12px 0 14px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-headline); font-size: 15px; }
	.button:hover { background: var(--surface-3); }
	.row { width: 100%; min-height: 44px; padding: 6px 8px; border-radius: var(--r-btn); text-align: start; }
	.row:hover, .row[aria-expanded='true'] { background: var(--surface-3); }
	/* Fixed so scroll containers cannot clip it. */
	.panel {
		position: fixed; z-index: 70; max-width: calc(100vw - 24px); max-height: min(70dvh, 480px); overflow: auto;
		padding: 6px; border-radius: 14px; background: var(--surface-1); color: var(--text);
		box-shadow: var(--shadow-lift), 0 0 0 1px light-dark(var(--line), transparent);
	}
	:global(html[data-tint]) .panel { background: color-mix(in srgb, var(--tint-bg, #1d1d22) 86%, #fff); color: #fff; }
	@media (pointer: coarse) { .icon { width: 44px; height: 44px; } .button { min-height: 44px; } }
</style>
