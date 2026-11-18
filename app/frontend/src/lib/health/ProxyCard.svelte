<script lang="ts">
	// The app proxy: every outside call goes through it unless the address skips it. Owner only.
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	interface Proxy {
		type: '' | 'http' | 'https' | 'socks5';
		host: string;
		port: number;
		username: string;
		password: string;
		bypass: string;
		bypass_local: boolean;
	}

	let saved = $state<Proxy | null>(null);
	let form = $state<Proxy>({ type: '', host: '', port: 8080, username: '', password: '', bypass: '', bypass_local: true });
	let gone = $state(false);
	let passwordSaved = $state(false);
	let busy = $state(false);
	let err = $state('');
	// The server's stand-in for a saved secret; sending it back keeps the secret.
	const MASK = '•'.repeat(8);
	const on = $derived(form.type !== '');
	const changed = $derived(saved !== null && JSON.stringify(saved) !== JSON.stringify(form));

	async function load() {
		try {
			take(await api.get<Proxy>('/settings/proxy'));
		} catch (e) {
			// An older server without the setting: no card rather than an error.
			if (e instanceof ApiError && e.status === 404) gone = true;
			else err = message(e);
		}
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		err = '';
		try {
			// An empty password keeps the saved one, unless the proxy is off or has no username.
			const keep = form.password === '' && passwordSaved && form.type !== '' && form.username !== '';
			const body = { ...form, host: form.host.trim(), port: Number(form.port) || 0, password: keep ? MASK : form.type && form.username ? form.password : '' };
			const p = await api.put<Proxy>('/settings/proxy', body);
			take(p);
			toasts.push(p.type ? t('settings.thisApp.proxySaved') : t('settings.thisApp.proxyOffSaved'), 'ok');
		} catch (e) {
			err = message(e);
		} finally {
			busy = false;
		}
	}

	// take shows a server answer: the password itself never comes back, only whether one is saved.
	function take(p: Proxy) {
		passwordSaved = p.password !== '';
		saved = { ...p, port: p.port || 8080, password: '' };
		form = { ...saved };
	}

	onMount(load);
</script>

{#if !gone}
	<SettingsCard id="proxy-card" title={t('settings.thisApp.proxy')} description={t('settings.thisApp.proxyDesc')}>
		<form class="proxy" onsubmit={save} data-testid="proxy-form">
			<Field id="proxy-type" label={t('settings.thisApp.proxyType')}>
				<select id="proxy-type" class="input" bind:value={form.type} disabled={!saved}>
					<option value="">{t('settings.thisApp.proxyOff')}</option>
					<option value="http">HTTP</option>
					<option value="https">HTTPS</option>
					<option value="socks5">SOCKS5</option>
				</select>
			</Field>
			{#if on}
				<div class="pair">
					<Field id="proxy-host" label={t('settings.thisApp.proxyHost')} required>
						<input id="proxy-host" class="input mono" bind:value={form.host} placeholder="proxy.lan" autocomplete="off" spellcheck="false" maxlength="253" required />
					</Field>
					<Field id="proxy-port" label={t('settings.thisApp.proxyPort')} required>
						<input id="proxy-port" class="input mono" type="number" inputmode="numeric" min="1" max="65535" bind:value={form.port} required />
					</Field>
				</div>
				<div class="pair even">
					<Field id="proxy-user" label={t('settings.thisApp.proxyUser')}>
						<input id="proxy-user" class="input mono" bind:value={form.username} autocomplete="off" spellcheck="false" maxlength="255" />
					</Field>
					<Field id="proxy-pass" label={t('settings.thisApp.proxyPasswordLabel')} hint={passwordSaved ? t('settings.thisApp.proxyPasswordKept') : undefined}>
						<input id="proxy-pass" class="input mono" type="password" bind:value={form.password} autocomplete="new-password" spellcheck="false" maxlength="255" />
					</Field>
				</div>
				<Field id="proxy-bypass" label={t('settings.thisApp.proxyBypass')} hint={t('settings.thisApp.proxyBypassHint')}>
					<input id="proxy-bypass" class="input mono" bind:value={form.bypass} placeholder="*.example.com, 10.0.0.0/8" autocomplete="off" spellcheck="false" maxlength="2000" />
				</Field>
				<label class="checkline">
					<input id="proxy-bypass-local" type="checkbox" bind:checked={form.bypass_local} />
					<span><b>{t('settings.thisApp.proxyBypassLocal')}</b><br /><span class="hint">{t('settings.thisApp.proxyBypassLocalHint')}</span></span>
				</label>
			{/if}
			{#if err}<p class="form-error" role="alert">{err}</p>{/if}
			<div class="row acts"><Button variant="primary" type="submit" disabled={!changed} aria-busy={busy}>{t('settings.thisApp.save')}</Button></div>
		</form>
	</SettingsCard>
{/if}

<style>
	.proxy {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
	}
	.pair {
		display: grid;
		grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
		gap: var(--s-3);
	}
	.pair.even {
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
	}
	@media (max-width: 600px) {
		.pair,
		.pair.even {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
