<script lang="ts">
	// A filter Chip that is a real checkbox underneath (label + input), for
	// filters that people and tests reach as checkboxes. `dot` adds a colour
	// dot before the word (the word always stays).
	let { label, checked = $bindable(false), dot, onchange }: { label: string; checked?: boolean; dot?: string; onchange?: (on: boolean) => void } = $props();
</script>

<label class="ops-chip" class:on={checked}>
	<input type="checkbox" bind:checked onchange={(e) => onchange?.(e.currentTarget.checked)} />
	{#if dot}<span class="dot" style="background: {dot}" aria-hidden="true"></span>{/if}
	<span>{label}</span>
</label>

<style>
	.ops-chip { position: relative; display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 0 14px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-subhead); font-size: 14px; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); user-select: none; }
	.ops-chip:hover { background: var(--surface-3); }
	.on, .on:hover { background: var(--text); color: var(--bg); }
	input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.ops-chip:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
	@media (pointer: coarse) { .ops-chip { min-height: 44px; } }
</style>
