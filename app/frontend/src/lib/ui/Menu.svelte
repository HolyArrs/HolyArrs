<script lang="ts" module>
	import type { IconName } from './Icon.svelte';
	export type MenuItem =
		| { kind?: 'item'; label: string; icon?: IconName; href?: string; onselect?: () => void; destructive?: boolean; disabled?: boolean; checked?: boolean; hint?: string }
		| { kind: 'separator' }
		| { kind: 'heading'; label: string };
</script>

<script lang="ts">
	// Dropdown menu with keyboard navigation, built on Popover.
	import Icon from './Icon.svelte';
	import Popover from './Popover.svelte';
	import type { Snippet } from 'svelte';

	let {
		label,
		items,
		icon = 'more',
		text,
		trigger,
		variant = 'icon',
		align = 'end',
		placement = 'below',
		width = 220,
		open = $bindable(false),
		tabindex
	}: {
		label: string;
		items: MenuItem[];
		icon?: IconName;
		text?: string;
		trigger?: Snippet;
		variant?: 'icon' | 'button' | 'row';
		align?: 'start' | 'end';
		placement?: 'below' | 'above';
		width?: number;
		open?: boolean;
		tabindex?: number;
	} = $props();

	// Set when the menu opens with ArrowUp so focus lands on the last entry.
	let startAtEnd = false;

	// Focus the checked entry if there is one, so a radio menu opens on the current value.
	function focusFirst(panel: HTMLElement) {
		const list = entries(panel);
		const first = startAtEnd ? list[list.length - 1] : (list.find((e) => e.getAttribute('aria-checked') === 'true') ?? list[0]);
		startAtEnd = false;
		first?.focus({ preventScroll: true });
	}
	function triggerKey(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		e.preventDefault();
		const panel = document.getElementById((e.currentTarget as HTMLElement).getAttribute('aria-controls') ?? '');
		if (open && panel) {
			const list = entries(panel);
			(e.key === 'ArrowUp' ? list[list.length - 1] : list[0])?.focus({ preventScroll: true });
			return;
		}
		startAtEnd = e.key === 'ArrowUp';
		open = true;
	}
	// Arrow keys wrap around; Tab closes the menu.
	function nav(e: KeyboardEvent) {
		const panel = e.currentTarget as HTMLElement;
		const list = entries(panel);
		const i = list.indexOf(document.activeElement as HTMLElement);
		let to = -1;
		if (e.key === 'ArrowDown') to = (i + 1) % list.length;
		else if (e.key === 'ArrowUp') to = (i - 1 + list.length) % list.length;
		else if (e.key === 'Home') to = 0;
		else if (e.key === 'End') to = list.length - 1;
		else if (e.key === 'Tab') open = false;
		if (to >= 0) {
			e.preventDefault();
			list[to]?.focus({ preventScroll: true });
		}
	}
	function pick(item: Extract<MenuItem, { label: string; kind?: 'item' }>, close: () => void) {
		if (item.disabled) return;
		close();
		item.onselect?.();
	}

	// Disabled entries are skipped so keys never land on them.
	function entries(panel: HTMLElement) {
		return [...panel.querySelectorAll<HTMLElement>('[role^="menuitem"]:not([aria-disabled="true"])')];
	}
</script>

<Popover {label} {icon} {text} {trigger} {variant} {align} {placement} {width} {tabindex} bind:open haspopup="menu" panelRole="none" onopen={focusFirst} ontriggerkey={triggerKey}>
	{#snippet children(close)}
		<div class="ui-menu" role="menu" aria-label={label} tabindex="-1" onkeydown={nav}>
			{#each items as item, i (i)}
				{#if item.kind === 'separator'}
					<div class="sep" role="separator"></div>
				{:else if item.kind === 'heading'}
					<div class="heading" role="presentation">{item.label}</div>
				{:else if item.href && !item.disabled}
					<a class="item" class:danger={item.destructive} role="menuitem" tabindex="-1" href={item.href} onclick={() => pick(item, close)}>
						<span class="ic">{#if item.icon}<Icon name={item.icon} size={17} />{/if}</span><span class="grow">{item.label}</span>{#if item.hint}<kbd>{item.hint}</kbd>{/if}
					</a>
				{:else}
					<button
						class="item"
						class:danger={item.destructive}
						type="button"
						role={item.checked === undefined ? 'menuitem' : 'menuitemradio'}
						aria-checked={item.checked === undefined ? undefined : item.checked}
						aria-disabled={item.disabled || undefined}
						tabindex="-1"
						onclick={() => pick(item, close)}
					>
						<span class="ic">{#if item.checked}<Icon name="check" size={17} />{:else if item.icon}<Icon name={item.icon} size={17} />{/if}</span><span class="grow">{item.label}</span>{#if item.hint}<kbd>{item.hint}</kbd>{/if}
					</button>
				{/if}
			{/each}
		</div>
	{/snippet}
</Popover>

<style>
	.ui-menu { display: flex; flex-direction: column; outline: none; }
	.item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 36px; padding: 0 10px 0 8px; border-radius: 8px; color: var(--text); font: var(--t-callout); font-size: 15px; text-align: left; }
	.item:hover, .item:focus-visible { background: var(--surface-3); outline: none; box-shadow: none; }
	.item:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
	.item[aria-disabled='true'] { opacity: var(--disabled-opacity); cursor: not-allowed; }
	.danger { color: var(--fail-text); }
	.ic { width: 20px; display: grid; place-items: center; color: var(--text-muted); flex: none; }
	.danger .ic { color: inherit; }
	.grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sep { height: 1px; margin: 5px 8px; background: var(--line); }
	/* Left padding lines the heading up with item labels past the icon column. */
	.heading { padding: 8px 10px 4px 38px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	kbd { color: var(--text-muted); font: var(--t-footnote); }
	/* Minimum touch target size. */
	@media (pointer: coarse) { .item { min-height: 44px; } }
</style>
