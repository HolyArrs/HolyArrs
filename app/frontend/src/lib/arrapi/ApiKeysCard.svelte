<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
	const headerLine = $derived(t('arrapi.keys.header', { code: '{code}' }).split('{code}'));

	function strongNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? t('arrapi.keys.signIn') : message(e);
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
			toasts.push(t('arrapi.keys.makeFail'), 'fail', { body: strongNote(err) });
		} finally {
			making = false;
		}
	}

	async function revoke(k: ApiKey) {
		if (!confirm(t('arrapi.keys.revokeConfirm', { label: k.label }))) return;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			toasts.push(t('arrapi.keys.revoked'), 'ok');
		} catch (e) {
			toasts.push(t('arrapi.keys.revokeFail'), 'fail', { body: strongNote(e) });
		}
	}

	async function copy() {
		if (!fresh) return;
		try {
			await navigator.clipboard.writeText(fresh.key);
			toasts.push(t('arrapi.keys.copied'), 'ok');
		} catch {
			toasts.push(t('arrapi.keys.copyFail'), 'fail', { body: t('arrapi.keys.copyFailBody') });
		}
	}
</script>

<SettingsCard id="keys-card" title={t('arrapi.keys.title')}>
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">{headerLine[0]}<code>X-Api-Key</code>{headerLine[1]}</p>

	{#if keysNote}
		<p class="notice warn">{keysNote}</p>
	{:else}
		{#if fresh}
			<div class="fresh" role="status">
				<p class="small"><b>{fresh.label}</b>{t('arrapi.keys.freshBody')}</p>
				<div class="row">
					<code class="key grow">{fresh.key}</code>
					<Button icon="copy" type="button" onclick={copy}>{t('arrapi.keys.copy')}</Button>
					<Button variant="plain" type="button" onclick={() => (fresh = null)}>{t('arrapi.keys.close')}</Button>
				</div>
			</div>
		{/if}

		<form class="row" onsubmit={makeKey}>
			<label class="sr-only" for="key-label">{t('arrapi.keys.name')}</label>
			<input id="key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder={t('arrapi.keys.placeholder')} maxlength="64" />
			<Button icon="key" variant="primary" type="submit" disabled={making || !label.trim()} aria-busy={making}>{t('arrapi.keys.make')}</Button>
		</form>

		{#if keys.length === 0}
			<p class="small muted" style="margin-top: var(--s-4)">{t('arrapi.keys.none')}</p>
		{:else}
			<ul class="rows" style="margin-top: var(--s-3)">
				{#each keys as k (k.id)}
					<li>
						<div class="grow">
							<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
							<div class="xs muted">{k.last_used_at ? t('arrapi.keys.madeUsed', { made: date(k.created_at), ago: ago(k.last_used_at) }) : t('arrapi.keys.madeNever', { made: date(k.created_at) })}</div>
						</div>
						<Button variant="destructive" type="button" onclick={() => revoke(k)}>{t('arrapi.keys.revoke')}</Button>
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
