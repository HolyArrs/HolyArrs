<script lang="ts" module>
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
import BringsPanel from './BringsPanel.svelte';
	export interface SavedKey {
		kind: string;
		address: string;
		allow_local_network: boolean;
	}
	export interface FoundApp {
		kind: string;
		address: string;
		version?: string;
	}
	export interface LinkedApp {
		kind: string;
		name?: string;
		address: string;
		four_k?: boolean;
		has_key: boolean;
		reachable: boolean;
		version?: string;
	}
	export interface ReadAnswer {
		snapshot: import('./move.ts').Snapshot;
		needs_reentry?: string[];
		notes?: string[];
		// Older reads of the same app at another address (same key): the server forgot them, so the app counts once.
		replaced?: number[];
		// What the read found, zeros included (the setup tiles).
		counts?: Record<string, number>;
		// Overseerr and Seerr: the Sonarr and Radarr they keep, read along.
		linked?: { kind: import('./move.ts').AppKind; snapshot: import('./move.ts').Snapshot; replaced?: number[] }[];
		unread?: /* not reached at the address Seerr keeps (often a Docker name) */ { kind: import('./move.ts').AppKind; address: string }[]; skipped?: /* read before: counted, not read again */ { kind: import('./move.ts').AppKind; address: string }[];
	}
</script>

