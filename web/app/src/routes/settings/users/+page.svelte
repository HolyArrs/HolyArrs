<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session, type Perm, type Role } from '#lib/session.svelte.ts';
	import { presetOf, presetPerms, presetsFor, type PresetId } from '#lib/presets.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, date } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import Sessions from '#lib/Sessions.svelte';
	import type { AskSettings, LimitKind, PersonLimit } from '#lib/types.ts';
	import { kindAsks } from '#lib/requests.ts';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { kidLabel, ratingChoices } from '#lib/kid.ts';
	import Avatar from '#lib/ui/Avatar.svelte';

	interface User {
		id: number;
		name: string;
		role: Role;
		state: 'active' | 'disabled' | 'locked';
		perms: Perm[];
		two_factor: boolean;
		password: boolean;
		links: { provider: string; name: string }[];
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

	const permWord: Record<Perm, string> = {
		ask: 'Ask for titles',
		auto_approve: 'Asks approved without waiting',
		approve: 'Approve or turn down asks',
		manage_library: 'Run the Library',
		settings: 'Change settings'
	};
	const defaults: Record<'admin' | 'member', Perm[]> = {
		admin: ['ask', 'auto_approve', 'approve', 'manage_library'],
		member: ['ask']
	};
	const roleWord: Record<string, string> = { owner: 'Owner', admin: 'Admin', member: 'Member' };
	const providerWord: Record<string, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };

	// Weekly ask limits, one per kind, from Request rules (absent when it is off).
	interface LimitRow extends PersonLimit {
		user_id: number;
		used: Record<LimitKind, number>;
		no_limit: boolean;
	}
	const limitFields = [
		{ kind: 'movies', key: 'movies_per_week', label: 'Movie asks' },
		{ kind: 'shows', key: 'shows_per_week', label: 'Show asks' },
		{ kind: 'music', key: 'music_per_week', label: 'Music asks' }
	] as const;
	let limits = $state<Record<number, LimitRow>>({});
	let defaultsPerWeek = $state<Partial<Record<LimitKind, number>>>({});
	let editLimits = $state<Record<LimitKind, string>>({ movies: '', shows: '', music: '' });

	function limitWords(l: LimitRow | undefined): string {
		if (!l) return '';
		if (l.no_limit) return 'No ask limit (may approve)';
		return limitFields
			.map((f) => {
				const own = l[f.key];
				const per = own ?? defaultsPerWeek[f.kind];
				const used = l.used?.[f.kind] ?? 0;
				if (per === 0) return `${f.label.toLowerCase()}: no limit`;
				return `${kindAsks(f.kind, used)} of ${per ?? 'the default'}${own === null ? '' : ' (own)'}`;
			})
			.join(' · ');
	}

	let users = $state<User[]>([]);
	let invites = $state<Invite[]>([]);
	let allPerms = $state<Perm[]>(['ask', 'auto_approve', 'approve', 'manage_library', 'settings']);
	let loaded = $state(false);
	let error = $state('');

	// New invite
	let role = $state<'admin' | 'member'>('member');
	let perms = $state<Perm[]>([...defaults.member]);
	let maxRating = $state('');
	let note = $state('');
	let days = $state(7);
	let making = $state(false);
	let link = $state('');

	// Editing one user
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
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	function loadLimits() {
		api
			.get<{ limits: LimitRow[] }>('/ask-limits')
			.then((r) => (limits = Object.fromEntries(r.limits.map((l) => [l.user_id, l]))))
			.catch(() => (limits = {}));
		api
			.get<AskSettings>('/settings/asks')
			.then((s) => (defaultsPerWeek = { movies: s.movies_per_week, shows: s.shows_per_week, music: s.music_per_week }))
			.catch(() => {});
	}

	/** limitValue reads one box: '' is the default, otherwise 0 to 1000. */
	function limitValue(v: string | number): number | null | undefined {
		const t = String(v ?? '').trim();
		if (t === '') return null;
		const n = Number(t);
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

	async function makeInvite(e: SubmitEvent) {
		e.preventDefault();
		making = true;
		try {
			const r = await api.post<{ path: string }>('/users/invites', { role, perms, note: note.trim(), days, max_rating: maxRating, rating_country: 'US' });
			link = location.origin + to(r.path);
			note = '';
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			making = false;
		}
	}

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(link);
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; select the link and copy it by hand.', 'fail');
		}
	}

	async function takeBack(i: Invite) {
		try {
			await api.del(`/users/invites/${i.id}`);
			invites = invites.filter((x) => x.id !== i.id);
			toasts.push('Invite taken back.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	// Signed-in devices: the owner sees everyone's, an admin members' (their
	// own are in My account).
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

	function mayChange(u: User): boolean {
		if (u.role === 'owner' || u.name === session.s.name) return false;
		return isOwner || u.role === 'member';
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
			toasts.push(`New password set for ${u.name}. They are signed out everywhere.`, 'ok');
			pwDialog?.close();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
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
			toasts.push(message(e), 'fail');
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
				toasts.push('Each weekly limit is a whole number from 0 to 1000, or empty for the default.', 'fail');
				return;
			}
			try {
				await api.put(`/ask-limits/${u.id}`, own);
			} catch (err) {
				toasts.push(message(err), 'fail');
				return;
			}
		}
		if (await put(u, body, `${u.name} saved.`)) dialog?.close();
	}

	async function setState(u: User, state: 'active' | 'disabled') {
		if (state === 'disabled' && !confirm(`Turn off ${u.name}? They are signed out everywhere and cannot sign in until you turn them back on.`)) return;
		await put(u, { state }, state === 'disabled' ? `${u.name} turned off.` : `${u.name} turned on.`);
	}

	async function resetTwoFactor(u: User) {
		if (!confirm(`Reset two-factor sign-in for ${u.name}? They sign in with their password alone until they set it up again.`)) return;
		try {
			await api.post(`/users/${u.id}/two-factor/reset`);
			toasts.push(`Two-factor sign-in reset for ${u.name}.`, 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	function inviteState(i: Invite): string {
		if (i.used_at) return `Used by ${i.used_by ?? 'someone'} ${ago(i.used_at)}`;
		if (new Date(i.expires_at).getTime() < Date.now()) return 'Ended';
		return `Open until ${date(i.expires_at)}`;
	}
</script>

<svelte:head><title>Users · HolyArrs</title></svelte:head>

<PageHeader title="Users and roles" />

{#if !loaded}
	<SkSettings rows={3} />
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else}
	<SettingsCard id="invite-card" title="Invite someone">
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">Send them a one-time link. They pick their own name and password.</p>

		{#if link}
			<div class="fresh" role="status">
				<p class="small"><b>Invite link.</b> It works once and is shown only now.</p>
				<div class="row">
					<code class="key grow">{link}</code>
					<Button icon="copy" type="button" onclick={copyLink}>Copy</Button>
					<Button variant="plain" type="button" onclick={() => (link = '')}>Done</Button>
				</div>
			</div>
		{/if}

		<form class="invite" onsubmit={makeInvite}>
			<fieldset class="field">
				<legend class="label">Role</legend>
				<div class="row">
					<label class="checkline"><input type="radio" name="role" checked={role === 'member'} onchange={() => pickRole('member')} /> Member</label>
					{#if isOwner}
						<label class="checkline"><input type="radio" name="role" checked={role === 'admin'} onchange={() => pickRole('admin')} /> Admin</label>
					{/if}
				</div>
			</fieldset>
			{@render presetPicker(perms, (next) => (perms = next), 'invite-preset', maxRating, (m) => (maxRating = m))}
			<fieldset class="field">
				<legend class="label">May</legend>
				{#each grantable as p (p)}
					<label class="checkline"
						><input type="checkbox" checked={perms.includes(p)} onchange={(e) => (perms = toggle(perms, p, e.currentTarget.checked))} />
						{permWord[p]}</label
					>
				{/each}
			</fieldset>
			<div class="row">
				<div class="field grow">
					<label for="inv-note">Note for yourself</label>
					<input id="inv-note" class="input" bind:value={note} maxlength="100" placeholder="Who is this for?" />
				</div>
				<Field id="inv-days" label="Days the link lasts">
					<input id="inv-days" class="input" type="number" min="1" max="30" bind:value={days} style="max-width: 120px" />
				</Field>
			</div>
			<Button icon="plus" variant="primary" type="submit" disabled={making} aria-busy={making}>Make an invite link</Button>
		</form>
	</SettingsCard>

	<SettingsCard id="people-card" title="People">
		<ul class="rows" style="margin-top: var(--s-3)">
			{#each users as u (u.id)}
				<li class="person">
					<Avatar name={u.name} src={u.avatar} size={40} />
					<div class="grow">
						<div class="name">
							{u.name}
							<span class="chip {u.role === 'member' ? '' : 'red'}">{roleWord[u.role]}</span>
							{#if u.state !== 'active'}<span class="chip fail">Off</span>{/if}
							{#if u.two_factor}<span class="chip ok">Two-factor</span>{/if}
							{#if u.max_rating}<span class="chip" data-testid="kid-label">{kidLabel(u.max_rating)}</span>{/if}
						</div>
						<div class="xs muted">
							{#if u.links.length}{u.links.map((l) => `${providerWord[l.provider] ?? l.provider}: ${l.name}`).join(' · ')} ·
							{/if}{u.role === 'owner' ? 'Can do everything' : u.perms.map((p) => permWord[p]).join(', ') || 'Nothing yet'} ·
							{u.last_seen_at ? `seen ${ago(u.last_seen_at)}` : 'not seen yet'}
						</div>
						{#if limits[u.id] && u.role !== 'owner'}
							<div class="xs muted" data-testid="user-limits">This week: {limitWords(limits[u.id])}</div>
						{/if}
					</div>
					{#if mayChange(u) || maySeeDevices(u)}
						<div class="row acts">
							{#if maySeeDevices(u)}
								<Button variant="plain" type="button" onclick={() => openDevices(u)} aria-label="Signed-in devices of {u.name}">Devices</Button>
							{/if}
						{#if mayChange(u)}
							<Button type="button" onclick={() => openEdit(u)} aria-label="Change {u.name}">Change</Button>
							{#if u.two_factor}
								<Button variant="plain" type="button" onclick={() => resetTwoFactor(u)} aria-label="Reset two-factor sign-in for {u.name}"
									>Reset two-factor</Button>
							{/if}
							{#if u.state === 'active'}
							<Button variant="plain" type="button" onclick={() => openPassword(u)} aria-label="Set a new password for {u.name}">Set password</Button>
								<Button variant="destructive" type="button" onclick={() => setState(u, 'disabled')} disabled={saving} aria-label="Turn off {u.name}"
									>Turn off</Button>
							{:else}
								<Button type="button" onclick={() => setState(u, 'active')} disabled={saving} aria-label="Turn on {u.name}"
									>Turn on</Button>
							{/if}
						{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	</SettingsCard>

	<SettingsCard id="invites-card" title="Invite links">
		{#if invites.length === 0}
			<p class="small muted" style="margin-top: var(--s-3)">No invites yet.</p>
		{:else}
			<ul class="rows" style="margin-top: var(--s-3)">
				{#each invites as i (i.id)}
					<li>
						<div class="grow">
							<div class="name">{i.note || `${roleWord[i.role]} invite`} <span class="chip">{roleWord[i.role]}</span>{#if i.max_rating}<span class="chip">{kidLabel(i.max_rating)}</span>{/if}</div>
							<div class="xs muted">Made by {i.created_by} {ago(i.created_at)} · {inviteState(i)}</div>
						</div>
						{#if !i.used_at && new Date(i.expires_at).getTime() > Date.now()}
							<Button variant="plain" type="button" onclick={() => takeBack(i)}>Take back</Button>
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
		<legend class="label">Preset</legend>
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
					{pr.label}{#if pr.soon}<span class="chip">Coming soon</span>{/if}</label
				>
			{/each}
			<span class="custom" aria-live="polite">{current === 'custom' ? 'Custom' : ''}</span>
		</div>
		{#each presetsFor(grantable).filter((x) => x.soon) as pr (pr.id)}
			<p id="{name}-{pr.id}-why" class="xs muted">{pr.soon}</p>
		{/each}
	</fieldset>
	{#if max}
		<div class="field">
			<span class="label" aria-hidden="true">Age rating</span>
			<SegmentedControl
				label="Age rating"
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
		<h2 id="h-devices" class="h2">Signed-in devices of {devicesOf.name}</h2>
		<p class="small muted" style="margin-bottom: var(--s-3)">Signing out ends the session at once; they sign in again with their password.</p>
		<Sessions path="/users/{devicesOf.id}/sessions" name={devicesOf.name} />
		<div class="row end" style="margin-top: var(--s-4)">
			<Button variant="plain" type="button" onclick={() => devDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={dialog} class="dlg" aria-labelledby="h-edit" onclose={() => (editing = null)}>
	{#if editing}
		<form onsubmit={saveEdit}>
			<h2 id="h-edit" class="h2">Change {editing.name}</h2>
			{#if isOwner}
<dialog bind:this={pwDialog} class="dlg" aria-labelledby="h-password" onclose={() => { pwFor = null; pwNew = ''; }}>
	{#if pwFor}
		<form onsubmit={savePassword}>
			<h2 id="h-password" class="h2">New password for {pwFor.name}</h2>
			<p class="small muted" style="margin-bottom: var(--s-3)">Use this when {pwFor.name} forgot theirs. They are signed out everywhere and sign in again with the new one. Tell them it in person.</p>
			<Field id="pw-new" label="New password">
				<input id="pw-new" class="input" type="password" autocomplete="new-password" bind:value={pwNew} required />
			</Field>
			<div class="row end" style="margin-top: var(--s-4)">
				<Button variant="plain" type="button" onclick={() => pwDialog?.close()}>Cancel</Button>
				<Button type="submit" disabled={!pwNew}>Set password</Button>
			</div>
		</form>
	{/if}
</dialog>

				<fieldset class="field">
					<legend class="label">Role</legend>
					<div class="row">
						<label class="checkline"><input type="radio" name="edit-role" value="member" bind:group={editRole} onchange={() => { editPerms = [...defaults.member]; editMax = ''; }} /> Member</label>
						<label class="checkline"><input type="radio" name="edit-role" value="admin" bind:group={editRole} onchange={() => { editPerms = [...defaults.admin]; editMax = ''; }} /> Admin</label>
					</div>
				</fieldset>
			{/if}
			{#if editRole === 'member'}
				{@render presetPicker(editPerms, (next) => (editPerms = next), 'edit-preset', editMax, (m) => (editMax = m))}
			{/if}
			<fieldset class="field">
				<legend class="label">May</legend>
				{#each grantable as p (p)}
					<label class="checkline"
						><input type="checkbox" checked={editPerms.includes(p)} onchange={(e) => (editPerms = toggle(editPerms, p, e.currentTarget.checked))} />
						{permWord[p]}</label
					>
				{/each}
			</fieldset>
			{#if limits[editing.id] && !limits[editing.id].no_limit}
				<fieldset class="field" aria-describedby="limits-hint">
					<legend class="label">Weekly asks</legend>
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
									placeholder="Default ({defaultsPerWeek[f.kind] ?? '?'})"
									bind:value={editLimits[f.kind]}
								/>
							</Field>
						{/each}
					</div>
					<span id="limits-hint" class="hint">Empty uses the default from Settings &gt; Request rules. 0 means no limit. Each kind counts on its own; joining someone else's ask is free.</span>
				</fieldset>
			{:else if limits[editing.id]?.no_limit}
				<p class="hint">No weekly ask limit: {editing.name} may approve asks.</p>
			{/if}
			<div class="row end">
				<Button variant="plain" type="button" onclick={() => dialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>Save</Button>
			</div>
		</form>
	{/if}
</dialog>

<style>
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
