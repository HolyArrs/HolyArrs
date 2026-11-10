<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	// Connect Jackett or Prowlarr: HolyArrs takes the address and key, tests them, and brings the sites in as Sources in one step. Jackett's sites stay in step on their own; Prowlarr writes its sites in through the arr API.
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { onMount, tick, untrack } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message, reachWords } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { ago } from '#lib/core/format.ts';
	import { to } from '#lib/core/paths.ts';

	type Kind = 'jackett' | 'prowlarr';
	interface Bridge {
		id: number;
		kind: Kind;
		url: string;
		callback_url?: string;
		last_check?: string;
		last_error?: string;
		sites: number;
		sources: number;
		local?: boolean;
	}

	// compact: the first-run step; a connected row offers only Remove there.
	let { onchange, compact = false }: { onchange?: () => void; compact?: boolean } = $props();

	const mayEdit = $derived(session.can('settings'));
	const names: Record<Kind, string> = { jackett: 'Jackett', prowlarr: 'Prowlarr' };
	const kindChoices: { value: Kind; label: string }[] = [
		{ value: 'prowlarr', label: 'Prowlarr' },
		{ value: 'jackett', label: 'Jackett' }
	];

	let bridges = $state<Bridge[] | null>(null);
	let loadError = $state('');
	let open = $state(false);
	// First run names Prowlarr first, so it starts picked there.
	let kind = $state<Kind>(untrack(() => (compact ? 'prowlarr' : 'jackett')));
	let address = $state('');
	let key = $state('');
	// The connection being edited: the form starts filled in; a new address replaces it.
	let editingId = $state<number | null>(null);
	let callback = $state('');
	// Filled from this page's address; it opens for typing only on Change, a server error on it, or when this page is on localhost (Prowlarr cannot use that).
	let callbackOpen = $state(false);
	let testing = $state(false);
	let connecting = $state(false);
	// A test or connect answer; a good test shows as a short tag ("Prowlarr v2.0.5 · 12 sites"). `tested` is the answer of Test, shown as a tag; otherwise a plain line (Connect's errors).
	let result = $state<{ ok: boolean; text: string; tested?: boolean; extra?: string } | null>(null);
	let errors = $state<Record<string, string>>({});
	let keysNeeded = $state<string[]>([]);
	let newKeys = $state(false);
	let typedKeys = $state<Record<string, string>>({ sonarr: '', radarr: '' });
	let checking = $state<number | null>(null);
	// Arr API keys a Prowlarr connect made, shown once (an app that used a replaced one, such as Seerr, needs it). Making arr API keys wants a fresh password: the server answers "reauth". Ask inline instead of showing that answer, then run the step again.
	let reauthFor = $state<'test' | 'connect' | null>(null);
	const isReauth = (e: unknown) => e instanceof ApiError && e.status === 403 && e.body.code === 'reauth';
	function askPassword(step: 'test' | 'connect') {
		session.s.localRecent = false;
		reauthFor = step;
	}
	$effect(() => {
		if (reauthFor && session.s.localRecent) {
			const again = reauthFor;
			reauthFor = null;
			untrack(() => void (again === 'test' ? test() : connect()));
		}
	});

	let removing = $state<Bridge | null>(null);
	let removeOpen = $state(false);
	let keepSources = $state(false);
	let removeBusy = $state(false);
	let removeError = $state('');

	async function load() {
		try {
			const r = await api.get<{ bridges: Bridge[]; home_ip?: string }>('/bridges');
			bridges = r.bridges;
			// Opened over Tailscale or on this machine: Prowlarr usually cannot use that address, so offer this machine's home-network one instead.
			const odd = /^(localhost|127\.|\[::1\]|100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.)|\.ts\.net$/.test(location.hostname);
			if (r.home_ip && odd && !callbackOpen) callback = `${location.protocol}//${r.home_ip}${location.port ? ':' + location.port : ''}${to('/').replace(/\/$/, '')}`;
			loadError = '';
			// First run: nothing linked yet, so the form is the whole step.
			if (compact && mayEdit && !r.bridges.length && !open) startConnect();
		} catch (e) {
			loadError = message(e);
		}
	}
	onMount(() => {
		void load();
		// How Prowlarr reaches HolyArrs: the address this page was opened at, unless that is this machine's own name, which Prowlarr's container cannot reach.
		if (!/^(localhost|127\.|\[::1\])/.test(location.hostname)) callback = location.origin + to('/').replace(/\/$/, '');
	});

	function editBridge(b: Bridge) {
		kind = b.kind;
		address = b.url;
		if (b.callback_url) callback = b.callback_url;
		key = '';
		startConnect();
		editingId = b.id;
	}

	function startConnect() {
		editingId = null;
		open = true;
		result = null;
		errors = {};
		keysNeeded = [];
		reauthFor = null;
	}

	function body() {
		const keys: Record<string, string> = {};
		for (const [side, k] of Object.entries(typedKeys)) if (k.trim()) keys[side] = k.trim();
		// First run: a bare host:port gets http:// in front.
		const withScheme = (v: string) => (compact && v && !/^[a-z][a-z0-9+.-]*:\/\//i.test(v) ? `http://${v}` : v);
		return { kind, url: withScheme(address.trim()), key: key.trim(), callback_url: kind === 'prowlarr' ? withScheme(callback.trim()) : '', new_keys: newKeys, keys };
	}

	function fieldError(e: unknown): boolean {
		const field = e instanceof ApiError && typeof e.body.field === 'string' ? e.body.field : '';
		if (['kind', 'url', 'key', 'callback_url'].includes(field)) {
			errors = { [field]: message(e) };
			return true;
		}
		return false;
	}

	// After a failed Test or Link: focus the field the answer is about (the address by default).
	async function focusError() {
		if (!Object.keys(errors).length && !(result && !result.ok)) return;
		await tick();
		const f = Object.keys(errors)[0];
		document.getElementById(f === 'callback_url' ? 'br-callback' : f === 'key' ? 'br-key' : 'br-url')?.focus();
	}

	async function test() {
		errors = {};
		result = null;
		testing = true;
		try {
			const r = await api.post<{ text: string; sites?: string[]; keys?: Record<string, string> }>('/bridges/test', body());
			let text = r.text;
			if (r.sites?.length) text += ` ${r.sites.slice(0, 8).join(', ')}${r.sites.length > 8 ? '…' : ''}.`;
			const extra = r.sites?.length ? t(r.sites.length === 1 ? 'sources.bridge.indexers.one' : 'sources.bridge.indexers.other', { count: r.sites.length }) : '';
			const have = Object.entries(r.keys ?? {}).filter(([, v]) => v === 'have').map(([k]) => k);
			keysNeeded = have;
			result = { ok: true, text, tested: true, extra };
		} catch (e) {
			if (isReauth(e)) askPassword('test');
			else if (!fieldError(e)) result = { ok: false, text: compact ? reachWords(message(e)) : message(e), tested: true };
		} finally {
			testing = false;
		}
		await focusError();
	}

	async function connect() {
		errors = {};
		result = null;
		connecting = true;
		try {
			const b = await api.post<Bridge & { keys?: Record<string, string> }>('/bridges', body());
			// Edited to a new address: the old connection goes, its Sources with it.
			if (editingId != null && b.id !== editingId) await api.del(`/bridges/${editingId}`).catch(() => {});
			editingId = null;
			toasts.push(t('sources.bridge.connected', { name: names[b.kind] }), 'ok', { body: b.kind === 'jackett' ? t(b.sources === 1 ? 'sources.bridge.added.one' : 'sources.bridge.added.other', { count: b.sources }) : t('sources.bridge.arrive') });
			open = false;
			key = '';
			typedKeys = { sonarr: '', radarr: '' };
			newKeys = false;
			keysNeeded = [];
			await load();
			onchange?.();
		} catch (e) {
			if (isReauth(e)) askPassword('connect');
			else if (e instanceof ApiError && e.status === 409 && e.body.need === 'keys' && Array.isArray(e.body.sides)) {
				keysNeeded = e.body.sides as string[];
				result = { ok: false, text: message(e) };
			} else if (!fieldError(e)) result = { ok: false, text: compact ? reachWords(message(e)) : message(e) };
		} finally {
			connecting = false;
		}
		await focusError();
	}

	async function check(b: Bridge) {
		checking = b.id;
		try {
			const r = await api.post<{ bridge: Bridge; result?: { added: number; removed: number; updated: number } }>(`/bridges/${b.id}/check`, {});
			const res = r.result;
			toasts.push(res ? (res.added + res.removed + res.updated ? t('sources.bridge.synced') : t('sources.bridge.upToDate')) : t('sources.bridge.syncingAgain'), 'ok', res && res.added + res.removed + res.updated ? { body: t('sources.bridge.syncBody', { added: res.added, removed: res.removed, changed: res.updated }) } : undefined);
			onchange?.();
		} catch (e) {
			toasts.push(t('sources.bridge.syncFailed'), 'fail', { body: message(e) });
		} finally {
			checking = null;
			await load();
		}
	}

	function askRemove(b: Bridge) {
		removing = b;
		keepSources = false;
		removeError = '';
		removeOpen = true;
	}

	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!removing) return;
		removeBusy = true;
		removeError = '';
		try {
			await api.del(`/bridges/${removing.id}${keepSources ? '?keep_sources=true' : ''}`);
			toasts.push(t('sources.bridge.disconnected', { name: names[removing.kind] }), 'ok');
			removeOpen = false;
			await load();
			onchange?.();
		} catch (err) {
			removeError = message(err);
		} finally {
			removeBusy = false;
		}
	}

	const countTag = (b: Bridge) =>
		b.kind === 'jackett' ? t(b.sites === 1 ? 'sources.bridge.indexers.one' : 'sources.bridge.indexers.other', { count: b.sites }) : t(b.sources === 1 ? 'sources.bridge.sources.one' : 'sources.bridge.sources.other', { count: b.sources });
