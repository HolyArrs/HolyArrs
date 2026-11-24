<!-- Under the language picker: a language that couldn't be downloaded, with a way to try again. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { packs, installLanguage } from '#lib/core/i18n/language-packs.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
</script>

{#if packs.failed}
	{@const code = packs.failed}
	<p class="lr" role="status" data-testid="language-retry">
		<span>{t('accounts.language.failed')}</span>
		<Button size="sm" busy={packs.busy === code} onclick={() => installLanguage(code)}>{t('accounts.language.retry')}</Button>
	</p>
{/if}

<style>
	.lr { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.lr span { flex: 1 1 220px; min-width: 0; }
</style>
