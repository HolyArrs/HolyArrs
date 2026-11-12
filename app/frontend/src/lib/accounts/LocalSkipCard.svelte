<!-- Settings > Sign-in: skip sign-in on the local network. Who opens HolyArrs at home,
     the home networks checked line by line, and the trusted proxies. One Save for all. -->
<script lang="ts" module>
	export interface LocalSkip {
		skip_login_local_network: boolean;
		local_networks?: string[];
		local_person?: number;
		local_skip_off?: '' | 'kid' | 'person_gone';
		media_sign_in?: string[];
		home_ranges?: string[];
		cgnat_range?: string;
		trusted_proxies?: string[];
		trusted_proxies_fixed?: string[];
		seen_address?: string;
		gateway_networks?: string[];
		proxy_untrusted?: boolean;
		seen_gateway?: boolean;
	}
</script>

<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { kidLabel } from './kid.ts';
	import { checkLines, covers, gatewayLike, lines, type LineFix } from './local-lines.ts';

	let { settings, isOwner, strongNote, onsaved }: { settings: LocalSkip; isOwner: boolean; strongNote: (e: unknown) => string; onsaved: (r: LocalSkip) => void } = $props();

	interface Person {
		id: number;
		name: string;
		role: 'owner' | 'admin' | 'member';
		state: string;
		avatar?: string | null;
		max_rating?: string;
	}

	// Editable copies of what the server last sent.
	let skip = $state(false);
	let nets = $state('');
	let proxies = $state('');
	let person = $state(0);
	let savedText = $state('');
	// Reads only the settings, so typing never re-runs the copy below and wipes the edits.
	const savedOf = (s: LocalSkip) => (s.local_networks ?? []).join('\n') + '|' + (s.trusted_proxies ?? []).join('\n') + '|' + (s.local_person ?? 0);
	$effect.pre(() => {
		skip = settings.skip_login_local_network;
		nets = (settings.local_networks ?? []).join('\n');
		proxies = (settings.trusted_proxies ?? []).join('\n');
		person = settings.local_person ?? 0;
		savedText = savedOf(settings);
	});

	let people = $state<Person[] | null>(null);
	let peopleError = $state('');
	async function loadPeople() {
		peopleError = '';
		try {
			const r = await api.get<{ users: Person[] }>('/users');
			people = r.users.filter((u) => u.state === 'active');
		} catch (e) {
			peopleError = message(e);
		}
	}
	$effect(() => {
		if (isOwner && people === null && !peopleError) void loadPeople();
	});
	// The owner is stored as 0.
	const ownerId = $derived(people?.find((p) => p.role === 'owner')?.id ?? -1);
	const pickedId = $derived(person === 0 ? ownerId : person);

	const gateways = $derived(settings.gateway_networks ?? []);
	const netProblems = $derived(checkLines(nets, 'net', gateways));
	const proxyProblems = $derived(checkLines(proxies, 'proxy'));
	const problems = $derived(netProblems.length + proxyProblems.length);
	const seen = $derived(settings.seen_address ?? '');
	const onBridge = $derived(seen !== '' && gatewayLike(seen) && !settings.seen_gateway);
	// The container's own gateway: LAN visits by IP arrive as it and count as home.
	const ownGateway = $derived(seen !== '' && settings.seen_gateway === true);
	const covered = $derived(seen ? covers(seen, lines(nets)) : null);
	const cgnat = $derived(settings.cgnat_range ?? '100.64.0.0/10');
	const hasCgnat = $derived(lines(nets).includes(cgnat));
	const dirty = $derived(nets + '|' + proxies + '|' + person !== savedText);
	const media = $derived((settings.media_sign_in ?? []).map((w) => ({ plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' })[w] ?? w));

	function fixText(f: LineFix): string {
		return t(`settings.signIn.${f.key}`, 'fix' in f ? { fix: f.fix, ch: 'ch' in f ? f.ch : '' } : {});
	}
	function roleLine(p: Person): string {
		if (p.max_rating) return kidLabel(p.max_rating);
		return t(p.role === 'owner' ? 'settings.signIn.ownerLine' : p.role === 'admin' ? 'settings.signIn.adminLine' : 'settings.signIn.memberLine');
	}

	function addHome() {
		const have = lines(nets);
		const add = (settings.home_ranges ?? []).filter((r) => !have.includes(r));
		if (add.length) nets = [...have, ...add].join('\n');
	}
	function setCgnat(on: boolean) {
		const have = lines(nets).filter((r) => r !== cgnat);
		nets = (on ? [...have, cgnat] : have).join('\n');
	}

	let savingSkip = $state(false);
	async function setSkip(v: boolean) {
		if (savingSkip) return;
		const before = skip;
		skip = v;
		savingSkip = true;
		try {
			// Turning it on saves the networks and person picked below with it.
			onsaved(await api.put<LocalSkip>('/auth/settings', v ? { skip_login_local_network: true, local_networks: lines(nets), local_person: person } : { skip_login_local_network: false }));
			toasts.push(v ? t('settings.signIn.skips') : t('settings.signIn.neededEverywhere'), 'ok');
		} catch (e) {
			skip = before;
			toasts.push(t('settings.signIn.couldntSave'), 'fail', { body: strongNote(e) });
		} finally {
			savingSkip = false;
		}
	}

	let saving = $state(false);
	let saveError = $state('');
	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (problems) return;
		saving = true;
		saveError = '';
		try {
			const body: Record<string, unknown> = { local_networks: lines(nets), local_person: person };
			const savedProxies = (settings.trusted_proxies ?? []).join('\n');
			if (proxies.trim() !== savedProxies.trim()) body.trusted_proxies = lines(proxies);
			onsaved(await api.put<LocalSkip>('/auth/settings', body));
			toasts.push(t('settings.signIn.localSaved'), 'ok');
		} catch (err) {
			saveError = strongNote(err);
		} finally {
			saving = false;
		}
	}
</script>

{#snippet problemList(list: ReturnType<typeof checkLines>, id: string)}
	{#if list.length}
		<div class="fixes" id={id} role="status">
			<p class="fix-head">{t(list.length === 1 ? 'settings.signIn.linesFixOne' : 'settings.signIn.linesFixOther', { n: list.length })}</p>
			<ul>
				{#each list as p (p.line)}
					<li><span class="xs muted">{t('settings.signIn.lineN', { n: p.line })}</span> <span class="mono xs" dir="ltr">{p.text}</span><br /><span class="xs">{fixText(p.fix)}</span></li>
				{/each}
			</ul>
		</div>
	{/if}
{/snippet}

<SettingsCard id="local-card" title={t('settings.signIn.local')}>
	<label class="switch" style="margin-top: var(--s-3)">
		<input type="checkbox" checked={skip} disabled={!isOwner || savingSkip} aria-busy={savingSkip} onchange={(e) => setSkip(e.currentTarget.checked)} />
		<span class="sw" aria-hidden="true"></span>
		<span>{t('settings.signIn.skipLocal')}</span>
	</label>
	<p class="hint">{t('settings.signIn.skipHome')}</p>
	{#if media.length}<p class="hint" data-testid="media-on">{t('settings.signIn.mediaOn', { apps: media.join(', ') })}</p>{/if}

	{#if skip && settings.local_skip_off === 'person_gone'}
		<p class="notice bad" role="status" data-testid="skip-off"><Icon name="info" size={18} /><span>{t('settings.signIn.offGone')}</span></p>
	{:else if skip && settings.local_skip_off === 'kid'}
		<p class="notice bad" role="status" data-testid="skip-off"><Icon name="info" size={18} /><span>{t('settings.signIn.offKid')}</span></p>
	{/if}
	{#if ownGateway}
		<div class="notice" role="status" data-testid="bridge-ok">
			<Icon name="info" size={18} />
			<div>
				<p class="strong">{t('settings.signIn.bridgeOkTitle', { ip: seen })}</p>
				<p>{t('settings.signIn.bridgeOkBody')}</p>
			</div>
		</div>
	{:else if onBridge}
		<div class="notice warn" role="status" data-testid="bridge-warn">
			<Icon name="info" size={18} />
			<div>
				<p class="strong">{t('settings.signIn.bridgeTitle', { ip: seen })}</p>
				<p>{t('settings.signIn.bridgeBody')}</p>
				<details class="how"><summary>{t('settings.signIn.howToFix')}</summary><p>{t('settings.signIn.howToFixBody')}</p></details>
			</div>
		</div>
	{/if}
	{#if settings.proxy_untrusted}<p class="hint" data-testid="proxy-untrusted">{t('settings.signIn.proxyA')} <span class="mono">HOLYARRS_TRUSTED_PROXIES</span> {t('settings.signIn.proxyB')}</p>{/if}

	<form class="local" onsubmit={save}>
		<fieldset disabled={!isOwner}>
			<section class="part" aria-labelledby="who-head">
				<h3 id="who-head" class="label">{t('settings.signIn.who')}</h3>
				{#if !isOwner}
					<!-- Only the owner sees and changes who this is. -->
				{:else if peopleError}
					<LoadError hasData={false} message={peopleError} onretry={loadPeople} />
				{:else if people === null}
					<div class="tiles" aria-busy="true">{#each [0, 1] as i (i)}<div class="tile sk"></div>{/each}</div>
				{:else}
					<div class="tiles" role="radiogroup" aria-labelledby="who-head">
						{#each people as p (p.id)}
							<label class="tile" class:on={p.id === pickedId}>
								<input class="vh" type="radio" name="local-person" checked={p.id === pickedId} onchange={() => (person = p.role === 'owner' ? 0 : p.id)} />
								<Avatar name={p.name} src={p.avatar} size={36} />
								<span class="who"><span class="name">{p.name}</span><span class="xs muted">{roleLine(p)}</span></span>
								{#if p.id === pickedId}<span class="tick"><Icon name="check" size={16} /></span>{/if}
							</label>
						{/each}
					</div>
				{/if}
				<p class="hint">{t('settings.signIn.whoHint')}</p>
			</section>

			<section class="part">
				<label class="label" for="local-nets">{t('settings.signIn.homeNets')}</label>
				<textarea id="local-nets" dir="ltr" class="input mono" class:bad={netProblems.length} rows="4" bind:value={nets} spellcheck="false" autocapitalize="off" autocomplete="off" aria-invalid={netProblems.length > 0} aria-describedby="nets-hint nets-fixes"></textarea>
				<p id="nets-hint" class="hint">{t('settings.signIn.homeNetsHint')}</p>
				{@render problemList(netProblems, 'nets-fixes')}
				{#if seen}
					<p class="you xs" data-testid="seen-address">
						<span class="dot" class:ok={ownGateway || (covered === true && !onBridge)}></span>
						{ownGateway ? t('settings.signIn.youBridgeOk', { ip: seen }) : onBridge ? t('settings.signIn.youBridge', { ip: seen }) : covered === true ? t('settings.signIn.youCovered', { ip: seen }) : t('settings.signIn.youNot', { ip: seen })}
					</p>
				{/if}
				<div><Button type="button" size="sm" onclick={addHome}>{t('settings.signIn.addHome')}</Button></div>
				<label class="checkline">
					<input type="checkbox" checked={hasCgnat} onchange={(e) => setCgnat(e.currentTarget.checked)} />
					<span>{t('settings.signIn.cgnat')}<br /><span class="xs muted">{t('settings.signIn.cgnatHint', { range: cgnat })}</span></span>
				</label>
			</section>

			<section class="part">
				<label class="label" for="trusted-proxies">{t('settings.signIn.proxies')}</label>
				{#each settings.trusted_proxies_fixed ?? [] as p (p)}
					<p class="fixed"><span class="mono" dir="ltr">{p}</span> <span class="xs muted">· {t('settings.signIn.fixedRow')}</span></p>
				{/each}
				<textarea id="trusted-proxies" dir="ltr" class="input mono" class:bad={proxyProblems.length} rows="2" bind:value={proxies} placeholder="10.0.0.5" spellcheck="false" autocapitalize="off" autocomplete="off" aria-invalid={proxyProblems.length > 0} aria-describedby="proxies-hint proxies-fixes"></textarea>
				<p id="proxies-hint" class="hint">{t('settings.signIn.proxiesHelp')}</p>
				{@render problemList(proxyProblems, 'proxies-fixes')}
			</section>

			{#if isOwner}
				<div class="save">
					<Button variant="primary" type="submit" disabled={saving || problems > 0 || !dirty} aria-busy={saving}>{t('settings.signIn.save')}</Button>
					{#if problems}<span class="xs muted">{t(problems === 1 ? 'settings.signIn.fixToSaveOne' : 'settings.signIn.fixToSaveOther', { n: problems })}</span>{/if}
				</div>
				{#if saveError}<p class="notice bad" role="alert" data-testid="local-error">{saveError}</p>{/if}
			{:else}
				<p class="hint">{t('settings.signIn.ownerOnly')}</p>
			{/if}
		</fieldset>
	</form>
</SettingsCard>

<style>
	.local fieldset { border: 0; padding: 0; margin: 0; min-width: 0; display: grid; gap: var(--s-4); }
	.part { display: grid; gap: var(--s-2); padding-top: var(--s-4); border-top: 1px solid var(--line); }
	.part:first-child { margin-top: var(--s-3); }
	h3.label { margin: 0; }
	.tiles { display: grid; gap: var(--s-2); }
	.tile { display: flex; align-items: center; gap: var(--s-3); min-height: 56px; padding: var(--s-2) var(--s-3); border: 1px solid var(--line); border-radius: var(--r-btn); background: var(--surface-2); cursor: pointer; transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.tile.sk { cursor: default; }
	.tile:hover { border-color: var(--line-strong, var(--text-muted)); }
	.tile.on { border-color: var(--ha-red); background: color-mix(in srgb, var(--ha-red) 14%, var(--surface-2)); }
	/* The red tint drops grey text under 4.5:1; pull it toward the main text colour. */
	.tile.on .muted { color: color-mix(in srgb, var(--text-muted) 70%, var(--text)); }
	.tile:has(input:focus-visible) { outline: 2px solid var(--focus, var(--ha-red)); outline-offset: 2px; }
	.who { display: grid; min-width: 0; flex: 1; }
	.who > span { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.name { font-weight: var(--weight-semibold); }
	.tick { display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: var(--ha-red); color: var(--text-on-red); flex: none; }
	.vh { position: absolute; opacity: 0; width: 1px; height: 1px; pointer-events: none; }
	textarea.input { height: auto; padding-top: 10px; }
	textarea.bad { border-color: var(--fail); }
	.fixes { display: grid; gap: var(--s-1); }
	.fix-head { margin: 0; color: var(--fail); font-size: var(--text-xs); font-weight: var(--weight-semibold); }
	.fixes ul { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--s-2); }
	.fixes li { border-inline-start: 2px solid var(--fail); padding-inline-start: var(--s-2); overflow-wrap: anywhere; }
	.you { display: flex; align-items: center; gap: 6px; margin: 0; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); flex: none; }
	.dot.ok { background: var(--ok); }
	.fixed { margin: 0; }
	.strong { font-weight: var(--weight-semibold); color: var(--text); margin: 0; }
	.notice p { margin: 0; }
	.notice > div { display: grid; gap: var(--s-1); }
	.how summary { cursor: pointer; color: var(--text); }
	.save { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s-3); }
</style>
