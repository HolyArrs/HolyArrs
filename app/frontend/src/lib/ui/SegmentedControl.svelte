<!-- Radio group shown as a pill with a sliding thumb. -->
<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from './Icon.svelte';

	let {
		options,
		value = $bindable(),
		label,
		size = 'md',
		testid,
		wrap = false,
		announce,
		onchange
	}: {
		options: { value: T; label: string; icon?: IconName; disabled?: boolean }[];
		value: T;
		label: string;
		size?: 'sm' | 'md';
		testid?: string;
		/** Many names: each segment is as wide as its name and the bar wraps onto more rows instead of cutting one off. */
		wrap?: boolean;
		announce?: (label: string) => string;
		onchange?: (v: T) => void;
	} = $props();

	let said = $state('');
	let buttons: HTMLButtonElement[] = $state([]);

	// -1 means the value matches no option; the thumb hides then.
	const found = $derived(options.findIndex((o) => o.value === value));
	const index = $derived(Math.max(0, found));
	function pick(v: T) {
		if (v === value || options.find((o) => o.value === v)?.disabled) return;
		value = v;
		onchange?.(v);
		if (announce) said = announce(options.find((o) => o.value === v)?.label ?? '');
	}
	// Arrow keys wrap around; Home and End jump to the ends. Segments that cannot be picked are stepped over.
	function key(e: KeyboardEvent, i: number) {
		const n = options.length;
		const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' || e.key === 'Home' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' || e.key === 'End' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		let to = e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : (i + step + n) % n;
		for (let tries = 0; options[to].disabled && tries < n; tries++) to = (to + step + n) % n;
		if (options[to].disabled) return;
		pick(options[to].value);
		buttons[to]?.focus();
	}
</script>

<div class="ui-seg {size}" class:wrap class:none={found < 0} role="radiogroup" aria-label={label} data-testid={testid} style="--n: {options.length}; --i: {index}">
	<span class="thumb" aria-hidden="true"></span>
	{#each options as o, i (o.value)}
		<button bind:this={buttons[i]} type="button" role="radio" aria-checked={o.value === value} aria-disabled={o.disabled || undefined} tabindex={o.value === value || (found < 0 && i === 0) ? 0 : -1} onclick={() => pick(o.value)} onkeydown={(e) => key(e, i)}>
			{#if o.icon}<Icon name={o.icon} size={16} />{/if}<span data-label={wrap ? o.label : undefined}>{o.label}</span>
		</button>
	{/each}
</div>
{#if announce}<span class="sr-only" aria-live="polite">{said}</span>{/if}

<style>
	.ui-seg { position: relative; flex: none; display: inline-grid; grid-auto-flow: column; grid-auto-columns: 1fr; padding: 2px; border-radius: 10px; background: var(--surface-2); isolation: isolate; max-width: 100%; }
	.thumb { position: absolute; z-index: -1; top: 2px; bottom: 2px; inset-inline-start: 2px; width: calc((100% - 4px) / var(--n)); border-radius: 8px; background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); transform: translateX(calc(100% * var(--i) * var(--dir, 1))); transition: transform var(--dur) var(--ease-spring); }
	button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 0 14px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; line-height: 1.15; text-align: center; white-space: normal; padding-block: 4px; transition: color var(--dur-fast) var(--ease-out); }
	/* Each segment is as wide as its words; when space runs out the words wrap to a second line, never cut off with dots. */
	button { min-width: min-content; }
	.none .thumb { opacity: 0; }
	button span { overflow-wrap: normal; }
	@media (max-width: 420px) { button { padding: 4px 8px; font-size: 13px; } }
	/* Wrapping bar: no sliding thumb, the picked segment carries the fill itself. Each name also holds its bold width, so picking never re-wraps the rows. */
	.wrap { display: flex; flex-wrap: wrap; gap: 4px 0; }
	.wrap .thumb { display: none; }
	.wrap button { flex: 1 0 auto; max-width: 100%; }
	.wrap button span { display: inline-flex; flex-direction: column; align-items: center; }
	.wrap button span::after { content: attr(data-label); height: 0; overflow: hidden; visibility: hidden; font-weight: 600; }
	.wrap button[aria-checked='true'] { background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); }
	button[aria-disabled='true'] { opacity: 0.45; cursor: default; }
	button[aria-checked='true'] { color: var(--text); font-weight: 600; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Only the picked segment takes focus in a wrapping bar: it shows the ring over its own fill. */
	.wrap button[aria-checked='true']:focus-visible { box-shadow: var(--focus-ring), 0 1px 2px rgb(0 0 0 / 12%); }
	@media (pointer: coarse) { button { min-height: 40px; } .wrap button { min-width: 44px; } }
	/* Larger invisible hit area for touch. */
	@media (max-width: 767px) and (pointer: coarse) {
		button { position: relative; }
		button::after { content: ''; position: absolute; inset-inline: 0; top: 50%; height: var(--seg-hit, 44px); transform: translateY(-50%); }
	}
</style>
