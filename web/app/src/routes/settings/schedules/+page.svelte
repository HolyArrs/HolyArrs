<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago } from '#lib/format.ts';
	import { runTone, runWord } from '#lib/health.ts';
	import { every, mayRun, stateTone, stateWords, unit, until, type LoopRow, type ScheduleRow } from '#lib/schedules.ts';

	let rows = $state<ScheduleRow[] | null>(null);
	let loops = $state<LoopRow[]>([]);
	let safeMode = $state(false);
	let error = $state('');
	let busy = $state<string | null>(null);
	let editing = $state<string | null>(null);
	let amount = $state(0);
	let editError = $state('');
	let now = $state(Date.now());

	async function load() {
		try {
			const r = await api.get<{ schedules: ScheduleRow[]; loops: LoopRow[]; safe_mode: boolean }>('/schedules');
			rows = r.schedules;
			loops = r.loops;
			safeMode = r.safe_mode;
			now = Date.now();
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	async function runNow(s: ScheduleRow) {
		busy = s.kind;
		try {
			const r = await api.post<{ job_id: number; already: boolean }>(`/schedules/${s.kind}/run`);
			toasts.push(r.already ? `${s.label} is already waiting or running.` : `${s.label} started.`, 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	async function setEnabled(s: ScheduleRow, enabled: boolean) {
		busy = s.kind;
		try {
			await api.put(`/schedules/${s.kind}`, { enabled });
			toasts.push(enabled ? `${s.label} will run automatically again.` : `${s.label} automatic runs are off.`, 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	function edit(s: ScheduleRow) {
		editing = s.kind;
		amount = s.every_s / unit(s.step_s).size;
		editError = '';
	}

	async function save(e: SubmitEvent, s: ScheduleRow) {
		e.preventDefault();
		editError = '';
		busy = s.kind;
		const secs = Math.round(Number(amount) * unit(s.step_s).size);
		try {
			await api.put(`/schedules/${s.kind}`, { every_s: secs });
			toasts.push(`${s.label} now runs ${every(secs)}.`, 'ok');
			editing = null;
			await load();
		} catch (err) {
			editError = message(err);
		} finally {
			busy = null;
		}
	}

	onMount(() => {
		void load();
		const t = setInterval(load, 30000);
		const offs = [live.on('job', load), live.on('resync', load)];
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>Schedules · HolyArrs</title></svelte:head>

<PageHeader title="Schedules" subtitle="Everything HolyArrs does on a timer: when it last ran, when it runs next, and how often." />

{#if safeMode}<p class="notice warn">Safe mode: only the core runs. Schedules outside the core wait for a normal start.</p>{/if}

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !rows}
	<SkSettings rows={4} />
{:else}
	<div data-testid="schedules">
		<SettingsCard title="Scheduled work">
			<ul class="rows sched">
				{#each rows as s (s.kind)}
					{@const u = unit(s.step_s)}
					<li data-testid="schedule" data-kind={s.kind} data-state={s.state}>
						<div class="grow small">
							<div class="head">
								<b>{s.label}</b>
								<span class="chip {stateTone(s.state)}">{stateWords[s.state]}</span>
							</div>
							<span class="muted">{s.about}</span>
							{#if s.kind === 'refresh_titles' && !s.enabled}
								<p class="xs muted stale-note" data-testid="movie-refresh-off-note">
									Automatic movie detail updates are off, so names, dates and artwork can become stale. A request already in flight may finish. Manual Library refresh remains available.
								</p>
							{/if}
							<div class="xs muted when">
								<span>Runs {every(s.every_s)}</span>
								<span
									>Last: {s.last_run ? ago(s.last_run, now) : 'not yet'}{#if s.last_job === 'failed'}
										(failed{s.last_job_error ? `: ${s.last_job_error}` : ''}){/if}</span
								>
								<span data-testid="next">Next: {s.enabled ? until(s.next_run, now) : 'turned off'}</span>
								{#if s.settings}<a href={to(s.settings)}>Its settings</a>{/if}
							</div>
							{#if editing === s.kind}
								<form class="edit" onsubmit={(e) => save(e, s)}>
									<label for="ev-{s.kind}" class="small">Every ({u.word})</label>
									<input
										id="ev-{s.kind}"
										class="input"
										type="number"
										min={s.min_s / u.size}
										max={s.max_s / u.size}
										step={s.step_s / u.size}
										bind:value={amount}
									/>
									<span class="hint">{s.min_s / u.size} to {s.max_s / u.size} {u.word}</span>
									<Button variant="primary" type="submit" disabled={busy === s.kind}>Save</Button>
									<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
									{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
								</form>
							{/if}
						</div>
						<div class="row acts">
							{#if session.can('manage_library') && mayRun(s)}
								<Button
									type="button"
									disabled={busy === s.kind}
									aria-busy={busy === s.kind}
									onclick={() => runNow(s)}
									aria-label="Run {s.label} now">Run now</Button>
							{/if}
							{#if session.can('settings') && editing !== s.kind}
								<Button variant="plain" type="button" onclick={() => edit(s)} aria-label="Change how often {s.label} runs">Change</Button>
							{/if}
							{#if session.can('settings') && s.kind === 'refresh_titles'}
								<Button variant="plain"
									type="button"
									disabled={busy === s.kind}
									onclick={() => setEnabled(s, !s.enabled)}
								>{s.enabled ? 'Turn off automatic refresh' : 'Turn on automatic refresh'}</Button>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		</SettingsCard>
	</div>

	<div data-testid="loops">
		<SettingsCard title="Background loops" description="Work that keeps running on its own. Pause or restart one in Settings > Parts and controls.">
			<ul class="rows sched">
				{#each loops as l (l.key)}
					<li data-testid="loop">
						<div class="grow small">
							<div class="head"><b>{l.name}</b> <span class="chip {runTone(l.state)}">{runWord(l)}</span></div>
							<div class="xs muted when">
								<span>{l.every_s ? `Runs ${every(l.every_s)}` : 'Runs all the time'}</span>
								{#if l.last_pass}<span>Last: {ago(l.last_pass, now)}</span>{/if}
								{#if l.next_pass}<span>Next: {until(l.next_pass, now)}</span>{/if}
							</div>
							{#if l.reason}<span class="xs muted">{l.reason}</span>{/if}
						</div>
					</li>
				{/each}
			</ul>
		</SettingsCard>
	</div>
{/if}

<style>
	.sched li {
		align-items: flex-start;
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.sched .grow {
		min-width: 0;
		flex: 1 1 260px;
		overflow-wrap: anywhere;
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.when {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-1) var(--s-3);
		margin-top: var(--s-1);
	}
	.edit {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
		margin-top: var(--s-2);
	}
	.edit .input {
		width: 110px;
	}
	.acts {
		flex-wrap: wrap;
	}
	.stale-note {
		margin: var(--s-2) 0 0;
	}
</style>
