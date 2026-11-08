<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Sessions from './Sessions.svelte';
	import AccountSecurity from './AccountSecurity.svelte';
	import { strong } from './strong-error.ts';
	import { profile, loadProfile, saveDisplayName } from './profile.svelte.ts';
	import { displayNameError, roleWord, shownName } from './profile.ts';
	import { showWatchlistCard, watchlistStateWord, type PlexWatchlist } from './watchlist.ts';
	import { detailPath } from '../metadata/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { personName } from '#lib/core/plain.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { to } from '../core/paths.ts';
	import { autoplayPref, setAutoplayPref, type AutoplayPref } from '../core/trailer/trailer.ts';

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
			toasts.push('Name saved', 'ok');
		} catch (err) {
			nameError = message(err);
		} finally {
			nameBusy = false;
		}
	}

	// Where Alerts reach this person (email, Discord).
	let emailReady = $state(true);
	let targets = $state<{ email: string; discord_id: string; new_episodes?: boolean }>({ email: '', discord_id: '', new_episodes: true });
	// Follow: everything I follow. Library titles, movies and shows not in it yet (by TMDB id, named here), and albums. `unfollow` is the address that stops one.
	type Followed = { title_id: number; kind: string; name: string; year?: number; tmdb_id?: string; mbid?: string; artist?: string; unfollow: string };
	let follows = $state<Followed[]>([]);
	let unfollowing = $state('');
	const followKey = (f: Followed) => f.unfollow;
	const followHref = (f: Followed) => (f.title_id ? `/library/${f.title_id}` : f.mbid ? `/music/album/${encodeURIComponent(f.mbid)}` : f.tmdb_id ? detailPath(f.kind === 'movie' ? 'movie' : 'show', f.tmdb_id) : '');
	const followKind = (f: Followed) => ({ artist: 'Artist', album: 'Album', movie: 'Movie' })[f.kind] ?? 'Show';
	function nameFollows(list: Followed[]) {
		for (const f of list) {
			if (f.name || !f.tmdb_id) continue;
			void api.get<Record<string, unknown>>(`/metadata/${f.kind}/${encodeURIComponent(f.tmdb_id)}`).then((m) => {
				const name = typeof m.name === 'string' ? m.name : typeof m.title === 'string' ? m.title : '';
				const year = typeof m.year === 'number' && m.year > 0 ? m.year : undefined;
				if (name) follows = follows.map((x) => (x.unfollow === f.unfollow ? { ...x, name, year } : x));
			}, () => {});
		}
	}
	async function unfollow(f: Followed) {
		if (unfollowing) return;
		unfollowing = f.unfollow;
		try {
			await api.del(f.unfollow);
			follows = follows.filter((x) => x.unfollow !== f.unfollow);
			toasts.push(`Unfollowed ${f.name || followKind(f).toLowerCase()}`, 'ok');
		} catch (err) {
			toasts.push("Couldn't unfollow", 'fail', { body: message(err) });
		} finally {
			unfollowing = '';
		}
	}
	// Reset my recommendations: Explore's category strip forgets what it learned from my opens.
	let resetBusy = $state(false);
	async function resetRecommendations() {
		resetBusy = true;
		try {
			await api.del('/explore/strip/counts');
			toasts.push('Recommendations reset', 'ok');
		} catch (err) {
			toasts.push("Couldn't reset", 'fail', { body: message(err) });
		} finally {
			resetBusy = false;
		}
	}
	// Plex watchlist: new entries are asked for as me, when the owner allows it.
	let watch = $state<PlexWatchlist | null>(null);
	let watchBusy = $state(false);
	async function setWatch(kind: 'movies' | 'shows', on: boolean) {
		if (!watch) return;
		watchBusy = true;
		try {
			watch = await api.put<PlexWatchlist>('/my/plex-watchlist', { movies: watch.movies, shows: watch.shows, [kind]: on });
			toasts.push(on ? 'Plex watchlist on' : 'Settings saved', 'ok', on ? { body: 'Read every few minutes' } : undefined);
		} catch (err) {
			toasts.push("Couldn't save", 'fail', { body: message(err) });
			watch = await api.get<PlexWatchlist>('/my/plex-watchlist').catch(() => watch);
		} finally {
			watchBusy = false;
		}
	}
	async function checkWatch() {
		watchBusy = true;
		try {
			await api.post('/my/plex-watchlist/check', {});
			toasts.push('Reading Plex watchlist', 'info');
			setTimeout(() => void api.get<PlexWatchlist>('/my/plex-watchlist').then((r) => (watch = r)).catch(() => {}), 4000);
		} catch (err) {
			toasts.push("Couldn't read watchlist", 'fail', { body: message(err) });
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
		api
			.get<{ email: string; discord_id: string; new_episodes?: boolean }>('/my/alerts')
			.then((r) => {
				targets = r;
				savedTargets = JSON.stringify(r);
			})
			.catch(() => {});
		// Only people who may read Settings can tell whether email Alerts exist; for them the Email box is off until one does.
		if (session.can('settings'))
			api
				.get<{ alerts: { kind: string; enabled?: boolean }[] }>('/alerts')
				.then((r) => (emailReady = r.alerts.some((a) => a.kind === 'email' && a.enabled !== false)))
				.catch(() => {});
		api
			.get<PlexWatchlist>('/my/plex-watchlist')
			.then((r) => (watch = r))
			.catch(() => {});
		api
			.get<{ follows: Followed[] }>('/my/follows')
			.then((r) => (nameFollows((follows = r.follows))))
			.catch(() => {});
	});
	async function saveTargets(e: SubmitEvent) {
		e.preventDefault();
		tgBusy = true;
		tgError = '';
		try {
			targets = await api.put('/my/alerts', targets);
			savedTargets = JSON.stringify(targets);
			toasts.push('Alert settings saved', 'ok');
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

	let current = $state('');
	let next = $state('');
	let again = $state('');
	let pwError = $state('');
	let pwBusy = $state(false);

	// A linked identity changes how this account can sign in, so it first requires a recent proof with this account's HolyArrs password. Password and linking open inline; Confirm asks for the password first when needed.
	let pwOpen = $state(false);
	// The Settings search links to #pw-current, which only exists once the form is open.
	$effect(() => {
		if (page.url.hash === '#pw-current' && (session.s.hasPassword || session.s.outsideRecent)) pwOpen = true;
	});

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		pwError = '';
		if (next.length < MIN_LENGTH) return void (pwError = `Use at least ${MIN_LENGTH} characters`);
		if (next !== again) return void (pwError = 'Passwords don’t match');
		pwBusy = true;
		const first = !session.s.hasPassword;
		try {
			await api.post('/auth/password', first ? { new: next } : { current, new: next });
			current = next = again = '';
			pwOpen = false;
			if (first) session.s.hasPassword = true;
			toasts.push(first ? 'Password set' : 'Password changed', 'ok', { body: first ? `Sign in as ${session.s.name} with it` : 'Other devices signed out' });
		} catch (err) {
			pwError = strong(err);
		} finally {
			pwBusy = false;
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
			toasts.push('Picture not supported', 'fail', { body: 'Use JPEG, PNG or WebP' });
			return;
		}
		if (f.size > AVATAR_MAX) {
			toasts.push('Picture is too big', 'fail', { body: 'Max 5 MB' });
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
			toasts.push('Profile picture saved', 'ok');
		} catch (err) {
			toasts.push("Couldn't save picture", 'fail', { body: message(err) });
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
			toasts.push('Profile picture removed', 'ok');
		} catch (err) {
			toasts.push("Couldn't remove picture", 'fail', { body: message(err) });
		} finally {
			avatarBusy = false;
		}
	}
</script>

<PageHeader title="My account" />
<div class="acct">

{#if session.s.mustTwoFactor}
	<p class="notice warn" role="alert">Admins need two-factor sign-in. Turn it on under Security to continue.</p>
{/if}

<SettingsCard id="profile-me" title="Profile" icon="user">
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
		<span class="hint" title="JPEG, PNG or WebP, up to 5 MB">Up to 5 MB.</span>
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
					<Button variant="plain" type="button" onclick={startName} data-testid="edit-display-name">Change</Button>
				</div>
			{/if}
		</div>
		<div class="box" data-testid="box-username">
			<span class="label">Username</span>
			<div class="line">
				<span class="value">{session.s.name}</span>
				<span class="lock" title="Used to sign in; can't change" role="img" aria-label="Locked: you sign in with this name"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
			</div>
		</div>
		<div class="box" data-testid="box-role">
			<span class="label">Role</span>
			<div class="line">
				<span class="value">{roleWord(session.s.role)}</span>
				<span class="lock" title="Only the owner can change this" role="img" aria-label="Locked: only the owner can change your role"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
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
						<input id="pw-new" class="input" type="password" placeholder="At least {MIN_LENGTH} characters" bind:value={next} autocomplete="new-password" />
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

<SettingsCard id="preferences-me" title="Preferences" icon="appearance">
	<div class="srow">
		<div class="txt"><span class="label">Trailers</span><span class="hint" title="Auto respects Reduce motion and data saver">Auto respects Reduce motion and data saver</span></div>
		<div class="ctl"><SegmentedControl size="sm" label="Play trailers" options={[{ value: 'auto', label: 'Auto' }, { value: 'always', label: 'Always' }, { value: 'never', label: 'Never' }] as { value: AutoplayPref; label: string }[]} value={pref} onchange={(v) => { pref = v; setAutoplayPref(v); location.reload(); }} /></div>
	</div>
	<div class="srow" id="recommendations-me">
		<div class="txt"><span class="label">Recommendations</span><span class="hint" title="Explore learns from what you open. Stays on this server.">Explore learns from what you open.</span></div>
		<div class="ctl"><Button variant="secondary" onclick={resetRecommendations} busy={resetBusy} data-testid="reset-recommendations">Reset</Button></div>
	</div>
</SettingsCard>
<AccountSecurity />

<SettingsCard id="alerts-me" title="Alerts" description="Requests always show on Requests. Add email or Discord too." icon="bell">
	<form id="alerts-form" class="rows" onsubmit={saveTargets} novalidate>
		<div class="srow">
			<div class="txt"><label for="tg-email">Email</label><span class="hint">{emailReady ? 'Needs email set up by the owner' : 'Email Alerts aren’t set up'}</span></div>
			<div class="ctl"><input id="tg-email" class="input" type="email" bind:value={targets.email} autocomplete="email" placeholder="you@example.com" disabled={!emailReady && !targets.email} /></div>
		</div>
		<div class="srow">
			<div class="txt"><label for="tg-discord">Discord user ID</label><span class="hint" title="In Discord: Developer Mode, right-click your name, Copy User ID">Developer Mode, then Copy User ID.</span></div>
			<div class="ctl"><input id="tg-discord" class="input mono" inputmode="numeric" bind:value={targets.discord_id} autocomplete="off" placeholder="e.g. 123456789012345678" /></div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">New episodes</span><span class="hint">Shows you requested, at most hourly</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" bind:checked={targets.new_episodes} data-testid="new-episodes" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">New episodes of shows I requested</span>
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
				{#each follows as f (followKey(f))}
					<li>
						<a href={to(followHref(f))}>{f.name || 'Loading…'}{#if f.year}&nbsp;({f.year}){/if}{#if f.artist}<span class="muted">{` · ${f.artist}`}</span>{/if}</a>
						<span class="xs muted">{followKind(f)}</span>
						<Button variant="secondary" disabled={!!unfollowing} busy={unfollowing === f.unfollow} onclick={() => unfollow(f)}>Unfollow</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</SettingsCard>

{#if showWatchlistCard(watch) && watch && !session.held('plex_watchlists')}
	<SettingsCard id="watchlist-me" title="Plex watchlist" description="Titles on your Plex watchlist become your requests" icon="requests">
		{#if !watch.can_read}
			<p class="hint" data-testid="watchlist-cannot-read">Link Plex under Security to read your watchlist</p>
		{/if}
		<div class="srow">
			<div class="txt"><span class="label">Movies</span><span class="hint">Your weekly Limit applies</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.movies} disabled={watchBusy || (!watch.can_read && !watch.movies)} onchange={(e) => setWatch('movies', e.currentTarget.checked)} data-testid="watchlist-movies" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">Request movies on my Plex watchlist</span>
				</label>
			</div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">Shows</span><span class="hint">Requests the whole show</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.shows} disabled={watchBusy || (!watch.can_read && !watch.shows)} onchange={(e) => setWatch('shows', e.currentTarget.checked)} data-testid="watchlist-shows" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">Request shows on my Plex watchlist</span>
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

<SettingsCard id="sessions-me" title="Devices" description="Sign out any you don't recognise" icon="tv">
	<Sessions path="/auth/sessions" first={5} />
</SettingsCard>
</div>

<SaveBar dirty={alertsDirty} form="alerts-form" saving={tgBusy} ondiscard={() => (targets = JSON.parse(savedTargets))} />

<style>
	/* One column, like every Settings page. */
	.acct { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; align-items: stretch; }
	.acct > :global(*) { margin: 0; }
	.pfp { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	.value { font: var(--t-callout); font-weight: 400; color: var(--text); overflow-wrap: anywhere; min-width: 0; }
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
	.dn .input { flex: 1 1 200px; min-width: 0; }
	/* One button height and one input height across My account, the Settings sizes. */
	.acct :global(.ui-btn.ui-btn) { min-height: 32px; }
	.acct :global(.input:not(textarea)) { height: 32px; min-height: 32px; padding-block: 0; }
	.acts { gap: 8px; flex-wrap: wrap; align-items: center; }
	.pw { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.pw { width: min(100%, 360px); }
	.rows { display: contents; }
	/* Shown-once secrets look the same as on Users and Sign-in: a red edge. */
	.follows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; min-width: 0; }
	.follows li { display: flex; align-items: center; gap: 10px; min-height: 44px; border-top: 1px solid var(--line); }
	.follows li a, .follows .wl-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.follows .xs { flex: none; }
	@media (prefers-reduced-motion: reduce) { .box { transition: none; } }
	@media (max-width: 600px) {
		.boxes { grid-template-columns: minmax(0, 1fr); }
		.dn .input { flex-basis: 100%; }
	}
	@media (max-width: 600px) {
		.acct :global(.ui-btn.ui-btn) { min-height: 40px; }
		.acct :global(.input:not(textarea)) { height: 40px; min-height: 40px; }
	}
	@media (pointer: coarse) {
		.acct :global(.ui-btn.ui-btn) { min-height: 44px; }
		.acct :global(.input:not(textarea)) { height: 44px; min-height: 44px; }
	}
</style>
