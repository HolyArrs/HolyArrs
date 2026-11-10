<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
		return e instanceof ApiError && e.status === 403 ? t('settings.signIn.signInToChange') : message(e);
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
		linkNote = t('settings.signIn.confirmPlex');
		const win = window.open('about:blank', 'plex-link', 'width=600,height=720');
		// The Plex page must not reach back into this tab.
		if (win) win.opener = null;
		try {
			const r = await api.post<{ id: number; auth_url: string }>('/auth/plex/link/start', {});
			if (win) win.location.href = r.auth_url;
			else linkNote = t('settings.signIn.blocked');
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
				linkNote = done.servers.length ? t('settings.signIn.linkedPick') : t('settings.signIn.linkedNone');
				toasts.push(t('settings.signIn.plexLinked', { name: done.name }), 'ok');
				return;
			}
			linkNote = t('settings.signIn.timedOut');
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
		if (!confirm(t('settings.signIn.confirmUnlink'))) return;
		try {
			plexOwner = await api.del<{ linked: boolean; name: string; can_import: boolean }>('/auth/plex/link');
			plexServers = [];
			toasts.push(t('settings.signIn.plexUnlinked'), 'ok');
		} catch (err) {
			toasts.push(t('settings.signIn.unlinkFailed'), 'fail', { body: strongNote(err) });
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
			tested[w] = r.ok ? { ok: true, app, text: `${app}${r.version ? ` ${r.version}` : ''}${r.name ? ` (${r.name})` : ''}` } : { ok: false, text: r.message || t('settings.signIn.noAnswer') };
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
			toasts.push(r.made.length ? t(r.made.length === 1 ? 'settings.signIn.madeOne' : 'settings.signIn.madeOther', { n: r.made.length }) : t('settings.signIn.nobodyNew'), 'ok', r.made.length ? { body: r.made.join(', ') } : undefined);
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
		const who = w === 'plex' ? (ways.plex.server_id ? t('settings.signIn.whoShared') : t('settings.signIn.whoEvery')) : t('settings.signIn.whoServer', { app: wayWord[w] });
		const count = n !== undefined ? ' ' + t('settings.signIn.lastList', { people: t(n === 1 ? 'settings.signIn.personOne' : 'settings.signIn.peopleOther', { n }) }) : '';
		return t('settings.signIn.everyoneText', { who, count });
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
					toasts.push(t('settings.signIn.unreachable', { app: wayWord[w] }), 'fail', { body: t('settings.signIn.savedNotWorking'), ms: 10000 });
					return;
				}
			}
			toasts.push(t('settings.signIn.waySaved', { app: wayWord[w] }), 'ok');
		} catch (err) {
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(err) });
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
			toasts.push(v ? t('settings.signIn.tfRequired') : t('settings.signIn.tfOptional'), 'ok');
		} catch (err) {
			requireAdmins = before;
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(err) });
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
			toasts.push(t('settings.signIn.minSaved'), 'ok', { body: t('settings.signIn.characters', { n: minLength }) });
		} catch (err) {
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(err) });
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
			toasts.push(skipLocal ? t('settings.signIn.skips') : t('settings.signIn.neededEverywhere'), 'ok');
		} catch (e) {
			skipLocal = before;
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(e) });
		} finally {
			savingSkip = false;
		}
	}

	async function saveNets(e: SubmitEvent) {
		e.preventDefault();
		savingNets = true;
		try {
			took(await api.put<SignInSettings>('/auth/settings', { local_networks: netList() }));
			toasts.push(t('settings.signIn.netsSaved'), 'ok');
		} catch (err) {
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(err) });
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
			toasts.push(t('settings.signIn.hostsSaved'), 'ok');
		} catch (err) {
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: message(err) });
		} finally {
			savingHosts = false;
		}
	}

</script>

