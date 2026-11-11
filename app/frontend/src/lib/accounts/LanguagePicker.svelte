<!-- The app's language: Automatic follows the browser; a pick is kept on the person's account. -->
<script lang="ts">
	import { LOCALES, lang, t, type LocaleChoice } from '#lib/core/i18n/i18n.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import { saveLanguage } from './profile.svelte.ts';

	let { id = 'language' }: { id?: string } = $props();

	// The most used languages come first; the rest sit under More languages.
	const TOP = new Set(['en', 'es', 'pt-br', 'fr', 'de', 'it', 'ru', 'zh-cn', 'ja', 'ar']);
	const top = LOCALES.filter((l) => TOP.has(l.code));
	const more = LOCALES.filter((l) => !TOP.has(l.code)).sort((a, b) => a.name.localeCompare(b.name));

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
	<optgroup label={t('accounts.language.top')}>
		{#each top as l (l.code)}<option value={l.code} lang={l.code}>{l.name}</option>{/each}
	</optgroup>
	<optgroup label={t('accounts.language.more')}>
		{#each more as l (l.code)}<option value={l.code} lang={l.code}>{l.name}</option>{/each}
	</optgroup>
</select>

<style>
	select { width: auto; min-width: 12rem; max-width: 100%; }
</style>
