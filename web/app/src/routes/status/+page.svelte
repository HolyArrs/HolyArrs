<script lang="ts">
	// Status: is every linked part working, and did every background job run?
	// Reads only the existing health, schedules, lists, alerts and metadata
	// routes. Preview picker: ?surface=1 (raised plates) or 3 (frosted).
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
	import Sections from '#lib/status/StatusSections.svelte';
	import { buildRows, groupOrder, groupRows, headline, needsAttention, surfaceOf, type StatusInput, type StatusRow } from '#lib/status/status.ts';

	const tab = $derived(['jobs', 'releases'].find((t) => t === page.url.searchParams.get('tab')) ?? 'parts');
	const surface = $derived(surfaceOf(page.url.searchParams.get('surface')));

	let input = $state<StatusInput | null>(null);
	let error = $state('');
	let busy = $state<string | null>(null);
	const canAct = $derived(session.can('settings'));

	const rows = $derived(input ? buildRows({ ...input, now: Date.now() }) : []);
	const groups = $derived(groupRows(rows));
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

	function setTab(t: string) {
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

<div class="status" data-testid="status-page" data-surface={surface}>
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
	{:else}
		<Sections {groups} {tab} ontab={setTab} {canAct} {busy} oncheck={check} {labelOf} />
	{/if}
</div>

<LookPills />

<style>
	/* 3 Frosted: see-through plates with a heavier blur (1, raised, is what the sections draw). */
	.status[data-surface='3'] :global(section) {
		background: color-mix(in srgb, var(--surface-1) 55%, transparent); -webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6);
		border-color: color-mix(in srgb, var(--text) 12%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 20px 50px -30px rgb(0 0 0 / 0.5);
	}
	.status {
		max-width: 1600px; margin: 0 auto; padding: var(--topbar, 0px) var(--gutter) calc(120px + env(safe-area-inset-bottom, 0px)); box-sizing: border-box;
		display: flex; flex-direction: column; gap: 16px;
	}
	.sbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 0; }
	h1 { margin: 0; font: 600 26px/1.15 var(--font-ui); color: var(--text); }
	.head { margin: 2px 0 0; font-size: 14px; }
	.head.good { color: var(--text-muted); }
	.head.bad { color: var(--fail-text); font-weight: 500; }
	.muted { margin: 0; color: var(--text-muted); font-size: 14px; }
	.notice { color: var(--fail-text); }
	.btn {
		min-height: 44px; padding: 0 16px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 14px/1 var(--font-ui); cursor: pointer;
	}
	.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled { opacity: 0.6; }
	@media (max-width: 767px) { h1 { font-size: 22px; } }
</style>
