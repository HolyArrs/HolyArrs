<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';
	import { press } from '../motion.ts';

	// A filter or choice token. With `selected` it is a toggle (aria-pressed);
	// with `onremove` it shows a remove button.
	let {
		label,
		selected,
		icon,
		count,
		note,
		onclick,
		onremove
	}: {
		label: string;
		selected?: boolean;
		icon?: IconName;
		count?: number;
		/** A short line after the count, e.g. "4 seeding". */
		note?: string;
		onclick?: () => void;
		onremove?: () => void;
	} = $props();
</script>

<span class="ui-chip" class:on={selected}>
	<button class="main" type="button" aria-pressed={selected === undefined ? undefined : selected} {onclick} use:press={'button'}>
		{#if icon}<Icon name={icon} size={15} />{/if}{label}{#if count !== undefined}<span class="n">{count}</span>{/if}{#if note}<span class="n">· {note}</span>{/if}
	</button>
	{#if onremove}<button class="rm" type="button" aria-label="Remove {label}" onclick={onremove}><Icon name="x" size={14} /></button>{/if}
</span>

<style>
	.ui-chip { display: inline-flex; align-items: center; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.ui-chip:hover { background: var(--surface-3); }
	.on { background: var(--text); color: var(--bg); }
	.on:hover { background: var(--text); }
	.main { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0 14px; border-radius: inherit; font: var(--t-subhead); font-size: 14px; color: inherit; }
	.n { color: inherit; opacity: .7; font-variant-numeric: tabular-nums; }
	.rm { display: grid; place-items: center; width: 28px; height: 28px; margin-right: 2px; border-radius: 50%; color: inherit; opacity: .75; }
	.rm:hover { opacity: 1; background: rgb(127 127 127 / 18%); }
	.main:focus-visible, .rm:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .main { min-height: 44px; } .rm { width: 40px; height: 40px; } }
</style>
