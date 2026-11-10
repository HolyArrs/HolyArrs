<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';

	// An AI app sent the person here to sign in: say what is asking and what it can do, then allow or cancel.
	type Info = { client_name: string; returns_to: string; local: boolean; name_verified?: boolean; enabled: boolean; deny_url: string };

	let info = $state<Info | null>(null);
	let error = $state('');
	let enabled = $state(true);
	let busy = $state(false);
	let done = $state<'allowed' | 'cancelled' | ''>('');

	const query = page.url.search.replace(/^\?/, '');
	const isOwner = $derived(session.s.role === 'owner');
	// Allowing gives the app a key, so it takes what making a key takes.
	const canAllow = $derived(session.can('settings'));

	onMount(async () => {
		try {
			info = await api.get<Info>(`/auth/ai-connect?${query}`);
			enabled = info.enabled;
		} catch (e) {
			error = message(e);
		}
	});

	async function allow() {
		busy = true;
		try {
			if (!enabled) enabled = (await api.put<{ enabled: boolean }>('/settings/ai', { enabled: true })).enabled;
			const r = await api.post<{ redirect: string }>('/auth/ai-connect', { query });
			done = 'allowed';
			window.location.href = r.redirect;
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}

	function cancel() {
		if (!info) return;
		done = 'cancelled';
		window.location.href = info.deny_url;
	}
</script>

<svelte:head><title>{t('arrapi.ai.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('arrapi.ai.title')} />

<div class="card" data-testid="ai-connect">
	{#if error}
		<p class="notice warn">{error}</p>
		<p class="small muted">{t('arrapi.ai.startAgain')}</p>
	{:else if !info}
		<p class="muted">{t('arrapi.ai.loading')}</p>
	{:else if done === 'allowed'}
		<h2>{t('arrapi.ai.connected', { name: info.client_name })}</h2>
		<p class="muted">{t('arrapi.ai.connectedBody', { name: info.client_name })}</p>
	{:else if done === 'cancelled'}
		<h2>{t('arrapi.ai.notConnected')}</h2>
		<p class="muted">{t('arrapi.ai.noAccess', { name: info.client_name })}</p>
	{:else}
		<!-- Any app can send any name, so the return address leads; the name is marked unless it comes back to this computer. -->
		{@const wants = t('arrapi.ai.wants', { host: '{host}', name: session.s.name }).split('{host}')}
		<h2 data-testid="ai-connect-host">{wants[0]}<b>{info.returns_to}</b>{wants[1]}</h2>
		<p class="small muted" data-testid="ai-connect-name">{t('arrapi.ai.says', { name: info.client_name })}{#if !info.name_verified} · {t('arrapi.ai.notVerified')}{/if}</p>
		<ul class="can small">
			<li>{t('arrapi.ai.can1')}</li>
			<li>{t('arrapi.ai.can2')}</li>
			<li>{t('arrapi.ai.can3')}</li>
		</ul>
		<p class="small muted">{t('arrapi.ai.history')}</p>
		{#if info.local}<p class="small muted">{t('arrapi.ai.localWarn')}</p>{/if}
		{#if !canAllow}
			<p class="notice warn">{t('arrapi.ai.cannot')}</p>
		{:else if !enabled}
			<p class="notice" class:warn={!isOwner}>
				{isOwner ? t('arrapi.ai.offOwner') : t('arrapi.ai.offOther')}
			</p>
		{/if}
		<!-- Allowing makes the app a key, so it takes a fresh password, as making a key does. -->
		<Confirm action="link" onCancel={cancel}>
			<div class="row">
				<Button variant="primary" type="button" onclick={allow} disabled={busy || !canAllow || (!enabled && !isOwner)} aria-busy={busy}>{t('arrapi.ai.allow')}</Button>
				<Button variant="plain" type="button" onclick={cancel} disabled={busy}>{t('arrapi.ai.cancel')}</Button>
			</div>
		</Confirm>
	{/if}
</div>

<style>
	.card {
		max-width: 560px;
		padding: var(--s-5, 24px);
		border: 1px solid var(--line, rgba(255, 255, 255, 0.1));
		border-radius: var(--r-card, 12px);
		background: var(--surface-1, transparent);
	}
	h2 {
		font-size: var(--text-lg, 18px);
		font-weight: 500;
		margin: 0 0 var(--s-3);
		overflow-wrap: anywhere;
	}
	.can {
		margin: 0 0 var(--s-3);
		padding-left: 1.2em;
		display: grid;
		gap: 4px;
	}
	.row {
		margin-top: var(--s-4);
		gap: var(--s-2);
		flex-wrap: wrap;
	}
</style>
