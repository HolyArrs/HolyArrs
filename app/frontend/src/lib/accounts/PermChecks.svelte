<script lang="ts">
	// The permission boxes of Settings > Users and roles: the same for changing a person, an invite and what new people start with.
	import type { Perm } from '#lib/core/session.svelte.ts';

	let { perms, choices, words, legend, onchange }: { perms: readonly Perm[]; choices: readonly Perm[]; words: Record<Perm, string>; legend: string; onchange: (next: Perm[]) => void } = $props();

	function toggle(p: Perm, on: boolean): Perm[] {
		return on ? [...new Set([...perms, p])] : perms.filter((x) => x !== p);
	}
</script>

<fieldset class="field">
	<legend class="label">{legend}</legend>
	{#each choices as p (p)}
		<label class="checkline"
			><input type="checkbox" checked={perms.includes(p)} onchange={(e) => onchange(toggle(p, e.currentTarget.checked))} />
			{words[p]}</label
		>
	{/each}
</fieldset>

<style>
	fieldset {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-3);
	}
	/* A legend starts where the plain labels of the bars above it start. */
	legend {
		padding: 0;
	}
</style>
