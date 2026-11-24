<script lang="ts">
	// "Forgot password?": the owner makes a one-time code, reads it from the
	// container log (or reset-owner-code.txt in the config folder), then picks
	// the owner name and a new password. The server answers the same whether
	// or not a code was made.
	import { untrack } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, ApiError, message } from '#lib/core/api.ts';
	import Button from '#lib/ui/Button.svelte';
	import { LOG_PLACES } from './reset-owner.ts';

	type Props = {
		minLength: number;
		// Start at the code step (HOLYARRS_RESET_OWNER already printed one).
		haveCode?: boolean;
		ondone: (name: string, password: string) => Promise<void>;
		onback: () => void;
	};
	let { minLength, haveCode = false, ondone, onback }: Props = $props();

	let step = $state<'ask' | 'code' | 'new'>(untrack(() => (haveCode ? 'code' : 'ask')));
	let code = $state('');
	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let busy = $state(false);
	let error = $state('');

	function failWith(err: unknown) {
		error = err instanceof ApiError && err.status === 401 ? t('accounts.forgot.bad') : message(err);
	}

	async function makeCode() {
		busy = true;
		error = '';
		try {
			await api.post('/auth/owner-reset/code', {});
			step = 'code';
		} catch (err) {
			failWith(err);
		} finally {
			busy = false;
		}
	}

	async function checkCode(e: SubmitEvent) {
		e.preventDefault();
		if (!code.trim()) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ name: string }>('/auth/owner-reset/check', { code });
			name = r.name ?? '';
			step = 'new';
		} catch (err) {
			failWith(err);
		} finally {
			busy = false;
		}
	}

	async function finish(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (pw.length < minLength) {
			error = t('accounts.login.useAtLeast', { n: minLength });
			return;
		}
		if (pw !== pwAgain) {
			error = t('accounts.login.noMatch');
			return;
		}
		busy = true;
		try {
			await api.post('/auth/owner-reset/finish', { code, name: name.trim(), password: pw });
			await ondone(name.trim(), pw);
		} catch (err) {
			failWith(err);
			if (err instanceof ApiError && err.status === 401) {
				step = 'ask';
				code = '';
			}
		} finally {
			busy = false;
		}
	}
</script>

<div class="forgot" data-testid="owner-reset">
	<h1 class="title">{t('accounts.forgot.title')}</h1>
	{#if step === 'ask'}
		<p class="lead">{t('accounts.forgot.lead')}</p>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="submit"><Button variant="primary" size="lg" full type="button" {busy} onclick={makeCode} data-testid="owner-reset-make">{t('accounts.forgot.make')}</Button></div>
	{:else if step === 'code'}
		<p class="lead" role="status">{t('accounts.forgot.sent')}</p>
		<form onsubmit={checkCode} novalidate>
			<div class="field">
				<label for="reset-code">{t('accounts.forgot.code')}</label>
				<input id="reset-code" class="input mono" bind:value={code} autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" maxlength="64" placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX" required />
			</div>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="submit"><Button variant="primary" size="lg" full type="submit" {busy} disabled={!code.trim()}>{t('accounts.forgot.next')}</Button></div>
		</form>
		<details class="where">
			<summary>{t('accounts.forgot.where')}</summary>
			<ul>
				{#each LOG_PLACES as p (p.name)}<li><b>{p.name}</b> <span class="mono">{p.path}</span></li>{/each}
			</ul>
			<p>{t('accounts.forgot.file')}</p>
		</details>
		<button class="link" type="button" onclick={() => { step = 'ask'; error = ''; }}>{t('accounts.forgot.make')}</button>
	{:else}
		<p class="lead">{t('accounts.forgot.newLead')}</p>
		<form onsubmit={finish} novalidate>
			<div class="field">
				<label for="reset-name">{t('accounts.forgot.name')}</label>
				<input id="reset-name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="64" required />
			</div>
			<div class="field">
				<label for="reset-pw">{t('accounts.forgot.pw')}</label>
				<input id="reset-pw" class="input" type="password" bind:value={pw} autocomplete="new-password" required />
				<span class="hint">{t('accounts.signin.atLeast', { n: minLength })}</span>
			</div>
			<div class="field">
				<label for="reset-pw2">{t('accounts.forgot.again')}</label>
				<input id="reset-pw2" class="input" type="password" bind:value={pwAgain} autocomplete="new-password" required />
			</div>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="submit"><Button variant="primary" size="lg" full type="submit" {busy}>{t('accounts.forgot.save')}</Button></div>
		</form>
	{/if}
	<button class="link" type="button" onclick={onback}>{t('accounts.forgot.back')}</button>
</div>

<style>
	.submit { margin-top: 20px; }
	.mono { font-family: var(--font-mono); }
	.where { margin-top: 14px; color: var(--text-muted); font: var(--t-footnote); }
	.where summary { cursor: pointer; width: fit-content; margin: 0 auto; padding: 0 10px; min-height: 36px; display: flex; align-items: center; border-radius: var(--r-sm); color: var(--accent-text); font: 500 14px/1 var(--font-ui); list-style: none; }
	.where summary::-webkit-details-marker { display: none; }
	.where summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.where ul { margin: 8px 0 0; padding: 0; list-style: none; text-align: left; }
	.where li { margin-top: 6px; overflow-wrap: anywhere; }
	.where li b { color: var(--text); font-weight: 600; }
	.where p { margin-top: 8px; text-align: left; }
	.link {
		display: block;
		margin: 12px auto 0;
		padding: 0 10px;
		min-height: 36px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--accent-text);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) {
		.link:hover, .where summary:hover { background: rgb(255 255 255 / 0.08); }
		:global(.light) .link:hover, :global(.light) .where summary:hover { background: rgb(0 0 0 / 0.05); }
	}
	:global(.light) .link, :global(.light) .where summary { color: #1d1d1f; font-weight: 600; text-decoration: underline; text-underline-offset: 3px; text-decoration-color: rgb(0 0 0 / 0.25); }
	:global(.light) .where { color: rgb(0 0 0 / 0.64); }
	@media (pointer: coarse) {
		.link, .where summary { min-height: 44px; }
	}
</style>
