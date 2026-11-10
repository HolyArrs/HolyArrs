<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { isConfirm, type Confirm } from '#lib/setup/index.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	const allowed = $derived(session.can('users'));
	let busy = $state('');
	// The control waiting for its second step: what it does, in the server's words.
	let asking = $state<{ key: string; label: string; path: string; confirm: Confirm } | null>(null);
	// The whole app stopped; nothing more can be done from this page.
	let gone = $state('');

	const stopped = (r: unknown) => (gone = (r as { text?: string })?.text ?? t('settings.danger.stopping'));

	/** Sends a whole-app control; the server always asks first, and the second step waits on this page. */
	async function app(how: 'restart' | 'safe-mode' | 'shutdown', label: string) {
		const key = `app-${how}`;
		busy = key;
		try {
			const r = await api.post<unknown>(`/app/${how}`, {});
			if (isConfirm(r)) asking = { key, label, path: `/app/${how}`, confirm: r };
			else stopped(r);
		} catch (err) {
			toasts.push(t('settings.danger.couldnt', { action: label.charAt(0).toLowerCase() + label.slice(1) }), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function confirmNow() {
		if (!asking) return;
		const a = asking;
		busy = a.key;
		try {
			const r = await api.post<unknown>(a.path, { confirm: a.confirm.confirm });
			asking = null;
			stopped(r);
		} catch (err) {
			asking = null;
			toasts.push(t('settings.danger.couldnt', { action: a.label.charAt(0).toLowerCase() + a.label.slice(1) }), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	// The second step takes the focus when it shows.
	$effect(() => {
		if (asking) queueMicrotask(() => document.getElementById('confirm-yes')?.focus());
	});
</script>

<svelte:head><title>{t('settings.danger.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.danger.title')} />

{#if !allowed}
	<p class="notice warn">{t('settings.danger.ownerOnly')}</p>
{:else if gone}
	<p class="notice warn" role="status" data-testid="app-gone">{gone}</p>
{:else}
	{#if asking}
		<div class="card confirm" role="alertdialog" aria-labelledby="h-confirm" aria-describedby="confirm-text" data-testid="confirm">
			<h2 id="h-confirm" class="h2">{t('settings.danger.ask', { label: asking.label })}</h2>
			<p id="confirm-text" class="small">{asking.confirm.text.replace(/, send the same request again.*$/, '.')}</p>
			<div class="row">
				<Button variant="destructive" id="confirm-yes" type="button" onclick={confirmNow} disabled={busy === asking.key} aria-busy={busy === asking.key}>{t('settings.danger.yes', { action: asking.label.toLowerCase() })}</Button>
				<Button variant="plain" type="button" onclick={() => (asking = null)}>{t('settings.danger.cancel')}</Button>
			</div>
			<p class="xs muted">{t('settings.danger.expires')}</p>
		</div>
	{/if}

	<div data-testid="control-app">
		<SettingsCard id="danger-card" tone="danger" icon="alert" title={t('settings.danger.wholeApp')}>
			<p class="small muted">
				<Words key="settings.danger.intro" />
			</p>
			<div class="row acts">
				<Button type="button" onclick={() => app('restart', t('settings.danger.restart'))} disabled={!!busy}>{t('settings.danger.restart')}</Button>
				<Button type="button" onclick={() => app('safe-mode', t('settings.danger.safe'))} disabled={!!busy}>{t('settings.danger.safe')}</Button>
				<Button variant="destructive" type="button" onclick={() => app('shutdown', t('settings.danger.shutdown'))} disabled={!!busy}>{t('settings.danger.shutdown')}</Button>
			</div>
			<p class="xs muted">{t('settings.danger.safeNote')}</p>
		</SettingsCard>
	</div>
{/if}

<style>
	.acts {
		flex-wrap: wrap;
	}
	.confirm {
		box-shadow: 0 0 0 1.5px var(--fail) !important;
		margin-bottom: 16px;
	}
	.confirm .row {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	p :global(code) {
		font-family: var(--font-mono);
	}
</style>
