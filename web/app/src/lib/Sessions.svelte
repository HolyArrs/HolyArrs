<script lang="ts">
	// Signed-in devices: My account (path /auth/sessions) or one person on
	// the Users page (path /users/<id>/sessions).
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { ago, dateTime } from './format.ts';
	import { endedWords, order, others, type SessionRow } from './sessions.ts';

	let { path, name = '' }: { path: string; name?: string } = $props();
	const mine = $derived(path === '/auth/sessions');
	let rows = $state<SessionRow[] | null>(null);
	let error = $state('');
	let busy = $state(false);

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
		<p class="small muted">{mine ? 'No devices are signed in.' : `${name || 'This person'} is not signed in anywhere.`}</p>
	{:else}
		<ul class="rows">
			{#each rows as s (s.id)}
				<li data-testid="session" data-current={s.current}>
					<span class="grow small">
						<b>{s.device}</b>
						{#if s.current}<span class="chip ok">This device</span>{/if}
						<br /><span class="xs muted"
							>Signed in {dateTime(s.created_at)} · last used <time datetime={s.last_used_at}>{ago(s.last_used_at)}</time></span
						>
					</span>
					{#if !s.current}
						<button class="btn btn-quiet" type="button" disabled={busy} onclick={() => endOne(s)} aria-label="Sign out {s.device}, last used {ago(s.last_used_at)}"
							>Sign out</button
						>
					{/if}
				</li>
			{/each}
		</ul>
		{#if mine ? others(rows) > 0 : rows.length > 0}
			<div class="row" style="margin-top: var(--s-3)">
				<button class="btn btn-ghost" type="button" disabled={busy} aria-busy={busy} onclick={endOthers}
					>{mine ? 'Sign out all other devices' : `Sign ${name || 'them'} out everywhere`}</button
				>
			</div>
		{/if}
	{/if}
</div>

<style>
	.sessions li {
		align-items: center;
		gap: var(--s-3);
	}
	.sessions .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.chip {
		margin-left: var(--s-2);
	}
</style>
