<script lang="ts">
	import { api, message, ApiError } from './api.ts';
	import { session } from './session.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import Icon from './Icon.svelte';
	import Sessions from './Sessions.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from './paths.ts';

	// Where Alerts reach this person (email, Discord).
	let targets = $state({ email: '', discord_id: '' });
	let tgBusy = $state(false);
	let tgError = $state('');
	onMount(() => {
		void session.loadWays().then(() => {
			if (page.url.searchParams.get('sso') === 'finish') void finishOIDCLink();
		});
		api
			.get<{ email: string; discord_id: string }>('/my/alerts')
			.then((r) => (targets = r))
			.catch(() => {});
	});
	async function saveTargets(e: SubmitEvent) {
		e.preventDefault();
		tgBusy = true;
		tgError = '';
		try {
			targets = await api.put('/my/alerts', targets);
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
		return method;
	}
	const roleWord: Record<string, string> = { owner: 'Owner', admin: 'Admin', member: 'Member' };

	// Password
	let current = $state('');
	let next = $state('');
	let again = $state('');
	let pwError = $state('');
	let pwBusy = $state(false);

	// Two-factor
	let setup = $state<{ secret: string; uri: string } | null>(null);
	let code = $state('');
	let offCode = $state('');
	let tfError = $state('');
	let tfBusy = $state(false);
	let codes = $state<string[]>([]);

	// A linked identity changes how this account can sign in, so it first
	// requires a recent proof with this account's HolyArrs password.
	let reauthPassword: string = $state('');
	let reauthCode = $state('');
	let reauthError = $state('');
	let reauthBusy = $state(false);
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
		try {
			await api.post('/auth/password', { current, new: next });
			current = next = again = '';
			toasts.push('Password changed. Your other devices are signed out.', 'ok');
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
			setup = await api.post<{ secret: string; uri: string }>('/auth/two-factor/setup');
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

	async function copyCodes() {
		try {
			await navigator.clipboard.writeText(codes.join('\n'));
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; write the codes down by hand.', 'fail');
		}
	}

	async function reauthenticate(e: SubmitEvent) {
		e.preventDefault();
		reauthError = '';
		reauthBusy = true;
		try {
			await session.reauth(reauthPassword, reauthCode.trim() || undefined);
			reauthPassword = '';
			reauthCode = '';
			toasts.push('Your local sign-in was confirmed.', 'ok');
		} catch (err) {
			reauthError = message(err);
			reauthPassword = '';
			reauthCode = '';
		} finally {
			reauthBusy = false;
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
			await goto(to('/account'), { replace: true });
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
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}
</script>

<header class="page-head">
	<div>
		<h1>My account</h1>
		<p>Your password and two-factor sign-in.</p>
	</div>
</header>

{#if session.s.mustTwoFactor}
	<p class="notice warn" role="alert">The owner asks every admin to use two-factor sign-in. Turn it on below to use the rest of HolyArrs.</p>
{/if}

<section class="card" aria-labelledby="h-me">
	<h2 id="h-me" class="h2" style="margin-bottom: var(--s-3)">Signed in</h2>
	<dl class="facts">
		<dt>Name</dt>
		<dd>{session.s.name}</dd>
		<dt>Role</dt>
		<dd>{roleWord[session.s.role] ?? session.s.role}</dd>
		<dt>How</dt>
		<dd>{via[session.s.via] ?? session.s.via}</dd>
		{#if session.s.authMethod}
			<dt>Sign-in method</dt>
			<dd>{authMethodWord(session.s.authMethod)}</dd>
		{/if}
	</dl>
</section>

<section class="card" aria-labelledby="h-confirm-local">
	<h2 id="h-confirm-local" class="h2">Confirm local sign-in</h2>
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
		{#if session.s.localRecent}
			Your local sign-in was confirmed recently. Confirm it again here if a sensitive action says that confirmation expired.
		{:else}
			Confirm your HolyArrs password before linking a sign-in method or making sensitive account changes.
		{/if}
		This replaces the current session with a freshly confirmed one.
	</p>
	<form class="pw" onsubmit={reauthenticate} novalidate>
		<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
		<div class="field">
			<label for="reauth-password">HolyArrs password</label>
			<input id="reauth-password" class="input" type="password" bind:value={reauthPassword} autocomplete="current-password" required />
		</div>
		{#if session.s.twoFactor}
			<div class="field">
				<label for="reauth-code">Two-factor code</label>
				<input id="reauth-code" class="input mono" bind:value={reauthCode} autocomplete="one-time-code" maxlength="11" required />
			</div>
		{/if}
		{#if reauthError}<p class="form-error" role="alert">{reauthError}</p>{/if}
		<button class="btn btn-red" type="submit" disabled={reauthBusy || !reauthPassword || (session.s.twoFactor && !reauthCode.trim())} aria-busy={reauthBusy}>
			Confirm local sign-in
		</button>
	</form>
</section>

{#if hasLinkWay}
	<section class="card" aria-labelledby="h-linked-signin">
		<h2 id="h-linked-signin" class="h2">Linked sign-in</h2>
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
			Link another sign-in only while you are already signed in. HolyArrs matches the provider's stable account id and never links accounts by email address.
		</p>

		{#if !session.s.localRecent}
			<p class="notice warn small">Confirm your local sign-in above before linking another sign-in method.</p>
		{:else}
			<div class="row">
				{#if session.s.ways.includes('oidc')}
					<button class="btn btn-ghost" type="button" onclick={startOIDCLink} disabled={linkBusy} aria-busy={linkBusy}>Link single sign-on</button>
				{/if}
				{#if session.s.ways.includes('forward_auth')}
					<button class="btn btn-ghost" type="button" onclick={linkForwardAuth} disabled={linkBusy} aria-busy={linkBusy}>Link trusted proxy sign-in</button>
				{/if}
			</div>
		{/if}
		{#if linkError}<p class="form-error" role="alert">{linkError}</p>{/if}
	</section>
{/if}

<section class="card" aria-labelledby="h-tf">
	<h2 id="h-tf" class="h2">Two-factor sign-in</h2>
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
		After your password, HolyArrs asks for a six-digit code from an authenticator app on your phone.
	</p>

	{#if codes.length}
		<div class="fresh" role="status">
			<p class="small"><b>Backup codes.</b> Each works once if you lose your phone. Keep them somewhere safe; they are shown only now.</p>
			<ul class="codes mono">
				{#each codes as c (c)}<li>{c}</li>{/each}
			</ul>
			<div class="row">
				<button class="btn btn-ghost" type="button" onclick={copyCodes}><Icon name="copy" size={18} /> Copy</button>
				<button class="btn btn-quiet" type="button" onclick={() => (codes = [])}>Done</button>
			</div>
		</div>
	{/if}

	{#if session.s.twoFactor}
		<p class="chip ok">On</p>
		<form class="tf-form" onsubmit={(e) => withCode(offMode === 'off' ? '/auth/two-factor/disable' : '/auth/two-factor/backup-codes', e)}>
			<fieldset class="field">
				<legend class="label">With a code from your app or a backup code</legend>
				<label class="checkline"><input type="radio" bind:group={offMode} value="codes" /> Make new backup codes</label>
				<label class="checkline"><input type="radio" bind:group={offMode} value="off" /> Turn two-factor sign-in off</label>
			</fieldset>
			<div class="field">
				<label for="off-code">Code</label>
				<input id="off-code" class="input mono" bind:value={offCode} autocomplete="one-time-code" inputmode="text" maxlength="11" />
			</div>
			<button class="btn {offMode === 'off' ? 'btn-danger' : 'btn-ghost'}" type="submit" disabled={tfBusy || !offCode.trim()} aria-busy={tfBusy}>
				{offMode === 'off' ? 'Turn off' : 'Make new codes'}
			</button>
		</form>
	{:else if setup}
		<ol class="steps small">
			<li>
				Open your authenticator app and add an account. On this phone you can <a class="ul" href={setup.uri}>open it in the app</a>, or
				type this key:
				<code class="key">{setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code>
			</li>
			<li>Type the six-digit code the app shows.</li>
		</ol>
		<form class="row" onsubmit={enable}>
			<label class="sr-only" for="tf-code">Six-digit code</label>
			<input id="tf-code" class="input mono" style="max-width: 160px" bind:value={code} autocomplete="one-time-code" inputmode="numeric" maxlength="6" placeholder="123456" />
			<button class="btn btn-red" type="submit" disabled={tfBusy || code.trim().length !== 6} aria-busy={tfBusy}>Turn on</button>
			<button class="btn btn-quiet" type="button" onclick={() => (setup = null)}>Cancel</button>
		</form>
	{:else}
		<button class="btn btn-red" type="button" onclick={startTwoFactor} disabled={tfBusy} aria-busy={tfBusy}>
			<Icon name="shield" size={18} /> Set up two-factor sign-in
		</button>
	{/if}
	{#if tfError}<p class="form-error" role="alert">{tfError}</p>{/if}
</section>

<section class="card" aria-labelledby="h-alerts">
	<h2 id="h-alerts" class="h2">Alerts for you</h2>
	<p class="small muted">You always see news about your asks on your Wishlist. Add these to also hear by email or in Discord, when the owner has set those up.</p>
	<form class="pw" onsubmit={saveTargets} novalidate>
		<div class="field">
			<label for="tg-email">Email address</label>
			<input id="tg-email" class="input" type="email" bind:value={targets.email} autocomplete="email" />
		</div>
		<div class="field">
			<label for="tg-discord">Discord user id</label>
			<input id="tg-discord" class="input mono" inputmode="numeric" bind:value={targets.discord_id} autocomplete="off" />
			<span class="hint">A long number: in Discord turn on Developer Mode, then right-click your name and Copy User ID. Discord Alerts then name you.</span>
		</div>
		{#if tgError}<p class="form-error" role="alert">{tgError}</p>{/if}
		<button class="btn btn-red" type="submit" disabled={tgBusy} aria-busy={tgBusy}>Save</button>
	</form>
</section>

<section class="card" aria-labelledby="h-pw">
	<h2 id="h-pw" class="h2">Password</h2>
	<form class="pw" onsubmit={changePassword} novalidate>
		<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
		<div class="field">
			<label for="pw-current">Current password</label>
			<input id="pw-current" class="input" type="password" bind:value={current} autocomplete="current-password" />
			<span class="hint">Confirm your local sign-in above before changing this password.</span>
		</div>
		<div class="field">
			<label for="pw-new">New password</label>
			<input id="pw-new" class="input" type="password" bind:value={next} autocomplete="new-password" />
			<span class="hint">At least {MIN_LENGTH} characters.</span>
		</div>
		<div class="field">
			<label for="pw-again">New password again</label>
			<input id="pw-again" class="input" type="password" bind:value={again} autocomplete="new-password" />
		</div>
		{#if pwError}<p class="form-error" role="alert">{pwError}</p>{/if}
		<button class="btn btn-red" type="submit" disabled={pwBusy || !next || !session.s.localRecent} aria-busy={pwBusy}>Change password</button>
	</form>
	{#if session.s.role === 'owner'}
		<p class="small muted" style="margin-top: var(--s-4)">
			Locked out? Run <code>holyarrs reset-owner</code> on the machine that runs HolyArrs. It sets a new name and password, turns off
			two-factor sign-in, and signs out every device and API key.
		</p>
	{/if}
</section>

<section class="card" aria-labelledby="h-devices">
	<h2 id="h-devices" class="h2">Signed-in devices</h2>
	<p class="small muted" style="margin-bottom: var(--s-3)">Every browser or phone where you are signed in. Sign out one you do not know, or all but this one.</p>
	<Sessions path="/auth/sessions" />
</section>

<style>
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.codes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(9rem, 1fr));
		gap: var(--s-1) var(--s-4);
		margin: var(--s-3) 0;
		list-style: none;
		padding: 0;
	}
	.steps {
		margin: 0 0 var(--s-4) var(--s-5);
		display: grid;
		gap: var(--s-2);
	}
	.key {
		display: block;
		margin-top: var(--s-2);
		padding: 10px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		overflow-wrap: anywhere;
	}
	.tf-form,
	.pw {
		margin-top: var(--s-4);
		max-width: 420px;
	}
	fieldset {
		border: 0;
		padding: 0;
		margin: 0;
	}
	.pw button,
	.tf-form button {
		margin-top: var(--s-4);
	}
</style>
