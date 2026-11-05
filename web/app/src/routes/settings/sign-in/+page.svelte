<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, date } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import type { ApiKey } from '#lib/types.ts';
	import { session } from '#lib/session.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import SsoCard from '#lib/signin/SsoCard.svelte';

	let skipLocal = $state(false);
	let loaded = $state(false);
	let error = $state('');
	let settingsOk = $state(false);
	let settingsLoading = $state(false);
	let savingSkip = $state(false);
	// Networks where sign-in may be skipped, one per line.
	let nets = $state('');
	let savingNets = $state(false);
	let hostNames = $state('');
	let savingHosts = $state(false);
	const netList = () => nets.split(/[\s,]+/).filter(Boolean);
	let savingRequire = $state(false);

	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	let fresh = $state<{ key: string; label: string } | null>(null);

	function strongNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? 'Sign in with your password to manage API keys.' : message(e);
	}

	interface Way {
		on: boolean;
		url?: string;
		server_id?: string;
		join: 'nobody' | 'list' | 'everyone';
		allow: string;
	}
	type WayName = 'plex' | 'jellyfin' | 'emby';
	interface SignInSettings {
		skip_login_local_network: boolean;
		local_networks?: string[];
		allowed_hosts?: string[];
		require_two_factor_admins: boolean;
		min_password_length: number;
		plex: Way;
		jellyfin: Way;
		emby: Way;
		plex_owner?: { linked: boolean; name: string; can_import: boolean };
	}
	const blank = (): Way => ({ on: false, url: '', server_id: '', join: 'nobody', allow: '' });
	let ways = $state<Record<WayName, Way>>({ plex: blank(), jellyfin: blank(), emby: blank() });
	let requireAdmins = $state(false);
	let minLength = $state(4);
	let savingMinLength = $state(false);
	let savingWay = $state<WayName | ''>('');
	const isOwner = $derived(session.s.role === 'owner');
	const wayWord: Record<WayName, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
	// The join rule as last saved: opening a way to everyone needs a confirm step.
	let savedJoin = $state<Record<WayName, Way['join']>>({ plex: 'nobody', jellyfin: 'nobody', emby: 'nobody' });
	let plexOwner = $state({ linked: false, name: '', can_import: false });

	// (a) Link my Plex account: a PIN the owner confirms on plex.tv themselves.
	let linking = $state(false);
	let linkNote = $state('');
	let plexServers = $state<{ id: string; name: string }[]>([]);
	type LinkDone = { name: string; servers: { id: string; name: string }[] };
	async function linkPlex() {
		linking = true;
		linkNote = 'Confirm on the Plex page that opened. This waits up to 10 minutes.';
		const win = window.open('about:blank', 'plex-link', 'width=600,height=720');
		// The Plex page must not reach back into this tab.
		if (win) win.opener = null;
		try {
			const r = await api.post<{ id: number; auth_url: string }>('/auth/plex/link/start', {});
			if (win) win.location.href = r.auth_url;
			else linkNote = 'Your browser blocked the Plex window. Allow pop-ups for HolyArrs and try again.';
			const until = Date.now() + 10 * 60 * 1000;
			while (Date.now() < until) {
				await new Promise((ok) => setTimeout(ok, 2000));
				let done: LinkDone | { waiting: true };
				try {
					done = await api.post<LinkDone | { waiting: true }>('/auth/plex/link/finish', { id: r.id });
				} catch (err) {
					if (err instanceof ApiError && err.status === 429) continue;
					throw err;
				}
				if ('waiting' in done) continue;
				plexServers = done.servers;
				plexOwner = { linked: true, name: done.name, can_import: true };
				if (!ways.plex.server_id && done.servers.length === 1) ways.plex.server_id = done.servers[0].id;
				linkNote = done.servers.length ? 'Linked. Pick the server people must share, then Save.' : 'Linked. Plex lists no server you own.';
				toasts.push(`Plex account ${done.name} linked.`, 'ok');
				return;
			}
			linkNote = 'The Plex sign-in timed out. Start again.';
		} catch (err) {
			linkNote = strongNote(err);
		} finally {
			linking = false;
			try {
				win?.close();
			} catch {
				// the window may already be gone
			}
		}
	}
	async function unlinkPlex() {
		if (!confirm('Unlink your Plex account? HolyArrs forgets the Plex key it kept for importing people.')) return;
		try {
			plexOwner = await api.del<{ linked: boolean; name: string; can_import: boolean }>('/auth/plex/link');
			plexServers = [];
			toasts.push('Plex account unlinked.', 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		}
	}

	// (b) Test a Jellyfin or Emby address from the server side.
	let testing = $state<WayName | ''>('');
	let tested = $state<Partial<Record<WayName, { ok: boolean; text: string }>>>({});
	async function testUrl(w: WayName) {
		testing = w;
		try {
			const r = await api.post<{ ok: boolean; name?: string; version?: string; product?: string; message?: string }>(`/auth/media-server/${w}/test`, { url: ways[w].url });
			tested[w] = r.ok ? { ok: true, text: `Found ${r.name || 'a server'}${r.version ? ` (${r.product || wayWord[w]} ${r.version})` : ''}.` } : { ok: false, text: r.message || 'No answer.' };
		} catch (err) {
			tested[w] = { ok: false, text: strongNote(err) };
		} finally {
			testing = '';
		}
	}

	// (c) Import members: a preview list, then the owner confirms.
	interface Candidate {
		id: string;
		name: string;
		email?: string;
		kid: boolean;
		server_admin: boolean;
		linked_to?: string;
	}
	let importFor = $state<WayName | ''>('');
	let importOpen = $state(false);
	let importName = $state('');
	let importPass = $state('');
	let importBusy = $state(false);
	let importNote = $state('');
	let candidates = $state<Candidate[] | null>(null);
	let picked = $state<Record<string, { on: boolean; kid: boolean }>>({});
	let lastCount = $state<Partial<Record<WayName, number>>>({});
	function openImport(w: WayName) {
		importFor = w;
		candidates = null;
		importNote = '';
		importName = '';
		importPass = '';
		importOpen = true;
		if (w === 'plex') void preview();
	}
	async function preview(e?: SubmitEvent) {
		e?.preventDefault();
		if (!importFor) return;
		const w = importFor;
		importBusy = true;
		importNote = '';
		try {
			const body = w === 'plex' ? {} : { name: importName, password: importPass };
			const r = await api.post<{ people: Candidate[] }>(`/auth/import/${w}/preview`, body);
			importPass = '';
			candidates = r.people;
			lastCount[w] = r.people.length;
			picked = Object.fromEntries(r.people.map((p) => [p.id, { on: !p.linked_to, kid: p.kid }]));
		} catch (err) {
			importNote = strongNote(err);
		} finally {
			importBusy = false;
		}
	}
	const pickedCount = $derived(candidates ? candidates.filter((p) => !p.linked_to && picked[p.id]?.on).length : 0);
	async function runImport() {
		if (!candidates || !importFor) return;
		const people = candidates.filter((p) => !p.linked_to && picked[p.id]?.on).map((p) => ({ id: p.id, kid: picked[p.id].kid }));
		importBusy = true;
		try {
			const r = await api.post<{ made: string[]; skipped: number }>(`/auth/import/${importFor}`, { people });
			toasts.push(r.made.length ? `Made ${r.made.length} ${r.made.length === 1 ? 'person' : 'people'}: ${r.made.join(', ')}.` : 'Nobody new to make.', 'ok');
			importOpen = false;
		} catch (err) {
			importNote = strongNote(err);
		} finally {
			importBusy = false;
		}
	}

	// (d) Opening a way to everyone asks first.
	let everyoneFor = $state<WayName | ''>('');
	let everyoneOpen = $state(false);
	function everyoneText(w: WayName): string {
		const n = lastCount[w];
		const who = w === 'plex' ? (ways.plex.server_id ? 'everyone your Plex server is shared with' : 'every Plex account') : `everyone with an account on your ${wayWord[w]} server`;
		return `This lets ${who} make a HolyArrs account the first time they sign in${n !== undefined ? ` (the last import list showed ${n} ${n === 1 ? 'person' : 'people'})` : ''}. They join as Members who may ask for titles.`;
	}

	function took(r: SignInSettings) {
		skipLocal = r.skip_login_local_network;
		nets = (r.local_networks ?? []).join('\n');
		hostNames = (r.allowed_hosts ?? []).join('\n');
		requireAdmins = r.require_two_factor_admins;
		minLength = r.min_password_length ?? 12;
		ways = { plex: { ...blank(), ...r.plex }, jellyfin: { ...blank(), ...r.jellyfin }, emby: { ...blank(), ...r.emby } };
		savedJoin = { plex: ways.plex.join, jellyfin: ways.jellyfin.join, emby: ways.emby.join };
		if (r.plex_owner) plexOwner = r.plex_owner;
	}

	async function saveWay(w: WayName, e?: SubmitEvent, confirmed = false) {
		e?.preventDefault();
		if (ways[w].join === 'everyone' && savedJoin[w] !== 'everyone' && !confirmed) {
			everyoneFor = w;
			everyoneOpen = true;
			return;
		}
		everyoneOpen = false;
		savingWay = w;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { [w]: ways[w], ...(confirmed ? { confirm_everyone: true } : {}) }));
			// An address never tested, or whose test failed: save anyway, test it now, warn when it does not answer.
			if (w !== 'plex' && ways[w].url && !tested[w]?.ok) {
				await testUrl(w);
				if (!tested[w]?.ok) {
					toasts.push(`${wayWord[w]} sign-in saved, but this address did not answer the test. People can't sign in with ${wayWord[w]} until it does.`, 'fail', { ms: 10000 });
					return;
				}
			}
			toasts.push(`${wayWord[w]} sign-in saved.`, 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingWay = '';
		}
	}

	async function setRequire(v: boolean) {
		if (savingRequire) return;
		const before = requireAdmins;
		requireAdmins = v;
		savingRequire = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { require_two_factor_admins: v }));
			toasts.push(v ? 'Admins must use two-factor sign-in.' : 'Two-factor sign-in is up to each admin.', 'ok');
		} catch (err) {
			requireAdmins = before;
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingRequire = false;
		}
	}

	async function saveMin(e: SubmitEvent) {
		e.preventDefault();
		savingMinLength = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { min_password_length: Number(minLength) }));
			session.s.minPassword = minLength;
			toasts.push(`New passwords need at least ${minLength} characters.`, 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingMinLength = false;
		}
	}

	async function loadSettings() {
		settingsLoading = true;
		error = '';
		try {
			took(await api.get<SignInSettings>('/auth/settings'));
			settingsOk = true;
		} catch (e) {
			error = message(e);
		} finally {
			settingsLoading = false;
		}
	}

	onMount(async () => {
		const [, k] = await Promise.allSettled([loadSettings(), api.get<{ keys: ApiKey[] }>('/auth/keys')]);
		if (k.status === 'fulfilled') keys = k.value.keys;
		else keysNote = strongNote(k.reason);
		loaded = true;
	});

	async function setSkip(v: boolean) {
		if (savingSkip) return;
		const before = skipLocal;
		skipLocal = v;
		savingSkip = true;
		try {
			// Turning it on saves the networks typed below with it.
			took(await api.put<SignInSettings>('/auth/settings', v ? { skip_login_local_network: v, local_networks: netList() } : { skip_login_local_network: v }));
			toasts.push(skipLocal ? 'No sign-in needed on your local network.' : 'Sign-in needed everywhere.', 'ok');
		} catch (e) {
			skipLocal = before;
			toasts.push(strongNote(e), 'fail');
		} finally {
			savingSkip = false;
		}
	}

	async function saveNets(e: SubmitEvent) {
		e.preventDefault();
		savingNets = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { local_networks: netList() }));
			toasts.push('Local networks saved.', 'ok');
		} catch (err) {
			toasts.push(strongNote(err), 'fail');
		} finally {
			savingNets = false;
		}
	}

	async function saveHosts(e: SubmitEvent) {
		e.preventDefault();
		savingHosts = true;
		try {
			const allowed_hosts = hostNames.split(/[\s,]+/).filter(Boolean);
			took(await api.put<SignInSettings>('/auth/settings', { allowed_hosts }));
			toasts.push('Host names saved.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			savingHosts = false;
		}
	}

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
			toasts.push(strongNote(err), 'fail');
		} finally {
			making = false;
		}
	}

	async function revoke(k: ApiKey) {
		if (!confirm(`Revoke the API key "${k.label}"? Anything using it stops working.`)) return;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			toasts.push('API key revoked.', 'ok');
		} catch (e) {
			toasts.push(strongNote(e), 'fail');
		}
	}

	async function copy() {
		if (!fresh) return;
		try {
			await navigator.clipboard.writeText(fresh.key);
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; select the key and copy it by hand.', 'fail');
		}
	}
