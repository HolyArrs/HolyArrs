<!-- The app's language: Automatic follows the browser; a pick is kept on the person's account. -->
<script lang="ts">
	import { LOCALES, lang, t, type LocaleChoice } from '#lib/core/i18n/i18n.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import { saveLanguage } from './profile.svelte.ts';

	let { id = 'language' }: { id?: string } = $props();

	let busy = $state(false);
	async function pick(e: Event) {
		const v = (e.currentTarget as HTMLSelectElement).value as LocaleChoice;
		busy = true;
		try {
			await saveLanguage(v);
		} catch (err) {
			toasts.push(t('accounts.language.saveFailed'), 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}
</script>

<select {id} class="input" aria-label={t('accounts.language.label')} value={lang.choice} onchange={pick} disabled={busy} data-testid="language-picker">
	<option value="auto">{t('accounts.language.auto')}</option>
	{#each LOCALES as l (l.code)}<option value={l.code} lang={l.code}>{l.name}</option>{/each}
</select>

<style>
	select { width: auto; min-width: 12rem; max-width: 100%; }
</style>
