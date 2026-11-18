<script lang="ts">
	// Text that may hold people's names: a deleted person's name gets a faint
	// slashed circle, a muted name, a "deleted" tooltip, and is read out as "maria (deleted)".
	import Icon from './Icon.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { personParts } from '#lib/core/plain.ts';

	let { text }: { text: string | null | undefined } = $props();
	const parts = $derived(personParts(text));
</script>

{#each parts as p, i (i)}{#if p.gone}<span class="gone" title={t('core.plain.deleted')}><span class="look" aria-hidden="true"><span class="mark"><Icon name="declined" size={12} stroke={2} /></span><span class="name">{p.text}</span></span><span class="sr-only">{t('core.plain.deletedName', { name: p.text })}</span></span>{:else}{p.text}{/if}{/each}

<style>
	.gone {
		white-space: nowrap;
	}
	.mark {
		display: inline-block;
		vertical-align: -0.05em;
		margin-inline-end: 4px;
		color: var(--text-faint);
	}
	.mark :global(svg) {
		width: 0.85em;
		height: 0.85em;
	}
	/* Its own token, mixed from the ink around it, so it stays grey on glass panels that redefine --text-muted. */
	.name {
		color: var(--person-gone, color-mix(in srgb, currentColor 55%, transparent));
	}
</style>
