<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { needsConfirm } from '#lib/ui/sensitive.ts';
	import QrCode from './QrCode.svelte';
	import { strong } from './strong-error.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let setup = $state<{ secret: string; uri: string; qr?: string[] } | null>(null);
	let code = $state('');
	let offCode = $state('');
	let tfError = $state('');
	let tfBusy = $state(false);
	let codes = $state<string[]>([]);

	let linkOpen = $state(false);
	let linkError = $state('');
	let linkBusy = $state(false);
	const hasLinkWay = $derived(session.s.ways.includes('oidc') || session.s.ways.includes('forward_auth'));

	onMount(() => {
		void loadLinks();
		void loadPlexMine();
		void session.loadWays().then(() => {
			if (page.url.searchParams.get('sso') === 'finish') void finishOIDCLink();
		});
	});

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
			toasts.push('Two-factor on', 'ok');
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
				toasts.push('Backup codes renewed', 'ok', { body: 'Old ones stop working' });
			} else {
				codes = [];
				tfOpen = false;
				await session.load();
				toasts.push('Two-factor off', 'ok');
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
			toasts.push('Codes copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Write the codes down' });
		}
	}

	// Single sign-on and trusted proxy identities linked to me. Unlinking one also signs out the sessions it made and needs a fresh password sign-in.
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
			toasts.push(`${linkWord(l.kind)} unlinked`, 'ok', { body: 'Its devices signed out' });
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
			toasts.push('Single sign-on linked', 'ok');
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
			toasts.push('Trusted proxy sign-in linked', 'ok');
			await loadLinks();
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}

	// My Plex account: linking keeps its sign-in on the server (never shown) so my Plex watchlist can be read. Anyone may link one.
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
				linkError = 'Pop-up blocked. Allow pop-ups and try again.';
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
				toasts.push(`Plex ${done.name} linked`, 'ok');
				return;
			}
			linkError = 'Plex sign-in timed out. Try again.';
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
			try {
				win?.close();
			} catch {
				// The window may already be gone.
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
			toasts.push('Plex unlinked', 'ok', { body: 'Its devices signed out' });
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}
</script>

<SettingsCard id="security-me" title="Security" icon="shield">
	<div class="srow" id="twofactor-me">
		<div class="txt"><span class="label">Two-factor</span><span class="hint">A code from your phone at sign-in</span></div>
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
						<p class="small"><b>Backup codes.</b> Each works once; save them now.</p>
						<ul class="codes mono">
							{#each codes as c (c)}<li>{c}</li>{/each}
						</ul>
						<div class="row acts">
							<Button icon="copy" type="button" onclick={copyCodes}>Copy</Button>
							<Button variant="secondary" type="button" onclick={() => (codes = [])}>Close</Button>
						</div>
					</div>
				{/if}
				{#if tfOpen || setup}
					<Confirm action="two-factor" onCancel={() => (tfOpen = false)}>
						{#if session.s.twoFactor}
							<form class="tf-form" onsubmit={(e) => withCode(offMode === 'off' ? '/auth/two-factor/disable' : '/auth/two-factor/backup-codes', e)}>
								<fieldset class="modes">
									<legend class="sr-only">Enter an app or backup code</legend>
									<label class="checkline"><input type="radio" bind:group={offMode} value="codes" /> New backup codes</label>
									<label class="checkline"><input type="radio" bind:group={offMode} value="off" /> Turn off</label>
								</fieldset>
								<div class="row acts">
									<label class="sr-only" for="off-code">Code</label>
									<input id="off-code" class="input mono six" placeholder="Code" bind:value={offCode} autocomplete="one-time-code" inputmode="text" maxlength="11" />
									<Button variant={offMode === 'off' ? 'destructive' : 'secondary'} type="submit" disabled={tfBusy || !offCode.trim()} busy={tfBusy}>
										{offMode === 'off' ? 'Turn off' : 'Make new codes'}
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
			<div class="txt"><span class="label">Linked sign-ins</span><span class="hint" title="Matched by account ID, never by email">Matched by account ID, never by email</span></div>
			<div class="ctl">{#if !linkOpen && (hasLinkWay || session.s.hasPassword)}<Button variant="secondary" type="button" onclick={() => (linkOpen = true)} data-testid="link-sign-in">Link</Button>{/if}</div>
			{#if plexMine?.linked}
				<ul class="below my-links" data-testid="my-plex-link">
					<li>
						<span class="small">Plex{#if plexMine.name}, as {plexMine.name}{/if}</span>
						{#if !session.s.hasPassword}
							<span class="xs muted" data-testid="plex-needs-password">Set a password above to unlink</span>
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
			<div class="txt"><span class="label">Locked out?</span><span class="hint">Run this where HolyArrs runs.</span></div>
			<div class="ctl"><code class="cmd">holyarrs reset-owner</code></div>
		</div>
	{/if}
</SettingsCard>

<style>
	.my-links { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
	.my-links li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; }
	.my-links .mono { overflow-wrap: anywhere; }
	code { font-family: var(--font-mono); color: var(--text); }
	.acts { gap: 8px; flex-wrap: wrap; align-items: center; }
	.tf, .tf-form { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.tf-form { width: min(100%, 360px); }
	.six { max-width: 140px; }
	.modes { border: 0; padding: 0; margin: 0; display: flex; gap: 16px; flex-wrap: wrap; }
	.modes .checkline { min-height: 32px; padding: 0; align-items: center; }
	/* Shown-once secrets look the same as on Users and Sign-in: a red edge. */
	.fresh { padding: var(--s-4); border: 1px solid var(--ha-red); background: var(--ha-red-soft); border-radius: var(--r-btn); }
	.codes { display: grid; grid-template-columns: repeat(auto-fill, minmax(8rem, 1fr)); gap: 2px 12px; margin: 8px 0; list-style: none; padding: 0; }
	.steps { margin: 0 0 0 var(--s-5); display: grid; gap: 6px; list-style: decimal; }
	.key { display: block; margin-top: 6px; padding: 8px 10px; background: var(--surface-2); border-radius: var(--r-sm); overflow-wrap: anywhere; }
	.cmd { font-size: 13px; padding: 4px 8px; border-radius: 6px; background: var(--surface-2); white-space: nowrap; }
		.six { max-width: none; flex: 1; }
	@media (max-width: 600px) {
		.six { max-width: none; flex: 1; }
	}
</style>