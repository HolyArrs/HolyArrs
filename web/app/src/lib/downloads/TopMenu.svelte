<script lang="ts">
	import Icon from '../Icon.svelte';
	import { tick } from 'svelte';

	// A small menu opened by a caret. The panel lives in the browser's top layer (the
	// Popover API), so no frame or side nav can cover it; it is placed from the caret and
	// kept inside the screen (flipped above when there is no room below). Arrow keys,
	// Home and End move; Escape and an outside click close it and give the focus back.
	type Item = { label: string; hint?: string; disabled?: boolean; onselect?: () => void };
	let { label, items }: { label: string; items: Item[] } = $props();

	let btn = $state<HTMLButtonElement>();
	let panel = $state<HTMLElement>();
	let open = $state(false);
	const id = `dlm-${Math.random().toString(36).slice(2, 8)}`;

	function place() {
		if (!btn || !panel) return;
		const pad = 8;
		const vw = document.documentElement.clientWidth;
		const vh = document.documentElement.clientHeight;
		const a = btn.getBoundingClientRect();
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
	const entries = () => [...(panel?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
	async function toggled(e: Event) {
		open = (e as ToggleEvent).newState === 'open';
		if (!open) return;
		place();
		await tick();
		entries()[0]?.focus({ preventScroll: true });
	}
	function keys(e: KeyboardEvent) {
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

<button type="button" class="caret" bind:this={btn} popovertarget={id} aria-haspopup="menu" aria-expanded={open} aria-controls={id} aria-label={label} data-testid="dl-menu-caret"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
<div {id} popover="auto" role="menu" tabindex="-1" aria-label={label} bind:this={panel} ontoggle={toggled} onkeydown={keys} class="tm" data-testid="dl-menu">
	{#each items as it (it.label)}
		<button type="button" role="menuitem" class="mi2" aria-disabled={it.disabled ? 'true' : undefined} tabindex="-1" onclick={() => pick(it)}><span class="t">{it.label}</span>{#if it.hint}<span class="h">{it.hint}</span>{/if}</button>
	{/each}
</div>

<style>
	.caret { display: grid; place-items: center; width: 24px; height: 34px; margin-left: -6px; padding: 0; border: 0; border-radius: var(--r-btn); background: none; color: var(--text-muted); cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.caret:hover { background: var(--surface-3); color: var(--text); }
	.caret:active { transform: scale(0.96); }
	.caret:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.caret[aria-expanded='true'] { background: var(--surface-3); color: var(--text); }
	.tm { position: fixed; inset: auto; margin: 0; min-width: 190px; padding: 4px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); color: var(--text); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28); }
	.tm:popover-open { display: grid; gap: 2px; }
	.mi2 { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; width: 100%; min-height: 32px; padding: 6px 10px; border: 0; border-radius: 6px; background: none; color: var(--text); font: var(--t-footnote); text-align: left; cursor: pointer; }
	.mi2 .h { color: var(--text-muted); font: var(--t-caption); white-space: nowrap; }
	.mi2:hover, .mi2:focus-visible { background: var(--surface-3); outline: none; }
	.mi2:focus-visible { box-shadow: var(--focus-ring); }
	.mi2[aria-disabled='true'] { cursor: not-allowed; }
	.mi2[aria-disabled='true'] .t { opacity: 0.55; }
</style>
