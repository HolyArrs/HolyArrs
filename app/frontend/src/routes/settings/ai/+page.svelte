<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
		return e instanceof ApiError && e.status === 403 ? t('settings.ai.signInKeys') : message(e);
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
			toasts.push(enabled ? t('settings.ai.on') : t('settings.ai.off'), 'ok');
		} catch (e) {
			enabled = before;
			toasts.push(t('settings.ai.couldntSave'), 'fail', { body: message(e) });
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
			toasts.push(t('settings.ai.makeFailed'), 'fail', { body: keyNote(err) });
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
			toasts.push(t('settings.ai.revoked', { name: k.label }), 'ok');
		} catch (e) {
			toasts.push(t('settings.ai.revokeFailed'), 'fail', { body: keyNote(e) });
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
			clientError = t('settings.ai.pasteRedirect');
			return;
		}
		try {
			const r = await fetch(`${getBase()}/oauth/register`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ client_name: app?.name ?? 'Gemini app', redirect_uris: [redirectURI.trim()] })
			});
			if (r.status === 400) {
				clientError = t('settings.ai.notAccepted');
				return;
			}
			if (!r.ok) throw new Error(String(r.status));
			clientID = (await r.json()).client_id;
		} catch {
			clientError = t('settings.ai.noAnswer');
		}
	}

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push(t('settings.ai.textCopied'), 'ok');
		} catch {
			toasts.push(t('settings.ai.copyFailed'), 'fail', { body: t('settings.ai.selectInstead') });
		}
	}

	const copyName = (code: string) => (code.includes('\n') ? t('settings.ai.copyConfig') : code === url ? t('settings.ai.copyAddress') : code.startsWith('Authorization:') ? t('settings.ai.copyHeader') : t('settings.ai.copyCommand'));
</script>

