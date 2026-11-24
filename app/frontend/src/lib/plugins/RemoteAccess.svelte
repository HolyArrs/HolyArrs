<script lang="ts">
	// Remote access: a forwarding plugin's state (Tailscale, NetBird, a tunnel), Connect (sign-in link and QR) while not connected, facts, and Disconnect / Sign out once connected. Refreshes every 5 s while shown.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Field from '#lib/ui/Field.svelte';
	import { QrCode } from '#lib/accounts/index.ts';
	import { Orbit } from '#lib/ui/sk/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { dateTime } from '#lib/core/format.ts';
	import { connectionLabel, remoteStateLabel, signOutNotice } from './view.ts';
	import type { Plugin, TailscaleStatus } from './types.ts';

	let { plugin, canRun, onchange }: { plugin: Plugin; canRun: boolean; onchange: () => Promise<void> | void } = $props();

	let st = $state<TailscaleStatus | null>(null);
	let error = $state('');
	let busy = $state('');
	let confirmOpen = $state(false);
	let notice = $state('');
	let nameOpen = $state(false);
	let newName = $state('');
	let nameError = $state('');
	// The first part of the address; the rest is the tailnet's own name, set in Tailscale.
	const tailnet = $derived(st?.tsnet_name?.includes('.') ? st.tsnet_name.slice(st.tsnet_name.indexOf('.')) : '');
	function openName() {
		newName = st?.device_name || st?.tsnet_name?.split('.')[0] || 'holyarrs';
		nameError = '';
		nameOpen = true;
	}
	async function saveName() {
		if (busy) return;
		busy = 'name';
		try {
			await api.post(`/plugins/${plugin.id}/remote/name`, { name: newName });
			nameOpen = false;
			toasts.push(t('plugins.remoteNameSaved'), 'ok');
		} catch (e) {
			nameError = message(e);
		} finally {
			await onchange();
			await load();
			busy = '';
		}
	}

	const label = $derived(st ? remoteStateLabel(st.state, plugin) : '');
	const signIn = $derived(plugin.running && st?.state === 'waiting_for_sign_in' && !!st.login_url);
	const connected = $derived(plugin.running && (st?.state === 'connected' || st?.state === 'login_expiring'));
	// Running but no link yet: the plugin is still asking Tailscale for one.
	const gettingLink = $derived(plugin.running && !signIn && (st?.state === 'not_connected' || st?.state === 'waiting_for_sign_in'));
	// Turned on but not up yet (starting, or restarting after sign-in): wait, don't offer Connect again.
	const starting = $derived(!plugin.running && plugin.enabled);
	// A login Tailscale ended can only be reset by signing out.
	const stale = $derived(st?.state === 'login_expired' || st?.state === 'device_deleted');
	// Stopped by the owner while still signed in reads as Reconnect, not Connect.
	const paused = $derived(!plugin.running && !plugin.enabled && plugin.disabled_reason !== 'signed out by the owner');
	function openSignIn() {
		if (st?.login_url?.startsWith('https://')) window.open(st.login_url, '_blank', 'noopener,noreferrer');
	}
	const facts = $derived(
		st
			? [
					{ k: t('plugins.remoteName'), v: st.tsnet_name ?? '' },
					{ k: t('plugins.remoteVersion'), v: st.tailscale_version ?? '' },
					{ k: t('plugins.remoteUpdated'), v: dateTime(st.last_updated) },
					{ k: t('plugins.remoteExpiry'), v: dateTime(st.login_expiry) },
					{ k: t('plugins.remoteLink'), v: connectionLabel(st.connection) }
				].filter((f) => f.v)
			: []
	);

	async function load() {
		try {
			st = await api.get<TailscaleStatus>(`/plugins/${plugin.id}/remote/status`);
			error = '';
		} catch (e) {
			error = message(e);
		}
	}
	// The plugin record (running/enabled) changes too, e.g. while it restarts after sign-in, so refresh both.
	async function tick() {
		await Promise.all([load(), onchange()]);
	}
	onMount(() => {
		void load();
		const timer = setInterval(tick, 5000);
		return () => clearInterval(timer);
	});

	async function act(key: string, path: string, done: string, fail: string) {
		if (busy) return;
		busy = key;
		try {
			const res = await api.post<{ removed_remotely?: boolean }>(path);
			notice = key === 'signout' ? signOutNotice(res) : '';
			toasts.push(t(done), 'ok');
		} catch (e) {
			toasts.push(t(fail), 'fail', { body: message(e) });
		} finally {
			await onchange();
			await load();
			busy = '';
		}
	}
	function signOut() {
		confirmOpen = false;
		void act('signout', `/plugins/${plugin.id}/remote/signout`, 'plugins.remoteSignedOut', 'plugins.remoteSignOutFailed');
	}
