<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import { tick } from 'svelte';

	// Menu opened by a caret. The panel uses the browser's top layer (Popover API) so no frame or side nav covers it, is placed from the caret and flips above when there is no room below. Arrow keys, Home and End move; Escape and an outside click close it and return focus.
	type Item = { label: string; hint?: string; disabled?: boolean; checked?: boolean; bad?: boolean; tid?: string; key?: string; onselect?: () => void };
	let { label, items, tight = false, text = '', bare = false }: { label: string; items: Item[]; tight?: boolean; text?: string; bare?: boolean } = $props();

	let btn = $state<HTMLButtonElement>();
	let anchor = $state<HTMLElement>();
	/** Opens the menu under any element. */
	export function show(el: HTMLElement) {
		anchor = el;
		panel?.showPopover();
	}
	let panel = $state<HTMLElement>();
	let open = $state(false);
	const id = `dlm-${Math.random().toString(36).slice(2, 8)}`;

	function place() {
		const at = anchor ?? btn;
		if (!at || !panel) return;
		const pad = 8;
		const vw = document.documentElement.clientWidth;
		const vh = document.documentElement.clientHeight;
		const a = at.getBoundingClientRect();
		const m = panel.getBoundingClientRect();
		let left = a.left;
		let top = a.bottom + 4;
		if (left + m.width > vw - pad) left = a.right - m.width;
		if (top + m.height > vh - pad) top = a.top - 4 - m.height;
		left = Math.max(pad, Math.min(vw - m.width - pad, left));
		top = Math.max(pad, Math.min(vh - m.height - pad, top));
		panel.style.left = `${Math.round(left)}px`;
		panel.style.top = `${Math.round(top)}px`;
	}
	const entries = () => [...(panel?.querySelectorAll<HTMLElement>('[role="menuitem"], [role="menuitemradio"]') ?? [])];
	async function toggled(e: Event) {
		open = (e as ToggleEvent).newState === 'open';
		if (!open) {
			// Return focus to the opener if it was inside the menu.
			const a = document.activeElement;
			if (!a || a === document.body || panel?.contains(a)) (anchor ?? btn)?.focus({ preventScroll: true });
			return;
		}
		place();
		await tick();
		entries()[0]?.focus({ preventScroll: true });
	}
	function keys(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			// Closed here so the page's own Escape handler never sees it.
			e.preventDefault();
			e.stopPropagation();
			panel?.hidePopover();
			return;
		}
		const list = entries();
		const i = list.indexOf(document.activeElement as HTMLElement);
		let n = -1;
		if (e.key === 'ArrowDown') n = (i + 1) % list.length;
		else if (e.key === 'ArrowUp') n = (i - 1 + list.length) % list.length;
		else if (e.key === 'Home') n = 0;
		else if (e.key === 'End') n = list.length - 1;
		if (n < 0) return;
		e.preventDefault();
		list[n]?.focus({ preventScroll: true });
	}
	function pick(it: Item) {
		if (it.disabled) return;
		panel?.hidePopover();
		it.onselect?.();
	}
</script>

{#if !bare}<button type="button" class="caret" class:tight class:txt={!!text} bind:this={btn} popovertarget={id} aria-haspopup="menu" aria-expanded={open} aria-controls={id} aria-label={label} data-testid="dl-menu-caret">{#if text}<span>{text}</span>{/if}<Icon name="chevronDown" size={14} stroke={2.2} /></button>{/if}
<div {id} popover="auto" role="menu" tabindex="-1" aria-label={label} bind:this={panel} ontoggle={toggled} onkeydown={keys} class="tm" data-testid="dl-menu">
	{#each items as it (it.label)}
		<button type="button" role={it.checked === undefined ? 'menuitem' : 'menuitemradio'} aria-checked={it.checked === undefined ? undefined : it.checked} class="mi2" class:bad={it.bad} data-testid={it.tid} data-key={it.key} aria-disabled={it.disabled ? 'true' : undefined} tabindex="-1" onclick={() => pick(it)}><span class="t">{it.label}</span>{#if it.hint}<span class="h">{it.hint}</span>{/if}</button>
	{/each}
</div>

<style>
	.caret { display: grid; place-items: center; width: 24px; height: 34px; margin-left: -6px; padding: 0; border: 0; border-radius: var(--r-btn); background: none; color: var(--text-muted); cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.caret.txt { width: auto; padding: 0 6px; gap: 4px; margin-left: 0; font: 600 11px/1 var(--font-ui); color: var(--text); }
	.caret.tight { height: 28px; margin-left: -4px; }
	.caret:hover { background: var(--surface-3); color: var(--text); }
	.caret:active { transform: scale(0.96); }
	.caret:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.caret[aria-expanded='true'] { background: var(--surface-3); color: var(--text); }
	.tm { position: fixed; inset: auto; margin: 0; min-width: 190px; max-height: calc(100vh - 16px); overflow: auto; padding: 4px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-lift); }
	.tm:popover-open { display: grid; gap: 2px; }
	.mi2 { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; width: 100%; min-height: 36px; padding: 6px 10px; border: 0; border-radius: 8px; background: none; color: var(--text); font: var(--t-callout); text-align: left; cursor: pointer; }
	.mi2 .h { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; }
	.mi2:hover, .mi2:focus-visible { background: var(--surface-3); outline: none; }
	.mi2:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
	.mi2[aria-checked='true'] .t { font-weight: 600; }
	.mi2.bad .t { color: var(--fail-text); }
	.mi2[aria-disabled='true'] { cursor: not-allowed; }
	.mi2[aria-disabled='true'] .t { opacity: 0.55; }
	@media (max-width: 640px) { .mi2 { align-items: center; min-height: 44px; font: var(--t-callout); } }
</style>