<script lang="ts">
	import { getContext, tick, untrack } from 'svelte';
	import './read-setup.css';
	import { BRINGS, KEY_PAGES, forgetRead, linkedTags } from './read-setup.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import KeyLogin from './KeyLogin.svelte';
	import { apps, appName, browsable, listNames, placeholderFor, splitHits, usableHits, type AppKind } from './move.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { reachWords } from '../downloads/index.ts';

	let {
		open = $bindable(false),
		start = 'radarr',
		startAddress = '',
		again = false,
		saved = [],
		known = [],
		reading = $bindable(''),
		inline = false,
		via,
		onread
	}: {
		open?: boolean;
		start?: AppKind;
		/** Edit: the address of the copy being read again ('' for a backup or an older read). */
		startAddress?: string;
		again?: boolean;
		saved?: SavedKey[];
		/** Addresses already read; a saved key for one of them is not offered when adding another app. */
		known?: string[];
		reading?: string;
		/** Shown in the page where it is placed (first-run setup), not as a sheet over it. */
		inline?: boolean;
		via?: /** Setup: a Sonarr or Radarr Seerr keeps but HolyArrs could not reach; the server adds the key Seerr keeps. */ { kind: string; url: string; address: string };
		onread: (r: ReadAnswer, kind: AppKind, usedSaved: boolean) => void;
	} = $props();

	let kind = $state<AppKind>('radarr');
	let way = $state<'api' | 'backup'>('api');
	let address = $state('');
	let key = $state('');
	let username = $state('');
	let signWord = $state('');
	let backupPath = $state('');
	let local = $state(true);
	let useSaved = $state(true);
	let error = $state('');
	let probe = $state<{ ok: boolean; text: string } | null>(null);
	let checking = $state(false);
	// Setup: how much of each line would come over (null: not counted or the count failed).
	let counts = $state<Record<string, number> | null>(null);
	let counting = $state(false);
	let countFor = '';
	// Setup: Check connection does the whole read; Continue then only hands it over (no second wait).
	let done = $state<{ r: ReadAnswer; k: AppKind; usedSaved: boolean } | null>(null);
	const dropDone = () => ((d) => ((done = null), forgetRead(d)))(done);
	const doneApps = $derived(linkedTags(done?.r));
	// Setup: the key came from the app's own startup file.
	let keyFound = $state(false), actsW = $state(0);
	// Fetch key pressed and the app kept its key to itself (its sign-in is on).
	let keyMissed = $state(false);
	let fetching = $state(false);
	const FETCH_KINDS = ['radarr', 'sonarr', 'lidarr', 'prowlarr', 'bazarr', 'seerr', 'overseerr'];
	// Fetch key found the app's sign-in on: what else it can sign in with (Plex, Seerr's Jellyfin or Emby; '' offers both).
	let needLogin = $state<{ plex: boolean; server: string } | null>(null);
	let form = $state<HTMLFormElement>();
	// "Find on my network" (owner only, on a click) and the apps an Overseerr, Seerr or Prowlarr is set up with.
	let found = $state<FoundApp[] | null>(null);
	let finding = $state(false);
	let searched = $state<string[]>([]);
	let showOthers = $state(false);
	// Hits of the app picked come first; others fold away. Changing the picker re-sorts without searching again.
	const hits = $derived(splitHits(found ?? [], kind));
	let linked = $state<LinkedApp[]>([]);
	let linkedFrom = $state<AppKind | ''>('');
	// First-run setup (inline): the address as Protocol | IP or hostname | Port, like the media-server screen.
	let ssl = $state(false);
	let host = $state('');
	let port = $state('');
	let pick = $state('manual');
	const appPort = (k: string) => String(apps.find((a) => a.kind === k)?.port ?? 8080);
	// Found copies of this app (Seerr and Overseerr count as one card).
	const pair = ['seerr', 'overseerr'];
	const setupHits = $derived((found ?? []).filter((f) => f.kind === kind || (pair.includes(kind) && pair.includes(f.kind))));
	const setupUrl = $derived(host.trim() ? `${ssl ? 'https' : 'http'}://${host.trim()}${port.trim() ? ':' + port.trim() : ''}` : '');
	let addingLinked = $state(false);

	const hidden = getContext<AppKind[] | undefined>('move-hide-apps') ?? [];
	// Seerr and Overseerr share a card: the check says which one answered.
	const tellDetected = getContext<((k: AppKind | '') => void) | undefined>('move-detected-kind');
	let detected = $state<AppKind | ''>('');
	const named = $derived(detected || kind);
	$effect(() => tellDetected?.(detected));
	$effect(() => () => tellDetected?.(''));
	const shown = apps.filter((a) => !hidden.includes(a.kind));
	// The last typed sign-in per copy of an app (kind and address).
	const lastRead: Record<string, { address: string; key: string; username: string; signWord: string }> = {};
	let ctrl: AbortController | null = null;
	let wasOpen = false;
	let startedAt = 0;
	let stopped = false;

	const qbit = $derived(kind === 'qbittorrent');
	// Edit uses the key kept for that copy; adding offers a kept key only for a copy not read yet.
	const savedKey = $derived(
		again
			? saved.find((x) => x.kind === kind && (startAddress ? x.address === startAddress : true))
			: saved.find((x) => x.kind === kind && !known.includes(x.address))
	);
	const withSaved = $derived(way === 'api' && !!savedKey && useSaved);
	const keptAgain = $derived(again && !!savedKey);
	// Fetch key shows whenever no saved key is used, also when an app is read again.
	const canFetch = $derived(inline && !withSaved && FETCH_KINDS.includes(kind));
	const busy = $derived(reading !== '');
	const fromVia = $derived(inline && !again && !!via && via.address !== '');
	const title = $derived(again ? t('migrate.edit', { name: appName(kind) }) : t('migrate.addAnApp'));

	$effect(() => {
		if (open && !wasOpen) {
			if (!busy) {
				kind = start;
				way = 'api';
				useSaved = true;
				const last = again ? lastRead[`${start} ${startAddress}`] : undefined;
				address = again ? startAddress || (saved.find((x) => x.kind === start)?.address ?? last?.address ?? '') : (via?.address ?? '');
				key = last?.key ?? '';
				username = last?.username ?? '';
				signWord = last?.signWord ?? '';
				backupPath = '';
				ssl = false;
				host = '';
				port = appPort(start);
				// Setup's Edit: the copy's address back in Protocol | host | Port.
				if (inline && (again || via) && address) {
					try {
						const u = new URL(/^[a-z]+:\/\//i.test(address) ? address : `http://${address}`);
						ssl = u.protocol === 'https:';
						host = u.hostname;
						port = u.port || (ssl ? '443' : '80');
					} catch {
						host = address;
					}
				}
				pick = 'manual';
				local = true;
				error = '';
				probe = null;
				detected = '';
				found = null;
				linked = [];
				linkedFrom = '';
			}
			setTimeout(() => document.getElementById(inline ? (again ? 'move-key' : 'rs-host') : again || busy ? 'move-address' : 'move-app')?.focus(), 0);
		}
		wasOpen = open;
	});

	function changed() {
		error = '';
		keyFound = false;
		keyMissed = false; needLogin = null;
		probe = null;
		detected = '';
		linked = [];
		linkedFrom = '';
		counts = null;
		countFor = '';
		if (done) dropDone();
	}
	// Edit with the kept key: count straight away.
	let autoChecked = false;
	$effect(() => {
		if (!open) autoChecked = false;
		else if (inline && again && withSaved && !busy && !checking && !done && !autoChecked) {
			autoChecked = true;
			void untrack(() => check());
		}
	});
	async function find() {
		finding = true;
		error = '';
		try {
			// In Docker the server only sees its bridge network; the address the browser used and the apps
			// already typed say which home network to search.
			const ask = () => api.post<{ found: FoundApp[]; searched?: string[] }>('/migrate/find', {
				host: location.hostname,
				addresses: [address, ...known, ...saved.map((x) => x.address)].filter(Boolean),
				// Setup asks about one app: only its port is tried, so the answer comes in a second or two.
				...(inline && !again ? { kinds: kind === 'seerr' ? ['seerr', 'overseerr'] : [kind] } : {})
			});
			let r = await ask();
			// A first search can miss an app whose network was still waking up: one quiet second try before saying none answered.
			if (!(r.found ?? []).length) r = await ask();
			searched = r.searched ?? [];
			showOthers = false;
			found = usableHits(r.found ?? [], shown.map((a) => a.kind));
		} catch (err) {
			if (!quiet(err)) error = reachWords(message(err));
		} finally {
			finding = false;
		}
	}
	function useFound(f: { kind: string; address: string }) {
		if (shown.some((a) => a.kind === f.kind)) kind = f.kind as AppKind;
		address = f.address.replace(/^http:\/\//, '');
		useSaved = false;
		found = null;
		changed();
		setTimeout(() => document.getElementById(qbit ? 'move-user' : 'move-key')?.focus(), 0);
	}
	// After a good test of Overseerr, Seerr or Prowlarr: the apps it is set up with.
	async function loadLinked() {
		const from = kind;
		if (inline || way !== 'api' || !['seerr', 'overseerr', 'prowlarr'].includes(from)) return;
		try {
			const r = await api.post<{ apps: LinkedApp[] }>('/migrate/linked', body());
			if (kind !== from || !probe?.ok) return;
			linked = (r.apps ?? []).filter((a) => !known.includes(a.address));
			linkedFrom = from;
		} catch {
			// The list is a help; the test result already shows.
		}
	}
	const addable = $derived(linked.filter((a) => a.has_key && a.reachable));
	const linkedNames = $derived(listNames([...new Set(addable.map((a) => appName(a.kind)))], appLocale()));
	async function addLinked() {
		if (addable.length === 0) return;
		addingLinked = true;
		error = '';
		const from = linkedFrom;
		try {
			const r = await api.post<{ read: (ReadAnswer & { kind: AppKind })[]; failed: { kind: string; error: string }[] }>('/migrate/linked', {
				...body(),
				add: addable.map((a) => a.address)
			});
			for (const x of r.read ?? []) onread(x, x.kind, true);
			for (const f of r.failed ?? []) toasts.push(t('migrate.couldnTRead', { name: appName(f.kind) }), 'fail', { body: reachWords(f.error) });
			const done = new Set((r.read ?? []).map((x) => x.snapshot.address ?? ''));
			linked = linked.filter((a) => !done.has(a.address));
			linkedFrom = linked.length ? from : '';
		} catch (err) {
			if (!quiet(err)) error = reachWords(message(err));
		} finally {
			addingLinked = false;
		}
	}
	function body(): Record<string, unknown> {
		const b: Record<string, unknown> = { kind };
		if (way === 'backup') {
			b.path = backupPath.trim();
			return b;
		}
		if (withSaved) {
			b.use_saved_key = true;
			if (savedKey) b.url = savedKey.address;
			return b;
		}
		const a = inline ? setupUrl : address.trim();
		b.url = a && !/^[a-z][a-z0-9+.-]*:\/\//i.test(a) ? `http://${a}` : a;
		b.allow_local_network = local;
		if (qbit && (username || signWord)) {
			b.username = username;
			b.password = signWord;
		}
		if (key) b.key = key;
		else if (fromVia) b.linked_from = via;
		return b;
	}
	function quiet(err: unknown): boolean {
		return (err instanceof ApiError && err.body.code === 'reauth') || (err instanceof DOMException && err.name === 'AbortError');
	}
	function missing(): boolean {
		if (way === 'api' && !withSaved && !(inline ? host.trim() : address.trim())) {
			error = t('migrate.addTheAddress');
			document.getElementById(inline ? 'rs-host' : 'move-address')?.focus();
			return true;
		}
		return !!form && !form.reportValidity();
	}
	// Setup, Radarr/Sonarr/Lidarr/Prowlarr with no key typed: ask the app for its key
	// (answered only when its sign-in is off). Any failure leaves the box empty.
	type Lookup = { key?: string; need_login?: boolean; plex_available?: boolean; problem?: string; media_server?: string };
	async function lookupKey(login?: { username?: string; password?: string; use_plex?: boolean; use_media_server?: boolean }): Promise<Lookup> {
		if (!inline || again || way !== 'api' || withSaved || key || !host.trim()) return {};
		if (!FETCH_KINDS.includes(kind)) return {};
		try {
			const b = body();
			const r = await api.post<Lookup>('/migrate/key-lookup', { kind, url: b.url, allow_local_network: b.allow_local_network, ...login });
			if (r.key) {
				key = r.key;
				await tick();
			}
			return r;
		} catch {
			return { problem: 'unreachable' };
		}
	}
	// Fetch key: ask the app for its key now and fill the box; says so when the app won't hand it out.
	const brings = $derived(BRINGS[kind] ?? []);
	// With a login (from KeyLogin) it signs in first; the login is sent once and never kept.
	async function fetchKey(login?: { username?: string; password?: string; use_plex?: boolean; use_media_server?: boolean }): Promise<Lookup> {
		if (fetching) return {};
		fetching = true;
		keyMissed = false;
		const had = key;
		key = '';
		const r = await lookupKey(login);
		fetching = false;
		if (r.key) {
			keyFound = true;
			needLogin = null;
		} else {
			key = had;
			if (login) return r;
			if (r.need_login) needLogin = { plex: !!r.plex_available, server: r.media_server ?? '' };
			else if (r.problem === 'unreachable') error = t('migrate.keyLogin.unreachable', { name: appName(named) });
			else keyMissed = true;
		}
		return r;
	}
	async function check() {
		checking = true;
		const gotKey = !!(await lookupKey()).key;
		checking = false;
		if (missing()) return;
		checking = true;
		changed();
		keyFound = keyFound || gotKey;
		try {
			const r = await api.post<{ app: { kind: string; version?: string } }>('/migrate/probe', body());
			detected = r.app?.kind && r.app.kind !== kind && pair.includes(kind) && pair.includes(r.app.kind) ? (r.app.kind as AppKind) : '';
			probe = { ok: true, text: `${appName(named)}${r.app?.version ? ' ' + r.app.version : ''}` };
			if (inline) {
				checking = false;
				await readNow();
			} else {
				void loadLinked();
			}
		} catch (err) {
			if (!quiet(err)) {
				error = reachWords(message(err));
				probe = { ok: false, text: error };
			}
		} finally {
			checking = false;
		}
		if (error) focusAddress();
	}
	async function focusAddress() {
		await tick();
		document.getElementById(inline ? 'rs-host' : 'move-address')?.focus();
	}
	// Setup: picking a found copy fills the three boxes; Enter it myself opens the host box.
	function pickFound(v: string) {
		pick = v;
		changed();
		const f = setupHits[Number(v)];
		if (v === 'manual' || !f) return void queueMicrotask(() => document.getElementById('rs-host')?.focus());
		if (shown.some((a) => a.kind === f.kind)) kind = f.kind as AppKind;
		try {
			const u = new URL(/^[a-z]+:\/\//i.test(f.address) ? f.address : `http://${f.address}`);
			ssl = u.protocol === 'https:';
			host = u.hostname;
			port = u.port || (ssl ? '443' : '80');
		} catch {
			host = f.address;
		}
		useSaved = false;
		setTimeout(() => document.getElementById(qbit ? 'move-user' : 'move-key')?.focus(), 0);
	}
	// The refresh button inside the address box: the only way a network search starts.
	async function findHere() {
		await find();
		if (setupHits.length) pickFound('0');
		else if (found) error = t('migrate.noneOfKind', { name: appName(kind) });
	}
	// The app's own page with its API key, once an address is filled in: opens in a new tab.
	const keyPage = $derived.by(() => {
		const h = host.trim();
		const path = KEY_PAGES[kind];
		if (!h || !path || /[\s/@]/.test(h)) return '';
		return `${ssl ? 'https' : 'http'}://${h}${port.trim() ? ':' + port.trim() : ''}${path}`;
	});
	function typed() {
		pick = 'manual'; if (again) useSaved = false; // a changed address never sends the saved key to the old one
		changed();
	}
	// Setup: the read itself, run by Check connection; the tiles spin while it goes, then show what it found.
	async function readNow() {
		const k = kind;
		const b = body();
		b.skip_linked = known.filter(Boolean);
		const usedSaved = !!b.use_saved_key;
		ctrl = new AbortController();
		startedAt = Date.now();
		stopped = false;
		reading = k;
		try {
			const r = await api.request<ReadAnswer>('/migrate/read', { method: 'POST', body: b, signal: ctrl.signal });
			if (!usedSaved) lastRead[`${k} ${r.snapshot.address ?? ''}`] = { address, key, username, signWord };
			done = { r, k, usedSaved };
			counts = r.counts ?? r.snapshot.counts ?? null;
		} catch (err) {
			if (!quiet(err)) {
				error = reachWords(message(err));
				probe = { ok: false, text: error };
			}
		} finally {
			reading = '';
			ctrl = null;
		}
	}
	async function read(e: SubmitEvent) {
		e.preventDefault();
		if (inline) {
			// Continue: the read Check connection made goes straight back to the app rows.
			const d = done;
			if (busy || checking || !d) return;
			done = null;
			key = '';
			signWord = '';
			open = false;
			onread(d.r, d.k, d.usedSaved);
			return;
		}
		if (busy || missing()) return;
		const k = kind;
		const b = body();
		if (inline) b.skip_linked /* Seerr's own Radarr and Sonarr come along, minus the ones read already */ = known.filter(Boolean);
		const usedSaved = !!b.use_saved_key;
		let keep = false;
		ctrl = new AbortController();
		startedAt = Date.now();
		stopped = false;
		reading = k;
		changed();
		try {
			const r = await api.request<ReadAnswer>('/migrate/read', { method: 'POST', body: b, signal: ctrl.signal });
			if (!usedSaved) lastRead[`${k} ${r.snapshot.address ?? ''}`] = { address, key, username, signWord };
			key = '';
			signWord = '';
			open = false;
			onread(r, k, usedSaved);
		} catch (err) {
			if (quiet(err)) {
				if (stopped) {
					keep = true;
					if (open) error = t('migrate.readingStopped');
				}
				return;
			}
			if (open) error = reachWords(message(err));
			else toasts.push(t('migrate.couldnTRead', { name: appName(k) }), 'fail', { body: reachWords(message(err)) });
		} finally {
			reading = '';
			ctrl = null;
			if (!keep) {
				key = '';
				signWord = '';
			}
		}
		if (open && error) focusAddress();
	}
	export function stop(e?: MouseEvent) {
		if (!ctrl) return;
		e?.preventDefault();
		if (e && e.detail === 0 && Date.now() - startedAt < 1000) return;
		stopped = true;
		ctrl.abort();
	}
</script>

{#snippet hit(f: FoundApp)}
	<li class="srow hit">
		<div class="txt"><span class="label">{appName(f.kind)}{f.version ? ' ' + f.version : ''}</span><span class="hint mono addr">{f.address.replace(/^http:\/\//, '')}</span></div>
		<div class="ctl"><Button size="sm" onclick={() => useFound(f)}>{t('migrate.useThis')}</Button></div>
	</li>
{/snippet}

{#snippet readForm()}
	<form id="move-read-form" bind:this={form} onsubmit={read} novalidate>
		<div class="srow">
			<div class="txt"><label for="move-app">{t('migrate.app')}</label></div>
			<div class="ctl">
				<select id="move-app" class="input" bind:value={kind} onchange={changed} disabled={busy || again}>
					{#each shown as a (a.kind)}<option value={a.kind}>{a.name}</option>{/each}
				</select>
			</div>
		</div>
		{#if way === 'api'}
			{#if savedKey && !keptAgain}
				<div class="srow" data-testid="use-saved-key">
					<div class="txt"><label for="move-saved">{t('migrate.useTheSavedKey')}</label><span class="hint">{t('migrate.sentOnlyTo', { address: savedKey.address })}</span></div>
					<div class="ctl">
						<label class="switch"><input id="move-saved" type="checkbox" bind:checked={useSaved} onchange={changed} /><span class="sw" aria-hidden="true"></span></label>
					</div>
				</div>
			{/if}
			<div class="srow">
				<div class="txt">
					{#if withSaved}<span class="label">{t('migrate.address')}</span>{:else}<label for="move-address">{t('migrate.address')}</label>{/if}
					{#if withSaved && keptAgain}<span class="hint">{t('migrate.savedKey')}</span>{/if}
				</div>
				<div class="ctl">
					{#if withSaved}
						<span class="value mono addr" data-testid="saved-address">{savedKey?.address}</span>
					{:else}
						<input id="move-address" class="input mono" type="text" inputmode="url" bind:value={address} oninput={changed} placeholder={placeholderFor(kind).replace('http://', '')} autocomplete="off" spellcheck="false" />
					{/if}
				</div>
			</div>
			{#if !again && !withSaved && !busy}
				<div class="srow" data-testid="find-row">
					<div class="txt"><span class="label">{t('migrate.findOnNetwork')}</span><span class="hint">{t('migrate.findHint')}</span></div>
					<div class="ctl"><Button size="sm" onclick={find} busy={finding} data-testid="find-apps">{t('migrate.search')}</Button></div>
				</div>
				{#if found}
					{#if found.length === 0}
						<p class="none" data-testid="find-none">{t('migrate.noAppsAnswered')}{searched.length ? ' ' + t('migrate.searchedNetworks', { networks: searched.join(', ') }) : ''}</p>
					{:else}
						{#if hits.mine.length === 0}
							<p class="none" data-testid="find-none-kind">{t('migrate.noneOfKind', { name: appName(kind) })}</p>
						{:else}
							<ul class="hits" data-testid="find-hits">
								{#each hits.mine as f (f.kind + f.address)}{@render hit(f)}{/each}
							</ul>
						{/if}
						{#if hits.others.length}
							<p class="other-hits"><button type="button" class="link" aria-expanded={showOthers} onclick={() => (showOthers = !showOthers)} data-testid="find-others">{t('migrate.otherAppsFound', { count: hits.others.length })}</button></p>
							{#if showOthers}
								<ul class="hits" data-testid="find-other-hits">
									{#each hits.others as f (f.kind + f.address)}{@render hit(f)}{/each}
								</ul>
							{/if}
						{/if}
					{/if}
				{/if}
			{/if}
			{#if withSaved && keptAgain}
				<p class="other"><button type="button" class="link" onclick={() => { useSaved = false; changed(); }}>{t('migrate.useADifferentKey')}</button></p>
			{/if}
			{#if !withSaved}
				{#if qbit}
					<div class="srow">
						<div class="txt"><label for="move-user">{t('migrate.userName')}</label></div>
						<div class="ctl"><input id="move-user" class="input" bind:value={username} oninput={changed} autocomplete="off" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-pass">{t('migrate.passwordLabel')}</label></div>
						<div class="ctl"><input id="move-pass" class="input" type="password" bind:value={signWord} oninput={changed} autocomplete="new-password" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-key">{t('migrate.apiKeyLabel')}</label><span class="hint">{t('migrate.replacesUserNameAnd')}</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" /></div>
					</div>
				{:else}
					<div class="srow">
						<div class="txt"><label for="move-key">{t('migrate.apiKeyLabel')}</label><span class="hint">{t('migrate.settingsGeneral', { name: appName(kind) })}</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" required /></div>
					</div>
				{/if}
			{/if}
		{/if}
	</form>
	{#if busy}
		<div class="cap" role="status"><StepCapsule tone="go" live fill={0.4} status="processing" word={t('migrate.adding', { name: appName(reading) })} detail={t('migrate.canTakeAFew')} testid="read-capsule" /></div>
	{/if}
	{#if probe}<p class="probe" data-testid={probe.ok ? 'probe-ok' : undefined}><TestTag ok={probe.ok} text={probe.text} app={appName(kind)} /></p>{/if}
	{#if linkedFrom && linked.length}
		<section class="linked" data-testid="linked-apps">
			{#if linkedFrom === 'prowlarr'}
				<p class="hint">{t('migrate.prowlarrLinked', { name: appName(linkedFrom) })}</p>
			{:else}
				<p class="hint">{t('migrate.linkedKeys', { name: appName(linkedFrom) })}</p>
			{/if}
			<ul class="hits">
				{#each linked as a (a.kind + a.address)}
					<li class="srow hit">
						<div class="txt">
							<span class="label">{a.name || appName(a.kind)}{a.four_k ? ' 4K' : ''}{a.version ? ' ' + a.version : ''}</span>
							<span class="hint mono addr">{a.address.replace(/^https?:\/\//, '')}</span>
							{#if a.has_key && !a.reachable}<span class="hint">{t('migrate.notReached')}</span>{/if}
						</div>
						{#if !a.has_key && shown.some((s) => s.kind === a.kind)}<div class="ctl"><Button size="sm" onclick={() => useFound(a)}>{t('migrate.useThis')}</Button></div>{/if}
					</li>
				{/each}
			</ul>
			{#if addable.length}
				<div class="also"><Button onclick={addLinked} busy={addingLinked} disabled={busy} data-testid="add-linked">{t('migrate.alsoAdd', { names: linkedNames, name: appName(linkedFrom) })}</Button></div>
			{/if}
		</section>
	{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
{/snippet}

{#snippet setupForm()}
	<!-- Desktop: the address and key on the left, what this app brings on the right (like Link your media servers). -->
	<div class="cols">
	<form id="move-read-form" class="server" bind:this={form} onsubmit={read} novalidate>
		{#if savedKey && (!keptAgain || inline)}
			<label class="saved-row" data-testid="use-saved-key"><span class="saved-txt">{t('migrate.useTheSavedKey')}<span class="hint">{t('migrate.sentOnlyTo', { address: savedKey.address })}</span></span><span class="switch"><input type="checkbox" bind:checked={useSaved} onchange={changed} /><span class="sw" aria-hidden="true"></span></span></label>
		{/if}
		{#if !withSaved || again}
			<!-- Editing: the saved address always shows, filled in; only the key hides while the saved one is used. -->
			<div class="field">
				<div class="addr-labels"><label for="rs-scheme" class="proto-label">{t('welcome.server.protocol')}</label><label for={pick === 'manual' ? 'rs-host' : 'rs-pick'}>{t('welcome.server.ipOrHostPlain')}</label><label for="rs-port" class="port-label">{t('welcome.server.port')}</label></div>
				<div class="addr-line">
					<button id="rs-scheme" type="button" class="input scheme" class:secure={ssl} onclick={() => { ssl = !ssl; typed(); }} disabled={busy} aria-label={t('welcome.server.schemeFlip', { scheme: ssl ? 'https' : 'http' })} data-testid="read-scheme">{ssl ? 'https' : 'http'}://</button>
					<div class="addr-row" class:is-select={setupHits.length && pick !== 'manual'}>
						{#if setupHits.length && pick !== 'manual'}
							<select id="rs-pick" class="input" value={pick} onchange={(e) => pickFound(e.currentTarget.value)} disabled={busy} data-testid="read-pick">
								<option value="manual">{t('welcome.server.manual')}</option>
								{#each setupHits as f, i (f.kind + f.address)}<option value={String(i)}>{f.address.replace(/^https?:\/\//, '').replace(/:\d+\/?$/, '')}</option>{/each}
							</select>
						{:else}
							<input id="rs-host" class="input mono" dir="ltr" bind:value={host} oninput={typed} placeholder="192.168.1.10" autocomplete="off" spellcheck="false" disabled={busy} />
						{/if}
						<span class="in-field">{#if setupHits.length && pick !== 'manual'}<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span><span class="sep" aria-hidden="true"></span>{/if}<IconButton icon="refresh" variant="tinted" label={t('welcome.server.refresh')} onclick={findHere} disabled={finding || busy} aria-busy={finding} data-testid="find-apps" /></span>
					</div>
					<input id="rs-port" class="input port" dir="ltr" inputmode="numeric" bind:value={port} oninput={typed} autocomplete="off" disabled={busy} />
				</div>
			</div>
			{/if}
		{#if !withSaved}
			{#if qbit}
				<div class="field">
					<label for="move-user">{t('migrate.userName')}</label>
					<input id="move-user" class="input" bind:value={username} oninput={changed} autocomplete="off" required={!key} disabled={busy} />
				</div>
				<div class="field">
					<label for="move-pass">{t('migrate.passwordLabel')}</label>
					<input id="move-pass" class="input" type="password" bind:value={signWord} oninput={changed} autocomplete="new-password" required={!key} disabled={busy} />
				</div>
				<div class="field">
					<label for="move-key">{t('migrate.apiKeyLabel')}</label>
					<input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" disabled={busy} />
					<span class="hint">{t('migrate.replacesUserNameAnd')}</span>
				</div>
			{:else}
				<div class="field">
					<label for="move-key">{t('migrate.apiKeyLabel')}</label>
					<!-- The key field holds a Get key button on its right: it opens the app's own page with the key, in a new tab. -->
					<div class="key-row">
						<input id="move-key" class="input mono" type="password" style:padding-inline-end={actsW ? `${actsW + 12}px` : undefined} bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" required={!fromVia} disabled={busy} />
						<span class="key-acts" bind:clientWidth={actsW}>{#if canFetch && keyPage}<button type="button" class="key-fetch" onclick={() => fetchKey()} disabled={busy || fetching || !host.trim()} aria-busy={fetching} data-testid="read-key-fetch">{t('welcome.link.fetchKey')}</button>{/if}
						{#if keyPage && browsable(host)}<a class="key-get" href={keyPage} target="_blank" rel="noopener noreferrer" title={t('migrate.settingsGeneral', { name: appName(named) })} data-testid="read-key-page">{t('welcome.link.getKey')}<Icon name="arrowUpRight" size={14} /></a>{/if}</span>
					</div>
					<!-- One line under the field: where the key lives, or what Fetch key found. -->
					{#if needLogin && !keyFound}
						<KeyLogin kind={named} {busy} {fetching} plexAvailable={needLogin.plex} mediaServer={needLogin.server} onsignin={fetchKey} />
					{:else if keyFound}<span class="hint ok-line" role="status" data-testid="read-key-found"><Icon name="check" size={14} />{t('migrate.keyFound', { name: appName(named) })}</span>
					{:else if keyMissed}<span class="hint" role="status" data-testid="read-key-missed">{t('migrate.keyNotHanded', { name: appName(named) })}</span>
					{:else if fromVia && !key}<span class="hint" data-testid="read-key-via">{t('migrate.linkedKeys', { name: appName(via?.kind ?? 'seerr') })}</span>
					{:else}<span class="hint">{t('migrate.settingsGeneral', { name: appName(named) })}</span>{/if}
				</div>
			{/if}
		{/if}
		<div class="checkrow">
			{#if probe?.ok}
				<div class="conn-ok" role="status" data-testid="probe-ok">
					<span class="conn-tick" aria-hidden="true"><Icon name="check" size={16} /></span>
					<div class="conn-text">
						<div class="conn-title">{t('welcome.server.connectedTo', { name: appName(named) })}</div>
						<div class="conn-meta">{probe.text}</div>
					</div>
					<button type="button" class="reconnect" disabled={busy || checking} onclick={check} data-testid="read-reconnect">{t('welcome.server.reconnect')}</button>
				</div>
			{:else}
				<Button onclick={check} busy={checking} disabled={busy} data-testid="read-check">{t('welcome.server.check')}</Button>
			{/if}
		</div>
	</form>
	<!-- While reading, each line spins in place of its tick (no capsule in setup). -->
	<aside class="brings" class:busy aria-labelledby="rs-brings" aria-busy={busy} data-testid="read-brings">
		<h2 id="rs-brings">{busy ? t('migrate.adding', { name: appName(reading) }) : t('migrate.brings.title')}</h2>
		<BringsPanel lines={brings} kind={named} name={appName(named)} {counts} {counting} {busy} apps={doneApps} />
		<p class="hint" role={busy ? 'status' : undefined}>{busy ? t('migrate.canTakeAFew') : t('migrate.brings.filesStay')}</p>
	</aside>
	</div>
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	<Button variant="primary" size="lg" full type="submit" form="move-read-form" disabled={busy || checking || !done} data-testid="read-continue">{t('welcome.continue')}</Button>
{/snippet}

{#snippet actions()}
		<Button variant="secondary" onclick={() => (open = false)}>{t('migrate.cancel')}</Button>
		<Button onclick={check} busy={checking} disabled={busy}>{t('migrate.test')}</Button>
		<Button variant="primary" type={busy ? 'button' : 'submit'} form={busy ? undefined : 'move-read-form'} onclick={busy ? stop : undefined}>{busy ? t('migrate.stop') : again ? t('migrate.save') : t('migrate.add', { name: appName(kind) })}</Button>
{/snippet}

{#if inline}
	{#if open}
		<section class="setup-read" aria-label={appName(named)} data-testid="read-inline">{@render setupForm()}</section>
	{/if}
{:else}
	<Sheet bind:open {title} size="md" placement="auto">
		{@render readForm()}
		{#snippet footer()}{@render actions()}{/snippet}
	</Sheet>
{/if}

<style>
	.srow { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; column-gap: 16px; min-height: 52px; padding: 7px 0; border-top: 1px solid var(--line); }
	.srow:first-child { border-top: 0; } .srow > .txt { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.srow > .txt > :is(.label, label) { font: var(--t-callout); color: var(--text); } .srow > .txt > .hint { color: var(--text-muted); font: var(--t-footnote); }
	.srow > .ctl { justify-self: end; display: flex; align-items: center; gap: 8px; min-width: 0; } .srow > .ctl > .input { width: 260px; }
	@media (max-width: 600px) {
		.srow > .ctl > .input { width: min(260px, 46vw); }
	}
	.addr { display: block; max-width: min(260px, 46vw); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } .cap { margin-top: 12px; }
	.other { margin: 4px 0 0; text-align: right; } .link { background: none; border: 0; padding: 6px 0; min-height: 44px; color: var(--text-muted); font: var(--t-footnote); text-decoration: underline; cursor: pointer; }
	.link:hover { color: var(--text); } .probe, .form-error { margin-top: 12px; }
	.probe { margin-bottom: 0; display: flex; } .hits { list-style: none; margin: 0; padding: 0; max-height: 40vh; overflow-y: auto; }
	.hit { min-height: 52px; } .other-hits { margin: 0; }
	.none { margin: 4px 0 0; color: var(--text-muted); font: var(--t-footnote); } .linked { margin-top: 12px; border-top: 1px solid var(--line); padding-top: 8px; }
	.linked > .hint { margin: 0 0 4px; color: var(--text-muted); font: var(--t-footnote); } .also { display: flex; justify-content: flex-end; margin-top: 8px; }
	select.input { width: 260px; }
	/* First-run setup: the media-server screen's form (ConnectServer), one field style, labels centred. */
	/* Fills the card: the form right under the heading, Continue pinned at the foot like every setup step. */
	/* Desktop setup card: two columns, like Link your media servers. */
	@media (min-width: 1100px) {
		.setup-read .cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); column-gap: 40px; align-items: start; }
		.setup-read .brings, .setup-read .brings.busy { display: block; margin-top: 0; }
	}
	/* Two buttons on the field's right: Fetch key (asks the app) then Get key (opens the app's page). */
	@media (hover: hover) { .setup-read .key-get:hover { filter: brightness(1.08); } .setup-read .key-fetch:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.12); } }
	.addr-labels { position: relative; } .addr-labels .proto-label { position: absolute; inset-inline-start: 0; top: 0; width: 80px; text-align: center; }
	.addr-labels .port-label { position: absolute; inset-inline-end: 0; top: 0; width: 80px; text-align: center; } .addr-labels > label { display: block; }
	.addr-line { display: grid; grid-template-columns: 80px minmax(0, 1fr) 80px; gap: 14px; } .addr-row { position: relative; min-width: 0; }
	.setup-read .addr-line .port { text-align: center; padding: 0 8px; } .scheme.secure { border-color: rgb(52 199 89 / 0.5); }
	.setup-read .addr-row.is-select .input { appearance: none; -webkit-appearance: none; padding-inline-end: 92px; cursor: pointer; } .in-field { position: absolute; inset-inline-end: 6px; top: 50%; transform: translateY(-50%); display: flex; align-items: center; gap: 2px; pointer-events: none; }
	.in-field :global(button) { pointer-events: auto; } .chev { display: flex; width: 28px; justify-content: center; opacity: 0.75; }
	.sep { width: 1px; height: 20px; background: currentColor; opacity: 0.2; margin-inline: 2px; }
	.conn-ok { box-sizing: border-box; min-height: 56px; width: 100%; display: flex; align-items: center; gap: 12px; padding-block: 6px; padding-inline: 14px 8px; border-radius: var(--g-r, 14px); border: 1px solid var(--g-line, rgb(255 255 255 / 0.1)); background: var(--g-bg, rgb(255 255 255 / 0.06)); }
	.conn-tick { display: flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: rgb(52 199 89 / 0.18); color: rgb(52 199 89); flex: none; }
	.conn-text { flex: 1 1 auto; min-width: 0; text-align: start; } .conn-title { font: 600 15px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.conn-meta { font: 400 13px/1.3 var(--font-ui); opacity: 0.7; }
	.reconnect { flex: none; min-height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--accent-text, var(--accent)); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	@media (hover: hover) { .reconnect:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.08); } }
	.saved-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; text-align: start; cursor: pointer; } .saved-txt { display: flex; flex-direction: column; gap: 1px; }
	.saved-txt .hint { color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 600px) {
		select.input { width: min(260px, 46vw); }
		.setup-read .key-acts /* phone: key buttons drop under the field */ { position: static; margin-top: 6px; justify-content: center; } .setup-read .key-acts > * { min-height: 44px; } .setup-read .key-row .input { padding-inline-end: 12px !important; }
	}
</style>
