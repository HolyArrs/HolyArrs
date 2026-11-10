<!-- Segmented view picker; the active option shows its label and an optional art size menu. -->
<script lang="ts" generics="T extends string">
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	let {
		options,
		value,
		onchange,
		sizeItems,
		sizeLabel = '',
		compact = false
	}: {
		options: { value: T; label: string; icon: IconName }[];
		value: T;
		onchange: (v: T) => void;

		sizeItems?: MenuItem[] | null;
		sizeLabel?: string;

		compact?: boolean;
	} = $props();
	let buttons: HTMLButtonElement[] = $state([]);

	// Arrows wrap around and move focus with the selection, like a radio group.
	function key(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		const to = (i + step + options.length) % options.length;
		onchange(options[to].value);
		buttons[to]?.focus();
	}
</script>

<div class="vs" class:compact role="radiogroup" aria-label={t('mediafiles.view.label')} data-testid="library-view">
	{#each options as o, i (o.value)}
		{@const on = o.value === value}
		<div class="seg" class:on>
			<button bind:this={buttons[i]} type="button" role="radio" aria-checked={on} aria-label={o.label} title={on ? undefined : o.label} tabindex={on ? 0 : -1} onclick={() => !on && onchange(o.value)} onkeydown={(e) => key(e, i)}>
				<Icon name={o.icon} size={16} /><span class="lbl" aria-hidden="true">{o.label}</span>
			</button>
			{#if on && sizeItems?.length}
				<span class="size"><Menu label={t('mediafiles.view.artSize', { size: sizeLabel })} variant="icon" icon="chevronDown" items={sizeItems} width={200} align="start" /></span>
			{/if}
		</div>
	{/each}
</div>

<style>
	.vs { flex: none; display: inline-flex; align-items: stretch; gap: 2px; padding: 2px; border-radius: 10px; background: var(--vs-bg, var(--surface-2)); }
	.seg { display: inline-flex; align-items: center; border-radius: 8px; transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.seg.on { background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); }
	button { display: inline-flex; align-items: center; justify-content: center; min-width: 36px; min-height: 32px; padding: 0 10px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: color var(--dur-fast) var(--ease-out); }
	button:hover { color: var(--text); }
	.on button { color: var(--text); font-weight: 600; padding-right: 4px; }
	.seg:not(:has(.size)) button { padding-right: 12px; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* Labels slide open on the active option only. */
	.lbl { display: inline-block; max-width: 0; margin-left: 0; overflow: hidden; opacity: 0; transition: max-width var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out), margin var(--dur) var(--ease-out); }
	.on .lbl { max-width: 96px; margin-left: 6px; opacity: 1; }
	.compact .on .lbl { max-width: 0; margin-left: 0; opacity: 0; }
	.compact .on button { padding-right: 10px; }
	.size { display: inline-flex; padding-right: 2px; }
	.size :global(.trigger.icon) { width: 26px; height: 28px; border-radius: 6px; }
	.size :global(.trigger.icon svg) { width: 15px; height: 15px; }
	@media (prefers-reduced-motion: reduce) { .lbl, .seg { transition: none; } }
	@media (pointer: coarse) { button { min-height: 40px; min-width: 44px; } .size :global(.trigger.icon) { width: 36px; height: 40px; } }
</style>
