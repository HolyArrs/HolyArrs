<script lang="ts">
	// Remote access: the Tailscale plugin's state, its sign-in link and QR, facts, and Reconnect, Disconnect, Sign out. Refreshes every 5 s while shown.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { QrCode } from '#lib/accounts/index.ts';
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

	const label = $derived(st ? remoteStateLabel(st.state, plugin) : '');
	const signIn = $derived(plugin.running && st?.state === 'waiting_for_sign_in' && !!st.login_url);
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
			st = await api.get<TailscaleStatus>('/plugins/tailscale/status');
			error = '';
		} catch (e) {
			error = message(e);
		}
	}
	onMount(() => {
		void load();
		const timer = setInterval(load, 5000);
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
		void act('signout', '/plugins/tailscale/signout', 'plugins.remoteSignedOut', 'plugins.remoteSignOutFailed');
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
					<Button variant="primary" iconAfter="arrowUpRight" onclick={() => st?.login_url?.startsWith('https://') && window.open(st.login_url, '_blank', 'noopener,noreferrer')}>{t('plugins.remoteOpenSignIn')}</Button>
					{#if st.login_qr?.length}
						<p class="xs muted">{t('plugins.remoteScan')}</p>
						<div class="qr"><QrCode rows={st.login_qr} label={t('plugins.remoteQr')} /></div>
					{/if}
				</div>
			{/if}
			{#if facts.length}
				<dl class="facts small">
					{#each facts as f (f.k)}<dt class="muted">{f.k}</dt><dd>{f.v}</dd>{/each}
				</dl>
			{/if}
		{/if}
		{#if notice}<p class="small" data-testid="remote-notice">{notice}</p>{/if}
		<div class="acts">
			{#if plugin.running}
				<Button disabled={!!busy} busy={busy === 'stop'} onclick={() => act('stop', '/plugins/tailscale/stop', 'plugins.remoteDisconnected', 'plugins.failed')}>{t('plugins.remoteDisconnect')}</Button>
			{:else}
				<Button disabled={!canRun || !!busy} busy={busy === 'start'} onclick={() => act('start', '/plugins/tailscale/start', 'plugins.remoteReconnecting', 'plugins.failed')}>{t('plugins.remoteReconnect')}</Button>
			{/if}
			<Button variant="destructive" disabled={!canRun || !!busy} busy={busy === 'signout'} onclick={() => (confirmOpen = true)}>{t('plugins.remoteSignOut')}</Button>
		</div>
	</div>
</SettingsCard>

<Sheet bind:open={confirmOpen} size="sm" title={t('plugins.remoteSignOutTitle')} description={t('plugins.remoteSignOutAbout')}>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (confirmOpen = false)}>{t('plugins.cancel')}</Button>
		<Button variant="destructive" onclick={signOut}>{t('plugins.remoteSignOut')}</Button>
	</div>
</Sheet>

<style>
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
