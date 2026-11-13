<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Asks for the password again before a sensitive action, else shows the content.
	import type { Snippet } from 'svelte';
	import Button from '#lib/ui/Button.svelte';
	import { needsConfirm, signInFirst, type Sensitive } from './sensitive.ts';
	import { page } from '$app/state';
	import { appPath, to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	let { action, onCancel, children }: { action: Sensitive; onCancel?: () => void; children: Snippet } = $props();
	const id = $props.id();

	let typed = $state('');
	let code = $state('');
	let error = $state('');
	let busy = $state(false);

	const need = $derived(needsConfirm(action, { localRecent: session.s.localRecent, via: session.s.via, hasPassword: session.s.hasPassword, mustTwoFactor: session.s.mustTwoFactor }));

	// A home visit that skipped sign-in cannot confirm a password: it signs in on the sign-in page and comes back here.
	const signInLink = $derived(signInFirst({ via: session.s.via }, appPath(page.url.pathname) + page.url.search));

	// Secrets are cleared whether or not it worked.
	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			await session.reauth(typed, code.trim() || undefined);
			toasts.push(t('ui.confirm.signInConfirmed'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			typed = '';
			code = '';
			busy = false;
		}
	}
</script>

{#if need && signInLink}
	<div class="confirm" data-testid="confirm-local-visit">
		<p class="hint">{t('ui.confirm.localVisit')}</p>
		<div class="row acts">
			<Button size="sm" variant="primary" href={to(signInLink)}>{t('ui.confirm.signInFirst')}</Button>
			{#if onCancel}<Button size="sm" variant="plain" type="button" onclick={onCancel}>{t('ui.confirm.cancel')}</Button>{/if}
		</div>
	</div>
{:else if need}
	<form class="confirm" onsubmit={submit} novalidate data-testid="confirm-sign-in">
		<p class="hint">{t('ui.confirm.confirmItSYou')}</p>
		<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
		<div class="field">
			<label for="{id}-pw">{t('ui.confirm.holyArrsPasswordLabel')}</label>
			<input id="{id}-pw" class="input" type="password" bind:value={typed} autocomplete="current-password" required />
		</div>
		{#if session.s.twoFactor}
			<div class="field">
				<label for="{id}-code">{t('ui.confirm.twoFactorCode')}</label>
				<input id="{id}-code" class="input mono" bind:value={code} autocomplete="one-time-code" maxlength="11" required />
			</div>
		{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="row acts">
			<Button size="sm" variant="primary" type="submit" disabled={busy || !typed || (session.s.twoFactor && !code.trim())} busy={busy}>{t('ui.confirm.confirm')}</Button>
			{#if onCancel}<Button size="sm" variant="plain" type="button" onclick={onCancel}>{t('ui.confirm.cancel')}</Button>{/if}
		</div>
	</form>
{:else}
	{@render children()}
{/if}

<style>
	.confirm { display: flex; flex-direction: column; gap: 10px; width: min(100%, 420px); }
	/* Tighter than the shared row gap. */
	.acts { gap: 8px; }
</style>
