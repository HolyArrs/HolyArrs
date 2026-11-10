<script lang="ts">
	// Status: is every linked part working, and did every background job run? Reads only the existing health, schedules, lists, alerts and metadata routes.
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { checkMetadata, metadataHealth } from '#lib/metadata/index.ts';
	import type { HealthChecks, HealthStates, ScheduleRow } from '#lib/health/index.ts';
	import type { OutsideList } from '#lib/requests/index.ts';
	import { Updates } from '#lib/updates/index.ts';
	import { HealthSections as Sections, buildRows, groupOrder, groupRows, headline, type StatusInput, type StatusRow } from '#lib/health/index.ts';
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SvelteSet } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { AlertView } from '#lib/core/types.ts';
	import { testLabel } from '#lib/core/test-tag.ts';

	const tab = $derived(['jobs', 'updates', 'about'].find((t) => t === page.url.searchParams.get('tab')) ?? 'parts');
	let input = $state<StatusInput | null>(null);
	// The trailers check is the YouTube reader's state; it is not a part, so it is not a row.
	const reader = $derived(input?.checks?.find((c) => (c.kind as string) === 'trailers'));
	let error = $state('');
	// Which sources failed on the last read, so a gap never looks healthy or empty.
	let failed = $state<string[]>([]);
	// Which reads this account may not do (403); their kinds leave the page.
	let hidden = $state<string[]>([]);
	// Every running action, so one finishing never re-enables another.
	const running = new SvelteSet<string>();
	const busy = (key: string) => running.has(key);
	// Goes up after each health check run, so Storage reloads its disks.
	let checkRuns = $state(0);
	const canAct = $derived(session.can('settings'));

	const rows = $derived(input ? buildRows({ ...input, now: Date.now() }) : []);
	const groups = $derived(groupRows(rows));
	const top = $derived(headline(rows, !failed.includes('parts')));
	const labelOf = (r: StatusRow) => groupOrder.find((g) => g.id === r.group)?.label ?? '';
	const failedWord: Record<string, string> = { parts: 'parts', 'health checks': 'healthChecks', jobs: 'jobs', 'outside lists': 'lists', alerts: 'alerts' };
	const failedText = $derived(failed.length ? t('health.page.couldNotLoad', { what: failed.map((f) => t(`health.page.what.${failedWord[f] ?? 'parts'}`)).join(', ') }) : '');

	// Only the newest read may write the page; an older reply that lands late is dropped. load() is true when its answer was the one kept; `latest` is the newest read.
	let seq = 0;
	let latest: Promise<boolean> = Promise.resolve(true);
	function load(force = false): Promise<boolean> {
		return (latest = read(force));
	}
	// Waits until the newest read has landed, so a toast never reads old rows.
	async function settle(p: Promise<boolean>) {
		while (!(await p)) p = latest;
	}
	async function read(force: boolean): Promise<boolean> {
		const n = ++seq;
		const [states, checks, sched, lists, alerts, meta] = await Promise.allSettled([
			api.get<HealthStates>('/health/states'),
			api.get<HealthChecks>('/health/checks'),
			api.get<{ schedules: ScheduleRow[] }>('/schedules'),
			api.get<{ outside_lists: OutsideList[] }>('/outside-lists'),
			api.get<{ alerts: AlertView[] }>('/alerts'),
			checkMetadata(force)
		]);
		if (n !== seq) return false;
		const val = <T,>(r: PromiseSettledResult<T>) => (r.status === 'fulfilled' ? r.value : undefined);
		if (states.status === 'rejected' && sched.status === 'rejected') {
			// Keep what was shown before; the inline notice says the read failed.
			error = message(states.reason);
			return true;
		}
		error = '';
		const named: [PromiseSettledResult<unknown>, string][] = [
			[states, 'parts'],
			[checks, 'health checks'],
			[sched, 'jobs'],
			[lists, 'outside lists'],
			[alerts, 'alerts']
		];
		// A 403 means this account may not read that list: leave it out, not a failure.
		const denied = (r: PromiseSettledResult<unknown>) => r.status === 'rejected' && (r.reason as { status?: number })?.status === 403;
		failed = named.filter(([r]) => r.status === 'rejected' && !denied(r)).map(([, w]) => w);
		hidden = named.filter(([r]) => denied(r)).map(([, w]) => w);
		input = {
			states: val(states)?.states,
			checks: val(checks)?.checks,
			schedules: val(sched)?.schedules,
			lists: val(lists)?.outside_lists,
			alerts: val(alerts)?.alerts,
			metadataUp: meta.status === 'fulfilled' ? meta.value : false,
			metadataAt: new Date(metadataHealth.checked || Date.now()).toISOString()
		};
		return true;
	}

	const retry = () => void load(true);

	// Live pushes come on every job progress step: read at most once per 400 ms, one read at a time.
	let soonTimer: ReturnType<typeof setTimeout> | undefined;
	let reading = false;
	let again = false;
	function soon() {
		if (soonTimer) return;
		soonTimer = setTimeout(async () => {
			soonTimer = undefined;
			if (reading) {
				again = true;
				return;
			}
			reading = true;
			try {
				await load();
			} finally {
				reading = false;
				if (again) {
					again = false;
					soon();
				}
			}
		}, 400);
	}

	// After a health check run, say what the row shows now.
	function afterChecks(r: StatusRow) {
		const now = rows.find((x) => x.key === r.key);
		if (!now) toasts.push(t('health.page.checksRan'), 'ok');
		else toasts.push(now.tone === 'ok' ? t('health.page.working', { name: now.name }) : t('health.page.nameWord', { name: now.name, word: now.word }), now.tone === 'ok' ? 'ok' : 'fail');
	}

	async function check(r: StatusRow) {
		const c = r.check;
		if (!c || running.has(r.key)) return;
		running.add(r.key);
		try {
			if (c.kind === 'job') {
				const res = await api.post<{ already: boolean }>(`/schedules/${c.job}/run`);
				toasts.push(res.already ? t('health.page.already', { name: r.name }) : t('health.page.started', { name: r.name }), 'ok');
				await load(true);
			} else if (c.kind === 'checks') {
				await api.post('/health/checks/run');
				await settle(load(true));
				checkRuns++;
				afterChecks(r);
			} else {
				const res = await api.post<{ ok: boolean; text: string }>(`/${c.kind}s/${c.id}/test`);
				toasts.push(res.ok ? t('health.page.connected', { name: r.name }) : t('health.page.cantConnect', { name: r.name }), res.ok ? 'ok' : 'fail', { body: res.ok ? testLabel(res.text) : res.text.trim() || undefined });
				await load(true);
			}
		} catch (e) {
			toasts.push(t('health.page.cantCheck', { name: r.name }), 'fail', { body: message(e) });
		} finally {
			running.delete(r.key);
		}
	}

	async function checkAll() {
		if (running.has('all')) return;
		running.add('all');
		try {
			await api.post('/health/checks/run');
			await load(true);
			checkRuns++;
			toasts.push(t('health.page.checksRan'), 'ok');
		} catch (e) {
			toasts.push(t('health.page.cantRun'), 'fail', { body: message(e) });
		} finally {
			running.delete('all');
		}
	}

	function setTab(t: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('tab', t);
		void goto(u.pathname + u.search, { reset: false, replace: true });
	}

	onMount(() => {
		void load(true);
		// Live pushes cover health and jobs; the 60s poll catches lists, alerts and the metadata server.
		const offs = [live.on('health', soon), live.on('job', soon)];
		const t = setInterval(() => {
			if (document.visibilityState === 'visible') void load();
		}, 60_000);
		const vis = () => document.visibilityState === 'visible' && void load();
		document.addEventListener('visibilitychange', vis);
		return () => {
			clearInterval(t);
			clearTimeout(soonTimer);
			offs.forEach((off) => off());
			document.removeEventListener('visibilitychange', vis);
		};
	});
