<script lang="ts">
	// Looks like SegmentedControl, but each segment is a link (with
	// aria-current, for sub-pages) or a toggle button (aria-pressed), so
	// pages keep plain link and button semantics.
	type Option = { value: string; label: string; href?: string; count?: number; countTestid?: string };
	let {
		options,
		value,
		label,
		nav = false,
		testid,
		onchange
	}: { options: Option[]; value: string; label: string; nav?: boolean; testid?: string; onchange?: (v: string) => void } = $props();
	const index = $derived(Math.max(0, options.findIndex((o) => o.value === value)));
</script>

<svelte:element this={nav ? 'nav' : 'div'} class="ops-seg" role={nav ? undefined : 'group'} aria-label={label} data-testid={testid} style="--n: {options.length}; --i: {index}">
	<span class="thumb" aria-hidden="true"></span>
	{#each options as o (o.value)}
		{#if o.href}
			<a href={o.href} aria-current={o.value === value ? 'page' : undefined}>
				<span>{o.label}</span>{#if o.count}<b class="n" data-testid={o.countTestid}>{o.count}</b>{/if}
			</a>
		{:else}
			<button type="button" aria-pressed={o.value === value} onclick={() => o.value !== value && onchange?.(o.value)}>
				<span>{o.label}</span>{#if o.count}<b class="n" data-testid={o.countTestid}>{o.count}</b>{/if}
			</button>
		{/if}
	{/each}
</svelte:element>

<style>
	.ops-seg { position: relative; display: inline-grid; grid-auto-flow: column; grid-auto-columns: 1fr; padding: 2px; border-radius: 10px; background: var(--surface-2); isolation: isolate; max-width: 100%; }
	.thumb { position: absolute; z-index: -1; top: 2px; bottom: 2px; left: 2px; width: calc((100% - 4px) / var(--n)); border-radius: 8px; background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 0.5px rgb(0 0 0 / 4%); transform: translateX(calc(100% * var(--i))); transition: transform var(--dur) var(--ease-spring); }
	a, button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-width: 0; min-height: 36px; padding: 0 14px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; white-space: nowrap; transition: color var(--dur-fast); }
	a span, button span { overflow: hidden; text-overflow: ellipsis; }
	a:hover, button:hover { color: var(--text); }
	[aria-current='page'], [aria-pressed='true'] { color: var(--text); font-weight: 600; }
	a:focus-visible, button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.n { min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: var(--accent); color: #fff; font: var(--t-caption); line-height: 20px; text-align: center; font-variant-numeric: tabular-nums; }
	@media (pointer: coarse) { a, button { min-height: 44px; } }
	@media (max-width: 420px) { a, button { padding: 0 8px; font-size: 13px; } }
	@media (prefers-reduced-motion: reduce) { .thumb { transition: none; } }
</style>
