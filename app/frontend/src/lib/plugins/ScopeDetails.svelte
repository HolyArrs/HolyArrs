<script lang="ts">
	// The red warnings for permissions that reach more than they seem to, the caps a plugin asks for, and the exact permissions one tap away.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { resourcesLine, scopeLines, scopeWarnings } from './view.ts';
	import type { Plugin } from './types.ts';

	let { p }: { p: Plugin } = $props();
	const warns = $derived(scopeWarnings(p.scope_groups));
	const asked = $derived(resourcesLine(p.resources));
	const scopes = $derived(scopeLines(p.scope_groups));
</script>

{#if warns.length || asked || scopes.length}
	<div class="scopes" data-testid="plugin-scopes">
		{#each warns as w (w)}<p class="small warn" role="note"><Icon name="info" size={15} /><span>{w}</span></p>{/each}
		{#if asked}<p class="small muted" dir="auto">{asked}</p>{/if}
		{#if scopes.length}
			<p class="xs muted">{t('plugins.scopeDetails')}</p>
			<ul class="list xs fit-grid" style:--fit-col="260px" data-testid="plugin-scope-lines">
				{#each scopes as s (s.scope)}<li><span dir="auto">{s.line}</span> <code dir="ltr">{s.scope}</code></li>{/each}
			</ul>
		{/if}
	</div>
{/if}

<style>
	.scopes { display: grid; gap: var(--s-1); min-width: 0; }
	.warn { display: flex; gap: var(--s-2); align-items: flex-start; margin: 0; color: var(--fail-text); }
	.warn :global(svg) { flex: none; margin-top: 2px; }
	.warn span, .muted { overflow-wrap: anywhere; margin: 0; }
	.list { margin: var(--s-1) 0 0; padding-inline-start: var(--s-5); max-height: 40vh; overflow-y: auto; }
	code { overflow-wrap: anywhere; }
</style>