</script>

<svelte:head><title>Sign-in and security · HolyArrs</title></svelte:head>

<PageHeader title="Sign-in and security" />

{#if !loaded}
	<SkSettings rows={3} />
{:else}
	{#if !settingsOk}
		<LoadError hasData={false} message={error} onretry={loadSettings} busy={settingsLoading} />
	{:else}
	<SettingsCard id="local-card" title="Local network">
			<label class="switch" style="margin-top: var(--s-3)">
				<input type="checkbox" checked={skipLocal} disabled={savingSkip} aria-busy={savingSkip} onchange={(e) => setSkip(e.currentTarget.checked)} />
				<span class="sw" aria-hidden="true"></span>
				<span>Skip sign-in on my local network</span>
			</label>
			<p class="hint">
				Devices on the networks below open HolyArrs as the owner, without a password. With family or friends on the same
				network, leave this off: they would get the owner's rights too.
			</p>
			<p class="hint">Changing keys and passwords still needs a real sign-in.</p>
			<form class="nets" onsubmit={saveNets}>
				<Field id="local-nets" label="Local networks, one per line" hint="Like 192.168.1.0/24, or one address like 192.168.1.20. Up to 32.">
					<textarea id="local-nets" class="input" rows="3" bind:value={nets} spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
				</Field>
				<Button type="submit" disabled={savingNets} aria-busy={savingNets}>Save networks</Button>
			</form>
	</SettingsCard>

	<SettingsCard id="twofactor-card" title="Two-factor sign-in">
		<label class="switch" style="margin-top: var(--s-3)">
			<input type="checkbox" checked={requireAdmins} disabled={!isOwner || savingRequire} aria-busy={savingRequire} onchange={(e) => setRequire(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Admins must use two-factor sign-in</span>
		</label>
		<p class="hint">
			An admin without it can open only My account until they turn it on. Anyone can turn it on for themselves in My account.{#if !isOwner}
				Only the owner can change this.{/if}
		</p>
	</SettingsCard>

	<SettingsCard id="passwords-card" title="Passwords">
		<form class="passwords" onsubmit={saveMin}>
			<fieldset disabled={!isOwner}>
				<Field id="min-pw" label="Shortest password allowed">
					<input id="min-pw" class="input" style="max-width: 120px" type="number" min="4" max="64" step="1" required bind:value={minLength} aria-describedby={Number(minLength) < 12 ? 'min-pw-hint min-pw-warn' : 'min-pw-hint'} />
					<span id="min-pw-hint" class="hint">4 to 64 characters. It counts for new passwords: first setup, invites, password changes and reset-owner. Passwords people already have keep working.</span>
				</Field>
				{#if Number(minLength) < 12}
					<p id="min-pw-warn" class="notice warn">
						Short passwords are easy to guess. HolyArrs slows down repeated wrong sign-ins, which helps, but it does not make a short
						password safe. If HolyArrs can be reached from outside your home, use 12 or more.
					</p>
				{/if}
				{#if isOwner}
					<Button variant="primary" type="submit" disabled={savingMinLength} aria-busy={savingMinLength}>Save</Button>
				{:else}
					<p class="hint">Only the owner can change this.</p>
				{/if}
			</fieldset>
		</form>
	</SettingsCard>

	{#each ['plex', 'jellyfin', 'emby'] as const as w (w)}
		<SettingsCard id="{w}-card" title={`Sign in with ${wayWord[w]}`}>
			<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
				{#if w === 'plex'}
					People sign in with their Plex account on Plex's own page; HolyArrs never sees their Plex password.
				{:else}
					People sign in with the name and password they use on your {wayWord[w]} server; HolyArrs checks them with the server and keeps neither.
				{/if}
			</p>
			<form class="way" onsubmit={(e) => saveWay(w, e)}>
				<fieldset disabled={!isOwner}>
					<label class="switch">
						<input type="checkbox" bind:checked={ways[w].on} />
						<span class="sw" aria-hidden="true"></span>
						<span>Show the {wayWord[w]} button on the sign-in page</span>
					</label>
					{#if w === 'plex'}
						<div class="link-row">
							{#if plexOwner.linked}
								<p class="small">Linked to the Plex account <b>{plexOwner.name}</b>.</p>
								{#if isOwner}<Button variant="plain" type="button" onclick={unlinkPlex}>Unlink</Button>{/if}
							{:else}
								<p class="small muted">Link your own Plex account to pick your server from a list and import the people you share it with.</p>
							{/if}
							{#if isOwner}
								<Button type="button" onclick={linkPlex} disabled={linking} aria-busy={linking}>{plexOwner.linked ? 'Link again' : 'Link my Plex account'}</Button>
							{/if}
						</div>
						{#if linkNote}<p class="hint" role="status">{linkNote}</p>{/if}
						{#if plexServers.length}
							<Field id="{w}-pick" label="Server people must share">
								<select id="{w}-pick" class="input" bind:value={ways[w].server_id}>
									<option value="">Any server (no share check)</option>
									{#each plexServers as sv (sv.id)}<option value={sv.id}>{sv.name}</option>{/each}
								</select>
							</Field>
						{/if}
						<details class="adv">
							<summary class="small">Advanced: type the server id</summary>
							<Field id="{w}-server" label="Server id people must share (optional)" hint="Your Plex server's machine identifier. When set, only people your server is shared with may join.">
								<input id="{w}-server" class="input mono" bind:value={ways[w].server_id} maxlength="200" spellcheck="false" />
							</Field>
						</details>
					{:else}
						<Field id="{w}-url" label="{wayWord[w]} server address">
							<div class="row">
								<input id="{w}-url" class="input mono grow" bind:value={ways[w].url} oninput={() => (tested[w] = undefined)} placeholder="http://192.168.1.10:8096" maxlength="500" spellcheck="false" />
								<Button type="button" onclick={() => testUrl(w)} disabled={!ways[w].url || testing === w} aria-busy={testing === w}>Test</Button>
							</div>
						</Field>
						{#if tested[w]?.ok}<p class="notice" role="status"><span class="ok-mark"><Icon name="check" size={17} /></span><span>{tested[w]?.text}</span></p>
						{:else if tested[w]}<p class="notice warn" role="status">{tested[w]?.text}</p>
						{:else if ways[w].url}<p class="notice warn" data-testid="{w}-untested">Not tested yet. Save runs the test; you can save even if it fails.</p>{/if}
					{/if}
					<Field id="{w}-join" label="Who may join the first time" hint="New people join as Members who may ask for titles. Someone the server limits (a Plex managed user, a parental rating) joins as a Kid, up to PG. Change them on Users and roles.">
						<select id="{w}-join" class="input" bind:value={ways[w].join}>
							<option value="nobody">Nobody new; only people already linked</option>
							<option value="list">Only the names or emails below</option>
							<option value="everyone">Everyone {w === 'plex' ? 'with a Plex account (who shares the server, when set)' : `with an account on the ${wayWord[w]} server`}</option>
						</select>
					</Field>
					{#if ways[w].join === 'list'}
						<Field id="{w}-allow" label="Names or emails, one per line">
							<textarea id="{w}-allow" class="input" rows="3" bind:value={ways[w].allow} maxlength="4000"></textarea>
						</Field>
					{/if}
					{#if isOwner}
						<div class="row">
							<Button variant="primary" type="submit" disabled={savingWay === w} aria-busy={savingWay === w}>Save</Button>
							<Button
								type="button"
								onclick={() => openImport(w)}
								disabled={w === 'plex' ? !plexOwner.can_import : !ways[w].url}
								title={w === 'plex' && !plexOwner.can_import ? 'Link your Plex account first' : undefined}>Import members</Button
							>
						</div>
					{/if}
				</fieldset>
			</form>
		</SettingsCard>
	{/each}
	{#if isOwner}<SsoCard id="sso-card" />{/if}
	{/if}

	{#if settingsOk}
	<SettingsCard id="hosts-card" title="Allowed host names">
		<p class="hint">
			Names you open HolyArrs by besides its IP address, like tower.local or media.example.com. The name you used at first-run
			setup is already here. Names in <code>HOLYARRS_ALLOWED_HOSTS</code> always work and are not listed.
		</p>
		<form class="nets" onsubmit={saveHosts}>
			<Field id="allowed-hosts" label="Host names, one per line">
				<textarea id="allowed-hosts" class="input" rows="3" bind:value={hostNames} spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
			</Field>
			<Button type="submit" disabled={savingHosts} aria-busy={savingHosts}>Save host names</Button>
		</form>
	</SettingsCard>
	{/if}

	<SettingsCard id="proxy-card" title="Trusted proxies">
		<p class="small muted" style="margin-top: var(--s-2)">
			Set when HolyArrs starts, not here: list your reverse proxy addresses in <code>HOLYARRS_TRUSTED_PROXIES</code> and extra host
			names in <code>HOLYARRS_ALLOWED_HOSTS</code>. Without a proxy, nothing is needed.
		</p>
	</SettingsCard>

	<SettingsCard id="keys-card" title="API keys">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">For scripts and other apps. Send the key in the <code>X-Api-Key</code> header, never in an address.</p>

		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			{#if fresh}
				<div class="fresh" role="status">
					<p class="small"><b>{fresh.label}</b>: copy this key now. It is shown only once.</p>
					<div class="row">
						<code class="key grow">{fresh.key}</code>
						<Button icon="copy" type="button" onclick={copy}>Copy</Button>
						<Button variant="plain" type="button" onclick={() => (fresh = null)}>Done</Button>
					</div>
				</div>
			{/if}

			<form class="row" onsubmit={makeKey}>
				<label class="sr-only" for="key-label">Key name</label>
				<input id="key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder="What uses this key?" maxlength="64" />
				<Button icon="key" variant="primary" type="submit" disabled={making || !label.trim()} aria-busy={making}>Make a key</Button>
			</form>

			{#if keys.length === 0}
				<p class="small muted" style="margin-top: var(--s-4)">No API keys yet.</p>
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
{/if}

<Sheet bind:open={importOpen} title={importFor ? `Import members from ${wayWord[importFor]}` : 'Import members'} size="md">
	{#if importFor && importFor !== 'plex' && !candidates}
		<form class="imp" onsubmit={preview}>
			<p class="small muted">Sign in once as an admin of your {wayWord[importFor]} server to list its people. HolyArrs ends that session right after and keeps neither the name nor the password.</p>
			<Field id="imp-name" label="Admin name"><input id="imp-name" class="input" bind:value={importName} autocomplete="off" required maxlength="128" /></Field>
			<Field id="imp-pass" label="Admin password"><input id="imp-pass" class="input" type="password" bind:value={importPass} autocomplete="off" maxlength="256" /></Field>
			<Button variant="primary" type="submit" disabled={importBusy || !importName} aria-busy={importBusy}>Show people</Button>
		</form>
	{:else if !candidates}
		{#if !importNote}<SkSettings rows={3} />{/if}
	{:else if candidates.length === 0}
		<p class="small muted">Nobody to import: the server lists no one else.</p>
	{:else}
		<p class="small muted">Nothing is made until you press Import. People join as Members who may ask for titles; Kid means asks only, rated PG or lower. Server admins do not become HolyArrs admins.</p>
		<ul class="rows imp-list">
			{#each candidates as p (p.id)}
				<li>
					<label class="checkline grow">
						<input type="checkbox" bind:checked={picked[p.id].on} disabled={!!p.linked_to} />
						<span>
							<span class="name">{p.name}</span>
							{#if p.email}<span class="xs muted"> {p.email}</span>{/if}
							{#if p.linked_to}<span class="xs muted"> · already {p.linked_to}</span>{/if}
							{#if p.server_admin}<span class="chip">server admin</span>{/if}
						</span>
					</label>
					<label class="checkline xs"><input type="checkbox" bind:checked={picked[p.id].kid} disabled={!!p.linked_to || !picked[p.id].on} /> Kid</label>
				</li>
			{/each}
		</ul>
	{/if}
	{#if importNote}<p class="notice warn" role="alert">{importNote}</p>{/if}
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (importOpen = false)}>Cancel</Button>
		{#if candidates && candidates.length}
			<Button variant="primary" type="button" onclick={runImport} disabled={importBusy || pickedCount === 0} aria-busy={importBusy}>Import {pickedCount}</Button>
		{/if}
	{/snippet}
</Sheet>

<Sheet bind:open={everyoneOpen} title="Let everyone join?" size="sm">
	{#if everyoneFor}<p class="small">{everyoneText(everyoneFor)}</p>{/if}
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (everyoneOpen = false)}>Cancel</Button>
		<Button variant="primary" type="button" onclick={() => everyoneFor && saveWay(everyoneFor, undefined, true)}>Let everyone join</Button>
	{/snippet}
</Sheet>

<style>
	.link-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-3);
	}
	.adv summary {
		cursor: pointer;
		color: var(--text-muted);
	}
	.imp {
		display: grid;
		gap: var(--s-3);
	}
	.imp :global(button) {
		justify-self: start;
	}
	.imp-list li {
		display: flex;
		align-items: center;
		gap: var(--s-3);
	}
	.imp-list .chip {
		margin-left: 6px;
	}
	.passwords fieldset {
		border: 0;
		padding: 0;
		margin: var(--s-3) 0 0;
		min-width: 0;
		display: grid;
		gap: var(--s-3);
	}
	.way fieldset {
		border: 0;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-3);
		max-width: 520px;
	}
	.nets {
		display: grid;
		gap: var(--s-3);
		margin-top: var(--s-3);
		max-width: 520px;
	}
	.way :global(button),
	.nets :global(button),
	.passwords :global(button) {
		justify-self: start;
	}
	textarea.input {
		height: auto;
		padding-top: 10px;
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
	.name {
		font-weight: var(--weight-semibold);
	}
	.ok-mark {
		display: inline-flex;
		flex: none;
		color: var(--ok-text);
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
