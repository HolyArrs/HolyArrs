<script lang="ts">
	// Status: is every linked part working, and did every background job run?
	// Four looks for the preview (?look=a|b|c|d, default d, the Board). Reads only the
	// existing health, schedules, lists, alerts and metadata routes.
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { checkMetadata } from '#lib/metadata-health.svelte.ts';
	import type { AlertView, HealthChecks, HealthStates, OutsideList } from '#lib/types.ts';
	import type { ScheduleRow } from '#lib/schedules.ts';
	import LookPills from '#lib/status/LookPills.svelte';
	import Row from '#lib/status/StatusRow.svelte';
	import Board from '#lib/status/StatusBoard.svelte';
	import { buildRows, groupLine, groupOrder, groupRows, headline, lookOf, needsAttention, type StatusInput, type StatusRow } from '#lib/status/status.ts';

	const look = $derived(lookOf(page.url.searchParams.get('look')));
	const tab = $derived(page.url.searchParams.get('tab') === 'jobs' ? 'jobs' : 'parts');

	let input = $state<StatusInput | null>(null);
	let error = $state('');
	let busy = $state<string | null>(null);
	let open = $state<Record<string, boolean>>({});
	const canAct = $derived(session.can('settings'));

	const rows = $derived(input ? buildRows({ ...input, now: Date.now() }) : []);
	const groups = $derived(groupRows(rows));
	const partGroups = $derived(groups.filter((g) => g.id !== 'jobs'));
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	const problems = $derived(needsAttention(rows));
	const top = $derived(headline(rows));
	const labelOf = (r: StatusRow) => groupOrder.find((g) => g.id === r.group)?.label ?? '';

	async function load() {
		const [states, checks, sched, lists, alerts, meta] = await Promise.allSettled([
			api.get<HealthStates>('/health/states'),
			api.get<HealthChecks>('/health/checks'),
			api.get<{ schedules: ScheduleRow[] }>('/schedules'),
			api.get<{ outside_lists: OutsideList[] }>('/outside-lists'),
			api.get<{ alerts: AlertView[] }>('/alerts'),
			checkMetadata(true)
		]);
		const val = <T,>(r: PromiseSettledResult<T>) => (r.status === 'fulfilled' ? r.value : undefined);
		if (states.status === 'rejected' && sched.status === 'rejected') {
			error = message(states.reason);
			return;
		}
		error = '';
		input = {
			states: val(states)?.states,
			checks: val(checks)?.checks,
			schedules: val(sched)?.schedules,
			lists: val(lists)?.outside_lists,
			alerts: val(alerts)?.alerts,
			metadataUp: meta.status === 'fulfilled' ? meta.value : false,
			metadataAt: new Date().toISOString()
		};
	}

	async function check(r: StatusRow) {
		const c = r.check;
		if (!c) return;
		busy = r.key;
		try {
			if (c.kind === 'job') {
				const res = await api.post<{ already: boolean }>(`/schedules/${c.job}/run`);
				toasts.push(res.already ? `${r.name} is already waiting or running.` : `${r.name} started.`, 'ok');
			} else if (c.kind === 'checks') {
				await api.post('/health/checks/run');
			} else {
				const res = await api.post<{ ok: boolean; text: string }>(`/${c.kind}s/${c.id}/test`);
				toasts.push(res.ok ? `${r.name} answered.` : res.text || `${r.name} did not answer.`, res.ok ? 'ok' : 'fail');
			}
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	async function checkAll() {
		busy = 'all';
		try {
			await api.post('/health/checks/run');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	function setTab(t: 'parts' | 'jobs') {
		const u = new URL(page.url.href);
		u.searchParams.set('tab', t);
		void goto(u.pathname + u.search);
	}

	onMount(() => {
		void load();
		// Live pushes cover health and jobs; the 60s poll catches lists, alerts and the metadata server.
		const offs = [live.on('health', load), live.on('job', load)];
		const t = setInterval(() => {
			if (document.visibilityState === 'visible') void load();
		}, 60_000);
		const vis = () => document.visibilityState === 'visible' && void load();
		document.addEventListener('visibilitychange', vis);
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
			document.removeEventListener('visibilitychange', vis);
		};
	});
</script>

<svelte:head><title>Status · HolyArrs</title></svelte:head>

{#snippet list(rs: StatusRow[], withKind = false)}
	<ul class="rows">
		{#each rs as r (r.key)}
			<Row row={r} actions={canAct} kind={withKind ? labelOf(r) : ''} busy={busy === r.key} oncheck={canAct ? check : undefined} />
		{/each}
	</ul>
{/snippet}

<div class="status" data-testid="status-page" data-look={look}>
	<header class="sbar">
		<div class="title">
			<h1>Status</h1>
			{#if input}<p class="head {problems.length ? 'bad' : 'good'}" data-testid="status-headline">{top}</p>{/if}
		</div>
		{#if canAct}
			<button type="button" class="btn" onclick={checkAll} disabled={busy === 'all'} aria-busy={busy === 'all'}>Check all</button>
		{/if}
	</header>

	{#if error}
		<p class="notice" role="alert">{error}</p>
	{:else if !input}
		<p class="muted">Loading…</p>
	{:else if look === 'd'}
		<Board {groups} {problems} {canAct} {busy} oncheck={check} {labelOf} />
	{:else if look === 'a'}
		{#each groups as g (g.id)}
			<section class="card" aria-labelledby="g-{g.id}">
				<h2 id="g-{g.id}">{g.label}</h2>
				{@render list(g.rows)}
			</section>
		{/each}
	{:else if look === 'b'}
		<section class="card attn" aria-labelledby="g-attn" data-testid="status-attention">
			<h2 id="g-attn">Needs attention{#if problems.length} ({problems.length}){/if}</h2>
			{#if problems.length}
				{@render list(problems, true)}
			{:else}
				<p class="empty">Nothing needs attention.</p>
			{/if}
		</section>
		<section class="card" aria-labelledby="g-all">
			<h2 id="g-all">All parts</h2>
			<ul class="folds">
				{#each partGroups as g (g.id)}
					<li>
						<button type="button" class="fold" aria-expanded={!!open[g.id]} onclick={() => (open[g.id] = !open[g.id])}>
							<span class="fname">{g.label}</span>
							<span class="fline">{groupLine(g)}</span>
							<span class="chev" aria-hidden="true">{open[g.id] ? '−' : '+'}</span>
						</button>
						{#if open[g.id]}{@render list(g.rows)}{/if}
					</li>
				{/each}
			</ul>
		</section>
		{#if jobs.length}
			<section class="card" aria-labelledby="g-jobs">
				<h2 id="g-jobs">Jobs</h2>
				{@render list(jobs)}
			</section>
		{/if}
	{:else}
		<div class="seg" role="tablist" aria-label="Status view">
			<button type="button" role="tab" aria-selected={tab === 'parts'} onclick={() => setTab('parts')}>Parts</button>
			<button type="button" role="tab" aria-selected={tab === 'jobs'} onclick={() => setTab('jobs')}>Jobs</button>
		</div>
		<p class="note">History bars need stored results. They come once HolyArrs keeps past checks.</p>
		{#if tab === 'parts'}
			{#each partGroups as g (g.id)}
				<section class="card" aria-labelledby="g-{g.id}">
					<h2 id="g-{g.id}">{g.label}</h2>
					{@render list(g.rows)}
				</section>
			{/each}
		{:else}
			<section class="card">{@render list(jobs)}</section>
		{/if}
	{/if}
</div>

<LookPills />

<style>
	.status[data-look='d'] { max-width: 1600px; }
	.status {
		max-width: 1080px; margin: 0 auto; padding: var(--topbar, 0px) var(--gutter) calc(120px + env(safe-area-inset-bottom, 0px)); box-sizing: border-box;
		display: flex; flex-direction: column; gap: 16px;
	}
	.sbar {
		position: sticky; top: var(--topbar, 0px); z-index: 5; display: flex; align-items: center; justify-content: space-between; gap: 12px;
		margin: 0 calc(-1 * var(--gutter)); padding: 14px var(--gutter);
		background: color-mix(in srgb, var(--bg, var(--surface-0, var(--surface-1))) 72%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		border-bottom: 1px solid var(--line);
	}
	h1 { margin: 0; font: 600 26px/1.15 var(--font-ui); color: var(--text); }
	.head { margin: 2px 0 0; font-size: 14px; }
	.head.good { color: var(--text-muted); }
	.head.bad { color: var(--fail-text); font-weight: 500; }
	.card { border: 1px solid var(--line); border-radius: var(--r-card, 16px); background: var(--surface-1); overflow: hidden; }
	h2 { margin: 0; padding: 14px 16px 6px; font: 600 13px/1.2 var(--font-ui); letter-spacing: 0.02em; text-transform: uppercase; color: var(--text-muted); }
	.rows, .folds { list-style: none; margin: 0; padding: 0; }
	.empty, .note, .muted { margin: 0; padding: 4px 16px 14px; color: var(--text-muted); font-size: 14px; }
	.note { padding: 0 2px; font-size: 13px; }
	.notice { color: var(--fail-text); }
	.fold {
		display: grid; grid-template-columns: 160px minmax(0, 1fr) 24px; align-items: center; gap: 12px; width: 100%;
		min-height: 56px; padding: 8px 16px; border: 0; border-top: 1px solid var(--line); background: none;
		color: var(--text); font: inherit; text-align: left; cursor: pointer;
	}
	.folds > li:first-child > .fold { border-top: 0; }
	.fold:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.fname { font-weight: 500; }
	.fline { color: var(--text-muted); font-size: 14px; }
	.chev { color: var(--text-muted); font-size: 18px; text-align: center; }
	.btn {
		min-height: 44px; padding: 0 16px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 14px/1 var(--font-ui); cursor: pointer;
	}
	.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled { opacity: 0.6; }
	.seg { display: inline-flex; align-self: flex-start; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.seg button { min-height: 44px; padding: 0 20px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.seg button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) {
		.fold { grid-template-columns: minmax(0, 1fr) 24px; }
		.fline { grid-column: 1; grid-row: 2; }
		.chev { grid-column: 2; grid-row: 1 / span 2; }
		h1 { font-size: 22px; }
		.sbar { position: static; background: none; -webkit-backdrop-filter: none; backdrop-filter: none; border-bottom: 0; }
	}
</style>
