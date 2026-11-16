<script lang="ts">
	// My account: link my account on one of the owner's Jellyfin or Emby servers, so the Library shows what I watched.
	// The password goes to that server only; HolyArrs keeps just the account's id there.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	type Server = { id: number; name: string; kind: 'jellyfin' | 'emby' };
	type Link = { provider: string; name: string };
	let servers = $state<Server[]>([]);
	let links = $state<Link[]>([]);
	let open = $state<Server | null>(null);
	let name = $state('');
	let typed = $state('');
	let busy = $state(false);
	let error = $state('');
	let unlinking = $state('');

	const kindWord = (k: string) => (k === 'emby' ? 'Emby' : 'Jellyfin');
	const linkOf = (k: string) => links.find((l) => l.provider === k);

	onMount(load);
	async function load() {
		const r = await api.get<{ servers: Server[]; links: Link[] }>('/auth/media-link').catch(() => null);
		servers = r?.servers ?? [];
		links = r?.links ?? [];
	}

	async function link(e: SubmitEvent) {
		e.preventDefault();
		if (!open) return;
		error = '';
		busy = true;
		try {
			const l = await api.post<Link>(`/auth/media-link/${open.id}`, { name: name.trim(), password: typed });
			links = [...links.filter((x) => x.provider !== l.provider), l];
			toasts.push(t('accounts.watch.linked', { name: l.name, server: open.name }), 'ok');
			open = null;
			name = '';
		} catch (err) {
			error = message(err);
		} finally {
			typed = '';
			busy = false;
		}
	}

	async function unlink(kind: string) {
		busy = true;
		try {
			await api.del(`/auth/media-link/${kind}`);
			links = links.filter((l) => l.provider !== kind);
			unlinking = '';
			toasts.push(t('accounts.watch.unlinked'), 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

{#if servers.length || links.length}
	<div class="srow" id="watch-link" data-testid="media-link">
		<div class="txt"><span class="label">{t('accounts.watch.title')}</span><span class="hint">{t('accounts.watch.hint')}</span></div>
		<div class="ctl">
			{#if !open}
				{#each servers as s (s.id)}
					<Button variant="secondary" type="button" onclick={() => { open = s; error = ''; }} data-testid="media-link-open">{linkOf(s.kind) ? t('accounts.account.change') : t('accounts.security.link')} · {s.name}</Button>
				{/each}
			{/if}
		</div>
		{#if links.length}
			<ul class="below my-links">
				{#each links as l (l.provider)}
					<li>
						<span class="small">{t('accounts.watch.as', { server: kindWord(l.provider), name: l.name })}</span>
						{#if unlinking === l.provider}
							<Confirm action="link" onCancel={() => (unlinking = '')}>
								<div class="row acts">
									<Button variant="secondary" type="button" onclick={() => unlink(l.provider)} busy={busy}>{t('accounts.security.unlink')}</Button>
									<Button variant="plain" type="button" onclick={() => (unlinking = '')}>{t('accounts.signin.cancel')}</Button>
								</div>
							</Confirm>
						{:else}
							<Button variant="plain" type="button" onclick={() => (unlinking = l.provider)}>{t('accounts.security.unlink')}</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		{#if open}
			<div class="below">
				<Confirm action="link" onCancel={() => (open = null)}>
					<form class="form" onsubmit={link}>
						<label class="field"><span class="small">{t('accounts.watch.nameOn', { server: open.name })}</span>
							<input class="input" bind:value={name} autocomplete="off" maxlength="128" required /></label>
						<label class="field"><span class="small">{t('accounts.signin.passwordLabel')}</span>
							<input class="input" type="password" bind:value={typed} autocomplete="off" /></label>
						<p class="xs muted">{t('accounts.watch.passwordOnce', { server: open.name })}</p>
						<div class="row acts">
							<Button variant="primary" type="submit" disabled={busy || !name.trim()} busy={busy}>{t('accounts.security.link')}</Button>
							<Button variant="plain" type="button" onclick={() => (open = null)}>{t('accounts.signin.cancel')}</Button>
						</div>
					</form>
				</Confirm>
				{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.my-links { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
	.my-links li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; }
	.form { display: flex; flex-direction: column; gap: 8px; width: min(100%, 360px); }
	.field { display: flex; flex-direction: column; gap: 4px; }
	.acts { gap: 8px; flex-wrap: wrap; align-items: center; }
	.srow > .ctl { flex-wrap: wrap; }
	/* On a phone the server buttons go under the words, so the words keep the row's width. */
	@media (max-width: 600px) {
		/* The id keeps this ahead of the shared settings row rule. */
		#watch-link.srow { grid-template-columns: minmax(0, 1fr); row-gap: 8px; }
		#watch-link.srow > .ctl { justify-self: stretch; }
	}
</style>
