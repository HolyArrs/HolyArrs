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
	import IconButton from '#lib/ui/IconButton.svelte';
	import { BRIDGE_BRINGS, BRIDGE_PORTS, bridgeKeyPage, joinAddress, splitAddress } from './bridge-setup.ts';

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
	// kind: the app to connect first (setup's Prowlarr and Jackett rows).
	// imported: indexers an import already brought through this app (Prowlarr feeds in Radarr or Sonarr): its address fills the form and the count shows under What comes over.
	let { onchange, compact = false, kind: first, imported }: { onchange?: () => void; compact?: boolean; kind?: 'jackett' | 'prowlarr'; imported?: { address: string; n: number; from: string[]; names?: string[] } } = $props();

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
	let kind = $state<Kind>(untrack(() => first ?? (compact ? 'prowlarr' : 'jackett')));
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
	let typedKeys = $state<Record<string, string>>({ sonarr: '', radarr: '', lidarr: '' });
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

	// First run: the address as Protocol | IP or hostname | Port (like the old-app step), with a network search and Fetch key.
	let ssl = $state(false);
	let host = $state('');
	let port = $state(untrack(() => BRIDGE_PORTS[kind]));
	$effect(() => {
		if (!imported?.address || host) return;
		const a = splitAddress(imported.address, BRIDGE_PORTS[kind]);
		untrack(() => ((ssl = a.ssl), (host = a.host), (port = a.port)));
	});
	let basePath = '';
	let found = $state<{ kind: string; address: string }[]>([]);
	let pick = $state('manual');
	let finding = $state(false);
	let fetching = $state(false);
	let keyNote = $state<'found' | 'kept' | 'unreachable' | ''>('');
	let actsW = $state(0);
	const keyPage = $derived(bridgeKeyPage(kind, ssl, host, port, basePath));
	function typed() {
		address = joinAddress(ssl, host, port, basePath);
		pick = 'manual';
		keyNote = '';
		result = null;
		errors = {};
	}
	// Switching app in setup: the port follows the app when it still holds the other app's default.
	let shownKind = untrack(() => kind);
	$effect(() => {
		const k = kind;
		untrack(() => {
			if (!compact || k === shownKind) return;
			shownKind = k;
			found = [];
			if (!port.trim() || Object.values(BRIDGE_PORTS).includes(port.trim())) port = BRIDGE_PORTS[k];
			typed();
		});
	});
	function fillFrom(url: string) {
		({ ssl, host, port, basePath } = splitAddress(url, BRIDGE_PORTS[kind]));
		address = joinAddress(ssl, host, port, basePath);
	}
	function pickFound(v: string) {
		const f = found[Number(v)];
		if (v === 'manual' || !f) {
			pick = 'manual';
			return void tick().then(() => document.getElementById('br-host')?.focus());
		}
		basePath = '';
		fillFrom(f.address);
		pick = v;
		keyNote = '';
		result = null;
		void tick().then(() => document.getElementById('br-key')?.focus());
	}
	// The refresh button inside the address box: searches this network for the picked app only.
	async function findHere() {
		finding = true;
		errors = {};
		result = null;
		try {
			const r = await api.post<{ found: { kind: string; address: string }[] }>('/migrate/find', { host: location.hostname, addresses: [address].filter(Boolean), kinds: [kind] });
			found = (r.found ?? []).filter((f) => f.kind === kind);
			if (found.length) pickFound('0');
			else result = { ok: false, text: t('sources.bridge.noneFound', { name: names[kind] }) };
		} catch (e) {
			result = { ok: false, text: reachWords(message(e)) };
		} finally {
			finding = false;
		}
	}
	// Fetch key: the app hands out its key only while it has no sign-in (Prowlarr) or no admin password (Jackett).
	async function fetchKey() {
		if (fetching || !host.trim()) return;
		fetching = true;
		keyNote = '';
		try {
			const r = await api.post<{ key?: string; problem?: string }>('/migrate/key-lookup', { kind, url: address, allow_local_network: true });
			if (r.key) {
				key = r.key;
				keyNote = 'found';
			} else keyNote = r.problem === 'unreachable' ? 'unreachable' : 'kept';
		} catch {
			keyNote = 'unreachable';
		} finally {
			fetching = false;
		}
	}

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
		if (compact) fillFrom(b.url);
		editingId = b.id;
	}

	function startConnect() {
		if (compact) {
			({ ssl, host, port, basePath } = { ssl: false, host: '', port: BRIDGE_PORTS[kind], basePath: '' });
			address = '';
			found = [];
			pick = 'manual';
			keyNote = '';
		}
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
		document.getElementById(f === 'callback_url' ? 'br-callback' : f === 'key' ? 'br-key' : compact ? 'br-host' : 'br-url')?.focus();
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
			typedKeys = { sonarr: '', radarr: '', lidarr: '' };
			newKeys = false;
			keysNeeded = [];
			await load();
			onchange?.();
			// The focused Link button is gone with the form: keep focus on the step.
			await tick();
			(document.querySelector<HTMLElement>('[data-testid="bridge-edit"]') ?? document.querySelector<HTMLElement>('[data-testid="bridge-start"]'))?.focus();
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

	// The step's Continue links what was typed but not linked yet, so it is never dropped. True when the step may move on.
	export async function connectTyped(): Promise<boolean> {
		if (!open || !address.trim() || !key.trim()) return true;
		await connect();
		return !open;
	}
	// Whether the form holds a typed address and key not linked yet.
	export function hasTyped(): boolean {
		return open && !!address.trim() && !!key.trim();
	}
	function cancelForm() {
		open = false;
		reauthFor = null;
		editingId = null;
		key = '';
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
			// The Remove that opened the sheet is gone: focus the form that opened, or what is left.
			await tick();
			(document.getElementById(compact ? 'br-host' : 'br-url') ?? document.querySelector<HTMLElement>('[data-testid="bridge-edit"], [data-testid="bridge-start"]'))?.focus();
		} catch (err) {
			removeError = message(err);
		} finally {
			removeBusy = false;
		}
	}

	const countTag = (b: Bridge) =>
		t(b.sources === 1 ? 'sources.bridge.sources.one' : 'sources.bridge.sources.other', { count: b.sources });
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
			<!-- First run: the form on the left, what linking brings on the right (one column on a phone). -->
			<div class:cols={compact}>
			<div class="form" data-testid="bridge-form">
				<!-- Setup opens one app at a time (its own tile): no switch there. -->
				{#if !(compact && first)}<SegmentedControl options={kindChoices} bind:value={kind} label={t('sources.bridge.app')} testid="br-kind" />{/if}
				{#if compact}
					<div class="sfield">
						<div class="addr-labels"><label for="br-scheme" class="proto-label">{t('welcome.server.protocol')}</label><label for={pick === 'manual' ? 'br-host' : 'br-pick'}>{t('welcome.server.ipOrHostPlain')}</label><label for="br-port" class="port-label">{t('welcome.server.port')}</label></div>
						<div class="addr-line">
							<button id="br-scheme" type="button" class="input scheme" class:secure={ssl} onclick={() => { ssl = !ssl; typed(); }} aria-label={t('welcome.server.schemeFlip', { scheme: ssl ? 'https' : 'http' })} data-testid="br-scheme">{ssl ? 'https' : 'http'}://</button>
							<div class="addr-row" class:is-select={found.length && pick !== 'manual'}>
								{#if found.length && pick !== 'manual'}
									<select id="br-pick" class="input" value={pick} onchange={(e) => pickFound(e.currentTarget.value)} data-testid="br-pick">
										<option value="manual">{t('welcome.server.manual')}</option>
										{#each found as f, i (f.address)}<option value={String(i)}>{f.address.replace(/^https?:\/\//, '').replace(/:\d+\/?$/, '')}</option>{/each}
									</select>
								{:else}
									<input id="br-host" class="input mono" dir="ltr" bind:value={host} oninput={typed} placeholder="192.168.1.10" autocomplete="off" spellcheck="false" aria-invalid={errors.url ? 'true' : undefined} aria-describedby={errors.url ? 'br-host-error' : undefined} />
								{/if}
								<span class="in-field">{#if found.length && pick !== 'manual'}<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span><span class="sep" aria-hidden="true"></span>{/if}<IconButton icon="refresh" variant="tinted" label={t('sources.bridge.find', { name: names[kind] })} onclick={findHere} disabled={finding} aria-busy={finding} data-testid="bridge-find" /></span>
							</div>
							<input id="br-port" class="input port" dir="ltr" inputmode="numeric" bind:value={port} oninput={typed} autocomplete="off" />
						</div>
						{#if errors.url}<span class="hint bad-text" id="br-host-error">{errors.url}</span>{:else if kind === 'jackett'}<span class="hint">{t('sources.bridge.basePath')}</span>{/if}
					</div>
					<div class="sfield">
						<label for="br-key">{t('sources.bridge.apiKeyLabel')}</label>
						<div class="key-row">
							<input id="br-key" class="input mono" dir="ltr" type="password" style:padding-inline-end={actsW ? `${actsW + 12}px` : undefined} bind:value={key} oninput={() => (keyNote = '')} autocomplete="new-password" spellcheck="false" aria-invalid={errors.key ? 'true' : undefined} aria-describedby="br-key-hint" />
							<span class="key-acts" bind:clientWidth={actsW}>{#if host.trim()}<button type="button" class="key-fetch" onclick={fetchKey} disabled={fetching} aria-busy={fetching} data-testid="bridge-key-fetch">{t('welcome.link.fetchKey')}</button>{/if}{#if keyPage}<a class="key-get" href={keyPage} target="_blank" rel="noopener noreferrer" data-testid="bridge-key-page">{t('welcome.link.getKey')}<Icon name="arrowUpRight" size={14} /></a>{/if}</span>
						</div>
						<span class="hint" class:ok-text={keyNote === 'found'} class:bad-text={!!errors.key} id="br-key-hint" role={keyNote ? 'status' : undefined} data-testid="bridge-key-hint">{errors.key ? errors.key : keyNote === 'found' ? t('sources.bridge.keyFound', { name: names[kind] }) : keyNote === 'kept' ? t(kind === 'jackett' ? 'sources.bridge.keyKeptJackett' : 'sources.bridge.keyKept', { name: names[kind] }) : keyNote === 'unreachable' ? t('sources.bridge.notReached', { name: names[kind] }) : editingId != null ? t('sources.bridge.keyAgain') : kind === 'jackett' ? t('sources.bridge.keyJackett') : t('sources.bridge.keyProwlarr')}</span>
					</div>
				{:else}
				<Field id="br-url" label={t('sources.bridge.address')} hint={kind === 'jackett' ? t('sources.bridge.basePath') : undefined} error={errors.url}>
					<input id="br-url" class="input" dir="ltr" class:mono={!compact} type={compact ? 'text' : 'url'} inputmode="url" autocomplete="off" spellcheck="false" placeholder={(compact ? '' : 'http://') + (kind === 'jackett' ? '192.168.1.10:9117' : '192.168.1.10:9696')} bind:value={address} aria-invalid={errors.url ? 'true' : undefined} aria-describedby={errors.url ? 'br-url-error' : kind === 'jackett' ? 'br-url-hint' : undefined} />
				</Field>
				<Field id="br-key" label={t('sources.bridge.apiKeyLabel')} hint={editingId != null ? t('sources.bridge.keyAgain') : kind === 'jackett' ? t('sources.bridge.keyJackett') : t('sources.bridge.keyProwlarr')} error={errors.key}>
					<input id="br-key" class="input" dir="ltr" class:mono={!compact} type="password" autocomplete="new-password" spellcheck="false" bind:value={key} aria-invalid={errors.key ? 'true' : undefined} aria-describedby={errors.key ? 'br-key-error' : 'br-key-hint'} />
				</Field>
				{/if}
				{#if kind === 'prowlarr'}
					{#if callback && !callbackOpen && !errors.callback_url}
						<p class="small muted callback-line">{t('sources.bridge.reaches')} <span class="cb-addr" class:mono={!compact}>{compact ? callback.replace(/^https?:\/\//, '') : callback}</span> <button type="button" class="link" onclick={() => (callbackOpen = true)}>{t('sources.bridge.change')}</button></p>
					{:else}
						<Field id="br-callback"  label={t('sources.bridge.callback')} hint={t('sources.bridge.callbackHint')} error={errors.callback_url}>
							<input id="br-callback" class="input" dir="ltr" class:mono={!compact} type={compact ? 'text' : 'url'} inputmode="url" autocomplete="off" spellcheck="false" bind:value={callback} aria-invalid={errors.callback_url ? 'true' : undefined} aria-describedby={errors.callback_url ? 'br-callback-error' : 'br-callback-hint'} />
						</Field>
					{/if}
					{#if keysNeeded.length}
						<div class="keys" data-testid="bridge-keys">
							<label class="check"><input type="checkbox" bind:checked={newKeys} data-testid="bridge-new-keys" /> <span>{t('sources.bridge.replaceKeys')}</span></label>
							{#if !newKeys}
								<p class="hint">{keysNeeded.length === 1 ? t('sources.bridge.currentKey') : t('sources.bridge.currentKeys')}</p>
								{#each keysNeeded as side (side)}
									<Field id="br-key-{side}" label={t(side === 'sonarr' ? 'sources.bridge.tvKey' : side === 'lidarr' ? 'sources.bridge.musicKey' : 'sources.bridge.movieKey')}>
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
					{#if !compact}<Button type="button" variant="plain" onclick={cancelForm}>{t('sources.bridge.cancel')}</Button>{/if}
					<Button type="button" size={compact ? 'lg' : 'md'} busy={testing} disabled={!address.trim() || (!key.trim() && !imported?.n)} onclick={test} data-testid="bridge-test">{compact ? t('welcome.server.check') : t('sources.bridge.test')}</Button>
					<Button type="button" size={compact ? 'lg' : 'md'} variant="primary" busy={connecting} disabled={!address.trim() || (!key.trim() && !imported?.n)} onclick={connect} data-testid="bridge-connect">{editingId != null ? t('sources.bridge.save') : compact ? t('sources.bridge.linkName', { name: names[kind] }) : t('sources.bridge.link')}</Button>
				</div>
				<!-- First run: changing or adding next to a linked one can be left as it was. -->
				{#if compact && (editingId != null || bridges.length)}<button type="button" class="quiet keep" onclick={cancelForm} data-testid="bridge-cancel">{t('sources.bridge.cancel')}</button>{/if}
			</div>
			{#if compact}
				<aside class="brings" aria-labelledby="br-brings" data-testid="bridge-brings">
					<h2 id="br-brings">{t('sources.bridge.brings.title')}</h2>
					{#if imported?.n}<p class="had" data-testid="bridge-had"><b>{t('sources.bridge.brings.had', { n: imported.n })}</b>{#if imported.from.length}<span>{t('sources.bridge.brings.hadFrom', { apps: imported.from.join(', ') })}</span>{/if}{#if imported.names?.length}<span class="names">{imported.names.join(' · ')}</span>{/if}{#if !key.trim()}<span>{t('sources.bridge.brings.keyKept')}</span>{/if}</p>{/if}
					<ul class="btiles">{#each BRIDGE_BRINGS as l (l)}<li class="btile"><span class="bname">{t(l, { name: names[kind] })}</span><Icon name="check" size={14} /></li>{/each}</ul>
					<p class="hint">{t('sources.bridge.brings.stays', { name: names[kind] })}</p>
				</aside>
			{/if}
			</div>
		{/if}
	{/if}
</SettingsCard>

<Sheet bind:open={removeOpen}  title={compact ? t('sources.bridge.removeTitle', { name: removing ? names[removing.kind] : '' }) : t('sources.bridge.unlinkTitle', { name: removing ? names[removing.kind] : '' })} description={removing?.url} size="sm">
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
		<Button variant="destructive" type="submit" form="remove-bridge" busy={removeBusy}>{compact ? t('sources.bridge.remove') : t('sources.bridge.unlink')}</Button>
	{/snippet}
</Sheet>

<style>
	.form { display: flex; flex-direction: column; gap: var(--s-4); }
	/* The flex gap is the only space between fields. */
	.form > :global(.field + .field) { margin-top: 0; }
	.bridges { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
	.bridges > li { display: flex; flex-direction: column; gap: 6px; padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.brow { display: flex; align-items: center; gap: 12px; }
	.brow .grow b { display: block; font: 600 15px/1.3 var(--font-ui); }
	.addr { display: block; margin-top: 2px; }
	.ctag { flex: none; padding: 4px 8px; border-radius: 999px; font: 500 12px/1.2 var(--font-ui); color: var(--ok-text, #30d158); border: 1px solid color-mix(in srgb, var(--ok-text, #30d158) 40%, transparent); }
	.ctag.bad { color: var(--fail-text, #ff6961); border-color: color-mix(in srgb, var(--fail-text, #ff6961) 40%, transparent); }
	.row-acts { display: flex; gap: 8px; }
	.quiet { display: inline-flex; align-items: center; min-height: 32px; border: 0; padding: 0 8px; border-radius: 6px; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet.danger:hover { color: var(--fail-text, #ff6961); }
	.bad-text { color: var(--fail-text); }
	.ok-text { color: var(--ok-text); }
	.keep { align-self: center; }
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
	.tag { margin-inline-end: auto; display: inline-flex; min-width: 0; }
	.keys { display: flex; flex-direction: column; gap: 12px; }
	.check { display: flex; gap: 8px; align-items: flex-start; font: var(--t-callout); }
	.check input { margin-top: 3px; flex: none; }
	/* First run: the old-app step's form (ReadSheet inline): address and key on the left, what comes over on the right. */
	.had { margin: 0 0 8px; display: flex; flex-direction: column; gap: 2px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--g-line, var(--line)); background: color-mix(in srgb, var(--ok-text) 10%, transparent); font: var(--t-footnote); color: var(--text-muted); text-align: start; }
	.btiles { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; text-align: start; }
	.btile { display: flex; flex-direction: column; gap: 2px; padding: 7px 10px; min-height: 48px; box-sizing: border-box; border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); border-radius: 10px; background: var(--g-bg, rgb(255 255 255 / 0.05)); }
	.btile :global(svg) { color: var(--ok-text); }
	.bname { font-size: 12px; opacity: 0.7; }
	.had .names { color: var(--text); }
	.had b { color: var(--ok-text); font: 600 14px/1.3 var(--font-ui); }
	.cols { display: flex; flex-direction: column; gap: var(--s-5, 24px); }
	@media (max-width: 1099px) { .cols .brings { display: none; } }
	@media (min-width: 1100px) { .cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); column-gap: 40px; align-items: start; } }
	.sfield { min-width: 0; display: grid; gap: 6px; text-align: center; }
	.sfield .input { text-align: start; width: 100%; box-sizing: border-box; }
	.sfield > .hint, .brings .hint { color: var(--text-muted); font: var(--t-footnote); }
	.sfield > .hint.ok-text { color: var(--ok-text); }
	.sfield > .hint.bad-text { color: var(--fail-text); }
	.addr-labels { position: relative; }
	.addr-labels > label { display: block; }
	.addr-labels .proto-label, .addr-labels .port-label { position: absolute; top: 0; width: 80px; text-align: center; }
	.addr-labels .proto-label { inset-inline-start: 0; }
	.addr-labels .port-label { inset-inline-end: 0; }
	.addr-line { display: grid; grid-template-columns: 80px minmax(0, 1fr) 80px; gap: 14px; }
	.addr-row { position: relative; min-width: 0; }
	.addr-row .input { width: 100%; padding-inline-end: 52px; }
	.addr-row.is-select .input { appearance: none; -webkit-appearance: none; padding-inline-end: 92px; cursor: pointer; }
	.addr-line .scheme { display: flex; align-items: center; justify-content: center; width: 80px; min-width: 0; padding: 0; font-weight: 400; cursor: pointer; }
	.addr-line .scheme.secure { border-color: rgb(52 199 89 / 0.5); }
	.addr-line .port { text-align: center; padding: 0 8px; }
	.in-field { position: absolute; inset-inline-end: 6px; top: 50%; transform: translateY(-50%); display: flex; align-items: center; gap: 2px; pointer-events: none; }
	.in-field :global(button) { pointer-events: auto; }
	.chev { display: flex; width: 28px; justify-content: center; opacity: 0.75; }
	.sep { width: 1px; height: 20px; background: currentColor; opacity: 0.2; margin-inline: 2px; }
	.key-row { position: relative; }
	.key-acts { position: absolute; inset-inline-end: 6px; top: 6px; bottom: 6px; display: flex; gap: 6px; }
	.key-fetch { display: inline-flex; align-items: center; padding: 0 12px; border: 1px solid var(--g-line, rgb(255 255 255 / 0.2)); border-radius: 10px; background: var(--g-bg, rgb(255 255 255 / 0.08)); color: inherit; font: 600 13px/1 var(--font-ui); cursor: pointer; white-space: nowrap; }
	.key-fetch:disabled { opacity: 0.55; cursor: default; }
	.key-fetch[aria-busy='true'] { opacity: 0.8; cursor: progress; }
	.key-get { display: inline-flex; align-items: center; gap: 4px; padding: 0 12px; border-radius: 10px; background: var(--accent); color: var(--on-accent, #fff); font: 600 13px/1 var(--font-ui); text-decoration: none; white-space: nowrap; }
	.key-get:focus-visible, .key-fetch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	@media (hover: hover) { .key-get:hover { filter: brightness(1.08); } .key-fetch:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.12); } }
	.brings h2 { margin: 0 0 10px; font: 600 15px/20px var(--font-ui); text-align: start; }
	.brings .hint { margin: 10px 0 0; text-align: start; }
	@media (max-width: 600px) {
		/* Phone: the key buttons drop under the field. */
		.key-acts { position: static; margin-top: 6px; justify-content: center; }
		.key-acts > * { min-height: 44px; }
		.key-row .input { padding-inline-end: 12px !important; }
	}
</style>
