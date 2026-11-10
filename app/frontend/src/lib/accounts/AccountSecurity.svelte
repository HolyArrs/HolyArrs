<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
			toasts.push(t('accounts.security.tfOnToast'), 'ok');
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
				toasts.push(t('accounts.security.renewed'), 'ok', { body: t('accounts.security.oldStop') });
			} else {
				codes = [];
				tfOpen = false;
				await session.load();
				toasts.push(t('accounts.security.tfOffToast'), 'ok');
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
			toasts.push(t('accounts.security.copied'), 'ok');
		} catch {
			toasts.push(t('accounts.sso.copyFailed'), 'fail', { body: t('accounts.security.writeDown') });
		}
	}

	// Single sign-on and trusted proxy identities linked to me. Unlinking one also signs out the sessions it made and needs a fresh password sign-in.
	type SsoLink = { kind: 'oidc' | 'forward_auth'; issuer: string; name: string; created_at: string };
	let myLinks = $state<SsoLink[]>([]);
	let unlinkKey = $state('');
	const linkKey = (l: SsoLink) => `${l.kind}|${l.issuer}`;
	const linkWord = (k: string) => (k === 'oidc' ? t('accounts.tiles.sso') : t('accounts.tiles.proxy'));
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
			toasts.push(t('accounts.security.unlinked', { what: linkWord(l.kind) }), 'ok', { body: t('accounts.security.devicesOut') });
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
			toasts.push(t('accounts.security.ssoLinked'), 'ok');
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
			toasts.push(t('accounts.security.proxyLinked'), 'ok');
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
				linkError = t('accounts.security.popup');
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
				toasts.push(t('accounts.security.plexLinked', { name: done.name }), 'ok');
				return;
			}
			linkError = t('accounts.security.plexTimeout');
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
			toasts.push(t('accounts.security.plexUnlinked'), 'ok', { body: t('accounts.security.devicesOut') });
		} catch (err) {
			linkError = message(err);
		} finally {
			linkBusy = false;
		}
	}
</script>