</script>

<svelte:head><title>{t('health.page.title')}</title></svelte:head>

<div class="status" class:jobs={tab === 'jobs'} data-testid="status-page">
	<PageHeader title={t('health.page.header')} subtitle={input ? top : undefined}>
		{#snippet actions()}
			{#if canAct}
				<Button onclick={checkAll} busy={running.has('all')} disabled={running.has('all')} data-testid="status-check-all">{t('health.page.checkAgain')}</Button>
			{/if}
		{/snippet}
	</PageHeader>

	{#if !input && error}
		<LoadError hasData={false} message={error} onretry={retry} />
	{:else}
		<PageLoader loading={!input} label={t('health.page.loading')} onretry={retry}>
			{#if input}
				<div class="body">
					{#if error}<LoadError hasData message={error} onretry={retry} />
					{:else if failedText}<LoadError hasData message={failedText} onretry={retry} />{/if}
					<Sections {groups} {tab} ontab={setTab} {canAct} {busy} oncheck={check} {labelOf} {reader} {failed} {hidden} {checkRuns}>
						{#snippet updates(p)}<Updates {...p} />{/snippet}
					</Sections>
				</div>
			{/if}
		</PageLoader>
	{/if}
</div>

<style>
	/* Desktop: this page ends where its last plate ends, so the Jobs tab fits a screen. Phones keep the layout's room for the tab bar. */
	@media (min-width: 768px) {
		:global(main:has(> .status)) { padding-bottom: 12px; }
	}
	.status {
		max-width: 1600px; margin: 0 auto; padding: 0 var(--gutter) 0; box-sizing: border-box;
		display: flex; flex-direction: column; gap: 16px;
	}
	.status :global(.ui-head) { margin-bottom: 0; }
	.body { display: flex; flex-direction: column; gap: 16px; }
</style>
