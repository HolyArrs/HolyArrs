<script lang="ts">
	import { personName } from '#lib/plain.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import { api, message, ApiError } from './api.ts';
	import { session } from './session.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import Icon from './Icon.svelte';
	import Sessions from './Sessions.svelte';
	import Confirm from './account/Confirm.svelte';
	import QrCode from './account/QrCode.svelte';
	import { needsConfirm } from './account/sections.ts';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from './paths.ts';
	import { setFollow } from './follow.ts';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { autoplayPref, setAutoplayPref, type AutoplayPref } from './trailer.ts';
	import { profile, loadProfile, saveDisplayName } from './account/profile.svelte.ts';
	import { displayNameError, roleWord, shownName } from './account/profile.ts';
	import { showWatchlistCard, watchlistStateWord, type PlexWatchlist } from './account/watchlist.ts';

	// Display name: edited inline; Save is on once it differs.
	let nameDraft = $state('');
	let nameEditing = $state(false);
	let nameBusy = $state(false);
	let nameError = $state('');
	const nameProblem = $derived(displayNameError(nameDraft));
	function startName() {
		nameDraft = profile.displayName;
		nameError = '';
		nameEditing = true;
	}
	async function saveName(e: SubmitEvent) {
		e.preventDefault();
		if (nameProblem) return void (nameError = nameProblem);
		nameBusy = true;
		nameError = '';
		try {
			await saveDisplayName(nameDraft);
			nameEditing = false;
			toasts.push('Display name saved.', 'ok');
		} catch (err) {
			nameError = message(err);
		} finally {
			nameBusy = false;
		}
	}

	// Where Alerts reach this person (email, Discord).
	let targets = $state<{ email: string; discord_id: string; new_episodes?: boolean }>({ email: '', discord_id: '', new_episodes: true });
	// Follow: the shows and artists I follow.
	type Followed = { title_id: number; kind: string; name: string; year?: number };
	let follows = $state<Followed[]>([]);
	async function unfollow(f: Followed) {
		try {
			await setFollow(api, f.title_id, false);
			follows = follows.filter((x) => x.title_id !== f.title_id);
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}
	// Reset my recommendations: Explore's category strip forgets what it learned from my opens.
	let resetBusy = $state(false);
	async function resetRecommendations() {
		resetBusy = true;
		try {
			await api.del('/explore/strip/counts');
			toasts.push('Your recommendations start fresh.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			resetBusy = false;
		}
	}
	// Plex watchlist: new entries are asked for as me (when the owner allows it).
	let watch = $state<PlexWatchlist | null>(null);
	let watchBusy = $state(false);
	async function setWatch(kind: 'movies' | 'shows', on: boolean) {
		if (!watch) return;
		watchBusy = true;
		try {
			watch = await api.put<PlexWatchlist>('/my/plex-watchlist', { movies: watch.movies, shows: watch.shows, [kind]: on });
			toasts.push(on ? 'HolyArrs reads your Plex watchlist now and every few minutes.' : 'Saved.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
			watch = await api.get<PlexWatchlist>('/my/plex-watchlist').catch(() => watch);
		} finally {
			watchBusy = false;
		}
	}
	async function checkWatch() {
		watchBusy = true;
		try {
			await api.post('/my/plex-watchlist/check', {});
			toasts.push('Reading your Plex watchlist now.', 'ok');
			setTimeout(() => void api.get<PlexWatchlist>('/my/plex-watchlist').then((r) => (watch = r)).catch(() => {}), 4000);
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			watchBusy = false;
		}
	}
	let savedTargets = $state('');
	const alertsDirty = $derived(savedTargets !== '' && JSON.stringify(targets) !== savedTargets);
	let tgBusy = $state(false);
	let tgError = $state('');
	onMount(() => {
		void loadProfile();
		void loadLinks();
		void loadPlexMine();
		void session.loadWays().then(() => {
			if (page.url.searchParams.get('sso') === 'finish') void finishOIDCLink();
		});
		api
			.get<{ email: string; discord_id: string; new_episodes?: boolean }>('/my/alerts')
			.then((r) => {
				targets = r;
				savedTargets = JSON.stringify(r);
			})
			.catch(() => {});
		api
			.get<PlexWatchlist>('/my/plex-watchlist')
			.then((r) => (watch = r))
			.catch(() => {});
		api
			.get<{ follows: Followed[] }>('/my/follows')
			.then((r) => (follows = r.follows))
			.catch(() => {});
	});
	async function saveTargets(e: SubmitEvent) {
		e.preventDefault();
		tgBusy = true;
		tgError = '';
		try {
			targets = await api.put('/my/alerts', targets);
			savedTargets = JSON.stringify(targets);
			toasts.push('Saved where Alerts reach you.', 'ok');
		} catch (err) {
			tgError = message(err);
		} finally {
			tgBusy = false;
		}
	}

	const MIN_LENGTH = $derived(session.s.minPassword);
	const via: Record<string, string> = {
		session: 'with your password or a linked account',
		api_key: 'with an API key',
		local_network: 'from your local network, without a password'
	};
	function authMethodWord(method: string): string {
		if (method === 'local_password' || method === 'local_setup') return 'HolyArrs password';
		if (method === 'oidc' || method.startsWith('oidc:')) return 'single sign-on';
		if (method === 'forward_auth' || method.startsWith('forward_auth:')) return 'trusted proxy';
		if (method === 'legacy') return 'an older session';
		const server: Record<string, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
		return server[method] ?? method;
	}
	const viaShort: Record<string, string> = { session: 'Password or linked account', api_key: 'API key', local_network: 'Local network' };

	// Password
	let current = $state('');
	let next = $state('');
	let again = $state('');
	let pwError = $state('');
	let pwBusy = $state(false);

	// Two-factor
	let setup = $state<{ secret: string; uri: string; qr?: string[] } | null>(null);
	let code = $state('');
	let offCode = $state('');
	let tfError = $state('');
	let tfBusy = $state(false);
	let codes = $state<string[]>([]);

	// A linked identity changes how this account can sign in, so it first
	// requires a recent proof with this account's HolyArrs password.
	// Password and linking open inline; Confirm asks for the password first when needed.
	let pwOpen = $state(false);
	let linkOpen = $state(false);
	let linkError = $state('');
	let linkBusy = $state(false);
	const hasLinkWay = $derived(session.s.ways.includes('oidc') || session.s.ways.includes('forward_auth'));

	function strong(e: unknown): string {
		return e instanceof ApiError && e.status === 403 && session.s.via === 'local_network'
			? 'Sign in with your password for this.'
			: message(e);
	}

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		pwError = '';
		if (next.length < MIN_LENGTH) return void (pwError = `The new password needs at least ${MIN_LENGTH} characters.`);
		if (next !== again) return void (pwError = 'The two new passwords are not the same.');
		pwBusy = true;
		const first = !session.s.hasPassword;
		try {
			await api.post('/auth/password', first ? { new: next } : { current, new: next });
			current = next = again = '';
			pwOpen = false;
			if (first) session.s.hasPassword = true;
			toasts.push(first ? `Password set. You can also sign in as ${session.s.name} with it.` : 'Password changed. Your other devices are signed out.', 'ok');
		} catch (err) {
			pwError = strong(err);
		} finally {
			pwBusy = false;
		}
	}

	async function startTwoFactor() {
		tfError = '';
		tfBusy = true;
		try {
			setup = await api.post<{ secret: string; uri: string; qr?: string[] }>('/auth/two-factor/setup');
		} catch (err) {
			tfError = strong(err);
		} finally {
			tfBusy = false;
		}
	}

	async function enable(e: SubmitEvent) {
		e.preventDefault();
		tfError = '';
		tfBusy = true;
		try {
			const r = await api.post<{ backup_codes: string[] }>('/auth/two-factor/enable', { code: code.trim() });
			codes = r.backup_codes;
			setup = null;
			tfOpen = false;
			code = '';
			await session.load();
			toasts.push('Two-factor sign-in is on.', 'ok');
		} catch (err) {
			tfError = strong(err);
		} finally {
			tfBusy = false;
		}
	}

	async function withCode(path: string, e: SubmitEvent) {
		e.preventDefault();
		tfError = '';
		tfBusy = true;
		try {
			const r = await api.post<{ backup_codes?: string[] } | undefined>(path, { code: offCode.trim() });
			offCode = '';
			if (r?.backup_codes) {
				codes = r.backup_codes;
				toasts.push('New backup codes made. The old ones no longer work.', 'ok');
			} else {
				codes = [];
				tfOpen = false;
				await session.load();
				toasts.push('Two-factor sign-in is off.', 'ok');
			}
		} catch (err) {
			tfError = strong(err);
		} finally {
			tfBusy = false;
		}
	}

	let offMode = $state<'off' | 'codes'>('codes');
	// Two-factor opens below its row; setting up goes straight on when no re-confirm is needed.
	let tfOpen = $state(false);
	function openTwoFactor() {
		tfOpen = true;
		if (!session.s.twoFactor && !needsConfirm('two-factor', { localRecent: session.s.localRecent, via: session.s.via, mustTwoFactor: session.s.mustTwoFactor })) void startTwoFactor();
	}

	async function copyCodes() {
		try {
			await navigator.clipboard.writeText(codes.join('\n'));
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; write the codes down by hand.', 'fail');
		}
	}

	// Single sign-on and trusted proxy identities linked to me. Unlinking one
	// also signs out the sessions it made; it needs a fresh password sign-in.
	type SsoLink = { kind: 'oidc' | 'forward_auth'; issuer: string; name: string; created_at: string };
	let myLinks = $state<SsoLink[]>([]);
	let unlinkKey = $state('');
	const linkKey = (l: SsoLink) => `${l.kind}|${l.issuer}`;
	const linkWord = (k: string) => (k === 'oidc' ? 'Single sign-on' : 'Trusted proxy');
	const hostOf = (u: string) => {
		try {
			return new URL(u).host;
		} catch {
			return u;
		}
	};
	async function loadLinks() {
		try {
			myLinks = (await api.get<{ links: SsoLink[] }>('/auth/sso/links')).links;
		} catch {
			myLinks = [];
		}
	}
	async function unlink(l: SsoLink) {
		linkError = '';
		linkBusy = true;
		try {
			await api.post('/auth/sso/unlink', { kind: l.kind, issuer: l.issuer });
			toasts.push(`${linkWord(l.kind)} unlinked. Sessions it signed in are signed out.`, 'ok');
			unlinkKey = '';
			await loadLinks();
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}

	async function startOIDCLink() {
		linkError = '';
		linkBusy = true;
		try {
			const { auth_url } = await session.oidcLink();
			window.location.assign(auth_url);
		} catch (err) {
			linkError = message(err);
			linkBusy = false;
		}
	}

	async function finishOIDCLink() {
		linkError = '';
		linkBusy = true;
		try {
			await session.oidcLinkFinish();
			toasts.push('Single sign-on linked.', 'ok');
			await loadLinks();
			await goto(to('/settings/account'), { replace: true });
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}

	async function linkForwardAuth() {
		linkError = '';
		linkBusy = true;
		try {
			await session.forwardLink();
			toasts.push('Trusted proxy sign-in linked.', 'ok');
			await loadLinks();
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}

	// My Plex account: linking keeps its sign-in on the server (never shown),
	// so my Plex watchlist can be read. Anyone may link one.
	type PlexMine = { linked: boolean; name: string; kept: boolean };
	let plexMine = $state<PlexMine | null>(null);
	let plexUnlinking = $state(false);
	async function loadPlexMine() {
		plexMine = await api.get<PlexMine>('/auth/plex/my-link').catch(() => null);
	}
	async function linkPlexMine() {
		linkError = '';
		linkBusy = true;
		const win = window.open('about:blank', 'plex-link', 'width=600,height=720');
		// The Plex page must not reach back into this tab.
		if (win) win.opener = null;
		try {
			const r = await api.post<{ id: number; auth_url: string }>('/auth/plex/my-link/start', {});
			if (!win) {
				linkError = 'Your browser blocked the Plex window. Allow pop-ups for HolyArrs and try again.';
				return;
			}
			win.location.href = r.auth_url;
			const until = Date.now() + 10 * 60 * 1000;
			while (Date.now() < until) {
				await new Promise((ok) => setTimeout(ok, 2000));
				let done: PlexMine | { waiting: true };
				try {
					done = await api.post<PlexMine | { waiting: true }>('/auth/plex/my-link/finish', { id: r.id });
				} catch (err) {
					if (err instanceof ApiError && err.status === 429) continue;
					throw err;
				}
				if ('waiting' in done) continue;
				plexMine = done;
				linkOpen = false;
				toasts.push(`Plex account ${done.name} linked.`, 'ok');
				return;
			}
			linkError = 'The Plex sign-in timed out. Start again.';
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
			try {
				win?.close();
			} catch {
				// the window may already be gone
			}
		}
	}
	async function unlinkPlexMine() {
		linkError = '';
		linkBusy = true;
		try {
			await api.del('/auth/plex/my-link');
			plexMine = { linked: false, name: '', kept: false };
			plexUnlinking = false;
			toasts.push('Plex account unlinked. Devices it signed in are signed out.', 'ok');
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}
	let pref = $state<AutoplayPref>(autoplayPref());

	// Profile picture: choose, see it in the circle, then save or cancel.
	const AVATAR_MAX = 5 * 1024 * 1024;
	let avatarInput: HTMLInputElement | undefined = $state();
	let picked = $state<File | null>(null);
	let preview = $state('');
	let avatarBusy = $state(false);
	function clearPick() {
		if (preview) URL.revokeObjectURL(preview);
		picked = null;
		preview = '';
		if (avatarInput) avatarInput.value = '';
	}
	function pickAvatar(e: Event) {
		const f = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!f) return;
		clearPick();
		if (!/^image\/(jpeg|png|webp)$/.test(f.type)) {
			toasts.push('Choose a JPEG, PNG or WebP picture.', 'fail');
			return;
		}
		if (f.size > AVATAR_MAX) {
			toasts.push('That picture is over 5 MB.', 'fail');
			return;
		}
		picked = f;
		preview = URL.createObjectURL(f);
	}
	async function saveAvatar() {
		if (!picked) return;
		avatarBusy = true;
		try {
			const r = await api.request<{ avatar: string | null }>('/account/avatar', { method: 'PUT', raw: picked });
			session.setAvatar(r.avatar);
			clearPick();
			toasts.push('Profile picture saved.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			avatarBusy = false;
		}
	}
	async function removeAvatar() {
		avatarBusy = true;
		try {
			await api.del('/account/avatar');
			session.setAvatar(null);
			clearPick();
			toasts.push('Profile picture removed.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			avatarBusy = false;
		}
	}
</script>

<PageHeader title="My account" />
<div class="acct">

{#if session.s.mustTwoFactor}
	<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on under Security to use the rest of HolyArrs.</p>
{/if}

<SettingsCard id="profile-me" title="Profile" description="How you show up across HolyArrs." icon="user">
	<div class="pfp">
		<Avatar name={shownName(profile.displayName, personName(session.s.name, 'You'))} src={preview || session.s.avatar} size={44} label="Your profile picture" />
		<input class="sr-only" type="file" accept="image/jpeg,image/png,image/webp" bind:this={avatarInput} onchange={pickAvatar} disabled={avatarBusy} aria-label="Choose a profile picture" />
		{#if picked}
			<Button variant="primary" type="button" onclick={saveAvatar} busy={avatarBusy}>Save</Button>
			<Button variant="plain" type="button" onclick={clearPick} disabled={avatarBusy}>Cancel</Button>
		{:else}
			<Button type="button" onclick={() => avatarInput?.click()} disabled={avatarBusy}>{session.s.avatar ? 'Change picture' : 'Add picture'}</Button>
			{#if session.s.avatar}<Button variant="plain" type="button" onclick={removeAvatar} disabled={avatarBusy}>Remove</Button>{/if}
		{/if}
		<span class="hint" title="JPEG, PNG or WebP up to 5 MB; cropped to a square.">Up to 5 MB.</span>
	</div>
	<div class="boxes" data-testid="profile-boxes">
		<div class="box wide" data-testid="box-display-name">
			<span class="label" id="dn-label">Display name</span>
			{#if nameEditing}
				<form class="dn" onsubmit={saveName} novalidate>
					<input class="input" aria-labelledby="dn-label" bind:value={nameDraft} maxlength="64" autocomplete="nickname" placeholder={session.s.name} />
					<Button variant="primary" type="submit" busy={nameBusy} disabled={nameBusy || nameDraft.trim() === profile.displayName || !!nameProblem}>Save</Button>
					<Button variant="plain" type="button" onclick={() => (nameEditing = false)} disabled={nameBusy}>Cancel</Button>
				</form>
				{#if nameError || nameProblem}<p class="form-error" role="alert">{nameError || nameProblem}</p>{/if}
			{:else}
				<div class="line">
					<span class="value">{profile.displayName || session.s.name}</span>
					<Button variant="plain" type="button" onclick={startName} data-testid="edit-display-name">Edit</Button>
				</div>
			{/if}
		</div>
		<div class="box" data-testid="box-username">
			<span class="label">Username</span>
			<div class="line">
				<span class="value">{session.s.name}</span>
				<span class="lock" title="You sign in with this name, so it does not change here." role="img" aria-label="Locked: you sign in with this name"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
			</div>
		</div>
		<div class="box" data-testid="box-role">
			<span class="label">Role</span>
			<div class="line">
				<span class="value">{roleWord(session.s.role)}</span>
				<span class="lock" title="Ask an owner to change your role" role="img" aria-label="Locked: ask an owner to change your role"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
			</div>
		</div>
		<div class="box" data-testid="box-sign-in">
			<span class="label">Sign-in method</span>
			<div class="line"><span class="value" title={via[session.s.via] ?? session.s.via}>{session.s.authMethod ? authMethodWord(session.s.authMethod) : (viaShort[session.s.via] ?? session.s.via)}</span></div>
		</div>
		<div class="box" class:wide={pwOpen} data-testid="box-password">
			<span class="label">Password</span>
			{#if !session.s.hasPassword && !session.s.outsideRecent}
				<span class="hint">None yet. To set one, sign out and sign in again with {authMethodWord(session.s.authMethod)}, then come back here within 10 minutes.</span>
			{:else if !session.s.hasPassword && !pwOpen}
				<div class="line"><Button type="button" onclick={() => (pwOpen = true)} data-testid="change-password">Set a password</Button></div>
				<span class="hint">So you can still sign in when {authMethodWord(session.s.authMethod)} is down.</span>
			{:else if pwOpen}
				<Confirm action="password" onCancel={() => (pwOpen = false)}>
					<form class="pw" onsubmit={changePassword} novalidate>
						<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
						{#if session.s.hasPassword}
							<label class="sr-only" for="pw-current">Current password</label>
							<input id="pw-current" class="input" type="password" placeholder="Current password" bind:value={current} autocomplete="current-password" />
						{/if}
						<label class="sr-only" for="pw-new">New password</label>
						<input id="pw-new" class="input" type="password" placeholder="New password (at least {MIN_LENGTH})" bind:value={next} autocomplete="new-password" />
						<label class="sr-only" for="pw-again">Confirm new password</label>
						<input id="pw-again" class="input" type="password" placeholder="Confirm new password" bind:value={again} autocomplete="new-password" />
						{#if pwError}<p class="form-error" role="alert">{pwError}</p>{/if}
						<div class="row acts">
							<Button variant="primary" type="submit" disabled={pwBusy || !next} busy={pwBusy}>{session.s.hasPassword ? 'Change password' : 'Set password'}</Button>
							<Button variant="plain" type="button" onclick={() => (pwOpen = false)}>Cancel</Button>
						</div>
						<span class="hint">{session.s.hasPassword ? 'Signs out your other devices.' : `Your ${authMethodWord(session.s.authMethod)} sign-in keeps working.`}</span>
					</form>
				</Confirm>
			{:else}
				<div class="line"><Button type="button" onclick={() => (pwOpen = true)} data-testid="change-password">Change password</Button></div>
			{/if}
		</div>
	</div>
</SettingsCard>

<SettingsCard id="preferences-me" title="Preferences" description="Kept for you; trailers only on this device." icon="appearance">
	<div class="srow">
		<div class="txt"><span class="label">Trailers</span><span class="hint" title="Auto follows your system (reduced motion, data saver). Always plays them anyway; Never turns them off.">Auto follows reduced motion and data saver.</span></div>
		<div class="ctl"><SegmentedControl size="sm" label="Play trailers" options={[{ value: 'auto', label: 'Auto' }, { value: 'always', label: 'Always' }, { value: 'never', label: 'Never' }] as { value: AutoplayPref; label: string }[]} value={pref} onchange={(v) => { pref = v; setAutoplayPref(v); location.reload(); }} /></div>
	</div>
	<div class="srow" id="recommendations-me">
		<div class="txt"><span class="label">Recommendations</span><span class="hint" title="Explore's category strip learns from the categories you open. This stays on this server.">Explore learns from what you open.</span></div>
		<div class="ctl"><Button variant="secondary" onclick={resetRecommendations} busy={resetBusy} data-testid="reset-recommendations">Reset</Button></div>
	</div>
</SettingsCard>

<SettingsCard id="security-me" title="Security" description="Two-factor sign-in and linked sign-ins." icon="shield">
	<div class="srow" id="twofactor-me">
		<div class="txt"><span class="label">Two-factor</span><span class="hint">A six-digit code from your phone after the password.</span></div>
		<div class="ctl">
			<span class="chip" class:ok={session.s.twoFactor}>{session.s.twoFactor ? 'On' : 'Off'}</span>
			{#if !tfOpen && !setup}
				<Button variant="secondary" type="button" onclick={openTwoFactor} busy={tfBusy} data-testid="two-factor-open">{session.s.twoFactor ? 'Manage' : 'Set up'}</Button>
			{/if}
		</div>
		{#if codes.length || tfOpen || setup || tfError}
			<div class="tf below">
				{#if codes.length}
					<div class="fresh" role="status">
						<p class="small"><b>Backup codes.</b> Each works once. Shown only now.</p>
						<ul class="codes mono">
							{#each codes as c (c)}<li>{c}</li>{/each}
						</ul>
						<div class="row acts">
							<Button icon="copy" type="button" onclick={copyCodes}>Copy</Button>
							<Button variant="secondary" type="button" onclick={() => (codes = [])}>Done</Button>
						</div>
					</div>
				{/if}
				{#if tfOpen || setup}
					<Confirm action="two-factor" onCancel={() => (tfOpen = false)}>
						{#if session.s.twoFactor}
							<form class="tf-form" onsubmit={(e) => withCode(offMode === 'off' ? '/auth/two-factor/disable' : '/auth/two-factor/backup-codes', e)}>
								<fieldset class="modes">
									<legend class="sr-only">With a code from your app or a backup code</legend>
									<label class="checkline"><input type="radio" bind:group={offMode} value="codes" /> New backup codes</label>
									<label class="checkline"><input type="radio" bind:group={offMode} value="off" /> Turn off</label>
								</fieldset>
								<div class="row acts">
									<label class="sr-only" for="off-code">Code</label>
									<input id="off-code" class="input mono six" placeholder="Code" bind:value={offCode} autocomplete="one-time-code" inputmode="text" maxlength="11" />
									<Button variant={offMode === 'off' ? 'destructive' : 'secondary'} type="submit" disabled={tfBusy || !offCode.trim()} busy={tfBusy}>
										{offMode === 'off' ? 'Turn off' : 'Make codes'}
									</Button>
									<Button variant="plain" type="button" onclick={() => (tfOpen = false)}>Cancel</Button>
								</div>
							</form>
						{:else if setup}
							<ol class="steps small">
								<li>
									{#if setup.qr?.length}
										Scan this with your authenticator app:
										<QrCode rows={setup.qr} label="QR code for your authenticator app" />
										Or <a class="ul" href={setup.uri}>open it in the app</a>, or type:
									{:else}
										Add an account in your authenticator app (<a class="ul" href={setup.uri}>open in the app</a>) or type:
									{/if}
									<code class="key">{setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code>
								</li>
								<li>Type the six-digit code it shows.</li>
							</ol>
							<form class="row acts" onsubmit={enable}>
								<label class="sr-only" for="tf-code">Six-digit code</label>
								<input id="tf-code" class="input mono six" bind:value={code} autocomplete="one-time-code" inputmode="numeric" maxlength="6" placeholder="123456" />
								<Button variant="primary" type="submit" disabled={tfBusy || code.trim().length !== 6} busy={tfBusy}>Turn on</Button>
								<Button variant="plain" type="button" onclick={() => { setup = null; tfOpen = false; }}>Cancel</Button>
							</form>
						{:else}
							<div class="row acts"><Button variant="primary" type="button" onclick={startTwoFactor} busy={tfBusy}>Continue</Button></div>
						{/if}
					</Confirm>
				{/if}
				{#if tfError}<p class="form-error" role="alert">{tfError}</p>{/if}
			</div>
		{/if}
	</div>
	{#if hasLinkWay || myLinks.length || plexMine?.linked || (plexMine && session.s.hasPassword)}
		<div class="srow" id="linked-me">
			<div class="txt"><span class="label">Linked sign-ins</span><span class="hint" title="HolyArrs matches the provider's stable account id and never links accounts by email address.">Matched by account id, never by email.</span></div>
			<div class="ctl">{#if !linkOpen && (hasLinkWay || session.s.hasPassword)}<Button variant="secondary" type="button" onclick={() => (linkOpen = true)} data-testid="link-sign-in">Link</Button>{/if}</div>
			{#if plexMine?.linked}
				<ul class="below my-links" data-testid="my-plex-link">
					<li>
						<span class="small">Plex{#if plexMine.name}, as {plexMine.name}{/if}</span>
						{#if !session.s.hasPassword}
							<span class="xs muted" data-testid="plex-needs-password">Set a HolyArrs password above to unlink or change it.</span>
						{:else if plexUnlinking}
							<Confirm action="link" onCancel={() => (plexUnlinking = false)}>
								<div class="row acts">
									<Button variant="secondary" type="button" onclick={unlinkPlexMine} busy={linkBusy}>Unlink</Button>
									<Button variant="plain" type="button" onclick={() => (plexUnlinking = false)}>Cancel</Button>
								</div>
							</Confirm>
						{:else}
							<Button variant="plain" type="button" onclick={() => (plexUnlinking = true)} aria-label="Unlink Plex">Unlink</Button>
						{/if}
					</li>
				</ul>
			{/if}
			{#if myLinks.length}
				<ul class="below my-links" data-testid="my-sso-links">
					{#each myLinks as l (linkKey(l))}
						<li>
							<span class="small">{linkWord(l.kind)} at <span class="mono">{hostOf(l.issuer)}</span>{#if l.name}, as {l.name}{/if}</span>
							{#if unlinkKey === linkKey(l)}
								<Confirm action="link" onCancel={() => (unlinkKey = '')}>
									<div class="row acts">
										<Button variant="secondary" type="button" onclick={() => unlink(l)} busy={linkBusy}>Unlink</Button>
										<Button variant="plain" type="button" onclick={() => (unlinkKey = '')}>Cancel</Button>
									</div>
								</Confirm>
							{:else}
								<Button variant="plain" type="button" onclick={() => (unlinkKey = linkKey(l))} aria-label={`Unlink ${linkWord(l.kind)} at ${hostOf(l.issuer)}`}>Unlink</Button>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if linkOpen || linkError}
				<div class="below">
					{#if linkOpen}
						<Confirm action="link" onCancel={() => (linkOpen = false)}>
							<div class="row acts">
								{#if session.s.hasPassword}
									<Button type="button" onclick={linkPlexMine} busy={linkBusy}>{plexMine?.linked ? 'Plex (another account)' : 'Plex'}</Button>
								{/if}
								{#if session.s.ways.includes('oidc')}
									<Button type="button" onclick={startOIDCLink} busy={linkBusy}>Single sign-on</Button>
								{/if}
								{#if session.s.ways.includes('forward_auth')}
									<Button type="button" onclick={linkForwardAuth} busy={linkBusy}>Trusted proxy</Button>
								{/if}
								<Button variant="plain" type="button" onclick={() => (linkOpen = false)}>Cancel</Button>
							</div>
						</Confirm>
					{/if}
					{#if linkError}<p class="form-error" role="alert">{linkError}</p>{/if}
				</div>
			{/if}
		</div>
	{/if}
	{#if session.s.role === 'owner'}
		<div class="srow">
			<div class="txt"><span class="label">Locked out?</span><span class="hint">Run this on the HolyArrs machine.</span></div>
			<div class="ctl"><code class="cmd">holyarrs reset-owner</code></div>
		</div>
	{/if}
</SettingsCard>

<SettingsCard id="alerts-me" title="Alerts" description="Asks always show on your Requests page; add email or Discord too." icon="bell">
	<form id="alerts-form" class="rows" onsubmit={saveTargets} novalidate>
		<div class="srow">
			<div class="txt"><label for="tg-email">Email</label><span class="hint">When the owner set up email.</span></div>
			<div class="ctl"><input id="tg-email" class="input" type="email" bind:value={targets.email} autocomplete="email" /></div>
		</div>
		<div class="srow">
			<div class="txt"><label for="tg-discord">Discord user ID</label><span class="hint" title="In Discord turn on Developer Mode, then right-click your name and Copy User ID. Discord Alerts then name you.">Developer Mode, then Copy User ID.</span></div>
			<div class="ctl"><input id="tg-discord" class="input mono" inputmode="numeric" bind:value={targets.discord_id} autocomplete="off" /></div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">New episodes</span><span class="hint">Of shows you asked for; at most hourly.</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" bind:checked={targets.new_episodes} data-testid="new-episodes" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">New episodes of shows I asked for</span>
				</label>
			</div>
		</div>
		{#if tgError}<p class="form-error" role="alert">{tgError}</p>{/if}
	</form>
	<div class="srow" id="follows-me">
		<div class="txt"><span class="label">Following</span><span class="hint">Follow from a title page.</span></div>
		<div class="ctl"><span class="chip">{follows.length ? follows.length : 'None'}</span></div>
		{#if follows.length}
			<ul class="follows below" data-testid="follows">
				{#each follows as f (f.title_id)}
					<li>
						<a href={to(`/library/${f.title_id}`)}>{f.name}{#if f.year}&nbsp;({f.year}){/if}</a>
						<span class="xs muted">{f.kind === 'artist' ? 'Artist' : 'Show'}</span>
						<Button variant="secondary" onclick={() => unfollow(f)}>Unfollow</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</SettingsCard>

{#if showWatchlistCard(watch) && watch}
	<SettingsCard id="watchlist-me" title="Plex watchlist" description="Titles you add to your Plex watchlist are asked for as you." icon="requests">
		{#if !watch.can_read}
			<p class="hint" data-testid="watchlist-cannot-read">HolyArrs has no Plex sign-in saved for you yet, so your watchlist can’t be read.</p>
		{/if}
		<div class="srow">
			<div class="txt"><span class="label">Movies</span><span class="hint">Asked like any of your asks; your weekly limit applies.</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.movies} disabled={watchBusy || (!watch.can_read && !watch.movies)} onchange={(e) => setWatch('movies', e.currentTarget.checked)} data-testid="watchlist-movies" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">Ask for movies on my Plex watchlist</span>
				</label>
			</div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">Shows</span><span class="hint">The whole show is asked for.</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.shows} disabled={watchBusy || (!watch.can_read && !watch.shows)} onchange={(e) => setWatch('shows', e.currentTarget.checked)} data-testid="watchlist-shows" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">Ask for shows on my Plex watchlist</span>
				</label>
			</div>
		</div>
		{#if watch.movies || watch.shows}
			<div class="srow">
				<div class="txt">
					<span class="label">Last read</span>
					<span class="hint">{#if watch.checked_at}<time datetime={watch.checked_at}>{new Date(watch.checked_at).toLocaleString()}</time>{:else}Not yet{/if}{#if watch.plex_name}{' '}· Plex: {watch.plex_name}{/if}</span>
				</div>
				<div class="ctl"><Button variant="secondary" disabled={watchBusy || !watch.can_read} onclick={checkWatch}>Read now</Button></div>
			</div>
			{#if watch.error}<p class="form-error" role="alert" data-testid="watchlist-error">{watch.error}</p>{/if}
		{/if}
		{#if watch.items.length}
			<ul class="follows" data-testid="watchlist-items" aria-label="Newest on your Plex watchlist">
				{#each watch.items as it, i (i)}
					<li title={it.note || undefined}>
						<span class="wl-name">{it.name}{#if it.year}&nbsp;({it.year}){/if}</span>
						<span class="xs muted">{it.kind === 'show' ? 'Show' : 'Movie'} · {watchlistStateWord(it.state)}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>
{/if}

<SettingsCard id="sessions-me" title="Devices" description="Where you are signed in. Sign out any you don't know." icon="tv">
	<Sessions path="/auth/sessions" first={5} />
</SettingsCard>
</div>

<SaveBar dirty={alertsDirty} form="alerts-form" saving={tgBusy} ondiscard={() => (targets = JSON.parse(savedTargets))} />

<style>
	.my-links { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
	.my-links li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; }
	.my-links .mono { overflow-wrap: anywhere; }
	/* Profile full width; Preferences and Security pair up; Alerts (with its inputs) and Devices full width. */
	/* One column, like every Settings page. */
	.acct { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; align-items: stretch; }
	.acct > :global(*) { margin: 0; }
	.pfp { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	.value { font: var(--t-callout); font-weight: 400; color: var(--text); overflow-wrap: anywhere; min-width: 0; }
	/* Profile: each field in its own labelled box. */
	.boxes { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px; }
	.box {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		padding: 8px 12px 9px;
		border-radius: 10px;
		background: var(--surface-2);
		transition: background var(--dur-fast) var(--ease-out);
	}
	.box.wide { grid-column: 1 / -1; }
	.box .label { font: var(--t-footnote); font-weight: 500; color: var(--text-muted); }
	.line { display: flex; align-items: center; gap: 8px; min-height: 30px; }
	.line .value { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lock { flex: none; display: grid; place-items: center; width: 24px; height: 24px; color: var(--text-faint); cursor: help; }
	.dn { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
	.dn .input { flex: 1 1 200px; min-width: 0; min-height: 34px; }
	code { font-family: var(--font-mono); color: var(--text); }
	.acts { gap: 8px; flex-wrap: wrap; align-items: center; }
	.pw, .tf, .tf-form { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.pw, .tf-form { width: min(100%, 360px); }
	.rows { display: contents; }
	.six { max-width: 140px; }
	.modes { border: 0; padding: 0; margin: 0; display: flex; gap: 16px; flex-wrap: wrap; }
	.modes .checkline { min-height: 32px; padding: 0; align-items: center; }
	/* Shown-once secrets look the same as on Users and Sign-in: a red edge. */
	.fresh { padding: var(--s-4); border: 1px solid var(--ha-red); background: var(--ha-red-soft); border-radius: var(--r-btn); }
	.codes { display: grid; grid-template-columns: repeat(auto-fill, minmax(8rem, 1fr)); gap: 2px 12px; margin: 8px 0; list-style: none; padding: 0; }
	.steps { margin: 0 0 0 var(--s-5); display: grid; gap: 6px; list-style: decimal; }
	.key { display: block; margin-top: 6px; padding: 8px 10px; background: var(--surface-2); border-radius: var(--r-sm); overflow-wrap: anywhere; }
	.cmd { font-size: 13px; padding: 4px 8px; border-radius: 6px; background: var(--surface-2); white-space: nowrap; }
	.follows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; min-width: 0; }
	.follows li { display: flex; align-items: center; gap: 10px; min-height: 44px; border-top: 1px solid var(--line); }
	.follows li a, .follows .wl-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.follows .xs { flex: none; }
	@media (prefers-reduced-motion: reduce) { .box { transition: none; } }
	@media (max-width: 600px) {
		.boxes { grid-template-columns: minmax(0, 1fr); }
		.dn .input { flex-basis: 100%; min-height: 40px; }
		.six { max-width: none; flex: 1; }
		.acts > :global(.ui-btn) { min-height: 40px; }
	}
	@media (max-width: 600px) and (pointer: coarse) {
		.acts > :global(.ui-btn) { min-height: 44px; }
	}
</style>
