<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { AI_SCOPE, CHECK_QUESTION, DEEPLINK_LABEL, GROUPS, GUIDES_URL, ROWS, agentPrompt, aiURL, appsFor, canSignIn, codeParts, deeplinkHref, isAIKey, onlyLocal, pickFor, testedLine, whereText, type AIApp, type Ctx, type Part, type Picked, type Row } from '#lib/ai/index.ts';
	import { onMount, tick } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, date } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { getBase } from '#lib/core/paths.ts';
	import type { ApiKey } from '#lib/core/types.ts';

	let loaded = $state(false);
	let error = $state('');
	let enabled = $state(false);
	let saving = $state(false);
	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	// A key made from an app's steps fills in only for that app (its steps and the agent prompt).
	let fresh = $state<{ id: number; key: string; label: string; app?: string } | null>(null);
	// The picked app sits in the address (?app=cursor), so a link opens on it. None by default.
	let picked = $state<Picked | undefined>(pickFor(page.url.searchParams.get('app')));
	// On phones the steps open under the picked row, on wider screens under the table.
	const phone = '(max-width: 767px)';
	let narrow = $state(typeof window !== 'undefined' && window.matchMedia(phone).matches);
	let panelEl = $state<HTMLElement>();
	let panelHead = $state<HTMLElement>();
	let freshEl = $state<HTMLElement>();
	let byHand = $state<HTMLElement>();
	let revoking = $state<ApiKey | null>(null);
	let revokeOpen = $state(false);
	let revokeBusy = $state(false);

	const isOwner = $derived(session.s.role === 'owner');
	const origin = $derived(typeof window === 'undefined' ? '' : (window.location.origin + getBase()).replace(/\/+$/, ''));
	const url = $derived(typeof window === 'undefined' ? '/api/v1/mcp' : aiURL(window.location.origin, getBase()));
	const aiKeys = $derived(keys.filter(isAIKey));
	const signIn = $derived(canSignIn(url));
	const local = $derived(onlyLocal(url));
	const app = $derived(picked?.app);
	// The key made for the picked app; never one made for another app or by hand.
	const appKey = $derived(app && fresh?.app === app.id ? fresh.key : undefined);
	const ctx = $derived<Ctx>({ url, key: appKey, signIn });
	const unreachable = $derived(!!app?.cloud && local);
	const needsKey = $derived(!!app && !app.cloud && (!signIn || !app.signIn));
	const promptKey = $derived(signIn ? undefined : appKey);
	const prompt = $derived(agentPrompt(origin, { key: promptKey, home: !signIn }));

	onMount(() => {
		const mq = window.matchMedia(phone);
		narrow = mq.matches;
		const on = () => (narrow = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});

	// A row whose guide covers several apps (for example local AI) switches between their steps.
	function pickApp(a: AIApp) {
		if (!picked?.row || a.id === app?.id) return;
		picked = { id: a.id, row: picked.row, app: a };
		const u = new URL(page.url.href);
		u.searchParams.set('app', a.id);
		replaceState(u, page.state);
	}

	async function pick(r: Row) {
		picked = picked?.row?.id === r.id ? undefined : { id: r.id, row: r, app: r.app };
		const u = new URL(page.url.href);
		if (picked) u.searchParams.set('app', picked.id);
		else u.searchParams.delete('app');
		replaceState(u, page.state);
		redirectURI = '';
		clientID = '';
		clientError = '';
		if (picked) {
			// Focus follows the steps, so a keyboard does not need to pass every later row.
			await tick();
			panelHead?.focus({ preventScroll: true });
			panelEl?.scrollIntoView({ block: 'nearest' });
		}
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
		const first = !loaded;
		loaded = true;
		// A ?app= link opens with its steps in view (on a phone they sit far down).
		if (first && picked) {
			await tick();
			panelEl?.scrollIntoView({ block: 'nearest' });
		}
	}

	onMount(load);

	async function setEnabled(v: boolean) {
		if (saving) return;
		const before = enabled;
		enabled = v;
		saving = true;
		try {
			enabled = (await api.put<{ enabled: boolean }>('/settings/ai', { enabled: v })).enabled;
			toasts.push(enabled ? 'AI apps on' : 'AI apps off', 'ok');
		} catch (e) {
			enabled = before;
			toasts.push("Couldn't save", 'fail', { body: message(e) });
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
			const r = await api.post<{ id: number; key: string; label: string }>('/auth/keys', { label: name, scopes: [AI_SCOPE] });
			fresh = { id: r.id, key: r.key, label: r.label, app: forApp };
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (err) {
			toasts.push("Couldn't make key", 'fail', { body: keyNote(err) });
		} finally {
			making = false;
		}
		// The pressed button is gone now; focus the new key's Copy.
		if (fresh && forApp) {
			await tick();
			freshEl?.querySelector('button')?.focus();
		}
	}

	async function doneWithKey() {
		const inPanel = !!app && fresh?.app === app.id;
		fresh = null;
		await tick();
		if (inPanel) panelHead?.focus();
		else byHand?.focus();
	}

	function askRevoke(k: ApiKey) {
		revoking = k;
		revokeOpen = true;
	}

	async function revoke(k: ApiKey) {
		if (revokeBusy) return;
		revokeBusy = true;
		revokeOpen = false;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			// A revoked key leaves the steps and the prompt, and Make a key comes back.
			if (fresh?.id === k.id) fresh = null;
			toasts.push(`${k.label} revoked`, 'ok');
		} catch (e) {
			toasts.push("Couldn't revoke key", 'fail', { body: keyNote(e) });
		} finally {
			revokeBusy = false;
		}
	}

	// Apps that do not register themselves (Gemini) get a client ID made from the return address they show.
	let redirectURI = $state('');
	let clientID = $state('');
	let clientError = $state('');
	async function makeClientID(e: SubmitEvent) {
		e.preventDefault();
		clientError = '';
		clientID = '';
		if (!redirectURI.trim()) {
			clientError = "Paste Gemini's redirect URI first.";
			return;
		}
		try {
			const r = await fetch(`${getBase()}/oauth/register`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ client_name: app?.name ?? 'Gemini app', redirect_uris: [redirectURI.trim()] })
			});
			if (r.status === 400) {
				clientError = "HolyArrs didn't accept that address. Copy the redirect URI from Gemini again, starting with https://.";
				return;
			}
			if (!r.ok) throw new Error(String(r.status));
			clientID = (await r.json()).client_id;
		} catch {
			clientError = "HolyArrs didn't answer. Try again.";
		}
	}

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push('Text copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}

	const copyName = (code: string) => (code.includes('\n') ? 'Copy config' : code === url ? 'Copy AI address' : code.startsWith('Authorization:') ? 'Copy header' : 'Copy command');
</script>

{#snippet text(parts: Part[])}{#each parts as p, i (i)}{#if p.code}<code>{p.text}</code>{:else if p.b}<b>{p.text}</b>{:else}{p.text}{/if}{/each}{/snippet}

{#snippet freshBox()}
	{#if fresh}
		<div class="fresh" role="status" bind:this={freshEl}>
			<p class="small"><b>Copy this key now</b></p>
			<p class="small">{fresh.label}: it's shown only once. If you lose it, make a new one.</p>
			<div class="fresh-row">
				<code class="key grow">{fresh.key}</code>
				<Button icon="copy" type="button" onclick={() => fresh && copy(fresh.key)}>Copy</Button>
				<Button variant="plain" type="button" onclick={doneWithKey}>Close</Button>
			</div>
		</div>
	{/if}
{/snippet}

{#snippet panel()}
	{#if picked && app}
		<div class="panel" id="ai-app-panel" data-testid="ai-guide" bind:this={panelEl}>
			<h4 class="h3" tabindex="-1" bind:this={panelHead}>{app.name}</h4>
			{#if picked.row && appsFor(picked.row).length > 1}
				<div class="which" role="group" aria-label="Which app">
					{#each appsFor(picked.row) as a (a.id)}
						<Chip label={a.name} selected={a.id === app.id} onclick={() => pickApp(a)} />
					{/each}
				</div>
			{/if}
			{#if unreachable}
				<p class="notice" data-testid="ai-unreachable">Needs a public https address.</p>
				<a class="small guide-link" href="{GUIDES_URL}#home-network" target="_blank" rel="noopener noreferrer">Only on your home network?</a>
			{:else}
				{#if app.needs}<p class="small muted">{app.needs}</p>{/if}
				{#if needsKey && fresh?.app !== app.id}
					<div class="keyrow">
						<Button icon="key" variant="primary" type="button" onclick={() => newKey(app.name, app.id)} disabled={making || !!keysNote} aria-busy={making} aria-label="Make a key for {app.name}">
							{making ? 'Making a key…' : 'Make a key'}
						</Button>
						<span class="xs muted">{keysNote || "It's filled into the steps and the agent prompt, and shown only now."}</span>
					</div>
				{:else if fresh && fresh.app === app.id}
					{@render freshBox()}
				{/if}
				<ol class="steps">
					{#each app.steps(ctx) as s, i (i)}
						<li>
							<span class="num" aria-hidden="true">{i + 1}</span>
							<div class="grow">
								<p class="st">{@render text(s.parts)}</p>
								{#if s.link}<p class="st"><a class="guide-link" href={s.link.href} target="_blank" rel="noopener noreferrer">{s.link.text}</a></p>{/if}
								{#if s.deeplink}
									<p class="st"><Button size="sm" href={deeplinkHref(s.deeplink, url)}>{DEEPLINK_LABEL[s.deeplink]}</Button></p>
								{/if}
								{#if s.code}
									<div class="codebox">
										<pre>{s.code}</pre>
										<Button icon="copy" size="sm" type="button" aria-label={copyName(s.code)} onclick={() => copy(s.code ?? '')}>Copy</Button>
									</div>
								{/if}
							</div>
						</li>
					{/each}
				</ol>
				{#if app.clientIdHelper}
					<form class="client-id" onsubmit={makeClientID} data-testid="ai-client-id" novalidate>
						<p class="small">Gemini needs a client ID</p>
						<label class="label" for="ai-redirect">Gemini's redirect URI</label>
						<div class="keyrow">
							<input
								id="ai-redirect"
								name="redirect_uri"
								type="url"
								inputmode="url"
								spellcheck="false"
								class="input grow"
								bind:value={redirectURI}
								placeholder="https://…"
								autocomplete="off"
								aria-describedby="ai-redirect-hint"
							/>
							<Button type="submit">Make client ID</Button>
						</div>
						<p class="hint" id="ai-redirect-hint">In Gemini, open <b>Show more</b> next to <b>Advanced features</b> and copy the redirect URI.</p>
						{#if clientError}<p class="notice warn" role="alert">{clientError}</p>{/if}
						{#if clientID}
							<div class="xs muted">Client ID</div>
							<div class="codebox">
								<pre>{clientID}</pre>
								<Button icon="copy" size="sm" type="button" aria-label="Copy client ID" onclick={() => copy(clientID)}>Copy</Button>
							</div>
							<p class="small">Paste into Gemini, leave the secret empty, press <b>Next</b></p>
						{/if}
					</form>
				{/if}
				{#if testedLine(app)}<p class="xs muted" data-testid="ai-tested">{testedLine(app)}</p>{/if}
				{#if app.check}<p class="xs muted"><b>Check:</b> {app.check}</p>{/if}
				{#if app.note}<p class="xs muted">{app.note}</p>{/if}
			{/if}
		</div>
	{/if}
{/snippet}

<svelte:head><title>AI apps · HolyArrs</title></svelte:head>

<PageHeader title="AI apps" />

{#if !loaded}
	<SkSettings rows={3} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} busy={false} />
{:else}
	<SettingsCard id="ai-switch" title="AI apps">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">
			AI apps like Claude, ChatGPT or Cursor can check your library, calendar and downloads, and request titles for you. They can only do what you allow,
			and every request they make shows in History.
		</p>
		<label class="switch">
			<input type="checkbox" checked={enabled} disabled={!isOwner || saving} aria-busy={saving} onchange={(e) => setEnabled(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Let AI apps connect</span>
		</label>
		{#if !isOwner}<p class="hint">Only the owner can turn this on or off.</p>{/if}
		<ul class="facts small muted">
			<li>AI apps act as you, with your permissions and Limits</li>
			<li>AI apps check with you before requesting. Only people approve.</li>
			<li>At most 60 calls a minute and 5 requests an hour per app.</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="ai-connect" title="Connect an AI app">
		<div class="small addr-label">AI address</div>
		<div class="addr-row">
			<code class="addr grow" data-testid="ai-address">{url}</code>
			<Button icon="copy" size="sm" type="button" aria-label="Copy AI address" onclick={() => copy(url)}>Copy</Button>
		</div>
		<p class="xs muted">Shows an error in a browser; that's expected</p>
		{#if !enabled}<p class="notice warn"><span>Turn on <b>Let AI apps connect</b> above first. Until then, AI apps get "not found".</span></p>{/if}
		<p class="small muted mode" data-testid="ai-mode">
			{#if signIn}
				Apps sign in through your browser. You press Allow once.
			{:else}
				This is a plain-http address, so most apps won't sign in through the browser. Use an AI key: press <b>Make a key</b> next to your app below.
			{/if}
		</p>

		<section class="part" aria-labelledby="ai-steps-h">
			<h3 class="h3" id="ai-steps-h">Connect in 3 steps</h3>
			<ol class="steps" data-testid="ai-steps">
				<li><span class="num" aria-hidden="true">1</span><p class="st">In your AI app, add a custom connector or MCP server.</p></li>
				<li><span class="num" aria-hidden="true">2</span><p class="st">Paste your AI address.</p></li>
				<li>
					<span class="num" aria-hidden="true">3</span>
					<p class="st">
						{#if signIn}
							HolyArrs opens in your browser. Sign in if it asks, then press <b>Allow</b>.
						{:else}
							Paste the AI key from <b>Make a key</b> where your app asks for a header or key.
						{/if}
					</p>
				</li>
			</ol>
			<p class="small muted">Cloud AI apps (Claude, ChatGPT, Gemini, Grok, Perplexity) need a public https address</p>
			<a class="small guide-link" href="{GUIDES_URL}#home-network" target="_blank" rel="noopener noreferrer">Only on your home network?</a>
		</section>

		<section class="part" aria-labelledby="ai-apps-h">
			<h3 class="h3" id="ai-apps-h">Where to add HolyArrs</h3>
			<table class="apps" data-testid="ai-table" aria-labelledby="ai-apps-h">
				<thead>
					<tr><th scope="col">App</th><th scope="col">Where to add it</th><th scope="col" class="guide-h"><span class="sr-only">Guide</span></th></tr>
				</thead>
				{#each GROUPS as [g, title] (g)}
					<tbody>
						<tr class="group"><th scope="rowgroup" colspan="3">{title}</th></tr>
						{#each ROWS.filter((r) => r.group === g) as r (r.id)}
							{@const off = !!r.app.cloud && local}
							{@const on = picked?.row?.id === r.id}
							<tr class="app-row" class:on class:off>
								<th scope="row" class="app">
									<button type="button" class="pick" aria-expanded={on} aria-controls={on ? 'ai-app-panel' : undefined} onclick={() => pick(r)}>{r.name}</button>
								</th>
								<td class="where">
									{#if off}
										Needs a public https address.
									{:else}
										{@render text(codeParts(whereText(r, url, signIn)))}{#if r.deeplink && signIn}, or <Button size="sm" href={deeplinkHref(r.deeplink, url)}>{DEEPLINK_LABEL[r.deeplink]}</Button>{/if}
									{/if}
								</td>
								<td class="guide"><a class="guide-link" href={r.guide} target="_blank" rel="noopener noreferrer" aria-label="Guide for {r.name}">Guide</a></td>
							</tr>
							{#if narrow && on}
								<tr class="panel-row"><td colspan="3">{@render panel()}</td></tr>
							{/if}
						{/each}
					</tbody>
				{/each}
			</table>
			{#if !narrow || !picked?.row}{@render panel()}{/if}
			<a class="small guide-link" href={GUIDES_URL} target="_blank" rel="noopener noreferrer">All AI guides</a>
		</section>

		<section class="part" aria-labelledby="ai-agent-h">
			<h3 class="h3" id="ai-agent-h">Or let your agent do it</h3>
			<p class="small">Paste this into Claude Code, Codex, Cursor or another agent</p>
			<div class="codebox">
				<pre data-testid="ai-prompt">{prompt}</pre>
				<Button icon="copy" size="sm" type="button" onclick={() => copy(prompt)}>Copy prompt</Button>
			</div>
			{#if promptKey}
				<p class="xs muted">Contains your key. Paste only into your own agent.</p>
			{:else if !signIn}
				<p class="xs muted">Press <b>Make a key</b> first to put your key in the prompt.</p>
			{/if}
		</section>

		<section class="part" aria-labelledby="ai-check-h" data-testid="ai-check">
			<h3 class="h3" id="ai-check-h">Check it works</h3>
			<p class="small">Ask your AI:</p>
			<div class="codebox">
				<pre>{CHECK_QUESTION}</pre>
				<Button icon="copy" size="sm" type="button" onclick={() => copy(CHECK_QUESTION)}>Copy question</Button>
			</div>
			<p class="xs muted">Any answer about downloads means it works</p>
		</section>
	</SettingsCard>

	<SettingsCard id="ai-keys" title="Linked AI apps">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">Apps that sign in or use a key show here.</p>
		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			<!-- A key made for the open app shows in its steps instead. -->
			{#if fresh && !(app && fresh.app === app.id)}{@render freshBox()}{/if}
			{#if aiKeys.length === 0}
				<p class="small muted">No AI apps linked</p>
			{:else}
				<ul class="rows">
					{#each aiKeys as k (k.id)}
						<li>
							<div class="grow">
								<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
								<div class="xs muted">Made {date(k.created_at)} · {k.last_used_at ? `last used ${ago(k.last_used_at)}` : 'never used'}</div>
							</div>
							<Button variant="destructive" type="button" onclick={() => askRevoke(k)}>Revoke</Button>
						</li>
					{/each}
				</ul>
			{/if}
			<details class="more" style="margin-top: var(--s-4)">
				<summary bind:this={byHand}>Make a key by hand</summary>
				<form class="row" onsubmit={makeKey}>
					<label class="sr-only" for="ai-key-label">Key name</label>
					<input id="ai-key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="For example, Claude Desktop" maxlength="64" />
					<Button icon="key" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make an AI key</Button>
				</form>
			</details>
		{/if}
	</SettingsCard>

	<Sheet bind:open={revokeOpen} size="sm" title={revoking ? `Revoke ${revoking.label}?` : 'Revoke'} description="The app loses access until you link it again">
		<div class="confirm-actions">
			<Button variant="plain" type="button" onclick={() => (revokeOpen = false)}>Keep it</Button>
			<Button variant="destructive" type="button" disabled={revokeBusy} onclick={() => revoking && void revoke(revoking)}>Revoke</Button>
		</div>
	</Sheet>
{/if}

<style>
	.facts {
		margin: var(--s-3) 0 0;
		padding-left: 1.2em;
		display: grid;
		gap: 4px;
	}
	.addr-label {
		margin-top: var(--s-2);
		font-weight: var(--weight-semibold);
	}
	.addr-row,
	.keyrow {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		margin: var(--s-2) 0;
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
		margin: var(--s-3) 0 0;
	}
	.notice {
		margin-top: var(--s-2);
	}
	.part {
		display: grid;
		gap: var(--s-3);
		margin-top: var(--s-5);
		min-width: 0;
	}
	.part > p,
	.part > h3 {
		margin: 0;
	}
	.steps {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: var(--s-3);
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
		overflow-wrap: anywhere;
	}
	.steps li > .st {
		margin-bottom: 0;
	}
	.st code,
	.where code {
		font-size: 0.92em;
		padding: 1px 4px;
		border-radius: 4px;
		background: var(--surface-2);
		overflow-wrap: anywhere;
	}
	.apps {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
		table-layout: fixed;
	}
	.apps thead th {
		text-align: left;
		color: var(--text-muted);
		font: var(--t-footnote);
		font-weight: 600;
		padding: 0 var(--s-2) var(--s-2);
		border-bottom: 1px solid var(--line);
	}
	.apps thead th:first-child {
		width: 34%;
	}
	.apps thead .guide-h {
		width: 72px;
	}
	.apps .group th {
		text-align: left;
		font-weight: var(--weight-semibold);
		padding: var(--s-4) var(--s-2) var(--s-2);
		border-bottom: 1px solid var(--line);
	}
	.apps .app-row th,
	.apps .app-row td {
		text-align: left;
		vertical-align: top;
		padding: var(--s-2);
		border-bottom: 1px solid var(--line);
		overflow-wrap: anywhere;
	}
	.apps .app-row.on th,
	.apps .app-row.on td {
		background: var(--surface-2);
	}
	.apps .app-row.off .where {
		color: var(--text-muted);
	}
	.apps .guide {
		text-align: right;
		white-space: nowrap;
	}
	.pick {
		display: inline-flex;
		align-items: center;
		min-height: 36px;
		background: none;
		border: 0;
		padding: 0;
		font: inherit;
		font-weight: var(--weight-semibold);
		color: var(--text);
		text-align: left;
		cursor: pointer;
		text-decoration: underline;
		text-decoration-color: var(--line-strong);
		text-underline-offset: 3px;
	}
	.pick:hover,
	.pick[aria-expanded='true'] {
		text-decoration-color: currentColor;
	}
	.panel-row td {
		padding: var(--s-3) 0;
		border-bottom: 1px solid var(--line);
	}
	.panel {
		display: grid;
		gap: var(--s-3);
		padding: var(--s-4);
		border-radius: var(--r-btn);
		border: 1px solid var(--line);
		min-width: 0;
	}
	.which {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	.panel > p,
	.panel > h4 {
		margin: 0;
	}
	.client-id {
		display: grid;
		gap: var(--s-2);
	}
	.client-id p {
		margin: 0;
	}
	.codebox {
		display: flex;
		align-items: flex-start;
		gap: var(--s-2);
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
	.guide-link {
		justify-self: start;
		color: var(--ha-red);
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.more summary {
		cursor: pointer;
		padding: 6px 0;
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.fresh p {
		margin: 0 0 var(--s-1);
	}
	.fresh-row {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		margin-top: var(--s-2);
	}
	.panel .fresh {
		margin-bottom: 0;
	}
	.confirm-actions {
		display: flex;
		justify-content: flex-end;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 6px;
	}
	@media (pointer: coarse) {
		.pick {
			min-height: 44px;
		}
	}
	@media (max-width: 767px) {
		.codebox {
			flex-direction: column;
		}
		/* The table stacks: app and Guide on one line, where under them. */
		.apps,
		.apps tbody {
			display: block;
		}
		.apps thead {
			display: none;
		}
		.apps .group,
		.apps .panel-row {
			display: block;
		}
		.apps .group th,
		.apps .panel-row td {
			display: block;
		}
		.apps .app-row {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			border-bottom: 1px solid var(--line);
		}
		.apps .app-row th,
		.apps .app-row td {
			border-bottom: 0;
		}
		.apps .app-row .where {
			grid-column: 1 / -1;
			grid-row: 2;
			padding-top: 0;
		}
	}
	/* Connect an AI app: address on top, then two clear parts; no divider between a label and its value. */
	:global(#ai-connect > .body) { gap: 8px !important; padding-top: 16px !important; padding-bottom: 18px !important; }
	:global(#ai-connect > .body > *) { box-shadow: none !important; padding-block: 0 !important; }
	.addr-label { margin-top: 0; font: 600 15px/1.3 var(--font-ui); }
	.addr-row { margin: 2px 0 0; }
	.addr { flex: 1; padding: 10px 14px; border-radius: 12px; font-size: 14px; }
	.mode { margin: 4px 0 0; }
	.part { margin-top: 14px !important; padding-top: 18px !important; box-shadow: inset 0 1px 0 var(--line) !important; gap: 12px; }
	.part > h3 { color: var(--text-muted); font: 600 11px/1.2 var(--font-ui) !important; letter-spacing: .08em; text-transform: uppercase; }
</style>
