<script lang="ts">
	// Plugins you linked to your account (a chat bot, a list sync): each may make requests and follow titles
	// for you, with your own rights and limits. A plugin shows you a code; you type it here.
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { dateTime } from '../core/format.ts';

	interface Link {
		plugin: string;
		external: string;
		created_at: string;
	}
	let links = $state<Link[] | null>(null);
	let code = $state('');
	let busy = $state(false);
	let error = $state('');

	async function load() {
		try {
			links = (await api.get<{ links: Link[] }>('/my/plugin-links')).links;
		} catch (e) {
			error = message(e);
		}
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			links = (await api.post<{ links: Link[] }>('/my/plugin-links', { code: code.trim() })).links;
			code = '';
			toasts.push(t('accounts.pluginLinks.linked'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function remove(l: Link) {
		busy = true;
		try {
			links = (await api.del<{ links: Link[] }>(`/my/plugin-links/${encodeURIComponent(l.plugin)}`)).links;
		} catch (err) {
			toasts.push(t('accounts.pluginLinks.removeFailed'), 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}

	onMount(load);
</script>

<div class="links" data-testid="plugin-links">
	<form class="code" onsubmit={add}>
		<label class="small" for="plugin-link-code">{t('accounts.pluginLinks.codeLabel')}</label>
		<div class="row">
			<input id="plugin-link-code" inputmode="numeric" autocomplete="one-time-code" maxlength="9" placeholder="1234-5678" bind:value={code} />
			<Button variant="primary" type="submit" disabled={busy || code.trim().length < 8}>{t('accounts.pluginLinks.link')}</Button>
		</div>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	{#if links && links.length}
		<ul class="rows">
			{#each links as l (l.plugin)}
				<li>
					<span class="grow small"><b>{l.plugin}</b><br /><span class="xs muted">{l.external} · {dateTime(l.created_at)}</span></span>
					<Button variant="secondary" type="button" disabled={busy} onclick={() => remove(l)} aria-label={t('accounts.pluginLinks.unlinkLabel', { plugin: l.plugin })}>{t('accounts.pluginLinks.unlink')}</Button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.code { display: flex; flex-direction: column; gap: 6px; }
	.row { display: flex; flex-wrap: wrap; gap: 8px; }
	.row input { flex: 1 1 140px; min-width: 0; min-height: 44px; }
	.links li { display: flex; align-items: center; gap: var(--s-3); min-height: 52px; padding: 8px 0; border-top: 1px solid var(--line); }
	.grow { flex: 1; min-width: 0; overflow-wrap: anywhere; }
	ul { list-style: none; margin: 8px 0 0; padding: 0; }
</style>
