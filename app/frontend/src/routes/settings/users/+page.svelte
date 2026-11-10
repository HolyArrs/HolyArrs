<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { presetOf, presetPerms, presetsFor, type PresetId, Sessions, kidLabel, ratingChoices } from '#lib/accounts/index.ts';
	import type { AskSettings, LimitKind, PersonLimit } from '#lib/requests/index.ts';
	import { kindAsks } from '#lib/requests/index.ts';
	import { onMount } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session, type Perm, type Role } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, date } from '#lib/core/format.ts';

	interface User {
		id: number;
		name: string;
		role: Role;
		state: 'active' | 'disabled' | 'locked';
		perms: Perm[];
		two_factor: boolean;
		password: boolean;
		links: { provider: string; name: string }[];
		sso_links?: { kind: 'oidc' | 'forward_auth'; issuer: string; name: string }[];
		created_at: string;
		avatar?: string | null;
		last_seen_at?: string;
		max_rating?: string;
		rating_country?: string;
	}
	interface Invite {
		id: string;
		role: Role;
		perms: Perm[];
		note: string;
		created_by: string;
		created_at: string;
		expires_at: string;
		used_at?: string;
		used_by?: string;
		max_rating?: string;
		rating_country?: string;
	}

	const permWord = $derived<Record<Perm, string>>({
		ask: t('settings.users.permAsk'),
		auto_approve: t('settings.users.permAuto'),
		approve: t('settings.users.permApprove'),
		manage_library: t('settings.users.permLibrary'),
		settings: t('settings.users.permSettings')
	});
	const defaults: Record<'admin' | 'member', Perm[]> = {
		admin: ['ask', 'auto_approve', 'approve', 'manage_library'],
		member: ['ask']
	};
	const roleWord = $derived<Record<string, string>>({ owner: t('settings.users.owner'), admin: t('settings.users.admin'), member: t('settings.users.member') });
	const providerWord: Record<string, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };

	// Weekly ask limits, one per kind, from Request rules (absent when it is off).
	interface LimitRow extends PersonLimit {
		user_id: number;
		used: Record<LimitKind, number>;
		no_limit: boolean;
	}
	const limitFields = $derived([
		{ kind: 'movies', key: 'movies_per_week', label: t('settings.users.movieReq') },
		{ kind: 'shows', key: 'shows_per_week', label: t('settings.users.showReq') },
		{ kind: 'music', key: 'music_per_week', label: t('settings.users.musicReq') }
	] as const);
	let limits = $state<Record<number, LimitRow>>({});
	let defaultsPerWeek = $state<Partial<Record<LimitKind, number>>>({});
	let editLimits = $state<Record<LimitKind, string>>({ movies: '', shows: '', music: '' });

	function limitWords(l: LimitRow | undefined): string {
		if (!l) return '';
		if (l.no_limit) return t('settings.users.noRequestLimit');
		return limitFields
			.map((f) => {
				const own = l[f.key];
				const per = own ?? defaultsPerWeek[f.kind];
				const used = l.used?.[f.kind] ?? 0;
				if (per === 0) return t('settings.users.noLimitFor', { what: f.label.toLowerCase() });
				return t('settings.users.limitOf', { asks: kindAsks(f.kind, used), per: per ?? t('settings.users.theDefault') }) + (own === null ? '' : ' ' + t('settings.users.own'));
			})
			.join(' · ');
	}

	let users = $state<User[]>([]);
	let invites = $state<Invite[]>([]);
	let allPerms = $state<Perm[]>(['ask', 'auto_approve', 'approve', 'manage_library', 'settings']);
	let loaded = $state(false);
	let error = $state('');
	let hasData = $state(false);

	// New invite.
	let role = $state<'admin' | 'member'>('member');
	let perms = $state<Perm[]>([...defaults.member]);
	let maxRating = $state('');
	let note = $state('');
	let days = $state(7);
	let making = $state(false);
	let link = $state('');

	// Editing one user.
	let editing = $state<User | null>(null);
	let editRole = $state<'admin' | 'member'>('member');
	let editPerms = $state<Perm[]>([]);
	let editMax = $state('');
	let saving = $state(false);
	let dialog: HTMLDialogElement | undefined = $state();

	const isOwner = $derived(session.s.role === 'owner');
	// Admins hand out only what they hold.
	const grantable = $derived(allPerms.filter((p) => session.can(p)));

	async function load() {
		try {
			const [u, i] = await Promise.all([api.get<{ users: User[]; perms: Perm[] }>('/users'), api.get<{ invites: Invite[] }>('/users/invites')]);
			users = u.users;
			allPerms = u.perms;
			invites = i.invites;
			loadLimits();
			hasData = true;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	// Settings search links to #inv-note: open the invite sheet so the layout can find and focus that field. The sheet moves focus to its first button when it opens, so put it on the note field once the dialog is showing.
	function openInviteAtNote() {
		inviteOpen = true;
		let tries = 0;
		const put = () => {
			const el = document.getElementById('inv-note');
			if (el && el.closest('dialog')?.open) {
				if (document.activeElement !== el) el.focus();
				if (tries > 3) return;
			}
			if (tries++ < 20) setTimeout(put, 100);
		};
		setTimeout(put, 50);
	}
	afterNavigate(({ to: dest }) => {
		if (dest?.url.hash === '#inv-note') openInviteAtNote();
	});
	onMount(() => {
		if (location.hash === '#inv-note') openInviteAtNote();
		void load();
	});

	function loadLimits() {
		api
			.get<{ limits: LimitRow[] }>('/ask-limits')
			.then((r) => (limits = Object.fromEntries(r.limits.map((l) => [l.user_id, l]))))
			.catch(() => (limits = {}));
		if (session.can('settings')) api
			.get<AskSettings>('/settings/asks')
			.then((s) => (defaultsPerWeek = { movies: s.movies_per_week, shows: s.shows_per_week, music: s.music_per_week }))
			.catch(() => {});
	}

	/** Reads one box: '' is the default, otherwise 0 to 1000. */
	function limitValue(v: string | number): number | null | undefined {
		const s = String(v ?? '').trim();
		if (s === '') return null;
		const n = Number(s);
		return Number.isInteger(n) && n >= 0 && n <= 1000 ? n : undefined;
	}

	function pickRole(r: 'admin' | 'member') {
		role = r;
		maxRating = '';
		perms = defaults[r].filter((p) => session.can(p));
	}

	function toggle(list: Perm[], p: Perm, on: boolean): Perm[] {
		return on ? [...new Set([...list, p])] : list.filter((x) => x !== p);
	}

	// The invite form opens as a sheet from the header; People come first.
	let inviteOpen = $state(false);

	async function makeInvite(e: SubmitEvent) {
		e.preventDefault();
		making = true;
		try {
			const r = await api.post<{ path: string }>('/users/invites', { role, perms, note: note.trim(), days, max_rating: maxRating, rating_country: 'US' });
			link = location.origin + to(r.path);
			note = '';
			inviteOpen = false;
			await load();
		} catch (err) {
			toasts.push(t('settings.users.inviteFailed'), 'fail', { body: message(err) });
		} finally {
			making = false;
		}
	}

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(link);
			toasts.push(t('settings.users.linkCopied'), 'ok');
		} catch {
			toasts.push(t('settings.users.copyFailed'), 'fail', { body: t('settings.users.selectInstead') });
		}
	}

	async function takeBack(i: Invite) {
		try {
			await api.del(`/users/invites/${i.id}`);
			invites = invites.filter((x) => x.id !== i.id);
			toasts.push(t('settings.users.takenBack'), 'ok');
		} catch (e) {
			toasts.push(t('settings.users.takeBackFailed'), 'fail', { body: message(e) });
		}
	}

	// Signed-in devices: the owner sees everyone's, an admin members' (their own are in My account).
	let devicesOf = $state<User | null>(null);
	let devDialog: HTMLDialogElement | undefined = $state();
	function maySeeDevices(u: User): boolean {
		if (u.name === session.s.name) return false;
		return isOwner || u.role === 'member';
	}
	function openDevices(u: User) {
		devicesOf = u;
		devDialog?.showModal();
	}

	let pwFor = $state<User | null>(null);
	let pwNew = $state('');
	let pwDialog: HTMLDialogElement | undefined = $state();
	function openPassword(u: User) {
		pwFor = u;
		pwNew = '';
		pwDialog?.showModal();
	}
	async function savePassword(e: SubmitEvent) {
		e.preventDefault();
		const u = pwFor;
		if (!u) return;
		try {
			await api.post(`/users/${u.id}/password`, { password: pwNew });
			toasts.push(t('settings.users.passwordSet', { name: u.name }), 'ok', { body: t('settings.users.signedOutEverywhere') });
			pwDialog?.close();
		} catch (err) {
			toasts.push(t('settings.users.passwordFailed'), 'fail', { body: message(err) });
		}
	}

	function mayChange(u: User): boolean {
		if (u.role === 'owner' || u.name === session.s.name) return false;
		return isOwner || u.role === 'member';
	}

	function openEdit(u: User) {
		editing = u;
		editRole = u.role === 'admin' ? 'admin' : 'member';
		editPerms = [...u.perms];
		editMax = u.max_rating ?? '';
		const l = limits[u.id];
		editLimits = {
			movies: l?.movies_per_week == null ? '' : String(l.movies_per_week),
			shows: l?.shows_per_week == null ? '' : String(l.shows_per_week),
			music: l?.music_per_week == null ? '' : String(l.music_per_week)
		};
		dialog?.showModal();
	}

	async function put(u: User, body: Record<string, unknown>, done: string) {
		saving = true;
		try {
			await api.put(`/users/${u.id}`, body);
			toasts.push(done, 'ok');
			await load();
			return true;
		} catch (e) {
			toasts.push(t('settings.users.couldntSave'), 'fail', { body: message(e) });
			return false;
		} finally {
			saving = false;
		}
	}

	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		if (!editing) return;
		const body: Record<string, unknown> = { perms: editPerms, max_rating: editRole === 'admin' ? '' : editMax, rating_country: 'US' };
		if (isOwner) body.role = editRole;
		const u = editing;
		if (limits[u.id] && !limits[u.id].no_limit) {
			const own = { movies_per_week: limitValue(editLimits.movies), shows_per_week: limitValue(editLimits.shows), music_per_week: limitValue(editLimits.music) };
			if (Object.values(own).includes(undefined)) {
				toasts.push(t('settings.users.limitInvalid'), 'fail', { body: t('settings.users.limitInvalidBody') });
				return;
			}
			try {
				await api.put(`/ask-limits/${u.id}`, own);
			} catch (err) {
				toasts.push(t('settings.users.couldntSave'), 'fail', { body: message(err) });
				return;
			}
		}
		if (await put(u, body, t('settings.users.saved', { name: u.name }))) dialog?.close();
	}

	async function setState(u: User, state: 'active' | 'disabled') {
		if (state === 'disabled' && !confirm(t('settings.users.confirmOff', { name: u.name }))) return;
		await put(u, { state }, state === 'disabled' ? t('settings.users.turnedOff', { name: u.name }) : t('settings.users.turnedOn', { name: u.name }));
	}

	async function resetTwoFactor(u: User) {
		if (!confirm(t('settings.users.confirmReset', { name: u.name }))) return;
		try {
			await api.post(`/users/${u.id}/two-factor/reset`);
			toasts.push(t('settings.users.resetDone', { name: u.name }), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('settings.users.resetFailed'), 'fail', { body: message(e) });
		}
	}

	// The owner unlinks someone's single sign-on or trusted proxy identity; the sessions it made end with it.
	const ssoWord = (k: string) => (k === 'oidc' ? t('settings.users.sso') : t('settings.users.proxy'));
	const hostOf = (u: string) => {
		try {
			return new URL(u).host;
		} catch {
			return u;
		}
	};
	async function unlinkSSO(u: User, l: { kind: string; issuer: string }) {
		if (!confirm(t('settings.users.confirmUnlink', { name: u.name, kind: ssoWord(l.kind) }))) return;
		const body: Record<string, unknown> = { kind: l.kind, issuer: l.issuer };
		try {
			try {
				await api.post(`/users/${u.id}/sso/unlink`, body);
			} catch (e) {
				if (!(e instanceof ApiError && e.body.code === 'no_way_in')) throw e;
				if (!confirm(t('settings.users.confirmLockout', { name: u.name }))) return;
				await api.post(`/users/${u.id}/sso/unlink`, { ...body, confirm_no_way_in: true });
			}
			toasts.push(t('settings.users.unlinked', { name: u.name }), 'ok', { body: ssoWord(l.kind) });
			await load();
		} catch (e) {
			toasts.push(t('settings.users.unlinkFailed'), 'fail', { body: e instanceof ApiError && e.body.code === 'reauth' ? t('settings.users.confirmPasswordAccount') : message(e) });
		}
	}

	function inviteState(i: Invite): string {
		if (i.used_at) return t('settings.users.usedBy', { who: i.used_by ?? t('settings.users.someone'), ago: ago(i.used_at) });
		if (new Date(i.expires_at).getTime() < Date.now()) return t('settings.users.ended');
		return t('settings.users.openUntil', { date: date(i.expires_at) });
	}
