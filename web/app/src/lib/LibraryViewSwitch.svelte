<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from './Icon.svelte';
	import Menu, { type MenuItem } from './ui/Menu.svelte';
	// The Library's view switch: the picked view shows its icon and name, the
	// others only their icon. The picked view also carries the art-size menu
	// (a small chevron inside its segment), so view and size are one control.
	let {
		options,
		value,
		onchange,
		sizeItems,
		sizeLabel = ''
	}: {
		options: { value: T; label: string; icon: IconName }[];
		value: T;
		onchange: (v: T) => void;
		/** The size menu for the picked view; none hides the chevron. */
		sizeItems?: MenuItem[] | null;
		sizeLabel?: string;
	} = $props();
	let buttons: HTMLButtonElement[] = $state([]);
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		const to = (i + step + options.length) % options.length;
		onchange(options[to].value);
		buttons[to]?.focus();
	}
</script>

<div class="vs" role="radiogroup" aria-label="View" data-testid="library-view">
	{#each options as o, i (o.value)}
		{@const on = o.value === value}
		<div class="seg" class:on>
			<button bind:this={buttons[i]} type="button" role="radio" aria-checked={on} aria-label={o.label} title={on ? undefined : o.label} tabindex={on ? 0 : -1} onclick={() => !on && onchange(o.value)} onkeydown={(e) => key(e, i)}>
				<Icon name={o.icon} size={16} /><span class="lbl" aria-hidden="true">{o.label}</span>
			</button>
			{#if on && sizeItems?.length}
				<span class="size"><Menu label="Art size: {sizeLabel}" variant="icon" icon="chevronDown" items={sizeItems} width={200} align="start" /></span>
			{/if}
		</div>
	{/each}
</div>

<style>
	.vs { flex: none; display: inline-flex; align-items: stretch; gap: 2px; padding: 2px; border-radius: 10px; background: var(--surface-2); }
	.seg { display: inline-flex; align-items: center; border-radius: 8px; transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.seg.on { background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); }
	button { display: inline-flex; align-items: center; justify-content: center; min-width: 36px; min-height: 32px; padding: 0 10px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: color var(--dur-fast) var(--ease-out); }
	button:hover { color: var(--text); }
	.on button { color: var(--text); font-weight: 600; padding-right: 4px; }
	.seg:not(:has(.size)) button { padding-right: 12px; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The name slides open on the picked view and shut on the others. */
	.lbl { display: inline-block; max-width: 0; margin-left: 0; overflow: hidden; opacity: 0; transition: max-width var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out), margin var(--dur) var(--ease-out); }
	.on .lbl { max-width: 96px; margin-left: 6px; opacity: 1; }
	.size { display: inline-flex; padding-right: 2px; }
	.size :global(.trigger.icon) { width: 26px; height: 28px; border-radius: 6px; }
	.size :global(.trigger.icon svg) { width: 15px; height: 15px; }
	@media (prefers-reduced-motion: reduce) { .lbl, .seg { transition: none; } }
	@media (pointer: coarse) { button { min-height: 40px; min-width: 44px; } .size :global(.trigger.icon) { width: 36px; height: 40px; } }
</style>
