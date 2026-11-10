<!-- Radio group shown as a pill with a sliding thumb. -->
<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from './Icon.svelte';

	let {
		options,
		value = $bindable(),
		label,
		size = 'md',
		testid,
		announce,
		onchange
	}: {
		options: { value: T; label: string; icon?: IconName }[];
		value: T;
		label: string;
		size?: 'sm' | 'md';
		testid?: string;
		announce?: (label: string) => string;
		onchange?: (v: T) => void;
	} = $props();

	let said = $state('');
	let buttons: HTMLButtonElement[] = $state([]);

	// -1 means the value matches no option; the thumb hides then.
	const found = $derived(options.findIndex((o) => o.value === value));
	const index = $derived(Math.max(0, found));
	function pick(v: T) {
		if (v === value) return;
		value = v;
		onchange?.(v);
		if (announce) said = announce(options.find((o) => o.value === v)?.label ?? '');
	}
	// Arrow keys wrap around; Home and End jump to the ends.
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
		const to = e.key === 'Home' ? 0 : e.key === 'End' ? options.length - 1 : step ? (i + step + options.length) % options.length : -1;
		if (to < 0) return;
		e.preventDefault();
		pick(options[to].value);
		buttons[to]?.focus();
	}
</script>

<div class="ui-seg {size}" class:none={found < 0} role="radiogroup" aria-label={label} data-testid={testid} style="--n: {options.length}; --i: {index}">
	<span class="thumb" aria-hidden="true"></span>
	{#each options as o, i (o.value)}
		<button bind:this={buttons[i]} type="button" role="radio" aria-checked={o.value === value} tabindex={o.value === value || (found < 0 && i === 0) ? 0 : -1} onclick={() => pick(o.value)} onkeydown={(e) => key(e, i)}>
			{#if o.icon}<Icon name={o.icon} size={16} />{/if}<span>{o.label}</span>
		</button>
	{/each}
</div>
{#if announce}<span class="sr-only" aria-live="polite">{said}</span>{/if}

<style>
	.ui-seg { position: relative; flex: none; display: inline-grid; grid-auto-flow: column; grid-auto-columns: minmax(0, 1fr); padding: 2px; border-radius: 10px; background: var(--surface-2); isolation: isolate; max-width: 100%; }
	.thumb { position: absolute; z-index: -1; top: 2px; bottom: 2px; inset-inline-start: 2px; width: calc((100% - 4px) / var(--n)); border-radius: 8px; background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); transform: translateX(calc(100% * var(--i) * var(--dir, 1))); transition: transform var(--dur) var(--ease-spring); }
	button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 0 14px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: color var(--dur-fast) var(--ease-out); }
	button { min-width: 0; }
	.none .thumb { opacity: 0; }
	button span { overflow: hidden; text-overflow: ellipsis; }
	@media (max-width: 420px) { button { padding: 0 8px; font-size: 13px; } }
	button[aria-checked='true'] { color: var(--text); font-weight: 600; }
	.sm button { min-height: 28px; padding: 0 10px; font-size: 13px; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { button { min-height: 40px; } }
	/* Larger invisible hit area for touch. */
	@media (max-width: 767px) and (pointer: coarse) {
		button { position: relative; }
		button::after { content: ''; position: absolute; inset-inline: 0; top: 50%; height: var(--seg-hit, 44px); transform: translateY(-50%); }
	}
</style>
