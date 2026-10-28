<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	// Signed-in devices: My account (path /auth/sessions) or one person on
	// the Users page (path /users/<id>/sessions).
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { ago, dateTime } from './format.ts';
	import { endedWords, order, others, type SessionRow } from './sessions.ts';

	// first: how many rows show before Show all (long lists stay short).
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
			toasts.push(`Signed out ${s.device}.`, 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
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
			toasts.push(message(e), 'fail');
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
			<p>No signed-in devices</p>
			<span class="small muted">{mine ? 'There are no active sessions to manage.' : `${name || 'This person'} is not signed in anywhere.`}</span>
		</div>
	{:else}
		<ul class="rows">
			{#each shown as s (s.id)}
				<li data-testid="session" data-current={s.current}>
					<span class="grow small">
						<b>{s.device}</b>
						{#if s.current}<StatusPill kind="in_library" label="This device" />{/if}
						<br /><span class="xs muted"
							>Signed in {dateTime(s.created_at)} · last used <time datetime={s.last_used_at}>{ago(s.last_used_at)}</time></span
						>
					</span>
					{#if !s.current}
						<Button variant="plain" type="button" disabled={busy} onclick={() => endOne(s)} aria-label="Sign out {s.device}, last used {ago(s.last_used_at)}"
							>Sign out</Button>
					{/if}
				</li>
			{/each}
		</ul>
		{#if rows.length > shown.length}
			<button type="button" class="more" onclick={() => (all = true)} data-testid="sessions-all">Show all {rows.length} devices</button>
		{/if}
		{#if mine ? others(rows) > 0 : rows.length > 0}
			<div class="row" style="margin-top: var(--s-3)">
				<Button type="button" disabled={busy} aria-busy={busy} onclick={endOthers}
					>{mine ? 'Sign out all other devices' : `Sign ${name || 'them'} out everywhere`}</Button>
			</div>
		{/if}
	{/if}
</div>

<style>
	.more { min-height: 40px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }
	.more:hover { text-decoration: underline; }
	.sessions li {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 64px;
		padding: var(--s-3) 0;
		border-bottom: 1px solid var(--line);
	}
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
		.sessions li {
			align-items: flex-start;
			flex-direction: column;
		}
		.sessions li > :global(.ui-btn) {
			width: 100%;
			min-height: 44px;
		}
	}
</style>
