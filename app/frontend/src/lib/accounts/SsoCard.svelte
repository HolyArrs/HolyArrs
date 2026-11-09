<script lang="ts">
	// Single sign-on (OpenID Connect) set up from Settings, for the owner. The server keeps the rules: the password goes with every save, a saved secret never follows a new issuer or client id, and moving to another provider removes the old links.
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
		if (mustRetype) return void (saveError = 'Type the client secret again, or tick "Public client": a saved secret never goes to a new issuer or client id.');
		if (opening && !confirmEveryone) return void (saveError = 'Tick the box to confirm that everyone this provider signs in may join.');
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
			const gone = [out.links_removed && `${out.links_removed} linked sign-in${out.links_removed === 1 ? '' : 's'} removed`, out.sessions_ended && `${out.sessions_ended} signed out`].filter(Boolean).join(', ');
			toasts.push('Single sign-on saved', 'ok', gone ? { body: gone } : undefined);
			await session.loadWays();
		} catch (err) {
			saveError = err instanceof ApiError && err.body.code === 'wrong_proof' ? 'The password or code is wrong.' : message(err);
		} finally {
			// A failed save keeps the typed secret, so fixing the password does not save without it.
			busy = false;
		}
	}

	async function copyCallback() {
		try {
			await navigator.clipboard.writeText(callback);
			toasts.push('Callback address copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}
</script>

<SettingsCard {id} title="Single sign-on">
	<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">
		A Sign in with single sign-on button for an OpenID Connect provider, such as Authentik, Authelia or Keycloak. Linked people sign in with
		their provider account; the owner keeps a HolyArrs password.
	</p>
	<Confirm action="link">
		{#if loadError}
			<p class="notice warn" role="alert">{loadError}</p>
			<Button type="button" onclick={load}>Try again</Button>
		{:else if !view}
			<p class="small muted">Loading…</p>
		{:else if env}
			<div class="sso" data-testid="sso-env">
				<p class="notice">Set in the environment (<code>HOLYARRS_OIDC_*</code>), so it is only shown here. Change it where HolyArrs starts.</p>
				<dl class="facts">
					<dt>Single sign-on</dt><dd>{view.on ? 'On' : 'Off'}</dd>
					{#if view.on}
						<dt>Issuer</dt><dd class="mono">{view.issuer}</dd>
						<dt>Client ID</dt><dd class="mono">{view.client_id}</dd>
						<dt>Callback</dt><dd class="mono">{view.redirect_url}</dd>
						<dt>New people</dt><dd>{view.allow_join ? 'May join' : 'Only people already linked'}</dd>
					{/if}
				</dl>
			</div>
		{:else}
			<form class="sso" onsubmit={save} data-testid="sso-form">
				<fieldset disabled={busy}>
					<label class="switch">
						<input type="checkbox" bind:checked={on} />
						<span class="sw" aria-hidden="true"></span>
						<span>Show the single sign-on button on the sign-in page</span>
					</label>
					<Field id="sso-issuer" label="Issuer" hint="The provider's issuer address, exactly as it publishes it (https://).">
						<div class="row">
							<input id="sso-issuer" class="input mono grow" bind:value={issuer} oninput={() => (tested = null)} placeholder="https://auth.example.com/application/o/holyarrs/" maxlength="1000" spellcheck="false" autocomplete="off" />
							<Button type="button" onclick={test} disabled={!issuer.trim() || testing} aria-busy={testing}>Test</Button>
						</div>
					</Field>
					{#if tested}<div class="testline" data-testid="sso-tested"><TestTag ok={tested.ok} text={tested.text} app="OpenID Connect" />{#if !tested.ok}<span class="small bad-text">{tested.text}</span>{/if}</div>{/if}
					<Field id="sso-client" label="Client ID">
						<input id="sso-client" class="input mono" bind:value={clientId} maxlength="1000" spellcheck="false" autocomplete="off" />
					</Field>
					<Field
						id="sso-secret"
						label="Client secret"
						hint={view.secret_set && !moving ? 'A secret is saved; it is never shown. Leave this empty to keep it.' : 'Kept encrypted and never shown again.'}>
						<input id="sso-secret" class="input mono" type="password" bind:value={typedCred} disabled={publicClient} maxlength="4096" autocomplete="off" spellcheck="false" />
					</Field>
					<label class="check small">
						<input type="checkbox" bind:checked={publicClient} />
						Public client (no secret; the provider is set up for that)
					</label>
					<Field id="sso-callback" label="Callback address" hint="Register this exact address at the provider." style="display: flex !important; flex-direction: column; align-items: stretch !important; gap: 6px">
						<div class="row callback-row">
							<input id="sso-callback" class="input mono grow" value={callback} readonly />
							<Button icon="copy" type="button" onclick={copyCallback}>Copy</Button>
						</div>
					</Field>
					<label class="check small">
						<input type="checkbox" bind:checked={allowJoin} />
						New people the provider signs in may join, as Members who may ask for titles
					</label>
					{#if opening}
						<label class="check small notice warn">
							<input type="checkbox" bind:checked={confirmEveryone} data-testid="sso-confirm-everyone" />
							Yes, let everyone this provider signs in make an account here
						</label>
					{/if}
					{#if moving && view.links}
						<p class="notice warn" data-testid="sso-moving">
							Saving a new issuer or client id removes the {view.links} linked sign-in{view.links === 1 ? '' : 's'} made through the old one and signs out everyone who used single sign-on. They link again from My account.
						</p>
					{/if}
					{#if plainHttp}<p class="small muted">Not on https://, so the secret is sent unencrypted. Use your https address.</p>{/if}
					<Field id="sso-password" label="Your HolyArrs password" hint="Needed for every change here.">
						<input id="sso-password" class="input" type="password" bind:value={proof} autocomplete="current-password" required />
					</Field>
					{#if session.s.twoFactor}
						<Field id="sso-code" label="Two-factor code">
							<input id="sso-code" class="input mono" bind:value={code} autocomplete="one-time-code" maxlength="11" required />
						</Field>
					{/if}
					{#if saveError}<p class="form-error" role="alert">{saveError}</p>{/if}
					<Button variant="primary" type="submit" disabled={busy || !proof} aria-busy={busy}>Save</Button>
				</fieldset>
			</form>
		{/if}
		{#if view}
			<p class="small muted forward" data-testid="sso-forward">
				Trusted proxy sign-in: {view.forward.on ? `on, through ${view.forward.issuer}` : 'off'}. It is set only in the environment (<code>HOLYARRS_FORWARD_AUTH_*</code>), because the
				addresses it trusts decide who may sign in as anyone.
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
