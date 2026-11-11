<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Sessions from './Sessions.svelte';
	import LanguagePicker from './LanguagePicker.svelte';
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
	import { minimalLook } from '../core/look.svelte.ts';

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
			toasts.push(t('accounts.account.nameSaved'), 'ok');
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
	const followKind = (f: Followed) => t(`accounts.account.kind.${['artist', 'album', 'movie'].includes(f.kind) ? f.kind : 'show'}`);
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
			toasts.push(t('accounts.account.unfollowed', { what: f.name || followKind(f).toLowerCase() }), 'ok');
		} catch (err) {
			toasts.push(t('accounts.account.unfollowFailed'), 'fail', { body: message(err) });
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
			toasts.push(t('accounts.account.recoReset'), 'ok');
		} catch (err) {
			toasts.push(t('accounts.account.resetFailed'), 'fail', { body: message(err) });
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
			toasts.push(on ? t('accounts.account.watchOn') : t('accounts.account.settingsSaved'), 'ok', on ? { body: t('accounts.account.everyFew') } : undefined);
		} catch (err) {
			toasts.push(t('accounts.account.saveFailed'), 'fail', { body: message(err) });
			watch = await api.get<PlexWatchlist>('/my/plex-watchlist').catch(() => watch);
		} finally {
			watchBusy = false;
		}
	}
	async function checkWatch() {
		watchBusy = true;
		try {
			await api.post('/my/plex-watchlist/check', {});
			toasts.push(t('accounts.account.reading'), 'info');
			setTimeout(() => void api.get<PlexWatchlist>('/my/plex-watchlist').then((r) => (watch = r)).catch(() => {}), 4000);
		} catch (err) {
			toasts.push(t('accounts.account.readFailed'), 'fail', { body: message(err) });
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
			toasts.push(t('accounts.account.alertsSaved'), 'ok');
		} catch (err) {
			tgError = message(err);
		} finally {
			tgBusy = false;
		}
	}

	const MIN_LENGTH = $derived(session.s.minPassword);
	const via = (k: string): string => (['session', 'api_key', 'local_network'].includes(k) ? t(`accounts.account.via.${k}`) : k);
	function authMethodWord(method: string): string {
		if (method === 'local_password' || method === 'local_setup') return t('accounts.account.method.passwordLabel');
		if (method === 'oidc' || method.startsWith('oidc:')) return t('accounts.account.method.sso');
		if (method === 'forward_auth' || method.startsWith('forward_auth:')) return t('accounts.account.method.proxy');
		if (method === 'legacy') return t('accounts.account.method.legacy');
		const server: Record<string, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
		return server[method] ?? method;
	}
	const viaShort = (k: string): string => (['session', 'api_key', 'local_network'].includes(k) ? t(`accounts.account.viaShort.${k}`) : k);

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
		if (next.length < MIN_LENGTH) return void (pwError = t('accounts.account.useAtLeast', { n: MIN_LENGTH }));
		if (next !== again) return void (pwError = t('accounts.account.noMatch'));
		pwBusy = true;
		const first = !session.s.hasPassword;
		try {
			await api.post('/auth/password', first ? { new: next } : { current, new: next });
			current = next = again = '';
			pwOpen = false;
			if (first) session.s.hasPassword = true;
			toasts.push(first ? t('accounts.account.pwSet') : t('accounts.account.pwChanged'), 'ok', { body: first ? t('accounts.account.signInAs', { name: session.s.name }) : t('accounts.account.othersOut') });
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
			toasts.push(t('accounts.account.picNotSupported'), 'fail', { body: t('accounts.account.picUse') });
			return;
		}
		if (f.size > AVATAR_MAX) {
			toasts.push(t('accounts.account.picBig'), 'fail', { body: t('accounts.account.picMax') });
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
			toasts.push(t('accounts.account.picSaved'), 'ok');
		} catch (err) {
			toasts.push(t('accounts.account.picSaveFailed'), 'fail', { body: message(err) });
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
			toasts.push(t('accounts.account.picRemoved'), 'ok');
		} catch (err) {
			toasts.push(t('accounts.account.picRemoveFailed'), 'fail', { body: message(err) });
		} finally {
			avatarBusy = false;
		}
	}
</script>

<PageHeader title={t('accounts.account.title')} />
<div class="acct">