</script>

<svelte:head><title>{t('settings.users.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.users.title')}>
	{#snippet actions()}
		{#if loaded && hasData}<Button icon="plus" variant="primary" type="button" onclick={() => (inviteOpen = true)}>{t('settings.users.invite')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if !loaded}
	<SkSettings rows={3} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if link}
		<div class="fresh" role="status">
			<p class="small"><b>{t('settings.users.inviteLink')}</b> {t('settings.users.worksOnce')}</p>
			<div class="row">
				<code class="key grow">{link}</code>
				<Button icon="copy" type="button" onclick={copyLink}>{t('settings.users.copy')}</Button>
				<Button variant="plain" type="button" onclick={() => (link = '')}>{t('settings.users.close')}</Button>
			</div>
		</div>
	{/if}

	<SettingsCard id="people-card" title={t('settings.users.people')}>
		<ul class="rows" style="margin-top: var(--s-3)">
			{#each users as u (u.id)}
				<li class="person">
					<Avatar name={u.name} src={u.avatar} size={40} />
					<div class="grow">
						<div class="name">
							{u.name}
							<span class="chip {u.role === 'member' ? '' : 'red'}">{roleWord[u.role]}</span>
							{#if u.state !== 'active'}<span class="chip fail">{t('settings.users.off')}</span>{/if}
							{#if u.two_factor}<span class="chip ok">{t('settings.users.twoFactor')}</span>{/if}
							{#if u.max_rating}<span class="chip" data-testid="kid-label">{kidLabel(u.max_rating)}</span>{/if}
						</div>
						<div class="xs muted">
							{#if u.links.length}{u.links.map((l) => `${providerWord[l.provider] ?? l.provider}: ${l.name}`).join(' · ')} ·
							{/if}{u.role === 'owner' ? t('settings.users.everything') : u.perms.map((p) => permWord[p]).join(', ') || t('settings.users.nothingYet')} ·
							{u.last_seen_at ? t('settings.users.seen', { ago: ago(u.last_seen_at) }) : t('settings.users.notSeen')}
						</div>
						{#if u.sso_links?.length}
							<ul class="sso-links" data-testid="user-sso-links">
								{#each u.sso_links as l (l.kind + l.issuer)}
									<li class="xs muted">
										<span>{l.kind === 'oidc' ? t('settings.users.ssoCap') : t('settings.users.proxyCap')}: <span class="mono">{hostOf(l.issuer)}</span>{#if l.name}, {l.name}{/if}</span>
										{#if isOwner}
											<Button variant="plain" size="sm" type="button" onclick={() => unlinkSSO(u, l)} aria-label={t('settings.users.unlinkLabel', { kind: ssoWord(l.kind), name: u.name })}>{t('settings.users.unlink')}</Button>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
						{#if limits[u.id] && u.role !== 'owner'}
							<div class="xs muted" data-testid="user-limits">{t('settings.users.thisWeek', { limits: limitWords(limits[u.id]) })}</div>
						{/if}
					</div>
					{#if mayChange(u) || maySeeDevices(u)}
						<div class="row acts">
							{#if maySeeDevices(u)}
								<Button variant="plain" type="button" onclick={() => openDevices(u)} aria-label={t('settings.users.devicesOf', { name: u.name })}>{t('settings.users.devices')}</Button>
							{/if}
						{#if mayChange(u)}
							<Button type="button" onclick={() => openEdit(u)} aria-label={t('settings.users.changeName', { name: u.name })}>{t('settings.users.change')}</Button>
							<Button variant="plain" type="button" onclick={() => openPassword(u)} aria-label={t('settings.users.setPasswordFor', { name: u.name })}>{t('settings.users.setPasswordLabel')}</Button>
							{#if u.two_factor}
								<Button variant="plain" type="button" onclick={() => resetTwoFactor(u)} aria-label={t('settings.users.resetFor', { name: u.name })}
									>{t('settings.users.resetTwoFactor')}</Button>
							{/if}
							{#if u.state === 'active'}
								<Button variant="destructive" type="button" onclick={() => setState(u, 'disabled')} disabled={saving} aria-label={t('settings.users.turnOffName', { name: u.name })}
									>{t('settings.users.turnOff')}</Button>
							{:else}
								<Button type="button" onclick={() => setState(u, 'active')} disabled={saving} aria-label={t('settings.users.turnOnName', { name: u.name })}
									>{t('settings.users.turnOn')}</Button>
							{/if}
						{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	</SettingsCard>

	<SettingsCard id="invites-card" title={t('settings.users.inviteLinks')}>
		{#if invites.length === 0}
			<p class="small muted" style="margin-top: var(--s-3)">{t('settings.users.noInvites')}</p>
		{:else}
			<ul class="rows" style="margin-top: var(--s-3)">
				{#each invites as i (i.id)}
					<li>
						<div class="grow">
							<div class="name">{i.note || t('settings.users.roleInvite', { role: roleWord[i.role] })} <span class="chip">{roleWord[i.role]}</span>{#if i.max_rating}<span class="chip">{kidLabel(i.max_rating)}</span>{/if}</div>
							<div class="xs muted">{t('settings.users.madeBy', { who: i.created_by, ago: ago(i.created_at) })} · {inviteState(i)}</div>
						</div>
						{#if !i.used_at && new Date(i.expires_at).getTime() > Date.now()}
							<Button variant="plain" type="button" onclick={() => takeBack(i)}>{t('settings.users.revoke')}</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>
{/if}

{#snippet presetPicker(list: Perm[], set: (next: Perm[]) => void, name: string, max: string, setMax: (m: string) => void)}
	{@const current = presetOf(list, grantable, max)}
	<fieldset class="field">
		<legend class="label">{t('settings.users.preset')}</legend>
		<div class="row presets">
			{#each presetsFor(grantable) as pr (pr.id)}
				<label class="checkline" class:soon={!!pr.soon}
					><input
						type="radio"
						{name}
						value={pr.id}
						checked={!pr.soon && current === pr.id}
						disabled={!!pr.soon}
						aria-describedby={pr.soon ? `${name}-${pr.id}-why` : undefined}
						onchange={() => {
							const next = presetPerms(pr.id as PresetId, grantable);
							if (next) {
								set(next);
								setMax(pr.maxRating ?? '');
							}
						}}
					/>
					{pr.label}{#if pr.soon}<span class="chip">{t('settings.users.comingSoon')}</span>{/if}</label
				>
			{/each}
			<span class="custom" aria-live="polite">{current === 'custom' ? t('settings.users.custom') : ''}</span>
		</div>
		{#each presetsFor(grantable).filter((x) => x.soon) as pr (pr.id)}
			<p id="{name}-{pr.id}-why" class="xs muted">{pr.soon}</p>
		{/each}
	</fieldset>
	{#if max}
		<div class="field">
			<span class="label" aria-hidden="true">{t('settings.users.ageRating')}</span>
			<SegmentedControl
				label={t('settings.users.ageRating')}
				testid="{name}-rating"
				options={ratingChoices.map((c) => ({ value: c.value, label: c.value }))}
				value={max}
				onchange={(v) => setMax(v)}
			/>
			<span class="hint" aria-live="polite">{ratingChoices.find((c) => c.value === max)?.hint ?? ''}</span>
		</div>
	{/if}
{/snippet}

<dialog bind:this={devDialog} class="dlg" aria-labelledby="h-devices" onclose={() => (devicesOf = null)}>
	{#if devicesOf}
		<h2 id="h-devices" class="h2">{t('settings.users.devicesOf', { name: devicesOf.name })}</h2>
		<Sessions path="/users/{devicesOf.id}/sessions" name={devicesOf.name} />
		<div class="row end" style="margin-top: var(--s-4)">
			<Button variant="plain" type="button" onclick={() => devDialog?.close()}>{t('settings.users.close')}</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={pwDialog} class="dlg" aria-labelledby="h-password" onclose={() => { pwFor = null; pwNew = ''; }}>
	{#if pwFor}
		<form onsubmit={savePassword}>
			<h2 id="h-password" class="h2">{t('settings.users.newPasswordFor', { name: pwFor.name })}</h2>
			<p class="small muted" style="margin-bottom: var(--s-3)">{t('settings.users.passwordNote', { name: pwFor.name })}</p>
			<Field id="pw-new" label={t('settings.users.newPasswordLabel')}>
				<input id="pw-new" class="input" type="password" autocomplete="new-password" bind:value={pwNew} required />
			</Field>
			<div class="row end" style="margin-top: var(--s-4)">
				<Button variant="plain" type="button" onclick={() => pwDialog?.close()}>{t('settings.users.cancel')}</Button>
				<Button type="submit" disabled={!pwNew}>{t('settings.users.setPasswordLabel')}</Button>
			</div>
		</form>
	{/if}
</dialog>

<dialog bind:this={dialog} class="dlg" aria-labelledby="h-edit" onclose={() => (editing = null)}>
	{#if editing}
		<form onsubmit={saveEdit}>
			<h2 id="h-edit" class="h2">{t('settings.users.change2', { name: editing.name })}</h2>
			{#if isOwner}
				<fieldset class="field">
					<legend class="label">{t('settings.users.role')}</legend>
					<div class="row">
						<label class="checkline"><input type="radio" name="edit-role" value="member" bind:group={editRole} onchange={() => { editPerms = [...defaults.member]; editMax = ''; }} /> {t('settings.users.member')}</label>
						<label class="checkline"><input type="radio" name="edit-role" value="admin" bind:group={editRole} onchange={() => { editPerms = [...defaults.admin]; editMax = ''; }} /> {t('settings.users.admin')}</label>
					</div>
				</fieldset>
			{/if}
			{#if editRole === 'member'}
				{@render presetPicker(editPerms, (next) => (editPerms = next), 'edit-preset', editMax, (m) => (editMax = m))}
			{/if}
			<fieldset class="field">
				<legend class="label">{t('settings.users.may')}</legend>
				{#each grantable as p (p)}
					<label class="checkline"
						><input type="checkbox" checked={editPerms.includes(p)} onchange={(e) => (editPerms = toggle(editPerms, p, e.currentTarget.checked))} />
						{permWord[p]}</label
					>
				{/each}
			</fieldset>
			{#if limits[editing.id] && !limits[editing.id].no_limit}
				<fieldset class="field" aria-describedby="limits-hint">
					<legend class="label">{t('settings.users.weeklyLimit')}</legend>
					<div class="limit-row">
						{#each limitFields as f (f.kind)}
							<Field id="limit-{f.kind}" label={f.label}>
								<input
									id="limit-{f.kind}"
									class="input"
									type="number"
									min="0"
									max="1000"
									inputmode="numeric"
									placeholder={t('settings.users.defaultN', { n: defaultsPerWeek[f.kind] ?? '?' })}
									bind:value={editLimits[f.kind]}
								/>
							</Field>
						{/each}
					</div>
					<span id="limits-hint" class="hint">{t('settings.users.limitsHint')}</span>
				</fieldset>
			{:else if limits[editing.id]?.no_limit}
				<p class="hint">{t('settings.users.noWeeklyLimit', { name: editing.name })}</p>
			{/if}
			<div class="row end">
				<Button variant="plain" type="button" onclick={() => dialog?.close()}>{t('settings.users.cancel')}</Button>
				<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>{t('settings.users.save')}</Button>
			</div>
		</form>
	{/if}
</dialog>

<Sheet bind:open={inviteOpen} title={t('settings.users.invite')} size="md">
	<div id="invite-card">
		<p class="small muted" style="margin-bottom: var(--s-4)">{t('settings.users.oneTime')}</p>
		<form class="invite" onsubmit={makeInvite}>
			<fieldset class="field">
				<legend class="label">{t('settings.users.role')}</legend>
				<div class="row">
					<label class="checkline"><input type="radio" name="role" checked={role === 'member'} onchange={() => pickRole('member')} /> {t('settings.users.member')}</label>
					{#if isOwner}
						<label class="checkline"><input type="radio" name="role" checked={role === 'admin'} onchange={() => pickRole('admin')} /> {t('settings.users.admin')}</label>
					{/if}
				</div>
			</fieldset>
			{@render presetPicker(perms, (next) => (perms = next), 'invite-preset', maxRating, (m) => (maxRating = m))}
			<fieldset class="field">
				<legend class="label">{t('settings.users.may')}</legend>
				{#each grantable as p (p)}
					<label class="checkline"
						><input type="checkbox" checked={perms.includes(p)} onchange={(e) => (perms = toggle(perms, p, e.currentTarget.checked))} />
						{permWord[p]}</label
					>
				{/each}
			</fieldset>
			<div class="row">
				<div class="field grow">
					<label for="inv-note">{t('settings.users.note')}</label>
					<input id="inv-note" class="input" bind:value={note} maxlength="100" placeholder={t('settings.users.noteExample')} />
				</div>
				<Field id="inv-days" label={t('settings.users.days')}>
					<input id="inv-days" class="input" type="number" min="1" max="30" bind:value={days} style="max-width: 120px" />
				</Field>
			</div>
			<Button icon="plus" variant="primary" type="submit" disabled={making} aria-busy={making}>{t('settings.users.makeInvite')}</Button>
		</form>
	</div>
</Sheet>

<style>
	.sso-links { list-style: none; margin: 4px 0 0; padding: 0; display: grid; gap: 2px; }
	.sso-links li { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; min-width: 0; }
	.sso-links .mono { overflow-wrap: anywhere; }
	.name {
		font-weight: var(--weight-semibold);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.key {
		padding: 10px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		overflow-wrap: anywhere;
		font-size: var(--text-sm);
		font-family: var(--font-mono);
	}
	fieldset {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-3);
	}
	/* A floated legend is a normal child, so it takes the label column like other Field rows. */
	.invite fieldset.field > legend {
		float: left;
	}
	.presets { align-items: center; }
	.soon { color: var(--text-muted); }
	.soon .chip { margin-left: 6px; }
	.custom { font-weight: 600; }
	.invite :global(button[type='submit']) {
		margin-top: var(--s-3);
	}
	.person {
		flex-wrap: wrap;
	}
	.acts {
		flex-wrap: wrap;
	}
	.dlg {
		width: min(460px, calc(100vw - 32px));
		background: var(--surface-1);
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: var(--r-modal);
		padding: var(--s-6);
	}
	.dlg::backdrop {
		background: var(--scrim);
	}
	.dlg h2 {
		margin-bottom: var(--s-4);
	}
	.limit-row {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--s-2);
	}
	@media (max-width: 600px) {
		.limit-row {
			grid-template-columns: 1fr;
		}
	}
	.end {
		justify-content: flex-end;
		margin-top: var(--s-4);
	}
</style>
