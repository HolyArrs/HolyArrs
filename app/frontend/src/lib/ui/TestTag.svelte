<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Pill showing whether a connection test passed.
	import Icon from '#lib/ui/Icon.svelte';
	import { testLabel } from '#lib/core/test-tag.ts';

	let {
		ok,
		text = '',
		app = '',
		extra = '',
		hover = '',
		testid
	}: { ok: boolean; text?: string; app?: string; extra?: string; hover?: string; testid?: string } = $props();
</script>

<!-- hover text falls back to the plain result text -->
{#if ok}
	<span class="testtag ok" title={hover || text || undefined} data-testid={testid}><Icon name="check" size={12} stroke={2.5} />{testLabel(text, app, extra)}</span>
{:else}
	<span class="testtag bad" title={hover || text || undefined} data-testid={testid}>{t('ui.testTag.notConnected')}</span>
{/if}

<style>
	/* fallback colours keep the pill readable outside the themed page */
	.testtag { flex: none; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-ui); white-space: nowrap; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
	.testtag.ok { color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.testtag.bad { color: var(--fail-text, #ff6961); border: 1px solid color-mix(in srgb, var(--fail-text, #ff6961) 40%, transparent); }
</style>
