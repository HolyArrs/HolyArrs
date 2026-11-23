<!-- First-run Connect server: the media server the owner signed in with, saved like Settings > Media servers saves it, then which of its libraries HolyArrs watches. Plex lists the owner's servers from the Plex account; Jellyfin and Emby start from the sign-in address. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import type { MediaServerView } from '#lib/mediaservers/index.ts';
	import { defaultPlexPick, plexAddress, plexFields, plexFormUrl, plexMayFallBack, plexShown, plexTries, type PlexChoice, type Library, type ServerKind } from './welcome-steps.ts';
	import { onMount, tick } from 'svelte';
	import { SigninServerForm } from '#lib/accounts/index.ts';
	import { session, type FoundServer } from '#lib/core/session.svelte.ts';

	type Choice = PlexChoice;

	let { kind, busy = $bindable(false), onnext }: { kind: ServerKind; busy?: boolean; onnext: (libs: Library[]) => void } = $props();
	const word = $derived({ plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' }[kind]);

	let server = $state<MediaServerView | null>(null);
	// A server of this kind left from an earlier link: reused on the next save instead of a second row.
	let spare: MediaServerView | null = null;
	let error = $state('');
	let checked = $state<{ ok: boolean; text: string } | null>(null);
	// The friendly summary under Check connection: server name, version, where it was reached, how fast.
	let info = $state<{ name: string; version: string; home: boolean; secure: boolean; ms: number } | null>(null);
	const homeIp = (h: string) => !h.includes('.') || /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.|169\.254\.)/.test(h) || /\.(local|lan|home|internal|home\.arpa)$/i.test(h);

	// Plex: the owner's servers from plex.tv, and the address picked or typed.
	let choices = $state<Choice[]>([]);
	let finding = $state(false);
	let pick = $state('manual');
	let host = $state('');
	let port = $state('32400');
	let ssl = $state(false);
	// The SSL box ticked by hand: no quiet fallback to plain http then.
	let sslByHand = $state(false);
	// A listed server's address in use (the fields show its plain IP).
	let listed = $state('');
	let webUrl = $state('');
	// Jellyfin and Emby: the address from sign-in and an API key.
	let address = $state('');
	let key = $state('');
	// The key field shows only when the sign-in's own key could not be used.
	let askKey = $state(false);

	let libs = $state<Library[]>([]);
	let syncing = $state(false);
	const anyOn = $derived(libs.some((l) => l.on));
	// A server with no movie, TV or music libraries can still be passed; a failed check cannot.
	const canGo = $derived(!!server && checked?.ok !== false && (anyOn || (!syncing && !libs.length)));

	const url = $derived(kind === 'plex' ? plexFormUrl({ pick, listed, host, port, ssl }) : address.trim() && !/^[a-z]+:\/\//i.test(address.trim()) ? `http://${address.trim()}` : address.trim());

	async function findServers(say = false) {
		if (finding) return;
		finding = true;
		try {
			choices = (await api.get<{ servers: Choice[] }>('/auth/plex/servers')).servers ?? [];
			if (say) toasts.push(choices.length ? t('welcome.server.found', { count: choices.length }) : t('welcome.server.noneFound'), choices.length ? 'ok' : 'info');
			// Nothing typed yet: take the first (local) one.
			// A saved address that is one of the listed ones shows as that row (its plain IP).
			const saved = server?.url ?? '';
			const at = saved ? choices.findIndex((c) => c.url === saved || c.secure_url === saved) : -1;
			if (at >= 0 && pick === 'manual') {
				pick = String(at);
				setAddress(saved);
			} else if (choices.length && !host.trim()) choose(defaultPlexPick(choices));
		} catch (err) {
			error = message(err);
		} finally {
			finding = false;
		}
	}
	function choose(v: string) {
		pick = v;
		checked = null;
		const c = choices[Number(v)];
		sslByHand = false;
		listed = '';
		if (v === 'manual' || !c) return;
		setAddress(plexTries(c)[0]);
	}
	function setAddress(u: string) {
		const c = choices[Number(pick)];
		if (pick !== 'manual' && c) {
			listed = u;
			({ host, port, ssl } = plexShown(c, u));
			return;
		}
		const f = plexFields(u);
		if (f) ({ host, port, ssl } = f);
	}
	// Typing in Host or Port: the address becomes exactly what is typed.
	function edited() {
		pick = 'manual';
		listed = '';
		checked = null;
	}
	// The SSL box on a listed server switches between its secure and plain address.
	function toggleSsl() {
		checked = null;
		const c = pick === 'manual' ? undefined : choices[Number(pick)];
		if (!c) return;
		// A remote server stays on its secure address.
		if (!c.local) return void (ssl = true);
		sslByHand = ssl;
		const want = ssl ? c.secure_url : c.url;
		if (want) setAddress(want);
		else ssl = !ssl;
	}
	// The protocol box: a listed home server switches between its plain and secure address; a typed one just flips.
	function flipScheme() {
		ssl = !ssl;
		if (pick === 'manual') return void (checked = null);
		toggleSsl();
	}
	// Picking a listed server checks it straight away; Enter it myself opens the Host box.
	function pickServer(v: string) {
		choose(v);
		if (v === 'manual') return void queueMicrotask(() => document.getElementById('cs-host')?.focus());
		void check();
	}
	// A library's total on its row, in the right plural form ("1,284 movies").
	const countText = (l: Library) => (l.count == null ? '' : t(`welcome.server.count.${l.kind}.${l.count === 1 ? 'one' : 'other'}`, { n: l.count.toLocaleString() }));

	// Jellyfin and Emby: one screen signs in to the server (which links the account and makes HolyArrs a key), then connects it and lists its libraries.
	let signAddr = $state('');
	let signName = $state('');
	let signPw = $state('');
	let signShow = $state(false);
	let signErr = $state('');
	let signFail = $state(0);
	let found = $state<FoundServer[]>([]);
	let looking = $state(false);
	// The form is drawn once the saved address and name are in, so it opens filled.
	let signReady = $state(false);
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;
	// Reconnect: Plex checks again; Jellyfin and Emby bring back Sign in and connect.
	async function reconnect() {
		if (kind === 'plex') return void check();
		checked = null;
		info = null;
		// The focused Reconnect is gone: focus the sign-in button that took its place.
		await tick();
		document.querySelector<HTMLElement>('[data-testid="welcome-connect"] .server button[type="submit"]')?.focus();
	}
	async function lookAround() {
		looking = true;
		found = (await session.setupFindServers()).filter((f) => f.kind === kind);
		looking = false;
	}
	async function signIn(e: SubmitEvent) {
		e.preventDefault();
		if (kind === 'plex' || busy) return;
		signErr = '';
		if (!signAddr.trim()) return void (signErr = t('accounts.login.enterAddress', { way: word }));
		if (!signName.trim()) return void (signErr = t('accounts.login.enterName', { way: word }));
		busy = true;
		try {
			const r = await api.post<{ url: string }>(`/auth/setup/link/${kind}`, { url: signAddr.trim(), name: signName.trim(), password: signPw });
			address = r.url || signAddr.trim();
		} catch (err) {
			signFail++;
			signErr = message(err);
			return;
		} finally {
			signPw = '';
			busy = false;
		}
		await check();
	}

	onMount(async () => {
		if (kind !== 'plex') {
			// Already linked to this server (Back, a reload): fill the address and name and connect again.
			try {
				const l = (await api.get<{ links: { provider: string; name: string }[] }>('/auth/media-link')).links?.find((x) => x.provider === kind);
				// Only a server linked now fills the form; a leftover saved address from an earlier link stays out.
				if (l) {
					signName = l.name;
					const st = await api.get<Record<string, { url?: string }>>('/auth/settings');
					signAddr = st[kind]?.url ?? '';
				}
			} catch {
				// Typed by hand then.
			}
			signReady = true;
		}
		try {
			const have = (await api.get<{ media_servers: MediaServerView[] }>('/media-servers')).media_servers ?? [];
			server = have.find((m) => m.kind === kind) ?? null;
		} catch {
			// None yet.
		}
		// Libraries show only once this account is linked to the server (Plex opens here only after its link).
		if (server && kind !== 'plex' && !signName) ((spare = server), (server = null));
		if (server) {
			fillFrom(server.url);
			webUrl = server.web_url ?? '';
			await loadLibraries();
		}
		// Connected only counts once this account is linked to the server too; otherwise the sign-in comes first.
		if (server && kind !== 'plex' && signName) await check();
		if (kind === 'plex') await findServers();
	});
	function fillFrom(u: string) {
		if (kind !== 'plex') return void (address = u);
		const f = plexFields(u);
		if (f) ({ host, port, ssl } = f);
		else host = u;
	}

	// Saves the server (new or changed), checks it answers, then lists its libraries.
	async function check(e?: SubmitEvent) {
		e?.preventDefault();
		if (busy) return;
		error = '';
		checked = null;
		if (!url) return void (error = t('accounts.login.enterAddress', { way: word }));
		if (kind !== 'plex' && !server && !key.trim() && askKey) return void (error = t('welcome.server.needKey'));
		busy = true;
		// A listed local Plex server: its plain IP address first, then its secure
		// address when the plain one can't be reached. Remote servers, a ticked SSL
		// box or a typed https address are tried as they are.
		const c = kind === 'plex' ? choices[Number(pick)] : undefined;
		const options = c ? plexTries(c) : [];
		const tries = c && c.local && !sslByHand && options[0] === url ? options : [url];
		let made = false;
		try {
			let used = '';
			for (const [i, u] of tries.entries()) {
				const body = { name: word, kind, url: u, token: kind === 'plex' ? '' : key.trim(), use_plex_sign_in: kind === 'plex' || !key.trim(), web_url: webUrl.trim() };
				try {
					if (server) server = await api.put<MediaServerView>(`/media-servers/${server.id}`, body);
					// A server of this kind saved earlier (an older link) is reused, never added twice.
					else if (spare) server = await api.put<MediaServerView>(`/media-servers/${spare.id}`, body);
					else {
						server = await api.post<MediaServerView>('/media-servers', body);
						made = true;
					}
					const t0 = performance.now();
					const res = await api.post<{ found?: string }>(`/media-servers/${server.id}/test`);
					const ms = Math.round(performance.now() - t0);
					const parts = (res?.found ?? '').trim().split(/\s+/);
					const version = parts.length > 1 && /^\d/.test(parts[parts.length - 1]) ? parts.pop()! : '';
					let h = '';
					try { h = new URL(u).hostname; } catch { /* kept empty */ }
					info = { name: parts.join(' ') || word, version, home: homeIp(h), secure: u.startsWith('https:'), ms };
					used = u;
					break;
				} catch (err) {
					if (i === tries.length - 1 || !plexMayFallBack(message(err))) throw err;
				}
			}
			if (kind === 'plex') setAddress(used);
			checked = { ok: true, text: `${t('welcome.server.connected')} · ${used.startsWith('https:') ? 'https' : 'http'}` };
			key = '';
			await loadLibraries();
			// The focused check button was replaced by the summary: keep focus on the step.
			await tick();
			if (document.activeElement === document.body) document.querySelector<HTMLElement>('[data-testid="connect-reconnect"]')?.focus();
		} catch (err) {
			checked = { ok: false, text: message(err) };
			// The key box shows only when the key made at sign-in was refused, not for any other failure.
			if (kind !== 'plex' && !key.trim() && /(401|403)|key|token|unauthori[sz]ed|forbidden/i.test(message(err))) askKey = true;
			// A new server that does not answer is taken back, so a reload starts clean.
			if (made && server) {
				await api.del(`/media-servers/${server.id}`).catch(() => {});
				server = null;
			}
		} finally {
			busy = false;
		}
	}

	async function loadLibraries(say = false) {
		if (!server) return;
		syncing = true;
		error = '';
		try {
			const got = (await api.get<{ libraries: Library[] }>(`/media-servers/${server.id}/libraries`)).libraries ?? [];
			// None picked yet: every library starts on.
			const none = !got.some((l) => l.on);
			const before = new Map(libs.map((l) => [l.key, l.on]));
			libs = got.map((l) => ({ ...l, on: before.get(l.key) ?? (none ? true : l.on) }));
			if (say) toasts.push(t('welcome.server.synced', { count: libs.length }), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			syncing = false;
		}
	}

	async function next() {
		if (!server || !canGo || busy) return;
		busy = true;
		error = '';
		try {
			await api.put(`/media-servers/${server.id}/libraries`, { keys: libs.filter((l) => l.on).map((l) => l.key), scan: true });
			onnext(libs.filter((l) => l.on));
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<div class="card connect" data-testid="welcome-connect">
	<!-- Connected: the status takes the main button's place, at its size, with Reconnect beside it. -->
	{#snippet summary()}
		{#if checked?.ok && info}
			<div class="conn-ok" role="status" data-testid="connect-summary">
				<span class="conn-tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span>
				<div class="conn-text">
					<div class="conn-title">{t('welcome.server.connectedTo', { name: found.find((f) => f.name.toLowerCase() === info?.name.toLowerCase())?.name ?? info.name })}</div>
					<div class="conn-meta">{info.version ? `${word} ${info.version}` : word}</div>
				</div>
				<button type="button" class="reconnect" disabled={busy} onclick={reconnect} data-testid="connect-reconnect">{kind === 'plex' ? t('welcome.server.reconnect') : t('accounts.account.change')}</button>
			</div>
		{/if}
	{/snippet}
	{#if kind !== 'plex'}
		<div class="server">
			{#if signReady}<SigninServerForm kind={kind === 'emby' ? 'emby' : 'jellyfin'} bind:address={signAddr} bind:name={signName} bind:pw={signPw} bind:showPw={signShow} foundHere={found} {looking} onrefresh={lookAround} error={signErr} waiting={0} {busy} failN={signFail} {wayWord} submitServer={signIn} focusStep={() => {}} submitLabel={t('welcome.server.signInConnect')} pair done={checked?.ok && info ? summary : undefined} />{/if}
			{#if askKey}
				<form onsubmit={check} class="keyform">
					<div class="field">
						<label for="cs-key">{t('settings.mediaServers.apiKeyLabel')}</label>
						<input id="cs-key" class="input mono" type="password" bind:value={key} autocomplete="new-password" placeholder={server ? '********' : ''} />
						<span class="hint">{t('welcome.server.keyHint')}</span>
					</div>
					<div class="checkrow"><Button type="submit" disabled={busy || !url} {busy} data-testid="connect-check">{t('welcome.server.check')}</Button></div>
				</form>
			{/if}
			{#if checked && !checked.ok}<p class="form-error" role="alert">{checked.text}</p>{/if}
		</div>
	{:else}
	<form onsubmit={check} class="server">
		{#if kind === 'plex'}
			<!-- One row like the Jellyfin sign-in: protocol | the found servers (Enter it myself on top) with refresh inside | port. -->
			<div class="field">
				<div class="addr-labels"><label for="cs-scheme" class="proto-label">{t('welcome.server.protocol')}</label><label for={pick === 'manual' ? 'cs-host' : 'cs-pick'}>{t('welcome.server.ipOrHostPlain')}</label><label for="cs-port" class="port-label">{t('welcome.server.port')}</label></div>
				<div class="addr-line">
					<button id="cs-scheme" type="button" class="input scheme" class:secure={ssl} onclick={flipScheme} disabled={busy} aria-label={t('welcome.server.schemeFlip', { scheme: ssl ? 'https' : 'http' })} data-testid="plex-scheme">{ssl ? 'https' : 'http'}://</button>
					<div class="addr-row" class:is-select={choices.length && pick !== 'manual'}>
						{#if choices.length && pick !== 'manual'}
							<select id="cs-pick" class="input" value={pick} onchange={(e) => pickServer(e.currentTarget.value)} disabled={busy} data-testid="plex-server-pick">
								<option value="manual">{t('welcome.server.manual')}</option>
								{#each choices as c, i (i)}<option value={String(i)}>{c.address}</option>{/each}
							</select>
						{:else}
							<input id="cs-host" class="input mono" dir="ltr" bind:value={host} oninput={edited} placeholder="192.168.1.10" autocomplete="off" spellcheck="false" />
						{/if}
						<span class="in-field">{#if choices.length && pick !== 'manual'}<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span><span class="sep" aria-hidden="true"></span>{/if}<IconButton icon="refresh" variant="tinted" label={t('welcome.server.refresh')} onclick={() => { pick = choices.length ? pick : 'manual'; void findServers(true); }} disabled={finding || busy} aria-busy={finding} data-testid="plex-server-refresh" /></span>
					</div>
					<input id="cs-port" class="input port" dir="ltr" inputmode="numeric" bind:value={port} oninput={edited} autocomplete="off" disabled={busy} />
				</div>
			</div>
			{#if pick !== 'manual' && listed.startsWith('https:')}<p class="hint secure" data-testid="plex-secure-note"><Icon name="shield" size={14} />{t('welcome.server.secureNote')}</p>{/if}
		{/if}
		<div class="checkrow">
			{#if checked?.ok && info}{@render summary()}{:else}<Button type="submit" disabled={busy || !url} {busy} data-testid="connect-check">{t('welcome.server.check')}</Button>{/if}
		</div>
		{#if checked && !checked.ok}<p class="form-error" role="alert">{checked.text}</p>{/if}
	</form>
	{/if}

		<section class="libs" aria-labelledby="cs-libs" aria-busy={syncing}>
			<div class="libhead">
				<h2 id="cs-libs">{t('welcome.server.libraries')}</h2>
				<IconButton icon="refresh" variant="tinted" label={t('welcome.server.sync')} onclick={() => loadLibraries(true)} disabled={syncing || busy || !server} aria-busy={syncing} data-testid="libraries-sync" />
			</div>
			{#if libs.length}
				<ul class="liblist">
					{#each libs as l (l.key)}
						<li>
							<label class="librow">
								<Icon name={l.kind === 'show' ? 'tv' : l.kind === 'music' ? 'music' : 'film'} size={18} />
								<span class="libname">{l.title}{#if l.count != null}<span class="libcount">&nbsp;· {countText(l)}</span>{/if}</span>
								<span class="switch"><input type="checkbox" bind:checked={l.on} disabled={busy} /><span class="sw" aria-hidden="true"></span></span>
							</label>
						</li>
					{/each}
				</ul>
				<p class="hint">{t('welcome.server.scanNote')}</p>
			{:else if !server || checked?.ok === false}
				<p class="hint libs-empty">{t('welcome.server.signInForLibs')}</p>
			{:else if !syncing}
				<p class="hint">{t('welcome.server.noLibraries')}</p>
			{:else}
				<p class="hint" role="status">{t('accounts.account.loading')}</p>
			{/if}
		</section>

	{#if error}<p class="form-error" role="alert">{error}</p>{:else if server && libs.length && !anyOn}<p class="form-error" role="alert">{t('welcome.server.pickOne')}</p>{/if}
	<Button variant="primary" size="lg" full type="button" disabled={busy || !canGo} {busy} onclick={next} data-testid="connect-continue">{t('welcome.continue')}</Button>
</div>

<style>
	.server { display: grid; gap: var(--s-4); }
	.secure { display: flex; align-items: center; gap: 6px; margin: calc(var(--s-4) * -0.5) 0 0; }
	.checkrow { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
	.libs { margin-top: var(--s-5); }
	.libhead { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 8px; }
	.libhead h2 { font: var(--t-headline); margin: 0; }
	.liblist { list-style: none; margin: 0 0 8px; padding: 0; display: grid; gap: 0; border: 1px solid var(--line); border-radius: var(--r-card); overflow: hidden; }
	.liblist > li + li { border-top: 1px solid var(--g-line, var(--line)); }
	.librow { display: flex; align-items: center; gap: 10px; min-height: 52px; padding: 0 14px; cursor: pointer; }
	.libname { flex: 1 1 auto; min-width: 0; overflow-wrap: anywhere; }
	.librow .switch { margin: 0; flex: none; }
	.connect > :global(.ui-btn:last-child) { margin-top: var(--s-5); }
	/* The app-wide .field + .field gap would drop the Port label below Host: fields here sit on the grid gap only. */
	.field { min-width: 0; margin: 0; }
	.server :global(.ui-btn), .libhead :global(.ui-btn) { min-height: var(--g-h, 44px); }
	/* Desktop setup card: its own sub-screen, full width in two columns (server on the left, libraries on the right), right under the lead, with no scrolling. */
	@media (min-width: 1100px) {
		:global(.sheet.glass) > .card.connect { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); column-gap: 40px; align-content: start; padding-top: 26px; overflow-x: hidden; }
		.connect > * { grid-column: 1 / -1; }
		.connect.connect > .server { grid-column: 1 / 2; grid-row: 1; align-self: start; min-width: 0; --gap-field: 20px; --gap: 24px; }
		.connect.connect > .libs { grid-column: 2 / 3; grid-row: 1; margin-top: 0; align-self: start; }
		/* The Libraries heading sits on the same line as the Protocol / IP / Port labels, and the list starts level with the address row. */
		.connect > .libs .libhead { height: 20px; margin-bottom: 8px; }
		/* The label row is 20 px tall: the refresh button sits in it at 28 px, so it never reaches the list below. */
		.connect > .libs .libhead :global(button) { width: 28px; height: 28px; min-height: 28px; min-width: 28px; }
		/* Check connection spans the left column, like the address row above it. */
		.connect .checkrow { display: flex; flex-direction: column; align-items: stretch; gap: 10px; margin-top: 20px; }
		.connect .checkrow :global(.ui-btn) { width: 100%; }
		.connect > .libs .libhead h2 { font: 600 15px/20px var(--font-ui); }
		/* Small only for a mouse; touch tablets in this width keep the 44 px button. */
		/* A long library list scrolls inside its column, so the card never scrolls and Continue stays in view. */
		.connect > .libs .liblist { max-height: clamp(208px, 38vh, 420px); overflow-y: auto; }
		.connect > :global(.ui-btn:last-child) { margin-top: var(--s-5); }
	}
	.libcount { color: var(--text-muted); font-weight: 400; }
	/* The address row, as on the Jellyfin sign-in: protocol and port boxes 80 px, labels centred over their boxes. */
	.field { text-align: center; }
	.field .input { text-align: start; }
	.addr-labels { position: relative; }
	.addr-labels .proto-label { position: absolute; inset-inline-start: 0; top: 0; width: 80px; text-align: center; }
	.addr-labels .port-label { position: absolute; inset-inline-end: 0; top: 0; width: 80px; text-align: center; }
	.addr-labels > label { display: block; }
	.addr-line { display: grid; grid-template-columns: 80px minmax(0, 1fr) 80px; gap: 14px; }
	.addr-row { position: relative; min-width: 0; }
	.addr-row .input { width: 100%; box-sizing: border-box; padding-inline-end: 52px; }
	/* As on the Jellyfin and Emby sign-in: the protocol sits centred in its box. */
	.addr-line .input.scheme { width: 80px; min-width: 0; padding: 0; display: flex; align-items: center; justify-content: center; text-align: center; font-weight: 400; cursor: pointer; }
	.addr-line .port { width: 100%; box-sizing: border-box; text-align: center; padding: 0 8px; }
	.scheme.secure { border-color: rgb(52 199 89 / 0.5); }
	.addr-row.is-select .input { appearance: none; -webkit-appearance: none; padding-inline-end: 92px; cursor: pointer; }
	.in-field { position: absolute; inset-inline-end: 6px; top: 50%; transform: translateY(-50%); display: flex; align-items: center; gap: 2px; pointer-events: none; }
	.in-field :global(button) { pointer-events: auto; }
	.chev { display: flex; width: 28px; justify-content: center; opacity: 0.75; }
	.sep { width: 1px; height: 20px; background: currentColor; opacity: 0.2; margin-inline: 2px; }
	.conn-ok { box-sizing: border-box; min-height: 56px; width: 100%; display: flex; align-items: center; gap: 12px; padding-block: 6px; padding-inline: 14px 8px; border-radius: var(--g-r, 14px); border: 1px solid var(--g-line, rgb(255 255 255 / 0.1)); background: var(--g-bg, rgb(255 255 255 / 0.06)); }
	.conn-tick { display: flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; background: rgb(52 199 89 / 0.18); color: rgb(52 199 89); flex: none; }
	:global(.light) .conn-tick { color: rgb(21 128 61); }
	.conn-title { font: 600 15px/1.3 var(--font-ui); }
	.conn-meta { margin-top: 0; font: 400 13px/1.3 var(--font-ui); opacity: 0.7; }
	.libs-empty { display: grid; place-items: center; min-height: 160px; margin: 0; border: 1px dashed var(--g-line, var(--line)); border-radius: var(--r-card); text-align: center; }
	.keyform { margin-top: 20px; }
	.conn-text { flex: 1 1 auto; min-width: 0; text-align: start; }
	.conn-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.reconnect { flex: none; min-height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--accent-text, var(--accent)); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.reconnect:disabled { cursor: not-allowed; }
	@media (max-width: 640px) { .addr-line { grid-template-columns: 64px minmax(0, 1fr) 64px; gap: 8px; } .addr-labels .proto-label, .addr-labels .port-label { width: 64px; } .addr-line .input.scheme { width: 64px; } }
	@media (hover: hover) { .reconnect:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.08); } :global(.light) .reconnect:hover:not(:disabled) { background: rgb(0 0 0 / 0.06); } }
	/* Continue keeps one spot at the card's foot, whatever the columns hold. */
	@media (min-width: 1100px) {
		:global(.sheet.glass) > .card.connect { grid-template-rows: auto minmax(0, 1fr) auto; }
		.connect.connect > :global(.ui-btn:last-child) { grid-row: 3; align-self: end; margin-top: 16px; }
		.connect.connect > .form-error { grid-row: 2; align-self: end; }
	}
</style>
