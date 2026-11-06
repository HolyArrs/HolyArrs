<script lang="ts">
	// A connection test's result as one small tag: a green tick with "<App> v<version>",
	// or a red "Not connected". The full answer stays in the tooltip; the place
	// that shows the tag puts a failure's reason on its own short red line.
	import Icon from '#lib/Icon.svelte';
	import { testLabel } from '#lib/test-tag.ts';

	let {
		ok,
		text = '',
		app = '',
		extra = '',
		hover = '',
		testid
	}: { ok: boolean; text?: string; app?: string; extra?: string; hover?: string; testid?: string } = $props();
	// hover: the tooltip when it should say more than text (a time, a server name); text alone is read for the version.
</script>

{#if ok}
	<span class="testtag ok" title={hover || text || undefined} data-testid={testid}><Icon name="check" size={12} stroke={2.5} />{testLabel(text, app, extra)}</span>
{:else}
	<span class="testtag bad" title={hover || text || undefined} data-testid={testid}>Not connected</span>
{/if}

<style>
	.testtag { flex: none; display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-ui); white-space: nowrap; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
	.testtag.ok { color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.testtag.bad { color: var(--fail-text, #ff6961); border: 1px solid color-mix(in srgb, var(--fail-text, #ff6961) 40%, transparent); }
</style>
