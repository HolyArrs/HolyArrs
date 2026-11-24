<script lang="ts">
	// Fetch key found the old app's sign-in on: a sign-in to it, sent once for the fetch and never kept.
	// Seerr also takes a Jellyfin or Emby account; Seerr and Overseerr can use the owner's kept Plex sign-in.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { appName } from './move.ts';

	let {
		kind,
		busy,
		fetching,
		plexAvailable,
		mediaServer,
		onsignin
	}: {
		kind: string;
		busy: boolean;
		fetching: boolean;
		plexAvailable: boolean;
		mediaServer: string;
		onsignin: (login: { username?: string; password?: string; use_plex?: boolean; use_media_server?: boolean }) => Promise<{ key?: string; problem?: string }>;
	} = $props();
	let user = $state('');
	let pass = $state('');
	let problem = $state('');
	// The typed password is cleared after every try.
	async function signIn(usePlex: boolean, useMediaServer = false) {
		problem = '';
		const r = await onsignin(usePlex ? { use_plex: true } : { username: user.trim(), password: pass, use_media_server: useMediaServer });
		pass = '';
		if (!r.key) problem = r.problem || 'wrong_login';
	}
	const userLabel = $derived(kind === 'overseerr' ? t('accounts.account.email') : t('migrate.userName'));
	// Enter in these fields signs in, instead of the outer form's still-disabled Continue.
	function enter(e: KeyboardEvent) {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		if (!busy && !fetching && user.trim() && pass) void signIn(false);
	}
</script>

<div class="key-login" data-testid="read-key-login">
	<div class="field"><label for="read-key-user">{userLabel}</label>
		<input id="read-key-user" class="input" type="text" bind:value={user} onkeydown={enter} autocomplete="off" spellcheck="false" disabled={busy || fetching} data-testid="read-key-user" /></div>
	<div class="field"><label for="read-key-pass">{t('migrate.passwordLabel')}</label>
		<input id="read-key-pass" class="input" type="password" bind:value={pass} onkeydown={enter} autocomplete="off" disabled={busy || fetching} data-testid="read-key-pass" /></div>
	<div class="key-login-row">
		<Button full onclick={() => signIn(false)} busy={fetching} disabled={busy || !user.trim() || !pass} data-testid="read-key-signin">{t('migrate.keyLogin.signInFetch')}</Button>
		{#if kind === 'seerr' && mediaServer}
			{#each [mediaServer] as ms (ms)}<Button variant="secondary" onclick={() => signIn(false, true)} disabled={busy || fetching || !user.trim() || !pass} data-testid="read-key-{ms}">{t('migrate.keyLogin.withServer', { server: ms === 'emby' ? 'Emby' : 'Jellyfin' })}</Button>{/each}
		{/if}
		{#if (kind === 'seerr' || kind === 'overseerr') && plexAvailable}<Button variant="secondary" onclick={() => signIn(true)} disabled={busy || fetching} data-testid="read-key-plex">{t('migrate.keyLogin.usePlex')}</Button>{/if}
	</div>
	{#if problem}<span class="form-error" role="alert" data-testid="read-key-problem">{t(problem === 'not_admin' ? 'migrate.keyLogin.notAdmin' : problem === 'unreachable' ? 'migrate.keyLogin.unreachable' : 'migrate.keyLogin.wrong', { name: appName(kind) })}</span>
	{:else}<span class="hint">{t('migrate.keyLogin.ask', { name: appName(kind) })}</span>{/if}
</div>

<style>
	.key-login { display: grid; gap: 8px; margin-top: 8px; }
	.key-login-row { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; }
	.key-login-row > :global(*) { flex: 1 1 100%; min-height: 44px; }
	.field { display: grid; gap: 6px; text-align: center; }
	.form-error { margin: 0; }
</style>
