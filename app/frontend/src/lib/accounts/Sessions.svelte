<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import { endedWords, order, others, type SessionRow } from './session-list.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { ago, dateTime } from '../core/format.ts';

	// Signed-in devices: My account (path /auth/sessions) or one person on the Users page (path /users/<id>/sessions).

	// How many rows show before Show all, so long lists stay short.
	let { path, name = '', first = Infinity }: { path: string; name?: string; first?: number } = $props();
	const mine = $derived(path === '/auth/sessions');
	let rows = $state<SessionRow[] | null>(null);
	let error = $state('');
	let busy = $state(false);
	let all = $state(false);
	const shown = $derived(rows ? (all ? rows : rows.slice(0, first)) : []);

	async function load() {
		try {
			rows = order((await api.get<{ sessions: SessionRow[] }>(path)).sessions);
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	async function endOne(s: SessionRow) {
		busy = true;
		try {
			await api.del(`${path}/${encodeURIComponent(s.id)}`);
			toasts.push(t('accounts.sessions.deviceOut', { device: s.device }), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('accounts.sessions.signOutFailed'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	async function endOthers() {
		busy = true;
		try {
			const r = mine ? await api.del<{ ended: number }>(path) : await api.post<{ ended: number }>(`${path}/end-all`);
			toasts.push(endedWords(r.ended), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('accounts.sessions.signOutFailed'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	onMount(load);
</script>

<div class="sessions" data-testid="sessions">
	{#if error}
		<p class="form-error" role="alert">{error}</p>
	{:else if !rows}
		<div class="sk" style="height: 64px"></div>
	{:else if rows.length === 0}
		<div class="empty-state">
			<p>{t('accounts.sessions.empty')}</p>
			<span class="small muted">{mine ? t('accounts.sessions.emptyMine') : t('accounts.sessions.emptyOther', { name: name || t('accounts.sessions.thisPerson') })}</span>
		</div>
	{:else}
		<ul class="rows">
			{#each shown as s (s.id)}
				<li data-testid="session" data-current={s.current}>
					<span class="grow small">
						<b>{s.device}</b>
						{#if s.current}<StatusPill kind="in_library" label={t('accounts.sessions.thisDevice')} />{/if}
						<br /><span class="xs muted"
							>{t('accounts.sessions.signedIn', { when: dateTime(s.created_at) })} · {t('accounts.sessions.lastUsed')} <time datetime={s.last_used_at}>{ago(s.last_used_at)}</time></span
						>
					</span>
					{#if !s.current}
						<Button variant="secondary" type="button" disabled={busy} onclick={() => endOne(s)} aria-label={t('accounts.sessions.signOutLabel', { device: s.device, ago: ago(s.last_used_at) })}
							>{t('accounts.sessions.signOut')}</Button>
					{/if}
				</li>
			{/each}
		</ul>
		{#if rows.length > shown.length}
			<button type="button" class="more" onclick={() => (all = true)} data-testid="sessions-all">{t('accounts.sessions.showAll', { n: rows.length })}</button>
		{/if}
		{#if mine ? others(rows) > 0 : rows.length > 0}
			<div class="all-row">
				<Button variant="secondary" type="button" disabled={busy} aria-busy={busy} onclick={endOthers}
					>{mine ? t('accounts.sessions.outOthers') : t('accounts.sessions.outEverywhere', { name: name || t('accounts.sessions.them') })}</Button>
			</div>
		{/if}
	{/if}
</div>

<style>
	.more { min-height: 40px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }
	.more:hover { text-decoration: underline; }
	@media (max-width: 767px) and (pointer: coarse) { .more { min-height: 44px; } }
	.sessions li {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 52px;
		padding: 8px 0;
		border-top: 1px solid var(--line);
	}
	.sessions li:first-child { border-top: 0; }
	.sessions .grow { flex: 1; }
	.all-row { display: flex; justify-content: flex-end; padding-top: 8px; border-top: 1px solid var(--line); }
	.sessions .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.empty-state {
		padding: var(--s-4);
		border: 1px dashed var(--line-strong);
		border-radius: var(--r-btn);
		background: var(--surface-2);
	}
	.empty-state p {
		font-weight: var(--weight-semibold);
	}
	@media (max-width: 600px) {
		.sessions li > :global(.ui-btn) { flex: none; }
	}
</style>
