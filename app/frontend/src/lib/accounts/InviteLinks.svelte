<script lang="ts" module>
	export interface Invite {
		id: string;
		role: Role;
		perms: Perm[];
		note: string;
		created_by: string;
		created_at: string;
		expires_at: string;
		used_at?: string;
		used_by?: string;
		max_rating?: string;
		rating_country?: string;
		// How many people the link takes (0 for no limit), how many joined, and who.
		max_uses: number;
		uses: number;
		joined: string[];
		never_expires: boolean;
	}
</script>

<script lang="ts">
	// The Invite links card of Settings > Users and roles: each link's state, who joined through it, and Revoke.
	import PersonText from '#lib/ui/PersonText.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import type { Perm, Role } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { shownPerson } from '#lib/core/plain.ts';
	import { ago, date } from '#lib/core/format.ts';
	import { kidLabel } from './kid.ts';

	let { invites, roleWord, onchange }: { invites: Invite[]; roleWord: Record<string, string>; onchange: () => Promise<void> } = $props();

	async function takeBack(i: Invite) {
		try {
			await api.del(`/users/invites/${i.id}`);
			// A link people joined through stays in the list, ended; one nobody used goes. The page reads the list again.
			await onchange();
			toasts.push(t('settings.users.takenBack'), 'ok');
		} catch (e) {
			toasts.push(t('settings.users.takeBackFailed'), 'fail', { body: message(e) });
		}
	}

	function inviteState(i: Invite): string {
		// A link for one person reads as it always did.
		if (i.max_uses === 1 && i.used_at) return t('settings.users.usedBy', { who: shownPerson(i.used_by) ?? t('settings.users.someone'), ago: ago(i.used_at) });
		if (i.used_at || new Date(i.expires_at).getTime() < Date.now()) return t('settings.users.ended');
		if (i.never_expires) return t('settings.users.openNoEnd');
		return t('settings.users.openUntil', { date: date(i.expires_at) });
	}

	// How many joined through a link for several people, with their names.
	function inviteJoined(i: Invite): string {
		if (i.max_uses === 1) return '';
		const count = i.max_uses === 0 ? t('settings.users.joined', { n: i.uses }) : t('settings.users.joinedOf', { n: i.uses, max: i.max_uses });
		const who = (i.joined ?? []).map((n) => shownPerson(n) ?? n).join(', ');
		return who ? `${count} (${who})` : count;
	}
</script>

<SettingsCard id="invites-card" title={t('settings.users.inviteLinks')}>
	{#if invites.length === 0}
		<p class="small muted">{t('settings.users.noInvites')}</p>
	{:else}
		<ul class="rows">
			{#each invites as i (i.id)}
				<li>
					<div class="grow">
						<div class="name">{i.note || t('settings.users.roleInvite', { role: roleWord[i.role] })} <span class="chip">{roleWord[i.role]}</span>{#if i.max_rating}<span class="chip">{kidLabel(i.max_rating)}</span>{/if}</div>
						<div class="xs muted"><PersonText text={t('settings.users.madeBy', { who: shownPerson(i.created_by), ago: ago(i.created_at) })} /> · <PersonText text={inviteState(i)} />{#if inviteJoined(i)} · <PersonText text={inviteJoined(i)} />{/if}</div>
					</div>
					{#if !i.used_at && new Date(i.expires_at).getTime() > Date.now()}
						<Button variant="secondary" type="button" onclick={() => takeBack(i)}>{t('settings.users.revoke')}</Button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</SettingsCard>

<style>
	/* The same name line the People rows of the page have. */
	.name {
		font-weight: var(--weight-semibold);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
	}
</style>
