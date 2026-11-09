<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import MoreOptions from '#lib/ui/MoreOptions.svelte';
	import SettingsLinks from '#lib/ui/SettingsLinks.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SsoCard } from '#lib/accounts/index.ts';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	let skipLocal = $state(false);
	let loaded = $state(false);
	let error = $state('');
	let settingsOk = $state(false);
	let settingsLoading = $state(false);
	let savingSkip = $state(false);
	// Networks where sign-in may be skipped, one per line.
	let nets = $state('');
	let seenAddress = $state('');
	let proxyUntrusted = $state(false);
	let gatewayNets = $state<string[]>([]);
	let savingNets = $state(false);
	let hostNames = $state('');
	let savingHosts = $state(false);
	const netList = () => nets.split(/[\s,]+/).filter(Boolean);
	let savingRequire = $state(false);

	function strongNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? 'Sign in with your password to change this.' : message(e);
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
		seen_address?: string;
		gateway_networks?: string[];
		proxy_untrusted?: boolean;
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
			else linkNote = 'Plex window blocked. Allow pop-ups and try again.';
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
				// Plex's page cuts this tab's hold on its window, so it may stay open.
				linkNote = done.servers.length ? 'Linked. Close the Plex window, pick the server people must share, then Save.' : 'Linked. Close the Plex window; Plex lists no server you own.';
				toasts.push(`Plex ${done.name} linked`, 'ok');
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
				// The window may already be gone.
			}
		}
	}
	async function unlinkPlex() {
		if (!confirm('Unlink Plex? People import stops working.')) return;
		try {
			plexOwner = await api.del<{ linked: boolean; name: string; can_import: boolean }>('/auth/plex/link');
			plexServers = [];
			toasts.push('Plex unlinked', 'ok');
		} catch (err) {
			toasts.push("Couldn't unlink Plex", 'fail', { body: strongNote(err) });
		}
	}

	// (b) Test a Jellyfin or Emby address from the server side.
	let testing = $state<WayName | ''>('');
	let tested = $state<Partial<Record<WayName, { ok: boolean; text: string; app?: string }>>>({});
	async function testUrl(w: WayName) {
		testing = w;
		try {
			const r = await api.post<{ ok: boolean; name?: string; version?: string; product?: string; message?: string }>(`/auth/media-server/${w}/test`, { url: ways[w].url });
			// The version leads the tooltip, so a server name with numbers in it is never read as one.
			const app = r.product || wayWord[w];
			tested[w] = r.ok ? { ok: true, app, text: `${app}${r.version ? ` ${r.version}` : ''}${r.name ? ` (${r.name})` : ''}` } : { ok: false, text: r.message || 'No answer.' };
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
			toasts.push(r.made.length ? `${r.made.length} ${r.made.length === 1 ? 'person' : 'people'} made` : 'Nobody new to make', 'ok', r.made.length ? { body: r.made.join(', ') } : undefined);
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
		return `This lets ${who} make a HolyArrs account the first time they sign in${n !== undefined ? ` (the last import list showed ${n} ${n === 1 ? 'person' : 'people'})` : ''}. They join as Members who may request titles.`;
	}

	function took(r: SignInSettings) {
		skipLocal = r.skip_login_local_network;
		nets = (r.local_networks ?? []).join('\n');
		seenAddress = r.seen_address ?? '';
		gatewayNets = r.gateway_networks ?? [];
		proxyUntrusted = r.proxy_untrusted === true;
		hostNames = (r.allowed_hosts ?? []).join('\n');
		requireAdmins = r.require_two_factor_admins;
		minLength = r.min_password_length ?? 8;
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
					toasts.push(`${wayWord[w]} address unreachable`, 'fail', { body: "Saved, sign-in won't work yet", ms: 10000 });
					return;
				}
			}
			toasts.push(`${wayWord[w]} sign-in saved`, 'ok');
		} catch (err) {
			toasts.push("Couldn't save", 'fail', { body: strongNote(err) });
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
			toasts.push(v ? 'Two-factor required for admins' : 'Two-factor optional for admins', 'ok');
		} catch (err) {
			requireAdmins = before;
			toasts.push("Couldn't save", 'fail', { body: strongNote(err) });
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
			toasts.push('Minimum length saved', 'ok', { body: `${minLength} characters` });
		} catch (err) {
			toasts.push("Couldn't save", 'fail', { body: strongNote(err) });
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
		await loadSettings();
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
			toasts.push(skipLocal ? 'Local network skips sign-in' : 'Sign-in needed everywhere', 'ok');
		} catch (e) {
			skipLocal = before;
			toasts.push("Couldn't save", 'fail', { body: strongNote(e) });
		} finally {
			savingSkip = false;
		}
	}

	async function saveNets(e: SubmitEvent) {
		e.preventDefault();
		savingNets = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { local_networks: netList() }));
			toasts.push('Local networks saved', 'ok');
		} catch (err) {
			toasts.push("Couldn't save", 'fail', { body: strongNote(err) });
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
			toasts.push('Host names saved', 'ok');
		} catch (err) {
			toasts.push("Couldn't save", 'fail', { body: message(err) });
		} finally {
			savingHosts = false;
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
	<SettingsCard id="passwords-card" title="Passwords">
		<form class="passwords" onsubmit={saveMin}>
			<fieldset disabled={!isOwner}>
				<div class="srow">
					<div class="txt"><label class="label" for="min-pw">Shortest password allowed</label><span id="min-pw-hint" class="hint">4 to 64 characters, for new passwords only</span></div>
					<div class="ctl pw-ctl"><input id="min-pw" class="input" type="number" min="4" max="64" step="1" required bind:value={minLength} aria-describedby={Number(minLength) < 8 ? 'min-pw-hint min-pw-warn' : 'min-pw-hint'} /></div>
				</div>
				{#if Number(minLength) < 8}
					<p id="min-pw-warn" class="notice warn">
						Short passwords are easy to guess. HolyArrs slows down repeated wrong sign-ins, which helps, but it does not make a short
						password safe. If HolyArrs can be reached from outside your home, use 8 or more.
					</p>
				{/if}
				{#if isOwner}
					<div class="pw-save"><Button variant="primary" size="sm" type="submit" disabled={savingMinLength} aria-busy={savingMinLength}>Save</Button></div>
				{:else}
					<p class="hint">Owner only</p>
				{/if}
			</fieldset>
		</form>
	</SettingsCard>

	<SettingsCard id="twofactor-card" title="Two-factor sign-in">
		<label class="switch" style="margin-top: var(--s-3)">
			<input type="checkbox" checked={requireAdmins} disabled={!isOwner || savingRequire} aria-busy={savingRequire} onchange={(e) => setRequire(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>Admins must use two-factor sign-in</span>
		</label>
		<p class="hint">
			An admin without it can open only My account until they turn it on. Anyone can turn it on for themselves in My account.{#if !isOwner}{' '}Only the owner can change this.{/if}
		</p>
	</SettingsCard>

	{#each (['plex', 'jellyfin', 'emby'] as const).filter((x) => x !== 'emby' || !session.held('emby_signin')) as w (w)}
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
								<p class="small muted">Link Plex to pick your server and import its people</p>
							{/if}
							{#if isOwner}
								<Button type="button" onclick={linkPlex} disabled={linking} aria-busy={linking}>{plexOwner.linked ? 'Link again' : 'Link my Plex account'}</Button>
							{/if}
						</div>
						{#if linkNote}<p class="hint" role="status">{linkNote}</p>{/if}
						{#if plexServers.length}
							<Field id="{w}-pick" label="Server people must share">
								<select id="{w}-pick" class="input" bind:value={ways[w].server_id}>
									<option value="">Any server</option>
									{#each plexServers as sv (sv.id)}<option value={sv.id}>{sv.name}</option>{/each}
								</select>
							</Field>
						{/if}
						<details class="adv">
							<summary class="small">Type the server ID</summary>
							<Field id="{w}-server" label="Server id people must share (optional)" hint="Only people it's shared with can join">
								<input id="{w}-server" class="input mono" bind:value={ways[w].server_id} maxlength="200" spellcheck="false" />
							</Field>
						</details>
					{:else}
						<Field id="{w}-url" label="{wayWord[w]} server address">
							<div class="row">
								<input id="{w}-url" class="input mono grow" bind:value={ways[w].url} oninput={() => (tested[w] = undefined)} placeholder="http://10.0.0.5:8096" maxlength="500" spellcheck="false" />
								<Button type="button" onclick={() => testUrl(w)} disabled={!ways[w].url || testing === w} aria-busy={testing === w}>Test</Button>
							</div>
						</Field>
						{#if tested[w]}{@const t = tested[w]}<div class="testline" data-testid="{w}-tested"><TestTag ok={t.ok} text={t.text} app={t.app} />{#if !t.ok}<span class="small bad-text">{t.text}</span>{/if}</div>
						{:else if ways[w].url}<p class="notice warn" data-testid="{w}-untested">Not tested. Saving tests it.</p>{/if}
					{/if}
					<Field id="{w}-join" label="Who may join the first time" hint={w === 'plex' ? 'New people join as Members. Plex managed users join as Kid (up to PG).' : 'New people join as Members.'}>
						<select id="{w}-join" class="input" bind:value={ways[w].join}>
							<option value="nobody">Only people already linked</option>
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
			<p class="hint">Works only for direct visits. Using a proxy? Add it to trusted proxies.</p>
			<p class="hint">Keys and passwords still need a sign-in</p>
			<form class="nets" onsubmit={saveNets}>
				<Field id="local-nets" label="Local networks, one per line" hint="e.g. 192.168.1.0/24 or 192.168.1.20. Up to 32.">
					<textarea id="local-nets" class="input" rows="3" bind:value={nets} spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
				</Field>
				<Button type="submit" disabled={savingNets} aria-busy={savingNets}>Save</Button>
			</form>
			{#if seenAddress}<p class="hint" data-testid="seen-address">HolyArrs sees this browser as <span class="mono">{seenAddress}</span>.</p>{/if}
			{#if proxyUntrusted}<p class="hint" data-testid="proxy-untrusted">
					A proxy in front of HolyArrs is not trusted, so sign-in is never skipped through it. Add the proxy's address to
					<span class="mono">HOLYARRS_TRUSTED_PROXIES</span> in the container settings.
				</p>{/if}
			{#if gatewayNets.length}
				<p class="notice warn" role="status" data-testid="gateway-warn">
					<Icon name="info" size={18} /><span
						>{gatewayNets.join(', ')} {gatewayNets.length === 1 ? 'looks' : 'look'} like a Docker or Podman gateway address. If every device shows up as that address, anyone who can reach
						HolyArrs opens it as the owner. Check the address above from another device; if it is the same, turn off Skip sign-in, then remove this entry.</span
					>
				</p>
			{/if}
	</SettingsCard>

	{/if}

	<SettingsLinks label="API keys" links={[{ href: '/settings/api#keys-card', label: 'API keys', note: 'Make and revoke them on the API page' }]} />

	<MoreOptions>
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
				<Button type="submit" disabled={savingHosts} aria-busy={savingHosts}>Save</Button>
			</form>
		</SettingsCard>
		{/if}

		<SettingsCard id="proxy-card" title="Trusted proxies">
			<p class="small muted" style="margin-top: var(--s-2)">
				Set when HolyArrs starts, not here: list your reverse proxy addresses in <code>HOLYARRS_TRUSTED_PROXIES</code> and extra host
				names in <code>HOLYARRS_ALLOWED_HOSTS</code>. Without a proxy, nothing is needed.
			</p>
		</SettingsCard>
	</MoreOptions>
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
		<p class="small muted">No one to import</p>
	{:else}
		<p class="small muted">People join as Members. Kid is PG or lower. Server admins stay Members.</p>
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
	.testline {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px 10px;
	}
	.passwords fieldset { border: 0; margin: 0; padding: 0; min-width: 0; }
	.pw-ctl { max-width: 120px; }
	.pw-save { padding: 4px 0 14px; }
</style>
