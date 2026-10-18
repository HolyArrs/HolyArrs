<script lang="ts">
	import type { Snippet } from 'svelte';

	// One form field: label, the control (input, select or textarea, passed as
	// children with the same id), an optional hint and an error line. The
	// control keeps its own name, id and validation; styles live in app.css
	// (.field, .input, .hint) so every form looks the same.
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
	{#if hint}<span class="hint">{hint}</span>{/if}
	{#if error}<span class="field-error" id="{id}-error" role="alert">{error}</span>{/if}
</div>

<style>
	.req { color: var(--fail-text); }
	.field-error { color: var(--fail-text); font: var(--t-footnote); }
</style>
