<script lang="ts">
	// What everyone who joins from now on starts with, on Settings > Users and roles. Only the owner sees it; people already here keep their own.
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import type { Perm } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import PermChecks from './PermChecks.svelte';
	import { newPeopleChoices, samePerms } from './presets.ts';

	let { saved, words, onsaved }: { saved: Perm[]; words: Record<Perm, string>; onsaved: (perms: Perm[]) => void } = $props();

	// The boxes start from what is saved and follow it after each save.
	let perms = $derived([...saved]);
	let busy = $state(false);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const r = await api.put<{ new_people_perms: Perm[] }>('/auth/settings', { new_people_perms: perms });
			onsaved(r.new_people_perms);
			toasts.push(t('settings.users.newPeopleSaved'), 'ok');
		} catch (err) {
			toasts.push(t('settings.users.couldntSave'), 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}
</script>

<SettingsCard id="new-people-card" title={t('settings.users.newPeople')} description={t('settings.users.newPeopleHint')}>
	<form onsubmit={save} data-testid="new-people">
		<PermChecks {perms} choices={newPeopleChoices} {words} legend={t('settings.users.may')} onchange={(next) => (perms = next)} />
		<Button variant="primary" type="submit" disabled={busy || samePerms(perms, saved)} aria-busy={busy}>{t('settings.users.save')}</Button>
	</form>
</SettingsCard>