<SettingsCard id="security-me" title={t('accounts.security.title')} icon="shield">
	<div class="srow" id="twofactor-me">
		<div class="txt"><span class="label">{t('accounts.security.tf')}</span><span class="hint">{t('accounts.security.tfHint')}</span></div>
		<div class="ctl">
			<span class="chip" class:ok={session.s.twoFactor}>{session.s.twoFactor ? t('accounts.sso.on') : t('accounts.sso.off')}</span>
			{#if !tfOpen && !setup}
				<Button variant="secondary" type="button" onclick={openTwoFactor} busy={tfBusy} data-testid="two-factor-open">{session.s.twoFactor ? t('accounts.security.manage') : t('accounts.security.setUp')}</Button>
			{/if}
		</div>
		{#if codes.length || tfOpen || setup || tfError}
			<div class="tf below">
				{#if codes.length}
					<div class="fresh" role="status">
						<p class="small"><b>{t('accounts.security.backupHead')}</b> {t('accounts.security.backupText')}</p>
						<ul class="codes mono">
							{#each codes as c (c)}<li>{c}</li>{/each}
						</ul>
						<div class="row acts">
							<Button icon="copy" type="button" onclick={copyCodes}>{t('accounts.sso.copy')}</Button>
							<Button variant="secondary" type="button" onclick={() => (codes = [])}>{t('accounts.security.close')}</Button>
						</div>
					</div>
				{/if}
				{#if tfOpen || setup}
					<Confirm action="two-factor" onCancel={() => (tfOpen = false)}>
						{#if session.s.twoFactor}
							<form class="tf-form" onsubmit={(e) => withCode(offMode === 'off' ? '/auth/two-factor/disable' : '/auth/two-factor/backup-codes', e)}>
								<fieldset class="modes">
									<legend class="sr-only">{t('accounts.security.enterCode')}</legend>
									<label class="checkline"><input type="radio" bind:group={offMode} value="codes" /> {t('accounts.security.newCodes')}</label>
									<label class="checkline"><input type="radio" bind:group={offMode} value="off" /> {t('accounts.security.turnOff')}</label>
								</fieldset>
								<div class="row acts">
									<label class="sr-only" for="off-code">{t('accounts.security.code')}</label>
									<input id="off-code" class="input mono six" placeholder={t('accounts.security.code')} bind:value={offCode} autocomplete="one-time-code" inputmode="text" maxlength="11" />
									<Button variant={offMode === 'off' ? 'destructive' : 'secondary'} type="submit" disabled={tfBusy || !offCode.trim()} busy={tfBusy}>
										{offMode === 'off' ? t('accounts.security.turnOff') : t('accounts.security.makeCodes')}
									</Button>
									<Button variant="plain" type="button" onclick={() => (tfOpen = false)}>{t('accounts.signin.cancel')}</Button>
								</div>
							</form>
						{:else if setup}
							<ol class="steps small">
								<li>
									{#if setup.qr?.length}
										{t('accounts.security.scan')}
										<QrCode rows={setup.qr} label={t('accounts.security.qr')} />
										<Words key="accounts.security.orOpenApp" links={{ a: setup.uri }} linkClass="ul" />
									{:else}
										{t('accounts.security.addBefore')} (<a class="ul" href={setup.uri}>{t('accounts.security.openAppShort')}</a>){t('accounts.security.addAfter')}
									{/if}
									<code class="key">{setup.secret.replace(/(.{4})/g, '$1 ').trim()}</code>
								</li>
								<li>{t('accounts.security.typeSix')}</li>
							</ol>
							<form class="row acts" onsubmit={enable}>
								<label class="sr-only" for="tf-code">{t('accounts.security.sixDigit')}</label>
								<input id="tf-code" class="input mono six" bind:value={code} autocomplete="one-time-code" inputmode="numeric" maxlength="6" placeholder="123456" />
								<Button variant="primary" type="submit" disabled={tfBusy || code.trim().length !== 6} busy={tfBusy}>{t('accounts.security.turnOn')}</Button>
								<Button variant="plain" type="button" onclick={() => { setup = null; tfOpen = false; }}>{t('accounts.signin.cancel')}</Button>
							</form>
						{:else}
							<div class="row acts"><Button variant="primary" type="button" onclick={startTwoFactor} busy={tfBusy}>{t('accounts.security.continue')}</Button></div>
						{/if}
					</Confirm>
				{/if}
				{#if tfError}<p class="form-error" role="alert">{tfError}</p>{/if}
			</div>
		{/if}
	</div>
	{#if hasLinkWay || myLinks.length || plexMine?.linked || (plexMine && session.s.hasPassword)}
		<div class="srow" id="linked-me">
			<div class="txt"><span class="label">{t('accounts.security.linked')}</span><span class="hint" title={t('accounts.security.matched')}>{t('accounts.security.matched')}</span></div>
			<div class="ctl">{#if !linkOpen && (hasLinkWay || session.s.hasPassword)}<Button variant="secondary" type="button" onclick={() => (linkOpen = true)} data-testid="link-sign-in">{t('accounts.security.link')}</Button>{/if}</div>
			{#if plexMine?.linked}
				<ul class="below my-links" data-testid="my-plex-link">
					<li>
						<span class="small">{plexMine.name ? t('accounts.security.plexAs', { name: plexMine.name }) : 'Plex'}</span>
						{#if !session.s.hasPassword}
							<span class="xs muted" data-testid="plex-needs-password">{t('accounts.security.needsPasswordLabel')}</span>
						{:else if plexUnlinking}
							<Confirm action="link" onCancel={() => (plexUnlinking = false)}>
								<div class="row acts">
									<Button variant="secondary" type="button" onclick={unlinkPlexMine} busy={linkBusy}>{t('accounts.security.unlink')}</Button>
									<Button variant="plain" type="button" onclick={() => (plexUnlinking = false)}>{t('accounts.signin.cancel')}</Button>
								</div>
							</Confirm>
						{:else}
							<Button variant="plain" type="button" onclick={() => (plexUnlinking = true)} aria-label={t('accounts.security.unlinkPlex')}>{t('accounts.security.unlink')}</Button>
						{/if}
					</li>
				</ul>
			{/if}
			{#if myLinks.length}
				<ul class="below my-links" data-testid="my-sso-links">
					{#each myLinks as l (linkKey(l))}
						<li>
							<span class="small">{t('accounts.security.linkAt', { what: linkWord(l.kind) })} <span class="mono">{hostOf(l.issuer)}</span>{#if l.name}{t('accounts.security.linkAs', { name: l.name })}{/if}</span>
							{#if unlinkKey === linkKey(l)}
								<Confirm action="link" onCancel={() => (unlinkKey = '')}>
									<div class="row acts">
										<Button variant="secondary" type="button" onclick={() => unlink(l)} busy={linkBusy}>{t('accounts.security.unlink')}</Button>
										<Button variant="plain" type="button" onclick={() => (unlinkKey = '')}>{t('accounts.signin.cancel')}</Button>
									</div>
								</Confirm>
							{:else}
								<Button variant="plain" type="button" onclick={() => (unlinkKey = linkKey(l))} aria-label={t('accounts.security.unlinkAt', { what: linkWord(l.kind), host: hostOf(l.issuer) })}>{t('accounts.security.unlink')}</Button>
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
									<Button type="button" onclick={linkPlexMine} busy={linkBusy}>{plexMine?.linked ? t('accounts.security.plexAnother') : 'Plex'}</Button>
								{/if}
								{#if session.s.ways.includes('oidc')}
									<Button type="button" onclick={startOIDCLink} busy={linkBusy}>{t('accounts.tiles.sso')}</Button>
								{/if}
								{#if session.s.ways.includes('forward_auth')}
									<Button type="button" onclick={linkForwardAuth} busy={linkBusy}>{t('accounts.tiles.proxy')}</Button>
								{/if}
								<Button variant="plain" type="button" onclick={() => (linkOpen = false)}>{t('accounts.signin.cancel')}</Button>
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
			<div class="txt"><span class="label">{t('accounts.security.locked')}</span><span class="hint">{t('accounts.signin.runThis')}</span></div>
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