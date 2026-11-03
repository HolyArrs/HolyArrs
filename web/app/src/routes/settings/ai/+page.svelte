<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, date } from '#lib/format.ts';
	import { session } from '#lib/session.svelte.ts';
	import { getBase } from '#lib/paths.ts';
	import type { ApiKey } from '#lib/types.ts';
	import { AI_SCOPE, FAMILIES, GUIDES_URL, TRY_FIRST, agentPrompt, aiURL, canSignIn, familyOf, isAIKey, onlyLocal, type Ctx } from '#lib/ai-connect.ts';

	let loaded = $state(false);
	let error = $state('');
	let enabled = $state(false);
	let saving = $state(false);
	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	// A key made from the guide fills in only for the app it was made for.
	let fresh = $state<{ key: string; label: string; app?: string } | null>(null);
	// The chosen app sits in the address (?app=cursor), so a link opens on it.
	let app = $state(page.url.searchParams.get('app') ?? FAMILIES[0].apps[0].id);
	let showPrompt = $state(false);

	const isOwner = $derived(session.s.role === 'owner');
	const url = $derived(typeof window === 'undefined' ? '/api/v1/mcp' : aiURL(window.location.origin, getBase()));
	const aiKeys = $derived(keys.filter(isAIKey));
	const signIn = $derived(canSignIn(url));
	const ctx = $derived<Ctx>({ url, key: fresh?.app === app ? fresh.key : undefined, signIn });
	const family = $derived(familyOf(app));
	const chosen = $derived(family.apps.find((x) => x.id === app) ?? family.apps[0]);
	const unreachable = $derived(!!chosen?.cloud && onlyLocal(url));
	const needsKey = $derived(!chosen.cloud && (!signIn || chosen.signIn === false));
	const host = $derived(url.replace(/^https?:\/\//, '').split('/')[0]);

	function pick(id: string) {
		app = id;
		showPrompt = false;
		const u = new URL(page.url.href);
		u.searchParams.set('app', id);
		replaceState(u, page.state);
	}

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
		if (label.trim()) await newKey(label.trim());
		label = '';
	}

	async function newKey(name: string, forApp?: string) {
		making = true;
		try {
			const r = await api.post<{ key: string; label: string }>('/auth/keys', { label: name, scopes: [AI_SCOPE] });
			fresh = { key: r.key, label: r.label, app: forApp };
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
			AI assistants like Claude, ChatGPT or Cursor can check your library, calendar and downloads, and ask for titles for you. They can only do what you
			allow, and every request they make shows in History.
		</p>
		<label class="switch">
			<input type="checkbox" checked={enabled} disabled={!isOwner || saving} aria-busy={saving} onchange={(e) => setEnabled(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Let AI apps connect</span>
		</label>
		{#if !isOwner}<p class="hint">Only the owner can turn this on or off.</p>{/if}
		<ul class="facts small muted">
			<li>An AI app acts as you: your permissions and request limits apply.</li>
			<li>Before it asks for a title, the AI app has to check with you. Approving requests stays with people, on the Requests page.</li>
			<li>At most 60 calls a minute and 5 requests an hour per app.</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="ai-connect" title="Connect an AI app">
		<div class="addr-row">
			<code class="addr grow" data-testid="ai-address">{url}</code>
			<Button icon="copy" size="sm" type="button" onclick={() => copy(url)}>Copy</Button>
		</div>
		<p class="small muted mode" data-testid="ai-mode">
			{#if signIn}
				Apps sign in through your browser with your HolyArrs password. Nothing to paste.
			{:else}
				On this address apps use an AI key. Press <b>Make a key</b> in the steps and it is filled in for you.
			{/if}
		</p>
		{#if !enabled}<p class="notice warn">Turn on AI connections above first, or the app gets "not found".</p>{/if}

		<div class="apps" role="group" aria-label="Kind of AI app">
			{#each FAMILIES as f (f.id)}
				<Chip label={f.name} selected={family.id === f.id} onclick={() => pick(f.apps[0].id)} />
			{/each}
		</div>
		{#if family.apps.length > 1}
			<div class="apps sub" role="group" aria-label="{family.name} app">
				{#each family.apps as a (a.id)}
					<Chip label={a.name} selected={chosen.id === a.id} onclick={() => pick(a.id)} />
				{/each}
			</div>
		{/if}

		<div class="guide" data-testid="ai-guide">
			{#if unreachable}
				<p class="notice" data-testid="ai-unreachable">
					{chosen.name} connects from the internet, so it cannot reach {host}. It works once HolyArrs has a public https address (your reverse proxy,
					Cloudflare Tunnel or Tailscale Funnel); open this page on that address for the steps.
					{#if family.id === 'claude'}On this network, pick "Claude Desktop (home)" instead.{/if}
				</p>
				<a class="xs guide-link" href={GUIDES_URL + 'reach-holyarrs/'} target="_blank" rel="noopener noreferrer">How to reach HolyArrs from outside your home</a>
			{:else}
				{#if chosen.needs}<p class="small muted needs">{chosen.needs}</p>{/if}
				{#if needsKey && fresh?.app !== chosen.id}
					<div class="keyrow">
						<Button icon="key" variant="primary" type="button" onclick={() => newKey(chosen.name, chosen.id)} disabled={making || !!keysNote} aria-busy={making}>
							Make a key for {chosen.name}
						</Button>
						<span class="xs muted">It is filled into the steps below and shown only now.</span>
					</div>
				{/if}
				<ol class="steps">
					{#each chosen.steps(ctx) as s, i (i)}
						<li>
							<span class="num" aria-hidden="true">{i + 1}</span>
							<div class="grow">
								<p class="st">
									{#each s.text.split('`') as part, j (j)}{#if j % 2}<code>{part}</code>{:else}{part}{/if}{/each}
								</p>
								{#if s.code}
									<div class="codebox">
										<pre>{s.code}</pre>
										<Button icon="copy" size="sm" type="button" onclick={() => copy(s.code ?? '')}>Copy</Button>
									</div>
								{/if}
							</div>
						</li>
					{/each}
				</ol>
				{#if chosen.agent && signIn}
					<button type="button" class="linkish small" aria-expanded={showPrompt} onclick={() => (showPrompt = !showPrompt)}>
						Or let {chosen.name} set itself up
					</button>
					{#if showPrompt}
						<div class="codebox">
							<pre>{agentPrompt(chosen.name, url)}</pre>
							<Button icon="copy" size="sm" type="button" onclick={() => copy(agentPrompt(chosen.name, url))}>Copy</Button>
						</div>
					{/if}
				{/if}
				{#if chosen.check}<p class="xs muted"><b>Check:</b> {chosen.check}</p>{/if}
				{#if chosen.note}<p class="xs muted">{chosen.note}</p>{/if}
				{#if chosen.untried}<p class="xs muted">Not tested with HolyArrs yet: these steps follow {chosen.name}'s docs.</p>{/if}
			{/if}
			<p class="xs links">
				<a class="guide-link" href={GUIDES_URL + chosen.guide} target="_blank" rel="noopener noreferrer" data-testid="ai-full-guide">Guide: {chosen.name}</a>
				{#if family.local}· <a class="guide-link" href={GUIDES_URL + 'local-models/'} target="_blank" rel="noopener noreferrer">Pick a local model</a>{/if}
				· <a class="guide-link" href={GUIDES_URL} target="_blank" rel="noopener noreferrer">All AI guides</a>
			</p>
		</div>

		<div class="try">
			<div class="xs muted">Try this first</div>
			<p class="small">"{TRY_FIRST}"</p>
		</div>
	</SettingsCard>

	<SettingsCard id="ai-keys" title="Connected apps and AI keys">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
			Each app that signs in gets its own key here. A key works only for AI connections; revoke it to disconnect the app.
		</p>
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
			{#if aiKeys.length === 0}
				<p class="small muted">No AI apps yet.</p>
			{:else}
				<ul class="rows">
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
			<details class="more" style="margin-top: var(--s-4)">
				<summary>Make a key by hand</summary>
				<form class="row" onsubmit={makeKey}>
					<label class="sr-only" for="ai-key-label">Key name</label>
					<input id="ai-key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="Which AI app? (Claude Desktop)" maxlength="64" />
					<Button icon="key" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make an AI key</Button>
				</form>
			</details>
		{/if}
	</SettingsCard>
{/if}

<style>
	.facts {
		margin: var(--s-3) 0 0;
		padding-left: 1.2em;
		display: grid;
		gap: 4px;
	}
	.addr-row,
	.keyrow {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		margin-top: var(--s-2);
	}
	.addr,
	.key {
		padding: 8px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		font-size: var(--text-sm);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.mode {
		margin: var(--s-2) 0 var(--s-3);
	}
	.apps {
		display: flex;
		gap: var(--s-2);
		overflow-x: auto;
		padding-bottom: 4px;
		margin-bottom: var(--s-3);
		scrollbar-width: thin;
	}
	.apps :global(*) {
		flex-shrink: 0;
	}
	.apps.sub {
		margin-top: calc(-1 * var(--s-2));
	}
	.links {
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.guide {
		display: grid;
		gap: var(--s-3);
	}
	.steps {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: var(--s-4);
	}
	.steps li {
		display: flex;
		gap: var(--s-3);
		min-width: 0;
	}
	.num {
		flex: none;
		width: 24px;
		height: 24px;
		border-radius: 50%;
		display: grid;
		place-items: center;
		font-size: var(--text-xs, 12px);
		background: var(--surface-2);
	}
	.st {
		margin: 2px 0 var(--s-2);
	}
	.needs {
		margin: 0;
	}
	.steps p {
		margin: 0 0 var(--s-2);
	}
	.codebox {
		display: flex;
		align-items: flex-start;
		gap: var(--s-2);
		margin-bottom: var(--s-2);
		min-width: 0;
	}
	.codebox pre {
		flex: 1;
		min-width: 0;
		margin: 0;
		padding: var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		font-size: var(--text-xs, 12px);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.linkish {
		justify-self: start;
		background: none;
		border: 0;
		padding: 0;
		color: var(--ha-red);
		cursor: pointer;
	}
	.guide-link {
		justify-self: start;
		color: var(--ha-red);
	}
	.more summary {
		cursor: pointer;
		padding: 6px 0;
	}
	.try {
		margin-top: var(--s-4);
		padding: var(--s-3);
		border-radius: var(--r-sm);
		background: var(--surface-2);
	}
	.try p {
		margin: 2px 0 0;
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
	@media (max-width: 767px) {
		.codebox {
			flex-direction: column;
		}
	}
</style>
