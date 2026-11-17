<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import type { Verb } from './history-kinds.ts';

	let { verb, size = 'md', bare = false }: { verb: Verb; size?: 'sm' | 'md'; bare?: boolean } = $props();
</script>

<span class="vm {verb.tone} {size}" class:bare data-tone={verb.tone} title={verb.word}>
	<span class="dot" aria-hidden="true"><Icon name={verb.icon} size={size === 'sm' ? 11 : 13} /></span>{#if !bare}<span class="w">{verb.word}</span>{:else}<span class="sr-only">{verb.word}</span>{/if}
</span>

<style>
	.vm { display: inline-flex; align-items: center; gap: 6px; min-width: 0; color: var(--text); font: var(--t-footnote); font-weight: 600; white-space: nowrap; }
	.dot { flex: none; display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--surface-3); color: var(--text-muted); }
	.sm .dot { width: 16px; height: 16px; }
	.ok .dot { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok); }
	.fail .dot { background: color-mix(in srgb, var(--fail) 18%, transparent); color: var(--fail); }
	.wait .dot { background: color-mix(in srgb, var(--wait) 20%, transparent); color: var(--wait); }
	.info .dot { background: color-mix(in srgb, var(--info) 18%, transparent); color: var(--info); }
	.ok .w { color: var(--ok-text, var(--ok)); }
	.fail .w { color: var(--fail-text, var(--fail)); }
	.wait .w { color: var(--wait-text, var(--wait)); }
	.w { overflow: hidden; text-overflow: ellipsis; }
</style>