{#if session.s.mustTwoFactor}
	<p class="notice warn" role="alert">{t('accounts.account.mustTf')}</p>
{/if}

<SettingsCard id="profile-me" title={t('accounts.account.profile')} icon="user">
	<div class="pfp">
		<Avatar name={shownName(profile.displayName, personName(session.s.name, t('accounts.account.you')))} src={preview || session.s.avatar} size={44} label={t('accounts.account.yourPicture')} />
		<input class="sr-only" type="file" accept="image/jpeg,image/png,image/webp" bind:this={avatarInput} onchange={pickAvatar} disabled={avatarBusy} aria-label={t('accounts.account.choosePicture')} />
		{#if picked}
			<Button variant="primary" type="button" onclick={saveAvatar} busy={avatarBusy}>{t('accounts.sso.save')}</Button>
			<Button variant="plain" type="button" onclick={clearPick} disabled={avatarBusy}>{t('accounts.signin.cancel')}</Button>
		{:else}
			<Button type="button" onclick={() => avatarInput?.click()} disabled={avatarBusy}>{session.s.avatar ? t('accounts.account.changePicture') : t('accounts.account.addPicture')}</Button>
			{#if session.s.avatar}<Button variant="plain" type="button" onclick={removeAvatar} disabled={avatarBusy}>{t('accounts.account.remove')}</Button>{/if}
		{/if}
		<span class="hint" title={t('accounts.account.picTip')}>{t('accounts.account.picUpTo')}</span>
	</div>
	<div class="boxes" data-testid="profile-boxes">
		<div class="box wide" data-testid="box-display-name">
			<span class="label" id="dn-label">{t('accounts.account.displayName')}</span>
			{#if nameEditing}
				<form class="dn" onsubmit={saveName} novalidate>
					<input class="input" aria-labelledby="dn-label" bind:value={nameDraft} maxlength="64" autocomplete="nickname" placeholder={session.s.name} />
					<Button variant="primary" type="submit" busy={nameBusy} disabled={nameBusy || nameDraft.trim() === profile.displayName || !!nameProblem}>{t('accounts.sso.save')}</Button>
					<Button variant="plain" type="button" onclick={() => (nameEditing = false)} disabled={nameBusy}>{t('accounts.signin.cancel')}</Button>
				</form>
				{#if nameError || nameProblem}<p class="form-error" role="alert">{nameError || nameProblem}</p>{/if}
			{:else}
				<div class="line">
					<span class="value">{profile.displayName || session.s.name}</span>
					<Button variant="plain" type="button" onclick={startName} data-testid="edit-display-name">{t('accounts.account.change')}</Button>
				</div>
			{/if}
		</div>
		<div class="box" data-testid="box-username">
			<span class="label">{t('accounts.account.username')}</span>
			<div class="line">
				<span class="value">{session.s.name}</span>
				<span class="lock" title={t('accounts.account.usernameTip')} role="img" aria-label={t('accounts.account.usernameLocked')}><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
			</div>
		</div>
		<div class="box" data-testid="box-role">
			<span class="label">{t('accounts.account.role')}</span>
			<div class="line">
				<span class="value">{roleWord(session.s.role)}</span>
				<span class="lock" title={t('accounts.account.roleTip')} role="img" aria-label={t('accounts.account.roleLocked')}><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg></span>
			</div>
		</div>
		<div class="box" data-testid="box-sign-in">
			<span class="label">{t('accounts.account.signInMethod')}</span>
			<div class="line"><span class="value" title={via(session.s.via)}>{session.s.authMethod ? authMethodWord(session.s.authMethod) : viaShort(session.s.via)}</span></div>
		</div>
		<div class="box" class:wide={pwOpen} data-testid="box-password">
			<span class="label">{t('accounts.signin.passwordLabel')}</span>
			{#if !session.s.hasPassword && !session.s.outsideRecent}
				<span class="hint">{t('accounts.account.noneYet', { method: authMethodWord(session.s.authMethod) })}</span>
			{:else if !session.s.hasPassword && !pwOpen}
				<div class="line"><Button type="button" onclick={() => (pwOpen = true)} data-testid="change-password">{t('accounts.account.setAPasswordLabel')}</Button></div>
				<span class="hint">{t('accounts.account.soStill', { method: authMethodWord(session.s.authMethod) })}</span>
			{:else if pwOpen}
				<Confirm action="password" onCancel={() => (pwOpen = false)}>
					<form class="pw" onsubmit={changePassword} novalidate>
						<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
						{#if session.s.hasPassword}
							<label class="sr-only" for="pw-current">{t('accounts.account.currentPasswordLabel')}</label>
							<input id="pw-current" class="input" type="password" placeholder={t('accounts.account.currentPasswordLabel')} bind:value={current} autocomplete="current-password" />
						{/if}
						<label class="sr-only" for="pw-new">{t('accounts.account.newPasswordLabel')}</label>
						<input id="pw-new" class="input" type="password" placeholder={t('accounts.signin.atLeast', { n: MIN_LENGTH })} bind:value={next} autocomplete="new-password" />
						<label class="sr-only" for="pw-again">{t('accounts.account.confirmPasswordLabel')}</label>
						<input id="pw-again" class="input" type="password" placeholder={t('accounts.account.confirmPasswordLabel')} bind:value={again} autocomplete="new-password" />
						{#if pwError}<p class="form-error" role="alert">{pwError}</p>{/if}
						<div class="row acts">
							<Button variant="primary" type="submit" disabled={pwBusy || !next} busy={pwBusy}>{session.s.hasPassword ? t('accounts.account.changePasswordLabel') : t('accounts.account.setPasswordLabel')}</Button>
							<Button variant="plain" type="button" onclick={() => (pwOpen = false)}>{t('accounts.signin.cancel')}</Button>
						</div>
						<span class="hint">{session.s.hasPassword ? t('accounts.account.signsOut') : t('accounts.account.keepsWorking', { method: authMethodWord(session.s.authMethod) })}</span>
					</form>
				</Confirm>
			{:else}
				<div class="line"><Button type="button" onclick={() => (pwOpen = true)} data-testid="change-password">{t('accounts.account.changePasswordLabel')}</Button></div>
			{/if}
		</div>
	</div>
</SettingsCard>

<SettingsCard id="preferences-me" title={t('accounts.account.preferences')} icon="appearance">
	<div class="srow" id="language-row-me">
		<div class="txt"><label class="label" for="language-me">{t('accounts.language.label')}</label><span class="hint">{t('accounts.language.hint')}</span></div>
		<div class="ctl"><LanguagePicker id="language-me" /></div>
	</div>
	<div class="srow">
		<div class="txt"><span class="label">{t('accounts.account.trailers')}</span>{#if minimalLook()}<span class="hint">{t('accounts.account.trailersMinimal')}</span>{:else}<span class="hint" title={t('accounts.account.trailersHint')}>{t('accounts.account.trailersHint')}</span>{/if}</div>
		<div class="ctl"><SegmentedControl size="sm" label={t('accounts.account.playTrailers')} options={[{ value: 'auto', label: t('accounts.account.auto') }, { value: 'always', label: t('accounts.account.always') }, { value: 'never', label: t('accounts.account.never') }] as { value: AutoplayPref; label: string }[]} value={pref} onchange={(v) => { pref = v; setAutoplayPref(v); location.reload(); }} /></div>
	</div>
	<div class="srow" id="recommendations-me">
		<div class="txt"><span class="label">{t('accounts.account.reco')}</span><span class="hint" title={t('accounts.account.recoTip')}>{t('accounts.account.recoHint')}</span></div>
		<div class="ctl"><Button variant="secondary" onclick={resetRecommendations} busy={resetBusy} data-testid="reset-recommendations">{t('accounts.account.reset')}</Button></div>
	</div>
</SettingsCard>
<AccountSecurity />

<SettingsCard id="alerts-me" title={t('accounts.account.alerts')} description={t('accounts.account.alertsDesc')} icon="bell">
	<form id="alerts-form" class="rows" onsubmit={saveTargets} novalidate>
		<div class="srow">
			<div class="txt"><label for="tg-email">{t('accounts.account.email')}</label><span class="hint">{emailReady ? t('accounts.account.emailNeeds') : t('accounts.account.emailNot')}</span></div>
			<div class="ctl"><input id="tg-email" class="input" type="email" bind:value={targets.email} autocomplete="email" placeholder={t('accounts.account.emailPlaceholder')} disabled={!emailReady && !targets.email} /></div>
		</div>
		<div class="srow">
			<div class="txt"><label for="tg-discord">{t('accounts.account.discord')}</label><span class="hint" title={t('accounts.account.discordTip')}>{t('accounts.account.discordHint')}</span></div>
			<div class="ctl"><input id="tg-discord" class="input mono" inputmode="numeric" bind:value={targets.discord_id} autocomplete="off" placeholder={t('accounts.account.discordPlaceholder')} /></div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">{t('accounts.account.newEpisodes')}</span><span class="hint">{t('accounts.account.newEpisodesHint')}</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" bind:checked={targets.new_episodes} data-testid="new-episodes" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">{t('accounts.account.newEpisodesSr')}</span>
				</label>
			</div>
		</div>
		{#if tgError}<p class="form-error" role="alert">{tgError}</p>{/if}
	</form>
	<div class="srow" id="follows-me">
		<div class="txt"><span class="label">{t('accounts.account.following')}</span><span class="hint">{t('accounts.account.followingHint')}</span></div>
		<div class="ctl"><span class="chip">{follows.length ? follows.length : t('accounts.account.none')}</span></div>
		{#if follows.length}
			<ul class="follows below" data-testid="follows">
				{#each follows as f (followKey(f))}
					<li>
						<a href={to(followHref(f))}>{f.name || t('accounts.account.loading')}{#if f.year}&nbsp;({f.year}){/if}{#if f.artist}<span class="muted">{` · ${f.artist}`}</span>{/if}</a>
						<span class="xs muted">{followKind(f)}</span>
						<Button variant="secondary" disabled={!!unfollowing} busy={unfollowing === f.unfollow} onclick={() => unfollow(f)}>{t('accounts.account.unfollow')}</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</SettingsCard>

{#if showWatchlistCard(watch) && watch}
	<SettingsCard id="watchlist-me" title={t('accounts.account.watchlist')} description={t('accounts.account.watchlistDesc')} icon="requests">
		{#if !watch.can_read}
			<p class="hint" data-testid="watchlist-cannot-read">{t('accounts.account.linkPlex')}</p>
		{:else if watch.everyone && !watch.chosen}
			<p class="hint" data-testid="watchlist-everyone">{t('accounts.account.watchEveryone')}</p>
		{/if}
		<div class="srow">
			<div class="txt"><span class="label">{t('accounts.account.movies')}</span><span class="hint">{t('accounts.account.limitApplies')}</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.movies} disabled={watchBusy || (!watch.can_read && !watch.movies)} onchange={(e) => setWatch('movies', e.currentTarget.checked)} data-testid="watchlist-movies" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">{t('accounts.account.reqMovies')}</span>
				</label>
			</div>
		</div>
		<div class="srow">
			<div class="txt"><span class="label">{t('accounts.account.shows')}</span><span class="hint">{t('accounts.account.wholeShow')}</span></div>
			<div class="ctl">
				<label class="switch">
					<input type="checkbox" checked={watch.shows} disabled={watchBusy || (!watch.can_read && !watch.shows)} onchange={(e) => setWatch('shows', e.currentTarget.checked)} data-testid="watchlist-shows" />
					<span class="sw" aria-hidden="true"></span><span class="sr-only">{t('accounts.account.reqShows')}</span>
				</label>
			</div>
		</div>
		{#if watch.movies || watch.shows}
			<div class="srow">
				<div class="txt">
					<span class="label">{t('accounts.account.lastRead')}</span>
					<span class="hint">{#if watch.checked_at}<time datetime={watch.checked_at}>{new Date(watch.checked_at).toLocaleString(appLocale())}</time>{:else}{t('accounts.account.notYet')}{/if}{#if watch.plex_name}{' '}· {t('accounts.account.plexName', { name: watch.plex_name })}{/if}</span>
				</div>
				<div class="ctl"><Button variant="secondary" disabled={watchBusy || !watch.can_read} onclick={checkWatch}>{t('accounts.account.readNow')}</Button></div>
			</div>
			{#if watch.error}<p class="form-error" role="alert" data-testid="watchlist-error">{watch.error}</p>{/if}
		{/if}
		{#if watch.items.length}
			<ul class="follows" data-testid="watchlist-items" aria-label={t('accounts.account.newest')}>
				{#each watch.items as it, i (i)}
					<li title={it.note || undefined}>
						<span class="wl-name">{it.name}{#if it.year}&nbsp;({it.year}){/if}</span>
						<span class="xs muted">{it.kind === 'show' ? t('accounts.account.kind.show') : t('accounts.account.kind.movie')} · {watchlistStateWord(it.state)}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>
{/if}

<SettingsCard id="sessions-me" title={t('accounts.account.devices')} description={t('accounts.account.devicesDesc')} icon="tv">
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