</script>

<SettingsCard id="remote-access" title={t('plugins.remoteTitle')} description={t('plugins.remoteAbout')}>
	<div class="body" data-testid="remote-access">
		{#if error && !st}
			<p class="small muted">{t('plugins.remoteLoadFailed')}</p>
		{:else if st}
			<div><Badge label={label} /></div>
			{#if signIn}
				<div class="signin">
					<Button variant="primary" iconAfter="arrowUpRight" onclick={openSignIn}>{t('plugins.remoteConnect')}</Button>
					<p class="small muted">{t('plugins.remoteConnectAbout')}</p>
					{#if st.login_qr?.length}
						<p class="xs muted">{t('plugins.remoteScan')}</p>
						<div class="qr"><QrCode rows={st.login_qr} label={t('plugins.remoteQr')} /></div>
					{/if}
				</div>
			{:else if gettingLink || starting}
				<div data-testid="remote-getting-link"><Orbit status={t(starting ? 'plugins.remoteStarting' : 'plugins.remoteGettingLink')} /></div>
			{/if}
			{#if facts.length}
				<dl class="facts small">
					{#each facts as f (f.k)}<dt class="muted">{f.k}</dt><dd>{f.v}</dd>{/each}
				</dl>
			{/if}
		{/if}
		{#if notice}<p class="small" data-testid="remote-notice">{notice}</p>{/if}
		<div class="acts">
			{#if connected}
				<Button disabled={!canRun || !!busy} onclick={openName}>{t('plugins.remoteChangeName')}</Button>
				<Button disabled={!!busy} busy={busy === 'stop'} onclick={() => act('stop', `/plugins/${plugin.id}/stop`, 'plugins.remoteDisconnected', 'plugins.failed')}>{t('plugins.remoteDisconnect')}</Button>
				<Button variant="destructive" disabled={!canRun || !!busy} busy={busy === 'signout'} onclick={() => (confirmOpen = true)}>{t('plugins.remoteSignOut')}</Button>
			{:else if stale}
				<Button variant="destructive" disabled={!canRun || !!busy} busy={busy === 'signout'} onclick={() => (confirmOpen = true)}>{t('plugins.remoteSignOut')}</Button>
			{/if}
			{#if !plugin.running && !starting}
				<Button variant={paused ? 'secondary' : 'primary'} disabled={!canRun || !!busy} busy={busy === 'start'} onclick={() => act('start', `/plugins/${plugin.id}/start`, 'plugins.remoteReconnecting', 'plugins.failed')}>{t(paused ? 'plugins.remoteReconnect' : 'plugins.remoteConnect')}</Button>
			{/if}
		</div>
	</div>
</SettingsCard>

<Sheet bind:open={confirmOpen} size="sm" title={t('plugins.remoteSignOutTitle')} description={t('plugins.remoteSignOutAbout')}>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (confirmOpen = false)}>{t('plugins.cancel')}</Button>
		<Button variant="destructive" onclick={signOut}>{t('plugins.remoteSignOut')}</Button>
	</div>
</Sheet>

<Sheet bind:open={nameOpen} size="sm" title={t('plugins.remoteChangeName')} description={t('plugins.remoteChangeNameAbout')}>
	<form class="name-form" onsubmit={(e) => { e.preventDefault(); void saveName(); }}>
		<Field id="ts-name" label={t('plugins.remoteName')} hint={tailnet ? newName + tailnet : ''} error={nameError}>
			<input id="ts-name" class="input mono" bind:value={newName} autocomplete="off" spellcheck="false" maxlength="63" />
		</Field>
		<div class="confirm-actions">
			<Button type="button" variant="plain" onclick={() => (nameOpen = false)}>{t('plugins.cancel')}</Button>
			<Button variant="primary" type="submit" busy={busy === 'name'} disabled={!!busy}>{t('plugins.remoteNameSave')}</Button>
		</div>
	</form>
</Sheet>

<style>
	.name-form { display: grid; gap: var(--s-3); }
	.body { display: grid; gap: var(--s-3); margin-top: var(--s-3); }
	.signin { display: grid; gap: var(--s-2); justify-items: start; }
	.qr { width: 180px; max-width: 100%; }
	.facts { display: grid; grid-template-columns: max-content 1fr; gap: 4px var(--s-3); margin: 0; }
	.facts dd { margin: 0; overflow-wrap: anywhere; }
	.acts { display: flex; flex-wrap: wrap; gap: 8px; }
	.confirm-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-top: var(--s-4); }
	@media (max-width: 560px) {
		.facts { grid-template-columns: 1fr; gap: 0; }
		.facts dd { margin-bottom: var(--s-2); }
	}
</style>