</script>

<SettingsCard
	id="connect-bridge"
	title={t('sources.bridge.title')}
	description={bridges?.length ? undefined : t('sources.bridge.description')}
>
	{#if loadError && !bridges}
		<LoadError hasData={false} message={loadError} onretry={load} />
	{:else if bridges}
		{#if bridges.length}
			<ul class="bridges" data-testid="bridges">
				{#each bridges as b (b.id)}
					<li data-testid="bridge">
						<div class="brow">
							<div class="grow">
								<b>{names[b.kind]}</b>
								<span class="mono xs muted addr" title={b.last_check ? t('sources.bridge.checked', { when: ago(b.last_check) }) : undefined}>{b.url}</span>
							</div>
							<span class="ctag" class:bad={!!b.last_error} aria-hidden={b.last_error ? 'true' : undefined}>{b.last_error ? t('sources.bridge.notConnected') : countTag(b)}</span>
						</div>
						{#if b.last_error}<div class="small bad-text" data-testid="bridge-error">{b.last_error}</div>{/if}
						{#if mayEdit}
							<div class="row-acts">
								{#if !compact}<button type="button" class="quiet" disabled={checking === b.id} onclick={() => check(b)} data-testid="bridge-check">{t('sources.bridge.sync')}</button>{/if}
								<button type="button" class="quiet" onclick={() => editBridge(b)} data-testid="bridge-edit">{t('sources.bridge.change')}</button>
								<button type="button" class="quiet danger" onclick={() => askRemove(b)} data-testid="bridge-remove">{compact ? t('sources.bridge.remove') : t('sources.bridge.unlink')}</button>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		{#if mayEdit && !open}
			{#if bridges.length}
				<button type="button" class="add-row" onclick={startConnect} data-testid="bridge-start"><Icon name="plus" size={16} />{t('sources.bridge.addAnother')}</button>
			{:else}
				<div><Button type="button" icon="plus" onclick={startConnect} data-testid="bridge-start">{t('sources.bridge.link')}</Button></div>
			{/if}
		{/if}
		{#if open}
			<div class="form" data-testid="bridge-form">
				<SegmentedControl options={kindChoices} bind:value={kind} label={t('sources.bridge.app')} testid="br-kind" />
				<Field id="br-url" label={t('sources.bridge.address')} hint={kind === 'jackett' ? t('sources.bridge.basePath') : undefined} error={errors.url}>
					<input id="br-url" class="input" class:mono={!compact} type={compact ? 'text' : 'url'} inputmode="url" autocomplete="off" spellcheck="false" placeholder={(compact ? '' : 'http://') + (kind === 'jackett' ? '192.168.1.10:9117' : '192.168.1.10:9696')} bind:value={address} aria-invalid={errors.url ? 'true' : undefined} aria-describedby={errors.url ? 'br-url-error' : kind === 'jackett' ? 'br-url-hint' : undefined} />
				</Field>
				<Field id="br-key" label={t('sources.bridge.apiKeyLabel')} hint={editingId != null ? t('sources.bridge.keyAgain') : kind === 'jackett' ? t('sources.bridge.keyJackett') : t('sources.bridge.keyProwlarr')} error={errors.key}>
					<input id="br-key" class="input" class:mono={!compact} type="password" autocomplete="new-password" spellcheck="false" bind:value={key} aria-invalid={errors.key ? 'true' : undefined} aria-describedby={errors.key ? 'br-key-error' : 'br-key-hint'} />
				</Field>
				{#if kind === 'prowlarr'}
					{#if callback && !callbackOpen && !errors.callback_url}
						<p class="small muted callback-line">{t('sources.bridge.reaches')} <span class="cb-addr" class:mono={!compact}>{compact ? callback.replace(/^https?:\/\//, '') : callback}</span> <button type="button" class="link" onclick={() => (callbackOpen = true)}>{t('sources.bridge.change')}</button></p>
					{:else}
						<Field id="br-callback"  label={t('sources.bridge.callback')} hint={t('sources.bridge.callbackHint')} error={errors.callback_url}>
							<input id="br-callback" class="input" class:mono={!compact} type={compact ? 'text' : 'url'} inputmode="url" autocomplete="off" spellcheck="false" bind:value={callback} aria-invalid={errors.callback_url ? 'true' : undefined} aria-describedby={errors.callback_url ? 'br-callback-error' : 'br-callback-hint'} />
						</Field>
					{/if}
					{#if keysNeeded.length}
						<div class="keys" data-testid="bridge-keys">
							<label class="check"><input type="checkbox" bind:checked={newKeys} data-testid="bridge-new-keys" /> <span>{t('sources.bridge.replaceKeys')}</span></label>
							{#if !newKeys}
								<p class="hint">{keysNeeded.length === 1 ? t('sources.bridge.currentKey') : t('sources.bridge.currentKeys')}</p>
								{#each keysNeeded as side (side)}
									<Field id="br-key-{side}" label={side === 'sonarr' ? t('sources.bridge.tvKey') : t('sources.bridge.movieKey')}>
										<input id="br-key-{side}" class="input mono" type="password" autocomplete="new-password" spellcheck="false" bind:value={typedKeys[side]} />
									</Field>
								{/each}
							{/if}
						</div>
					{/if}
				{/if}
				{#if result && !(result.tested && result.ok)}<p class="small" class:bad-text={!result.ok} class:ok-text={result.ok} role="status" data-testid="bridge-result">{result.text}</p>{/if}
				{#if reauthFor}
					{#if session.s.hasPassword}
						<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
					{:else}
						<p class="small" role="status" data-testid="bridge-need-password"><Words key="sources.bridge.setPasswordLine" links={{ a: to('/account') }} /></p>
					{/if}
				{/if}
				<div class="acts" class:one-row={compact}>
					{#if result?.tested}<span class="tag"><TestTag ok={result.ok} text={result.text} app={names[kind]} extra={result.extra} testid={result.ok ? 'bridge-result' : undefined} /></span>{/if}
					{#if !compact}<Button type="button" variant="plain" onclick={() => { open = false; reauthFor = null; editingId = null; key = ''; }}>{t('sources.bridge.cancel')}</Button>{/if}
					{#if !compact}<Button type="button" busy={testing} disabled={!address.trim() || !key.trim()} onclick={test} data-testid="bridge-test">{t('sources.bridge.test')}</Button>{/if}
					<Button type="button" size={compact ? 'lg' : 'md'} variant="primary" busy={connecting} disabled={!address.trim() || !key.trim()} onclick={connect} data-testid="bridge-connect">{editingId != null ? t('sources.bridge.save') : compact ? t('sources.bridge.linkName', { name: names[kind] }) : t('sources.bridge.link')}</Button>
				</div>
			</div>
		{/if}
	{/if}
</SettingsCard>

<Sheet bind:open={removeOpen}  title={t('sources.bridge.unlinkTitle', { name: removing ? names[removing.kind] : '' })} description={removing?.url} size="sm">
	<form id="remove-bridge" class="form" onsubmit={remove}>
		{#if removing?.kind === 'jackett'}
			<label class="check"><input type="radio" name="keep" checked={!keepSources} onchange={() => (keepSources = false)} /> <span>{t(removing.sources === 1 ? 'sources.bridge.removeIts.one' : 'sources.bridge.removeIts.other', { count: removing.sources })}</span></label>
			<label class="check"><input type="radio" name="keep" checked={keepSources} onchange={() => (keepSources = true)} /> <span>{t('sources.bridge.keepStop')}</span></label>
		{:else if removing}
			<p class="small">{t(removing.sources === 1 ? 'sources.bridge.removesProwlarr.one' : 'sources.bridge.removesProwlarr.other', { count: removing.sources })}</p>
			<label class="check"><input type="checkbox" bind:checked={keepSources} /> <span>{t('sources.bridge.unlinkHere')}</span></label>
		{/if}
		{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('sources.bridge.cancel')}</Button>
		<Button variant="destructive" type="submit" form="remove-bridge" busy={removeBusy}>{t('sources.bridge.unlink')}</Button>
	{/snippet}
</Sheet>

<style>
	.form { display: flex; flex-direction: column; gap: 16px; }
	.bridges { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
	.bridges > li { display: flex; flex-direction: column; gap: 6px; padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.brow { display: flex; align-items: center; gap: 12px; }
	.brow .grow b { display: block; font: 600 15px/1.3 var(--font-ui); }
	.addr { display: block; margin-top: 2px; }
	.ctag { flex: none; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-ui); color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.ctag.bad { color: var(--danger-text, #ff6961); border-color: color-mix(in srgb, var(--danger-text, #ff6961) 40%, transparent); }
	.row-acts { display: flex; gap: 16px; }
	.quiet { border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet.danger:hover { color: var(--danger-text, #ff6961); }
	.quiet:disabled { opacity: 0.5; cursor: default; }
	.quiet:focus-visible, .add-row:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.add-row { width: 100%; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 16px; border: 1px dashed color-mix(in srgb, var(--text) 18%, transparent); border-radius: 16px; background: none; color: var(--text-muted); font: 500 15px/1 var(--font-ui); cursor: pointer; transition: color 0.15s, border-color 0.15s; }
	.add-row:hover { color: var(--text); border-color: color-mix(in srgb, var(--text) 32%, transparent); }
	.grow { flex: 1 1 16rem; min-width: 0; overflow-wrap: anywhere; }
	.acts { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; justify-content: flex-end; }
	.callback-line { margin: 0; overflow-wrap: anywhere; }
	.cb-addr { color: var(--text); }
	.link { display: inline-flex; align-items: center; min-height: 32px; vertical-align: middle; border: 0; padding: 0 8px; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.link:hover { color: var(--text); }
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	/* First run: Test and Link share one row; the answer tag takes its own line. */
	.acts.one-row .tag { flex: 1 0 100%; margin: 0; }
	.acts.one-row > :global(.ui-btn) { flex: 1 1 0; }
	.tag { margin-right: auto; display: inline-flex; min-width: 0; }
	.keys { display: flex; flex-direction: column; gap: 12px; }
	.check { display: flex; gap: 8px; align-items: flex-start; font: var(--t-callout); }
	.check input { margin-top: 3px; flex: none; }
</style>
