<script lang="ts">
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

	const stopped = (r: unknown) => (gone = (r as { text?: string })?.text ?? 'HolyArrs is stopping.');

	/** Sends a whole-app control; the server always asks first, and the second step waits on this page. */
	async function app(how: 'restart' | 'safe-mode' | 'shutdown', label: string) {
		const key = `app-${how}`;
		busy = key;
		try {
			const r = await api.post<unknown>(`/app/${how}`, {});
			if (isConfirm(r)) asking = { key, label, path: `/app/${how}`, confirm: r };
			else stopped(r);
		} catch (err) {
			toasts.push(`Couldn't ${label.charAt(0).toLowerCase()}${label.slice(1)}`, 'fail', { body: message(err) });
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
			toasts.push(`Couldn't ${a.label.charAt(0).toLowerCase()}${a.label.slice(1)}`, 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	// The second step takes the focus when it shows.
	$effect(() => {
		if (asking) queueMicrotask(() => document.getElementById('confirm-yes')?.focus());
	});
</script>

<svelte:head><title>Danger zone · HolyArrs</title></svelte:head>

<PageHeader title="Danger zone" />

{#if !allowed}
	<p class="notice warn">Owner and admins only</p>
{:else if gone}
	<p class="notice warn" role="status" data-testid="app-gone">{gone}</p>
{:else}
	{#if asking}
		<div class="card confirm" role="alertdialog" aria-labelledby="h-confirm" aria-describedby="confirm-text" data-testid="confirm">
			<h2 id="h-confirm" class="h2">{asking.label}?</h2>
			<p id="confirm-text" class="small">{asking.confirm.text.replace(/, send the same request again.*$/, '.')}</p>
			<div class="row">
				<Button variant="destructive" id="confirm-yes" type="button" onclick={confirmNow} disabled={busy === asking.key} aria-busy={busy === asking.key}>Yes, {asking.label.toLowerCase()}</Button>
				<Button variant="plain" type="button" onclick={() => (asking = null)}>Cancel</Button>
			</div>
			<p class="xs muted">Expires in 60 s</p>
		</div>
	{/if}

	<div data-testid="control-app">
		<SettingsCard id="danger-card" tone="danger" icon="alert" title="The whole app">
			<p class="small muted">
				These stop HolyArrs. Docker's restart policy starts it again in a few seconds, unless the policy is <code>no</code>; then start the
				container yourself. Each one asks first.
			</p>
			<div class="row acts">
				<Button type="button" onclick={() => app('restart', 'Restart HolyArrs')} disabled={!!busy}>Restart HolyArrs</Button>
				<Button type="button" onclick={() => app('safe-mode', 'Restart in safe mode')} disabled={!!busy}>Restart in safe mode</Button>
				<Button variant="destructive" type="button" onclick={() => app('shutdown', 'Shut down')} disabled={!!busy}>Shut down</Button>
			</div>
			<p class="xs muted">Starts once with only sign-in, Settings and Library. Use it when a Part blocks startup.</p>
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
	code {
		font-family: var(--font-mono);
	}
</style>
