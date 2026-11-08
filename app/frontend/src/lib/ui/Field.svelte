<script lang="ts">
	// Label, input slot, hint and error text, linked by the input id.
	import type { Snippet } from 'svelte';

	let {
		id,
		label,
		hint,
		error,
		required = false,
		style,
		children
	}: { id: string; label: string; hint?: string; error?: string; required?: boolean; style?: string; children: Snippet } = $props();
</script>

<div class="field" {style}>
	<label for={id}>{label}{#if required}<span class="req" aria-hidden="true"> *</span>{/if}</label>
	{@render children()}
	{#if hint}<span class="hint" id="{id}-hint">{hint}</span>{/if}
	{#if error}<span class="field-error" id="{id}-error" role="alert">{error}</span>{/if}
</div>

<style>
	/* Required star and error text share the failure color. Other field styles are global. */
	.req { color: var(--fail-text); }
	.field-error { color: var(--fail-text); font: var(--t-footnote); }
</style>
