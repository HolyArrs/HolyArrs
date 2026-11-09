<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, date } from '#lib/core/format.ts';
	import type { ApiKey } from '#lib/core/types.ts';

	// API keys for other apps: make one (shown once), see when each was last used, revoke.
	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	let fresh = $state<{ key: string; label: string } | null>(null);

	function strongNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? 'Sign in with your password to manage API keys.' : message(e);
	}

	onMount(async () => {
		try {
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (e) {
			keysNote = strongNote(e);
		}
	});

	async function makeKey(e: SubmitEvent) {
		e.preventDefault();
		if (!label.trim()) return;
		making = true;
		try {
			const r = await api.post<{ id: number; label: string; key: string }>('/auth/keys', { label: label.trim() });
			fresh = { key: r.key, label: r.label };
			label = '';
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (err) {
			toasts.push("Couldn't make key", 'fail', { body: strongNote(err) });
		} finally {
			making = false;
		}
	}

	async function revoke(k: ApiKey) {
		if (!confirm(`Revoke the API key "${k.label}"? Anything using it stops working.`)) return;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			toasts.push('API key revoked', 'ok');
		} catch (e) {
			toasts.push("Couldn't revoke key", 'fail', { body: strongNote(e) });
		}
	}

	async function copy() {
		if (!fresh) return;
		try {
			await navigator.clipboard.writeText(fresh.key);
			toasts.push('API key copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}
</script>

<SettingsCard id="keys-card" title="API keys">
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">Send in the <code>X-Api-Key</code> header, never in an address</p>

	{#if keysNote}
		<p class="notice warn">{keysNote}</p>
	{:else}
		{#if fresh}
			<div class="fresh" role="status">
				<p class="small"><b>{fresh.label}</b>: copy this key now. It is shown only once.</p>
				<div class="row">
					<code class="key grow">{fresh.key}</code>
					<Button icon="copy" type="button" onclick={copy}>Copy</Button>
					<Button variant="plain" type="button" onclick={() => (fresh = null)}>Close</Button>
				</div>
			</div>
		{/if}

		<form class="row" onsubmit={makeKey}>
			<label class="sr-only" for="key-label">Key name</label>
			<input id="key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="e.g. Home Assistant" maxlength="64" />
			<Button icon="key" variant="primary" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make a key</Button>
		</form>

		{#if keys.length === 0}
			<p class="small muted" style="margin-top: var(--s-4)">No API keys</p>
		{:else}
			<ul class="rows" style="margin-top: var(--s-3)">
				{#each keys as k (k.id)}
					<li>
						<div class="grow">
							<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
							<div class="xs muted">Made {date(k.created_at)} · {k.last_used_at ? `last used ${ago(k.last_used_at)}` : 'never used'}</div>
						</div>
						<Button variant="destructive" type="button" onclick={() => revoke(k)}>Revoke</Button>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</SettingsCard>

<style>
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
	.name {
		font-weight: var(--weight-semibold);
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.fresh .row {
		margin-top: var(--s-2);
	}
	.key {
		padding: 10px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		overflow-wrap: anywhere;
		font-size: var(--text-sm);
	}
</style>
