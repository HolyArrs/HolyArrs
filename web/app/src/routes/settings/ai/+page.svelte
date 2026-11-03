<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, date } from '#lib/format.ts';
	import { session } from '#lib/session.svelte.ts';
	import { getBase } from '#lib/paths.ts';
	import type { ApiKey } from '#lib/types.ts';
	import { AI_SCOPE, aiURL, claudeCodeCommand, claudeDesktopConfig, isAIKey } from '#lib/ai-connect.ts';

	let loaded = $state(false);
	let error = $state('');
	let enabled = $state(false);
	let saving = $state(false);
	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	let fresh = $state<{ key: string; label: string } | null>(null);
	let tab = $state<'code' | 'desktop'>('code');
	const appOptions: { value: 'code' | 'desktop'; label: string }[] = [
		{ value: 'code', label: 'Claude Code' },
		{ value: 'desktop', label: 'Claude Desktop' }
	];

	const isOwner = $derived(session.s.role === 'owner');
	const url = $derived(typeof window === 'undefined' ? '/api/v1/mcp' : aiURL(window.location.origin, getBase()));
	const aiKeys = $derived(keys.filter(isAIKey));
	const setup = $derived(tab === 'code' ? claudeCodeCommand(url, fresh?.key) : claudeDesktopConfig(url, fresh?.key));

	function keyNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? 'Sign in with your password to manage AI keys.' : message(e);
	}

	async function load() {
		error = '';
		try {
			enabled = (await api.get<{ enabled: boolean }>('/settings/ai')).enabled;
		} catch (e) {
			error = message(e);
		}
		try {
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
			keysNote = '';
		} catch (e) {
			keysNote = keyNote(e);
		}
		loaded = true;
	}

	onMount(load);

	async function setEnabled(v: boolean) {
		if (saving) return;
		const before = enabled;
		enabled = v;
		saving = true;
		try {
			enabled = (await api.put<{ enabled: boolean }>('/settings/ai', { enabled: v })).enabled;
			toasts.push(enabled ? 'AI connections are on.' : 'AI connections are off.', 'ok');
		} catch (e) {
			enabled = before;
			toasts.push(message(e), 'fail');
		} finally {
			saving = false;
		}
	}

	async function makeKey(e: SubmitEvent) {
		e.preventDefault();
		if (!label.trim()) return;
		making = true;
		try {
			const r = await api.post<{ key: string; label: string }>('/auth/keys', { label: label.trim(), scopes: [AI_SCOPE] });
			fresh = { key: r.key, label: r.label };
			label = '';
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (err) {
			toasts.push(keyNote(err), 'fail');
		} finally {
			making = false;
		}
	}

	async function revoke(k: ApiKey) {
		if (!confirm(`Revoke the AI key "${k.label}"? The AI app using it stops working.`)) return;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			toasts.push('AI key revoked.', 'ok');
		} catch (e) {
			toasts.push(keyNote(e), 'fail');
		}
	}

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; select the text and copy it by hand.', 'fail');
		}
	}
</script>

<svelte:head><title>AI connections · HolyArrs</title></svelte:head>

<PageHeader title="AI connections" />

{#if !loaded}
	<SkSettings rows={3} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} busy={false} />
{:else}
	<SettingsCard id="ai-switch" title="AI connections">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">
			AI assistants like Claude can check your library, calendar and downloads, and ask for titles for you. They can only do what you allow, and
			everything they do shows in History.
		</p>
		<label class="switch">
			<input type="checkbox" checked={enabled} disabled={!isOwner || saving} aria-busy={saving} onchange={(e) => setEnabled(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Let AI apps connect</span>
		</label>
		{#if !isOwner}<p class="hint">Only the owner can turn this on or off.</p>{/if}
		<ul class="facts small muted">
			<li>An AI key acts as you: your permissions and request limits apply.</li>
			<li>Before it asks for a title, the AI app has to check with you. Approving requests stays with people, on the Requests page.</li>
			<li>At most 60 look-ups a minute and 5 actions an hour per key.</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="ai-keys" title="AI keys">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">An AI key works only for AI connections, not for the rest of HolyArrs.</p>
		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			{#if fresh}
				<div class="fresh" role="status">
					<p class="small"><b>{fresh.label}</b>: copy this key now. It is shown only once.</p>
					<div class="row">
						<code class="key grow">{fresh.key}</code>
						<Button icon="copy" type="button" onclick={() => fresh && copy(fresh.key)}>Copy</Button>
						<Button variant="plain" type="button" onclick={() => (fresh = null)}>Done</Button>
					</div>
				</div>
			{/if}
			<form class="row" onsubmit={makeKey}>
				<label class="sr-only" for="ai-key-label">Key name</label>
				<input id="ai-key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="Which AI app? (Claude Desktop)" maxlength="64" />
				<Button icon="key" variant="primary" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make an AI key</Button>
			</form>
			{#if aiKeys.length === 0}
				<p class="small muted" style="margin-top: var(--s-4)">No AI keys yet.</p>
			{:else}
				<ul class="rows" style="margin-top: var(--s-3)">
					{#each aiKeys as k (k.id)}
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

	<SettingsCard id="ai-connect" title="Connect Claude">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">
			Address: <code class="addr">{url}</code>. Send the key as <code>Authorization: Bearer</code>, never in the address.
		</p>
		<div class="tabs"><SegmentedControl label="AI app" size="sm" options={appOptions} bind:value={tab} /></div>
		<p class="small muted">
			{#if tab === 'code'}
				Run this in a terminal:
			{:else}
				Add this to <code>claude_desktop_config.json</code> (Settings, Developer, Edit config), then restart Claude Desktop. It needs Node.js.
			{/if}
		</p>
		<pre class="setup">{setup}</pre>
		<Button icon="copy" type="button" onclick={() => copy(setup)}>Copy</Button>
		{#if !fresh}<p class="hint">Make an AI key first; it is filled in here while it is shown.</p>{/if}
		{#if !enabled}<p class="hint">Turn on AI connections above, or the AI app gets "not found".</p>{/if}
	</SettingsCard>
{/if}

<style>
	.facts {
		margin: var(--s-3) 0 0;
		padding-left: 1.2em;
		display: grid;
		gap: 4px;
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
	.row {
		flex-wrap: wrap;
	}
	.key,
	.addr {
		overflow-wrap: anywhere;
	}
	.key {
		padding: 10px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		font-size: var(--text-sm);
		min-width: 0;
	}
	.tabs {
		margin-bottom: var(--s-3);
	}
	.setup {
		margin: var(--s-2) 0 var(--s-3);
		padding: var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		font-size: var(--text-xs, 12px);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		max-width: 100%;
	}
</style>