<svelte:head><title>{t('settings.signIn.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.signIn.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else}
	{#if !settingsOk}
		<LoadError hasData={false} message={error} onretry={loadSettings} busy={settingsLoading} />
	{:else}
	<SettingsCard id="passwords-card" title={t('settings.signIn.passwords')}>
		<form class="passwords" onsubmit={saveMin}>
			<fieldset disabled={!isOwner}>
				<div class="srow">
					<div class="txt"><label class="label" for="min-pw">{t('settings.signIn.shortest')}</label><span id="min-pw-hint" class="hint">{t('settings.signIn.shortestHint')}</span></div>
					<div class="ctl pw-ctl"><input id="min-pw" class="input" type="number" min="4" max="64" step="1" required bind:value={minLength} aria-describedby={Number(minLength) < 8 ? 'min-pw-hint min-pw-warn' : 'min-pw-hint'} /></div>
				</div>
				{#if Number(minLength) < 8}
					<p id="min-pw-warn" class="notice warn">
						{t('settings.signIn.shortWarn')}
					</p>
				{/if}
				{#if isOwner}
					<div class="pw-save"><Button variant="primary" size="sm" type="submit" disabled={savingMinLength} aria-busy={savingMinLength}>{t('settings.signIn.save')}</Button></div>
				{:else}
					<p class="hint">{t('settings.signIn.ownerOnly')}</p>
				{/if}
			</fieldset>
		</form>
	</SettingsCard>

	<SettingsCard id="twofactor-card" title={t('settings.signIn.twoFactor')}>
		<label class="switch" style="margin-top: var(--s-3)">
			<input type="checkbox" checked={requireAdmins} disabled={!isOwner || savingRequire} aria-busy={savingRequire} onchange={(e) => setRequire(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>{t('settings.signIn.adminsMust')}</span>
		</label>
		<p class="hint">
			{t('settings.signIn.adminsNote')}{#if !isOwner}{' '}{t('settings.signIn.onlyOwnerChange')}{/if}
		</p>
	</SettingsCard>

	{#each (['plex', 'jellyfin', 'emby'] as const).filter((x) => x !== 'emby' || !session.held('emby_signin')) as w (w)}
		<SettingsCard id="{w}-card" title={t('settings.signIn.signInWith', { app: wayWord[w] })}>
			<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
				{#if w === 'plex'}
					{t('settings.signIn.plexIntro')}
				{:else}
					{t('settings.signIn.otherIntro', { app: wayWord[w] })}
				{/if}
			</p>
			<form class="way" onsubmit={(e) => saveWay(w, e)}>
				<fieldset disabled={!isOwner}>
					<label class="switch">
						<input type="checkbox" bind:checked={ways[w].on} />
						<span class="sw" aria-hidden="true"></span>
						<span>{t('settings.signIn.showButton', { app: wayWord[w] })}</span>
					</label>
					{#if w === 'plex'}
						<div class="link-row">
							{#if plexOwner.linked}
								<p class="small"><Words key="settings.signIn.linkedTo" vars={{ name: plexOwner.name }} /></p>
								{#if isOwner}<Button variant="plain" type="button" onclick={unlinkPlex}>{t('settings.signIn.unlink')}</Button>{/if}
							{:else}
								<p class="small muted">{t('settings.signIn.linkNote')}</p>
							{/if}
							{#if isOwner}
								<Button type="button" onclick={linkPlex} disabled={linking} aria-busy={linking}>{plexOwner.linked ? t('settings.signIn.linkAgain') : t('settings.signIn.linkMine')}</Button>
							{/if}
						</div>
						{#if linkNote}<p class="hint" role="status">{linkNote}</p>{/if}
						{#if plexServers.length}
							<Field id="{w}-pick" label={t('settings.signIn.serverShare')}>
								<select id="{w}-pick" class="input" bind:value={ways[w].server_id}>
									<option value="">{t('settings.signIn.anyServer')}</option>
									{#each plexServers as sv (sv.id)}<option value={sv.id}>{sv.name}</option>{/each}
								</select>
							</Field>
						{/if}
						<details class="adv">
							<summary class="small">{t('settings.signIn.typeId')}</summary>
							<Field id="{w}-server" label={t('settings.signIn.serverId')} hint={t('settings.signIn.serverIdHint')}>
								<input id="{w}-server" class="input mono" bind:value={ways[w].server_id} maxlength="200" spellcheck="false" />
							</Field>
						</details>
					{:else}
						<Field id="{w}-url" label={t('settings.signIn.serverAddress', { app: wayWord[w] })}>
							<div class="row">
								<input id="{w}-url" class="input mono grow" bind:value={ways[w].url} oninput={() => (tested[w] = undefined)} placeholder="http://10.0.0.5:8096" maxlength="500" spellcheck="false" />
								<Button type="button" onclick={() => testUrl(w)} disabled={!ways[w].url || testing === w} aria-busy={testing === w}>{t('settings.signIn.test')}</Button>
							</div>
						</Field>
						{#if tested[w]}{@const res = tested[w]}<div class="testline" data-testid="{w}-tested"><TestTag ok={res.ok} text={res.text} app={res.app} />{#if !res.ok}<span class="small bad-text">{res.text}</span>{/if}</div>
						{:else if ways[w].url}<p class="notice warn" data-testid="{w}-untested">{t('settings.signIn.notTested')}</p>{/if}
					{/if}
					<Field id="{w}-join" label={t('settings.signIn.whoJoin')} hint={w === 'plex' ? t('settings.signIn.joinHintPlex') : t('settings.signIn.joinHint')}>
						<select id="{w}-join" class="input" bind:value={ways[w].join}>
							<option value="nobody">{t('settings.signIn.joinNobody')}</option>
							<option value="list">{t('settings.signIn.joinList')}</option>
							<option value="everyone">{w === 'plex' ? t('settings.signIn.joinEveryonePlex') : t('settings.signIn.joinEveryone', { app: wayWord[w] })}</option>
						</select>
					</Field>
					{#if ways[w].join === 'list'}
						<Field id="{w}-allow" label={t('settings.signIn.names')}>
							<textarea id="{w}-allow" class="input" rows="3" bind:value={ways[w].allow} maxlength="4000"></textarea>
						</Field>
					{/if}
					{#if isOwner}
						<div class="row">
							<Button variant="primary" type="submit" disabled={savingWay === w} aria-busy={savingWay === w}>{t('settings.signIn.save')}</Button>
							<Button
								type="button"
								onclick={() => openImport(w)}
								disabled={w === 'plex' ? !plexOwner.can_import : !ways[w].url}
								title={w === 'plex' && !plexOwner.can_import ? t('settings.signIn.linkFirst') : undefined}>{t('settings.signIn.importMembers')}</Button
							>
						</div>
					{/if}
				</fieldset>
			</form>
		</SettingsCard>
	{/each}
	{#if isOwner}<SsoCard id="sso-card" />{/if}
	<SettingsCard id="local-card" title={t('settings.signIn.local')}>
			<label class="switch" style="margin-top: var(--s-3)">
				<input type="checkbox" checked={skipLocal} disabled={savingSkip} aria-busy={savingSkip} onchange={(e) => setSkip(e.currentTarget.checked)} />
				<span class="sw" aria-hidden="true"></span>
				<span>{t('settings.signIn.skipLocal')}</span>
			</label>
			<p class="hint">
				{t('settings.signIn.skipNote')}
			</p>
			<p class="hint">{t('settings.signIn.directOnly')}</p>
			<p class="hint">{t('settings.signIn.keysNeed')}</p>
			<form class="nets" onsubmit={saveNets}>
				<Field id="local-nets" label={t('settings.signIn.nets')} hint={t('settings.signIn.netsHint')}>
					<textarea id="local-nets" class="input" rows="3" bind:value={nets} spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
				</Field>
				<Button type="submit" disabled={savingNets} aria-busy={savingNets}>{t('settings.signIn.save')}</Button>
			</form>
			{#if seenAddress}<p class="hint" data-testid="seen-address">{t('settings.signIn.seesAs')} <span class="mono">{seenAddress}</span>.</p>{/if}
			{#if proxyUntrusted}<p class="hint" data-testid="proxy-untrusted">
					{t('settings.signIn.proxyA')}
					<span class="mono">HOLYARRS_TRUSTED_PROXIES</span> {t('settings.signIn.proxyB')}
				</p>{/if}
			{#if gatewayNets.length}
				<p class="notice warn" role="status" data-testid="gateway-warn">
					<Icon name="info" size={18} /><span
						>{t(gatewayNets.length === 1 ? 'settings.signIn.gatewayOne' : 'settings.signIn.gatewayMany', { nets: gatewayNets.join(', ') })}</span
					>
				</p>
			{/if}
	</SettingsCard>

	{/if}

	<SettingsLinks label={t('settings.signIn.apiKeys')} links={[{ href: '/settings/api#keys-card', label: t('settings.signIn.apiKeys'), note: t('settings.signIn.apiKeysNote') }]} />

	<MoreOptions>
		{#if settingsOk}
		<SettingsCard id="hosts-card" title={t('settings.signIn.hosts')}>
			<p class="hint">
				<Words key="settings.signIn.hostsHint" />
			</p>
			<form class="nets" onsubmit={saveHosts}>
				<Field id="allowed-hosts" label={t('settings.signIn.hostNames')}>
					<textarea id="allowed-hosts" class="input" rows="3" bind:value={hostNames} spellcheck="false" autocapitalize="off" autocomplete="off"></textarea>
				</Field>
				<Button type="submit" disabled={savingHosts} aria-busy={savingHosts}>{t('settings.signIn.save')}</Button>
			</form>
		</SettingsCard>
		{/if}

		<SettingsCard id="proxy-card" title={t('settings.signIn.proxies')}>
			<p class="small muted" style="margin-top: var(--s-2)">
				<Words key="settings.signIn.proxiesHint" />
			</p>
		</SettingsCard>
	</MoreOptions>
{/if}

<Sheet bind:open={importOpen} title={importFor ? t('settings.signIn.importFrom', { app: wayWord[importFor] }) : t('settings.signIn.importMembers')} size="md">
	{#if importFor && importFor !== 'plex' && !candidates}
		<form class="imp" onsubmit={preview}>
			<p class="small muted">{t('settings.signIn.importIntro', { app: wayWord[importFor] })}</p>
			<Field id="imp-name" label={t('settings.signIn.adminName')}><input id="imp-name" class="input" bind:value={importName} autocomplete="off" required maxlength="128" /></Field>
			<Field id="imp-pass" label={t('settings.signIn.adminPass')}><input id="imp-pass" class="input" type="password" bind:value={importPass} autocomplete="off" maxlength="256" /></Field>
			<Button variant="primary" type="submit" disabled={importBusy || !importName} aria-busy={importBusy}>{t('settings.signIn.showPeople')}</Button>
		</form>
	{:else if !candidates}
		{#if !importNote}<SkSettings rows={3} />{/if}
	{:else if candidates.length === 0}
		<p class="small muted">{t('settings.signIn.noOne')}</p>
	{:else}
		<p class="small muted">{t('settings.signIn.importNote')}</p>
		<ul class="rows imp-list">
			{#each candidates as p (p.id)}
				<li>
					<label class="checkline grow">
						<input type="checkbox" bind:checked={picked[p.id].on} disabled={!!p.linked_to} />
						<span>
							<span class="name">{p.name}</span>
							{#if p.email}<span class="xs muted"> {p.email}</span>{/if}
							{#if p.linked_to}<span class="xs muted"> · {t('settings.signIn.already', { to: p.linked_to })}</span>{/if}
							{#if p.server_admin}<span class="chip">{t('settings.signIn.serverAdmin')}</span>{/if}
						</span>
					</label>
					<label class="checkline xs"><input type="checkbox" bind:checked={picked[p.id].kid} disabled={!!p.linked_to || !picked[p.id].on} /> {t('settings.signIn.kid')}</label>
				</li>
			{/each}
		</ul>
	{/if}
	{#if importNote}<p class="notice warn" role="alert">{importNote}</p>{/if}
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (importOpen = false)}>{t('settings.signIn.cancel')}</Button>
		{#if candidates && candidates.length}
			<Button variant="primary" type="button" onclick={runImport} disabled={importBusy || pickedCount === 0} aria-busy={importBusy}>{t('settings.signIn.importN', { n: pickedCount })}</Button>
		{/if}
	{/snippet}
</Sheet>

<Sheet bind:open={everyoneOpen} title={t('settings.signIn.letEveryone')} size="sm">
	{#if everyoneFor}<p class="small">{everyoneText(everyoneFor)}</p>{/if}
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (everyoneOpen = false)}>{t('settings.signIn.cancel')}</Button>
		<Button variant="primary" type="button" onclick={() => everyoneFor && saveWay(everyoneFor, undefined, true)}>{t('settings.signIn.letEveryoneButton')}</Button>
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
	p :global(code) {
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
