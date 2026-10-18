<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from '../Icon.svelte';

	// A small set of mutually exclusive choices. Arrow keys move the choice
	// (radio group pattern); the thumb slides to the picked segment.
	let {
		options,
		value = $bindable(),
		label,
		size = 'md',
		testid,
		onchange
	}: { options: { value: T; label: string; icon?: IconName }[]; value: T; label: string; size?: 'sm' | 'md'; testid?: string; onchange?: (v: T) => void } = $props();

	const index = $derived(Math.max(0, options.findIndex((o) => o.value === value)));
	let buttons: HTMLButtonElement[] = $state([]);

	function pick(v: T) {
		if (v === value) return;
		value = v;
		onchange?.(v);
	}
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
		const to = e.key === 'Home' ? 0 : e.key === 'End' ? options.length - 1 : step ? (i + step + options.length) % options.length : -1;
		if (to < 0) return;
		e.preventDefault();
		pick(options[to].value);
		buttons[to]?.focus();
	}
</script>

<div class="ui-seg {size}" role="radiogroup" aria-label={label} data-testid={testid} style="--n: {options.length}; --i: {index}">
	<span class="thumb" aria-hidden="true"></span>
	{#each options as o, i (o.value)}
		<button bind:this={buttons[i]} type="button" role="radio" aria-checked={o.value === value} tabindex={o.value === value ? 0 : -1} onclick={() => pick(o.value)} onkeydown={(e) => key(e, i)}>
			{#if o.icon}<Icon name={o.icon} size={16} />{/if}<span>{o.label}</span>
		</button>
	{/each}
</div>

<style>
	.ui-seg { position: relative; display: inline-grid; grid-auto-flow: column; grid-auto-columns: 1fr; padding: 2px; border-radius: 10px; background: var(--surface-2); isolation: isolate; max-width: 100%; }
	.thumb { position: absolute; z-index: -1; top: 2px; bottom: 2px; left: 2px; width: calc((100% - 4px) / var(--n)); border-radius: 8px; background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); transform: translateX(calc(100% * var(--i))); transition: transform var(--dur) var(--ease-spring); }
	button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 0 14px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: color var(--dur-fast); }
	button[aria-checked='true'] { color: var(--text); font-weight: 600; }
	.sm button { min-height: 28px; padding: 0 10px; font-size: 13px; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { button { min-height: 40px; } }
</style>
