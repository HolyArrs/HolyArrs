<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Confirm from '#lib/account/Confirm.svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';

	// An AI app (Claude Code, Cursor, ChatGPT…) sent the person here to sign
	// in: say what is asking and what it can do, then allow or cancel.
	type Info = { client_name: string; returns_to: string; local: boolean; enabled: boolean; deny_url: string };

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

<svelte:head><title>Connect an AI app · HolyArrs</title></svelte:head>

<PageHeader title="Connect an AI app" />

<div class="card" data-testid="ai-connect">
	{#if error}
		<p class="notice warn">{error}</p>
		<p class="small muted">Start again from your AI app.</p>
	{:else if !info}
		<p class="muted">Loading…</p>
	{:else if done === 'allowed'}
		<h2>{info.client_name} is connected</h2>
		<p class="muted">Go back to {info.client_name}; you can close this tab. To disconnect it later, revoke its key under Settings, AI connections.</p>
	{:else if done === 'cancelled'}
		<h2>Not connected</h2>
		<p class="muted">{info.client_name} did not get access. You can close this tab.</p>
	{:else}
		<h2><b>{info.client_name}</b> wants to use HolyArrs as {session.s.name}</h2>
		<ul class="can small">
			<li>Look up your Library, calendar, downloads and your own requests.</li>
			<li>Find titles, and ask for one after checking with you first.</li>
			<li>It cannot approve requests, change settings or see other people's requests.</li>
		</ul>
		<p class="small muted">Afterwards you go back to {info.returns_to}. Every request it makes shows in History.</p>
		{#if info.local}<p class="small muted">Allow only if you just started this from an AI app on your own device.</p>{/if}
		{#if !canAllow}
			<p class="notice warn">Only people who can change settings can connect AI apps. Ask the owner.</p>
		{:else if !enabled}
			<p class="notice" class:warn={!isOwner}>
				{isOwner ? 'AI connections are off. Allowing turns them on.' : 'AI connections are off. Ask the owner to turn them on, then try again.'}
			</p>
		{/if}
		<!-- Allowing makes the app a key, so it takes a fresh password, as making a key does. -->
		<Confirm action="link" onCancel={cancel}>
			<div class="row">
				<Button variant="primary" type="button" onclick={allow} disabled={busy || !canAllow || (!enabled && !isOwner)} aria-busy={busy}>Allow</Button>
				<Button variant="plain" type="button" onclick={cancel} disabled={busy}>Cancel</Button>
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
