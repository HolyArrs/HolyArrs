<script lang="ts">
	// Single sign-on (OpenID Connect) set up from Settings, for the owner. The server keeps the rules: the password goes with every save, a saved secret never follows a new issuer or client id, and moving to another provider removes the old links.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	// The card's anchor; the page passes it so Settings search finds it there.
	let { id = 'sso-card' }: { id?: string } = $props();

	interface SsoView {
		source: 'settings' | 'environment';
		on: boolean;
		running: boolean;
		issuer: string;
		client_id: string;
		secret_set: boolean;
		redirect_url: string;
		suggested_redirect_url: string;
		allow_join: boolean;
		links: number;
		local_network_allowed: boolean;
		forward: { on: boolean; issuer?: string; allow_join: boolean; links: number };
	}

	let view = $state<SsoView | null>(null);
	let loadError = $state('');
	let on = $state(false);
	let issuer = $state('');
	let clientId = $state('');
	let typedCred = $state(''); // The client secret as typed; cleared after a save that worked.
	let publicClient = $state(false);
	let allowJoin = $state(false);
	let proof = $state(''); // The owner's HolyArrs password, sent with every save.
	let code = $state('');
	let busy = $state(false);
	let saveError = $state('');
	let tested = $state<{ ok: boolean; text: string } | null>(null);
	let testing = $state(false);
	let confirmEveryone = $state(false);

	const env = $derived(view?.source === 'environment');
	const callback = $derived(view?.redirect_url || view?.suggested_redirect_url || '');
	const trimmed = (s: string) => s.trim().replace(/\/+$/, '');
	// A new issuer or client id: the old links stop and the secret must be typed again.
	const moving = $derived(!!view?.issuer && (trimmed(issuer) !== trimmed(view.issuer) || clientId.trim() !== view.client_id));
	const mustRetype = $derived(moving && !!view?.secret_set && !typedCred && !publicClient);
	const opening = $derived(on && allowJoin && !(view?.on && view?.allow_join && !moving));
	const plainHttp = typeof location !== 'undefined' && location.protocol !== 'https:';

	function fill(v: SsoView) {
		view = v;
		on = v.on;
		issuer = v.issuer;
		clientId = v.client_id;
		allowJoin = v.allow_join;
		publicClient = !v.secret_set && !!v.issuer;
		typedCred = '';
	}

	async function load() {
		loadError = '';
		try {
			fill(await api.get<SsoView>('/auth/sso'));
		} catch (e) {
			loadError = message(e);
		}
	}

	// The setup opens once the local sign-in is fresh (Confirm asks first).
	$effect(() => {
		if (session.s.localRecent && !view && !loadError) void load();
	});

	async function test() {
		testing = true;
		tested = null;
		try {
			await api.post('/auth/sso/test', { issuer: issuer.trim() });
			tested = { ok: true, text: issuer.trim() };
		} catch (e) {
			tested = { ok: false, text: message(e) };
		} finally {
			testing = false;
		}
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		saveError = '';
		if (mustRetype) return void (saveError = t('accounts.sso.retype'));
		if (opening && !confirmEveryone) return void (saveError = t('accounts.sso.confirmBox'));
		busy = true;
		try {
			const out = await api.put<{ sso: SsoView; links_removed: number; sessions_ended: number }>('/auth/sso', {
				password: proof,
				code: code.trim() || undefined,
				on,
				issuer: issuer.trim(),
				client_id: clientId.trim(),
				client_secret: typedCred || undefined,
				public_client: publicClient,
				redirect_url: callback,
				allow_join: allowJoin,
				confirm_everyone: confirmEveryone
			});
			fill(out.sso); // Clears the typed secret too.
			proof = code = '';
			confirmEveryone = false;
			const gone = [out.links_removed && t(out.links_removed === 1 ? 'accounts.sso.linkRemoved.one' : 'accounts.sso.linkRemoved.other', { n: out.links_removed }), out.sessions_ended && t('accounts.sso.signedOut', { n: out.sessions_ended })].filter(Boolean).join(', ');
			toasts.push(t('accounts.sso.saved'), 'ok', gone ? { body: gone } : undefined);
			await session.loadWays();
		} catch (err) {
			saveError = err instanceof ApiError && err.body.code === 'wrong_proof' ? t('accounts.sso.wrongProof') : message(err);
		} finally {
			// A failed save keeps the typed secret, so fixing the password does not save without it.
			busy = false;
		}
	}

	async function copyCallback() {
		try {
			await navigator.clipboard.writeText(callback);
			toasts.push(t('accounts.sso.copied'), 'ok');
		} catch {
			toasts.push(t('accounts.sso.copyFailed'), 'fail', { body: t('accounts.sso.selectInstead') });
		}
	}
</script>

