<!-- Language packs on screen: the one-time "Use HolyArrs in …?" question, and the note when a language couldn't be downloaded. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { packs, answerOffer, installLanguage } from '#lib/core/i18n/language-packs.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { languageName } from './language-list.ts';

	let hidden = $state(false);
	const failed = $derived(hidden ? null : packs.failed);
</script>

{#if packs.offer}
	<div class="lpn" role="dialog" aria-labelledby="lpn-q" data-testid="language-offer">
		<Icon name="globe" size={20} />
		<p id="lpn-q" class="q">{t('accounts.language.offer', { language: languageName(packs.offer) })}</p>
		<div class="acts">
			<Button size="sm" variant="plain" onclick={() => answerOffer(false)}>{t('accounts.language.offerKeep')}</Button>
			<Button size="sm" variant="primary" busy={packs.busy === packs.offer} onclick={() => answerOffer(true)}>{t('accounts.language.offerInstall')}</Button>
		</div>
	</div>
{:else if failed}
	{@const code = failed}
	<div class="lpn" role="status" data-testid="language-failed">
		<Icon name="info" size={20} />
		<p class="q">{t('accounts.language.failed')}</p>
		<div class="acts">
			<Button size="sm" busy={packs.busy === code} onclick={() => installLanguage(code)}>{t('accounts.language.retry')}</Button>
			<IconButton icon="x" label={t('accounts.language.dismiss')} onclick={() => (hidden = true)} />
		</div>
	</div>
{/if}

<style>
	.lpn { position: fixed; z-index: 60; inset-inline: 16px; bottom: calc(16px + env(safe-area-inset-bottom)); margin-inline: auto; max-width: 520px; display: flex; flex-wrap: wrap; align-items: center; gap: 10px 12px; padding: 12px 14px; border-radius: 14px; background: var(--surface-1); border: 1px solid var(--line); box-shadow: 0 8px 28px rgb(0 0 0 / 28%); color: var(--text); }
	.lpn > :global(svg) { flex: none; color: var(--text-muted); }
	.q { flex: 1 1 200px; min-width: 0; margin: 0; font: var(--t-callout); overflow-wrap: anywhere; }
	.acts { display: flex; gap: 8px; margin-inline-start: auto; align-items: center; }
	@media (max-width: 767px) { .lpn { bottom: calc(72px + env(safe-area-inset-bottom)); } }
</style>