{#snippet text(parts: Part[])}{#each parts as p, i (i)}{#if p.code}<code>{p.text}</code>{:else if p.b}<b>{p.text}</b>{:else}{p.text}{/if}{/each}{/snippet}

{#snippet freshBox()}
	{#if fresh}
		<div class="fresh" role="status" bind:this={freshEl}>
			<p class="small"><b>{t('settings.ai.copyKeyNow')}</b></p>
			<p class="small">{t('settings.ai.shownOnce', { name: fresh.label })}</p>
			<div class="fresh-row">
				<code class="key grow">{fresh.key}</code>
				<Button icon="copy" type="button" onclick={() => fresh && copy(fresh.key)}>{t('settings.ai.copy')}</Button>
				<Button variant="plain" type="button" onclick={doneWithKey}>{t('settings.ai.close')}</Button>
			</div>
		</div>
	{/if}
{/snippet}

{#snippet panel()}
	{#if picked && app}
		<div class="panel" id="ai-app-panel" data-testid="ai-guide" bind:this={panelEl}>
			<h4 class="h3" tabindex="-1" bind:this={panelHead}>{app.name}</h4>
			{#if picked.row && appsFor(picked.row).length > 1}
				<div class="which" role="group" aria-label={t('settings.ai.whichApp')}>
					{#each appsFor(picked.row) as a (a.id)}
						<Chip label={a.name} selected={a.id === app.id} onclick={() => pickApp(a)} />
					{/each}
				</div>
			{/if}
			{#if unreachable}
				<p class="notice" data-testid="ai-unreachable">{t('settings.ai.needsPublic')}</p>
				<a class="small guide-link" href="{GUIDES_URL}#home-network" target="_blank" rel="noopener noreferrer">{t('settings.ai.homeNetwork')}</a>
			{:else}
				{#if app.needs}<p class="small muted">{app.needs}</p>{/if}
				{#if needsKey && fresh?.app !== app.id}
					<div class="keyrow">
						<Button icon="key" variant="primary" type="button" onclick={() => newKey(app.name, app.id)} disabled={making || !!keysNote} aria-busy={making} aria-label={t('settings.ai.makeKeyFor', { name: app.name })}>
							{making ? t('settings.ai.makingKey') : t('settings.ai.makeKey')}
						</Button>
						<span class="xs muted">{keysNote || t('settings.ai.filledIn')}</span>
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
										<Button icon="copy" size="sm" type="button" aria-label={copyName(s.code)} onclick={() => copy(s.code ?? '')}>{t('settings.ai.copy')}</Button>
									</div>
								{/if}
							</div>
						</li>
					{/each}
				</ol>
				{#if app.clientIdHelper}
					<form class="client-id" onsubmit={makeClientID} data-testid="ai-client-id" novalidate>
						<p class="small">{t('settings.ai.needsClientId')}</p>
						<label class="label" for="ai-redirect">{t('settings.ai.redirectUri')}</label>
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
							<Button type="submit">{t('settings.ai.makeClientId')}</Button>
						</div>
						<p class="hint" id="ai-redirect-hint"><Words key="settings.ai.geminiHint" /></p>
						{#if clientError}<p class="notice warn" role="alert">{clientError}</p>{/if}
						{#if clientID}
							<div class="xs muted">{t('settings.ai.clientId')}</div>
							<div class="codebox">
								<pre>{clientID}</pre>
								<Button icon="copy" size="sm" type="button" aria-label={t('settings.ai.copyClientId')} onclick={() => copy(clientID)}>{t('settings.ai.copy')}</Button>
							</div>
							<p class="small"><Words key="settings.ai.pasteGemini" /></p>
						{/if}
					</form>
				{/if}
				{#if testedLine(app)}<p class="xs muted" data-testid="ai-tested">{testedLine(app)}</p>{/if}
				{#if app.check}<p class="xs muted"><b>{t('settings.ai.check')}</b> {app.check}</p>{/if}
				{#if app.note}<p class="xs muted">{app.note}</p>{/if}
			{/if}
		</div>
	{/if}
{/snippet}

<svelte:head><title>{t('settings.ai.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.ai.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} busy={false} />
{:else}
	<SettingsCard id="ai-switch" title={t('settings.ai.title')}>
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">
			{t('settings.ai.intro')}
		</p>
		<label class="switch">
			<input type="checkbox" checked={enabled} disabled={!isOwner || saving} aria-busy={saving} onchange={(e) => setEnabled(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>{t('settings.ai.letConnect')}</span>
		</label>
		{#if !isOwner}<p class="hint">{t('settings.ai.ownerOnly')}</p>{/if}
		<ul class="facts small muted">
			<li>{t('settings.ai.fact1')}</li>
			<li>{t('settings.ai.fact2')}</li>
			<li>{t('settings.ai.fact3')}</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="ai-connect" title={t('settings.ai.connect')}>
		<div class="small addr-label">{t('settings.ai.address')}</div>
		<div class="addr-row">
			<code class="addr grow" data-testid="ai-address">{url}</code>
			<Button icon="copy" size="sm" type="button" aria-label={t('settings.ai.copyAddress')} onclick={() => copy(url)}>{t('settings.ai.copy')}</Button>
		</div>
		<p class="xs muted">{t('settings.ai.errorExpected')}</p>
		{#if !enabled}<p class="notice warn"><span><Words key="settings.ai.turnOnFirst" /></span></p>{/if}
		<p class="small muted mode" data-testid="ai-mode">
			{#if signIn}
				{t('settings.ai.modeSignIn')}
			{:else}
				<Words key="settings.ai.modeHttp" />
			{/if}
		</p>

		<section class="part" aria-labelledby="ai-steps-h">
			<h3 class="h3" id="ai-steps-h">{t('settings.ai.threeSteps')}</h3>
			<ol class="steps" data-testid="ai-steps">
				<li><span class="num" aria-hidden="true">1</span><p class="st">{t('settings.ai.step1')}</p></li>
				<li><span class="num" aria-hidden="true">2</span><p class="st">{t('settings.ai.step2')}</p></li>
				<li>
					<span class="num" aria-hidden="true">3</span>
					<p class="st">
						{#if signIn}
							<Words key="settings.ai.step3SignIn" />
						{:else}
							<Words key="settings.ai.step3Key" />
						{/if}
					</p>
				</li>
			</ol>
			<p class="small muted">{t('settings.ai.cloudNeeds')}</p>
			<a class="small guide-link" href="{GUIDES_URL}#home-network" target="_blank" rel="noopener noreferrer">{t('settings.ai.homeNetwork')}</a>
		</section>

		<section class="part" aria-labelledby="ai-apps-h">
			<h3 class="h3" id="ai-apps-h">{t('settings.ai.whereAdd')}</h3>
			<table class="apps" data-testid="ai-table" aria-labelledby="ai-apps-h">
				<thead>
					<tr><th scope="col">{t('settings.ai.colApp')}</th><th scope="col">{t('settings.ai.colWhere')}</th><th scope="col" class="guide-h"><span class="sr-only">{t('settings.ai.guide')}</span></th></tr>
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
										{t('settings.ai.needsPublic')}
									{:else}
										{@render text(codeParts(whereText(r, url, signIn)))}{#if r.deeplink && signIn}, {t('settings.ai.or')} <Button size="sm" href={deeplinkHref(r.deeplink, url)}>{DEEPLINK_LABEL[r.deeplink]}</Button>{/if}
									{/if}
								</td>
								<td class="guide"><a class="guide-link" href={r.guide} target="_blank" rel="noopener noreferrer" aria-label={t('settings.ai.guideFor', { name: r.name })}>{t('settings.ai.guide')}</a></td>
							</tr>
							{#if narrow && on}
								<tr class="panel-row"><td colspan="3">{@render panel()}</td></tr>
							{/if}
						{/each}
					</tbody>
				{/each}
			</table>
			{#if !narrow || !picked?.row}{@render panel()}{/if}
			<a class="small guide-link" href={GUIDES_URL} target="_blank" rel="noopener noreferrer">{t('settings.ai.allGuides')}</a>
		</section>

		<section class="part" aria-labelledby="ai-agent-h">
			<h3 class="h3" id="ai-agent-h">{t('settings.ai.agent')}</h3>
			<p class="small">{t('settings.ai.agentPaste')}</p>
			<div class="codebox">
				<pre data-testid="ai-prompt">{prompt}</pre>
				<Button icon="copy" size="sm" type="button" onclick={() => copy(prompt)}>{t('settings.ai.copyPrompt')}</Button>
			</div>
			{#if promptKey}
				<p class="xs muted">{t('settings.ai.containsKey')}</p>
			{:else if !signIn}
				<p class="xs muted"><Words key="settings.ai.pressFirst" /></p>
			{/if}
		</section>

		<section class="part" aria-labelledby="ai-check-h" data-testid="ai-check">
			<h3 class="h3" id="ai-check-h">{t('settings.ai.checkWorks')}</h3>
			<p class="small">{t('settings.ai.askAi')}</p>
			<div class="codebox">
				<pre>{CHECK_QUESTION}</pre>
				<Button icon="copy" size="sm" type="button" onclick={() => copy(CHECK_QUESTION)}>{t('settings.ai.copyQuestion')}</Button>
			</div>
			<p class="xs muted">{t('settings.ai.anyAnswer')}</p>
		</section>
	</SettingsCard>

	<SettingsCard id="ai-keys" title={t('settings.ai.linked')}>
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">{t('settings.ai.linkedNote')}</p>
		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			<!-- A key made for the open app shows in its steps instead. -->
			{#if fresh && !(app && fresh.app === app.id)}{@render freshBox()}{/if}
			{#if aiKeys.length === 0}
				<p class="small muted">{t('settings.ai.noneLinked')}</p>
			{:else}
				<ul class="rows">
					{#each aiKeys as k (k.id)}
						<li>
							<div class="grow">
								<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
								<div class="xs muted">{t('settings.ai.made', { date: date(k.created_at) })} · {k.last_used_at ? t('settings.ai.lastUsed', { ago: ago(k.last_used_at) }) : t('settings.ai.neverUsed')}</div>
							</div>
							<Button variant="destructive" type="button" onclick={() => askRevoke(k)}>{t('settings.ai.revoke')}</Button>
						</li>
					{/each}
				</ul>
			{/if}
			<details class="more" style="margin-top: var(--s-4)">
				<summary bind:this={byHand}>{t('settings.ai.byHand')}</summary>
				<form class="row" onsubmit={makeKey}>
					<label class="sr-only" for="ai-key-label">{t('settings.ai.keyName')}</label>
					<input id="ai-key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder={t('settings.ai.keyExample')} maxlength="64" />
					<Button icon="key" type="submit" disabled={making || !label.trim()} aria-busy={making}>{t('settings.ai.makeAiKey')}</Button>
				</form>
			</details>
		{/if}
	</SettingsCard>

	<Sheet bind:open={revokeOpen} size="sm" title={revoking ? t('settings.ai.revokeName', { name: revoking.label }) : t('settings.ai.revoke')} description={t('settings.ai.revokeDesc')}>
		<div class="confirm-actions">
			<Button variant="plain" type="button" onclick={() => (revokeOpen = false)}>{t('settings.ai.keepIt')}</Button>
			<Button variant="destructive" type="button" disabled={revokeBusy} onclick={() => revoking && void revoke(revoking)}>{t('settings.ai.revoke')}</Button>
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