<SettingsCard {id} title={t('accounts.sso.title')}>
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
		{t('accounts.sso.intro')}
	</p>
	<Confirm action="link">
		{#if loadError}
			<p class="notice warn" role="alert">{loadError}</p>
			<Button type="button" onclick={load}>{t('accounts.sso.tryAgain')}</Button>
		{:else if !view}
			<p class="small muted">{t('accounts.sso.loading')}</p>
		{:else if env}
			<div class="sso" data-testid="sso-env">
				<p class="notice">{t('accounts.sso.envBefore')} (<code>HOLYARRS_OIDC_*</code>){t('accounts.sso.envAfter')}</p>
				<dl class="facts">
					<dt>{t('accounts.sso.title')}</dt><dd>{view.on ? t('accounts.sso.on') : t('accounts.sso.off')}</dd>
					{#if view.on}
						<dt>{t('accounts.sso.issuer')}</dt><dd class="mono">{view.issuer}</dd>
						<dt>{t('accounts.sso.clientId')}</dt><dd class="mono">{view.client_id}</dd>
						<dt>{t('accounts.sso.callback')}</dt><dd class="mono">{view.redirect_url}</dd>
						<dt>{t('accounts.sso.newPeople')}</dt><dd>{view.allow_join ? t('accounts.sso.mayJoin') : t('accounts.sso.onlyLinked')}</dd>
					{/if}
				</dl>
			</div>
		{:else}
			<form class="sso" onsubmit={save} data-testid="sso-form">
				<fieldset disabled={busy}>
					<label class="switch">
						<input type="checkbox" bind:checked={on} />
						<span class="sw" aria-hidden="true"></span>
						<span>{t('accounts.sso.showButton')}</span>
					</label>
					<Field id="sso-issuer" label={t('accounts.sso.issuer')} hint={t('accounts.sso.issuerHint')}>
						<div class="row">
							<input id="sso-issuer" class="input mono grow" bind:value={issuer} oninput={() => (tested = null)} placeholder="https://auth.example.com/application/o/holyarrs/" maxlength="1000" spellcheck="false" autocomplete="off" />
							<Button type="button" onclick={test} disabled={!issuer.trim() || testing} aria-busy={testing}>{t('accounts.sso.test')}</Button>
						</div>
					</Field>
					{#if tested}<div class="testline" data-testid="sso-tested"><TestTag ok={tested.ok} text={tested.text} app="OpenID Connect" />{#if !tested.ok}<span class="small bad-text">{tested.text}</span>{/if}</div>{/if}
					<Field id="sso-client" label={t('accounts.sso.clientId')}>
						<input id="sso-client" class="input mono" bind:value={clientId} maxlength="1000" spellcheck="false" autocomplete="off" />
					</Field>
					<Field
						id="sso-secret"
						label={t('accounts.sso.secretLabel')}
						hint={view.secret_set && !moving ? t('accounts.sso.secretSaved') : t('accounts.sso.secretNew')}>
						<input id="sso-secret" class="input mono" type="password" bind:value={typedCred} disabled={publicClient} maxlength="4096" autocomplete="off" spellcheck="false" />
					</Field>
					<label class="check small">
						<input type="checkbox" bind:checked={publicClient} />
						{t('accounts.sso.publicClient')}
					</label>
					<Field id="sso-callback" label={t('accounts.sso.callbackAddress')} hint={t('accounts.sso.callbackHint')} style="display: flex !important; flex-direction: column; align-items: stretch !important; gap: 6px">
						<div class="row callback-row">
							<input id="sso-callback" class="input mono grow" value={callback} readonly />
							<Button icon="copy" type="button" onclick={copyCallback}>{t('accounts.sso.copy')}</Button>
						</div>
					</Field>
					<label class="check small">
						<input type="checkbox" bind:checked={allowJoin} />
						{t('accounts.sso.allowJoin')}
					</label>
					{#if opening}
						<label class="check small notice warn">
							<input type="checkbox" bind:checked={confirmEveryone} data-testid="sso-confirm-everyone" />
							{t('accounts.sso.confirmEveryone')}
						</label>
					{/if}
					{#if moving && view.links}
						<p class="notice warn" data-testid="sso-moving">
							{t(view.links === 1 ? 'accounts.sso.movingOne' : 'accounts.sso.movingMany', { n: view.links })}
						</p>
					{/if}
					{#if plainHttp}<p class="small muted">{t('accounts.sso.plainHttp')}</p>{/if}
					<Field id="sso-password" label={t('accounts.sso.yourPasswordLabel')} hint={t('accounts.sso.yourPasswordHint')}>
						<input id="sso-password" class="input" type="password" bind:value={proof} autocomplete="current-password" required />
					</Field>
					{#if session.s.twoFactor}
						<Field id="sso-code" label={t('accounts.sso.twoFactor')}>
							<input id="sso-code" class="input mono" bind:value={code} autocomplete="one-time-code" maxlength="11" required />
						</Field>
					{/if}
					{#if saveError}<p class="form-error" role="alert">{saveError}</p>{/if}
					<Button variant="primary" type="submit" disabled={busy || !proof} aria-busy={busy}>{t('accounts.sso.save')}</Button>
				</fieldset>
			</form>
		{/if}
		{#if view}
			<p class="small muted forward" data-testid="sso-forward">
				{t('accounts.sso.proxyLead', { state: view.forward.on ? t('accounts.sso.proxyOn', { issuer: view.forward.issuer ?? '' }) : t('accounts.sso.off').toLowerCase() })} (<code>HOLYARRS_FORWARD_AUTH_*</code>){t('accounts.sso.proxyAfter')}
			</p>
		{/if}
	</Confirm>
</SettingsCard>

<style>
	.testline { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; }
	.sso fieldset {
		border: 0;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-3);
		max-width: 560px;
		min-width: 0;
	}
	.sso :global(button) {
		justify-self: start;
	}
	.check {
		display: flex;
		align-items: flex-start;
		gap: 8px;
	}
	.check input {
		margin-top: 3px;
	}
	/* Touch phones: 44px tap targets; desktop stays as it is. */
	@media (max-width: 767px) and (pointer: coarse) {
		.check {
			min-height: 44px;
			align-items: center;
		}
		.check input {
			margin-top: 0;
		}
	}
	/* Phones: the whole callback address shows, Copy goes under it. */
	@media (max-width: 560px) {
		.callback-row {
			flex-direction: column;
			align-items: stretch;
		}
		.callback-row :global(button) {
			align-self: flex-start;
		}
	}
	.facts {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 6px 16px;
		margin: var(--s-3) 0 0;
	}
	.facts dt {
		color: var(--text-muted);
	}
	.facts dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.forward {
		margin-top: var(--s-4);
	}
</style>
