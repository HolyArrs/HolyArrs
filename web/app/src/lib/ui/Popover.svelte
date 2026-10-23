<script lang="ts">
	import type { Snippet } from 'svelte';
	import { scale } from 'svelte/transition';
	import { backOut, cubicIn } from 'svelte/easing';
	import Icon, { type IconName } from '../Icon.svelte';
	import { press, reducedMotion } from '../motion.ts';

	// A trigger and a floating panel. Closes on Escape, an outside click, or
	// when content calls close(); focus goes back to the trigger.
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
		ontriggerkey
	}: {
		/** Accessible name of the trigger. */
		label: string;
		icon?: IconName;
		/** Visible trigger text (button and row variants). */
		text?: string;
		/** Custom trigger content, replaces icon/text. */
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
		/** Keys pressed on the trigger (a Menu opens on the arrow keys). */
		ontriggerkey?: (e: KeyboardEvent) => void;
	} = $props();

	const id = $props.id();
	let root = $state<HTMLElement>();
	let button = $state<HTMLButtonElement>();
	let panel = $state<HTMLElement>();

	export function close(returnFocus = true) {
		if (!open) return;
		open = false;
		if (returnFocus) button?.focus({ preventScroll: true });
	}
	function toggle() {
		open = !open;
	}

	$effect(() => {
		if (!open) return;
		const outside = (e: PointerEvent) => {
			if (root && !root.contains(e.target as Node)) close(false);
		};
		const esc = (e: KeyboardEvent) => {
			if (e.key === 'Escape') {
				// Caught first, so a Sheet behind the panel stays open.
				e.preventDefault();
				e.stopPropagation();
				close();
			}
		};
		document.addEventListener('pointerdown', outside, true);
		document.addEventListener('keydown', esc, true);
		queueMicrotask(() => panel && onopen?.(panel));
		return () => {
			document.removeEventListener('pointerdown', outside, true);
			document.removeEventListener('keydown', esc, true);
		};
	});

	// Keep the panel on screen: a menu near the right edge of a phone would
	// otherwise reach past it and make the whole page scroll sideways.
	// The panel floats above the page (fixed to the viewport), so a parent
	// that clips its overflow (a hero card) never cuts it off or scrolls.
	let pos = $state('');
	let flip = $state(false);
	function place() {
		if (!button || !panel) return;
		const b = button.getBoundingClientRect();
		const vw = document.documentElement.clientWidth;
		const vh = window.innerHeight;
		const h = panel.offsetHeight;
		const w = Math.min(width, vw - 24);
		// Flip to the roomier side when the asked side has no space.
		const up = placement === 'above' ? b.top - 6 >= h || b.top > vh - b.bottom : vh - b.bottom - 6 < h && b.top > vh - b.bottom;
		flip = up !== (placement === 'above');
		let left = align === 'start' ? b.left : b.right - w;
		left = Math.max(8, Math.min(left, vw - 8 - w));
		// A transformed ancestor (the pinned pills slide with the top bar) becomes the
		// containing block of a fixed panel; measure from it so the panel still lands on the button.
		const p = panel.offsetParent;
		const o = p && p !== document.body && p !== document.documentElement ? p.getBoundingClientRect() : { left: 0, top: 0, bottom: vh };
		left -= o.left;
		pos = up ? `left: ${left}px; bottom: ${o.bottom - b.top + 6}px;` : `left: ${left}px; top: ${b.bottom - o.top + 6}px;`;
	}
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

	const origin = $derived(`${(placement === 'below') !== flip ? 'top' : 'bottom'} ${align === 'start' ? 'left' : 'right'}`);
	function popIn(node: Element) {
		return scale(node, { start: 0.96, opacity: 0, duration: reducedMotion() ? 0 : 240, easing: backOut });
	}
	function popOut(node: Element) {
		return scale(node, { start: 0.97, opacity: 0, duration: reducedMotion() ? 0 : 150, easing: cubicIn });
	}
</script>

<div class="ui-pop" bind:this={root}>
	<button
		bind:this={button}
		class="trigger {variant}"
		type="button"
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
	.button { min-height: 36px; padding: 0 12px 0 14px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-headline); font-size: 15px; }
	.button:hover { background: var(--surface-3); }
	.row { width: 100%; min-height: 44px; padding: 6px 8px; border-radius: var(--r-btn); text-align: left; }
	.row:hover, .row[aria-expanded='true'] { background: var(--surface-3); }
	.panel {
		position: fixed; z-index: 70; max-width: calc(100vw - 24px); max-height: min(70dvh, 480px); overflow: auto;
		padding: 6px; border-radius: 14px; background: var(--surface-1); color: var(--text);
		box-shadow: var(--shadow-lift), 0 0 0 1px var(--line);
	}
	@media (pointer: coarse) { .icon { width: 44px; height: 44px; } .button { min-height: 44px; } }
</style>
