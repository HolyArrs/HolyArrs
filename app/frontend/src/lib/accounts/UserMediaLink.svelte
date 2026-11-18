<script lang="ts">
	// Settings > Users, owner only: link a person to their account on a connected Jellyfin or Emby server,
	// so the Library shows what they watched. The servers' account lists load when the picker opens.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { accountChoice, type ServerAccount as Account } from './media-link.ts';

	type Server = { id: number; name: string; kind: string; accounts: Account[]; error?: string };
	let { user, links, onchange }: { user: { id: number; name: string }; links: { provider: string; name: string }[]; onchange: () => void } = $props();

	let servers = $state<Server[] | null>(null);
	let open = $state(false);
	let pick = $state('');
	let busy = $state(false);
	let error = $state('');
	const linked = $derived(links.filter((l) => l.provider === 'jellyfin' || l.provider === 'emby'));

	async function start() {
		open = true;
		error = '';
		if (!servers) {
			try {
				servers = (await api.get<{ servers: Server[] }>('/users/media-accounts')).servers;
			} catch (err) {
				error = message(err);
			}
		}
	}

	async function save() {
		const [server, ...rest] = pick.split(':');
		busy = true;
		error = '';
		try {
			const l = await api.put<{ name: string }>(`/users/${user.id}/media-link`, { server: Number(server), account: rest.join(':') });
			toasts.push(t('settings.users.accountLinked', { name: l.name }), 'ok');
			open = false;
			pick = '';
			onchange();
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function unlink(kind: string) {
		busy = true;
		try {
			await api.del(`/users/${user.id}/media-link/${kind}`);
			onchange();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

<div class="media-link" data-testid="user-media-link">
	{#each linked as l (l.provider)}
		<Button variant="secondary" type="button" onclick={() => unlink(l.provider)} disabled={busy}
			aria-label={t('settings.users.unlinkLabel', { kind: l.provider === 'emby' ? 'Emby' : 'Jellyfin', name: user.name })}>{t('settings.users.unlink')}</Button>
	{/each}
	{#if !open}
		<Button variant="secondary" type="button" onclick={start}>{t('settings.users.serverAccount')}</Button>
	{:else}
		<label class="sr-only" for="ml-{user.id}">{t('settings.users.pickAccount')}</label>
		<select id="ml-{user.id}" class="input" bind:value={pick}>
			<option value="">{t('settings.users.pickAccount')}</option>
			{#each servers ?? [] as s (s.id)}
				<optgroup label={s.error ? `${s.name}: ${message(s.error)}` : s.name}>
					{#each s.accounts as a (a.id)}{@const c = accountChoice(a, user.id, t)}<option value="{s.id}:{a.id}" disabled={c.taken}>{c.label}</option>{/each}
				</optgroup>
			{/each}
		</select>
		<Button type="button" onclick={save} disabled={!pick || busy} busy={busy}>{t('accounts.security.link')}</Button>
		<Button variant="plain" type="button" onclick={() => (open = false)}>{t('accounts.signin.cancel')}</Button>
	{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
</div>

<style>
	.media-link { display: contents; }
	.media-link .form-error { flex-basis: 100%; }
	.media-link select { max-width: 100%; min-width: 0; }
</style>
