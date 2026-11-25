<script lang="ts" module>
	import type { Perm } from '#lib/core/session.svelte.ts';
	/** What new people start with, as the Users list sends it. */
	export interface NewPeople {
		perms: Perm[];
		options: Perm[];
	}
</script>

<script lang="ts">
		import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { outsideWay } from '#lib/ui/sensitive.ts';

	// people is what the server holds; onsaved gets the saved list.
	let { people, permWord, isOwner, onsaved }: { people: NewPeople; permWord: Record<Perm, string>; isOwner: boolean; onsaved: (next: NewPeople) => void } = $props();

	let draft = $state<Perm[]>([]);
	let saving = $state(false);
	let note = $state('');
	// The ticks start from, and return to, what the server holds.
	$effect.pre(() => {
		draft = [...people.perms];
	});
	const dirty = $derived(draft.length !== people.perms.length || draft.some((p) => !people.perms.includes(p)));

	function strongNote(e: unknown): string {
		if (!(e instanceof ApiError && e.status === 403)) return message(e);
		const way = outsideWay(session.s);
		return way ? t('ui.confirm.outsideAgain', { method: way }) : t('settings.signIn.signInToChange');
	}
	// 4K approval needs approval: ticking 4K ticks approval, unticking approval unticks 4K.
	function tick(p: Perm, on: boolean) {
		let next = on ? [...new Set([...draft, p])] : draft.filter((x) => x !== p);
		if (p === 'auto_approve' && !on) next = next.filter((x) => x !== 'auto_approve_four_k');
		if (p === 'auto_approve_four_k' && on) next = [...new Set([...next, 'auto_approve' as Perm])];
		draft = next;
		note = '';
	}
	async function save() {
		saving = true;
		note = '';
		try {
			const r = await api.put<{ perms: Perm[] }>('/users/new-people', { perms: draft });
			onsaved({ ...people, perms: r.perms });
			toasts.push(t('settings.users.saved', { name: t('accounts.sso.newPeople') }), 'ok');
		} catch (e) {
			note = strongNote(e);
			toasts.push(t('settings.users.couldntSave'), 'fail', { body: note });
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard id="new-people-card" title={t('accounts.sso.newPeople')}>
	<fieldset class="field" disabled={!isOwner}>
		<legend class="label">{t('settings.users.may')}</legend>
		{#each people.options as p (p)}
			<label class="checkline"
				><input type="checkbox" data-testid="new-people-{p}" checked={draft.includes(p)} onchange={(e) => tick(p, e.currentTarget.checked)} />
				{permWord[p]}</label
			>
		{/each}
	</fieldset>
	{#if !isOwner}
		<p class="xs muted">{t('settings.signIn.onlyOwnerChange')}</p>
	{:else}
		<div class="row">
			<Button variant="primary" type="button" data-testid="new-people-save" disabled={!dirty || saving} onclick={save}>{t('settings.users.save')}</Button>
		</div>
		{#if note}<p class="xs" role="alert">{note}</p>{/if}
	{/if}
</SettingsCard>

<style>
	fieldset {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-3);
	}
	legend {
		padding: 0;
	}
</style>
