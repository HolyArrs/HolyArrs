<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { runTone, runWord, busyNow, dot, every, groupSchedules, mayRun, stateWords, unit, until, type LoopRow, type ScheduleRow } from '#lib/health/index.ts';
	import { onMount, tick } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, dateTime } from '#lib/core/format.ts';

	let rows = $state<ScheduleRow[] | null>(null);
	let loops = $state<LoopRow[]>([]);
	let safeMode = $state(false);
	let error = $state('');
	let busy = $state<string | null>(null);
	let editing = $state<string | null>(null);
	let amount = $state(0);
	let editError = $state('');
	let now = $state(Date.now());

	const groups = $derived(rows ? groupSchedules(rows) : []);
	const running = $derived(rows ? busyNow(rows) : []);
	const canRun = $derived(session.can('manage_library'));
	const canChange = $derived(session.can('settings'));

	// "every day" reads as a column value: "Every day".
	const often = (s: number) => {
		const w = every(s);
		return w.charAt(0).toUpperCase() + w.slice(1);
	};
	// What the Next column says when the schedule is not waiting for its time.
	const nextWords = (s: ScheduleRow) => {
		if (!s.enabled) return t('settings.schedules.turnedOff');
		if (s.state === 'part_off' || s.state === 'safe_mode') return stateWords[s.state];
		if (s.state === 'running') return t('settings.schedules.running');
		if (s.state === 'queued') return t('settings.schedules.waiting');
		return until(s.next_run, now);
	};

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
			toasts.push(r.already ? t('settings.schedules.alreadyQueued', { label: s.label }) : t('settings.schedules.started', { label: s.label }), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('settings.schedules.couldntStart', { label: s.label }), 'fail', { body: message(e) });
		} finally {
			busy = null;
		}
	}

	async function setEnabled(s: ScheduleRow, enabled: boolean) {
		busy = s.kind;
		try {
			await api.put(`/schedules/${s.kind}`, { enabled });
			toasts.push(enabled ? t('settings.schedules.autoOn', { label: s.label }) : t('settings.schedules.autoOff', { label: s.label }), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('settings.schedules.couldntSave'), 'fail', { body: message(e) });
		} finally {
			busy = null;
		}
	}

	async function edit(s: ScheduleRow) {
		editing = s.kind;
		amount = s.every_s / unit(s.step_s).size;
		editError = '';
		await tick();
		document.getElementById(`ev-${s.kind}`)?.focus();
	}

	function closeEditor(s: ScheduleRow) {
		editing = null;
		void tick().then(() => document.getElementById(`change-${s.kind}`)?.focus());
	}

	async function save(e: SubmitEvent, s: ScheduleRow) {
		e.preventDefault();
		editError = '';
		busy = s.kind;
		const secs = Math.round(Number(amount) * unit(s.step_s).size);
		try {
			await api.put(`/schedules/${s.kind}`, { every_s: secs });
			toasts.push(t('settings.schedules.saved', { label: s.label }), 'ok', { body: t('settings.schedules.runs', { every: every(secs) }) });
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
		const timer = setInterval(load, 30000);
		const offs = [live.on('job', load), live.on('resync', load)];
		return () => {
			clearInterval(timer);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>{t('settings.schedules.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.schedules.title')} />

{#if safeMode}<p class="notice warn">{t('settings.schedules.safeMode')}</p>{/if}

{#if !rows}
	{#if error}
		<LoadError hasData={false} message={error} onretry={load} />
	{:else}
		<SkSettings rows={4} />
	{/if}
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if running.length}
		<section class="now" aria-labelledby="h-now" data-testid="running-now">
			<h2 id="h-now" class="group-head">{t('settings.schedules.runningNow')}</h2>
			<ul class="s-card now-card">
				{#each running as s (s.kind)}
					<li>
						<span class="dot busy" aria-hidden="true"></span>
						<b>{s.label}</b>
						<span class="now-state">{s.state === 'running' ? t('settings.schedules.running') : t('settings.schedules.waiting')}</span>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<div class="groups" data-testid="schedules">
		{#each groups as g (g.id)}
			<section class="sched-group" aria-labelledby="sg-{g.id}">
				<h2 id="sg-{g.id}" class="group-head">{g.label}</h2>
				<div class="s-card sched-card">
					<div class="cols" aria-hidden="true">
						<span>{t('settings.schedules.name')}</span><span>{t('settings.schedules.howOften')}</span><span>{t('settings.schedules.lastRun')}</span><span>{t('settings.schedules.nextRun')}</span><span></span>
					</div>
					<ul>
						{#each g.rows as s (s.kind)}
							{@const u = unit(s.step_s)}
							<li data-testid="schedule" data-kind={s.kind} data-state={s.state} class:open={editing === s.kind}>
								<div class="line">
									<div class="name">
										<span class="dot {dot(s)}" title={stateWords[s.state]}><span class="sr-only">{stateWords[s.state]}</span></span>
										<span class="name-copy">
											<b>{s.label}</b>
											<span class="about" title={s.about}>{s.about}</span>
										</span>
									</div>
									<div class="meta">
									<div class="cell"><span class="k">{t('settings.schedules.howOften')}</span><span>{often(s.every_s)}</span></div>
									<div class="cell"><span class="k">{t('settings.schedules.lastRun')}</span>
										{#if s.last_run}<time datetime={s.last_run} title={dateTime(s.last_run)}>{ago(s.last_run, now)}</time>{:else}<span class="faint">{t('settings.schedules.notYet')}</span>{/if}
									</div>
									<div class="cell" data-testid="next"><span class="k">{t('settings.schedules.nextRun')}</span>
										{#if s.enabled && s.next_run && s.state === 'waiting'}<time datetime={s.next_run} title={dateTime(s.next_run)}>{nextWords(s)}</time>{:else}<span class:faint={!s.enabled || s.state === 'part_off' || s.state === 'safe_mode'}>{nextWords(s)}</span>{/if}
									</div>
									</div>
									<div class="acts">
										{#if canRun && mayRun(s)}
											<Button type="button" busy={busy === s.kind && editing !== s.kind} disabled={busy === s.kind} onclick={() => runNow(s)} aria-label={t('settings.schedules.runLabel', { label: s.label })}>{t('settings.schedules.runNow')}</Button>
										{/if}
										{#if canChange && editing !== s.kind}
											<Button variant="plain" type="button" id="change-{s.kind}" onclick={() => edit(s)} aria-label={t('settings.schedules.changeLabel', { label: s.label })} aria-expanded="false" aria-controls="ed-{s.kind}">{t('settings.schedules.change')}</Button>
										{/if}
									</div>
								</div>

								{#if s.last_job === 'failed'}
									<p class="failed" title={s.last_job_ended ? t('settings.schedules.ended', { when: dateTime(s.last_job_ended) }) : undefined}>
										<Icon name="alert" size={13} />{s.last_job_error ? t('settings.schedules.failedWith', { error: s.last_job_error }) : t('settings.schedules.failed')}
									</p>
								{/if}
								{#if s.kind === 'refresh_titles' && !s.enabled}
									<p class="stale-note" data-testid="movie-refresh-off-note">
										{t('settings.schedules.refreshOffNote')}
									</p>
								{/if}
								{#if s.kind === 'rescan_library' && !s.enabled}
									<p class="stale-note" data-testid="rescan-off-note">{t('settings.schedules.rescanOffNote')}</p>
								{/if}

								{#if editing === s.kind}
									<form class="editor" id="ed-{s.kind}" aria-label={t('settings.schedules.howOftenRuns', { label: s.label })} onsubmit={(e) => save(e, s)}>
										<div class="ed-row">
											<div class="ed-copy">
												<span class="ed-about">{s.about}</span>
												<label for="ev-{s.kind}">{t('settings.schedules.every', { unit: u.word })}</label>
												<span class="hint">{t('settings.schedules.range', { min: s.min_s / u.size, max: s.max_s / u.size, unit: u.word })}</span>
											</div>
											<input id="ev-{s.kind}" class="input" type="number" min={s.min_s / u.size} max={s.max_s / u.size} step={s.step_s / u.size} bind:value={amount} onkeydown={(e) => e.key === 'Escape' && closeEditor(s)} />
										</div>
										{#if s.kind === 'refresh_titles'}
											<div class="ed-row">
												<div class="ed-copy">
													<b>{t('settings.schedules.autoRefresh')}</b>
													<span class="hint">{s.enabled ? t('settings.schedules.refreshOn') : t('settings.schedules.refreshOff')}</span>
												</div>
												<Button variant={s.enabled ? 'destructive' : 'secondary'} type="button" disabled={busy === s.kind} onclick={() => setEnabled(s, !s.enabled)}>{s.enabled ? t('settings.schedules.refreshTurnOff') : t('settings.schedules.refreshTurnOn')}</Button>
											</div>
										{:else if s.kind === 'rescan_library'}
											<div class="ed-row">
												<div class="ed-copy">
													<b>{t('settings.schedules.autoRescan')}</b>
													<span class="hint">{s.enabled ? t('settings.schedules.rescanOn') : t('settings.schedules.rescanOff')}</span>
												</div>
												<Button variant={s.enabled ? 'destructive' : 'secondary'} type="button" disabled={busy === s.kind} onclick={() => setEnabled(s, !s.enabled)}>{s.enabled ? t('settings.schedules.rescanTurnOff') : t('settings.schedules.rescanTurnOn')}</Button>
											</div>
										{/if}
										{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
										<div class="ed-acts">
											{#if s.settings}<a class="its" href={to(s.settings)}>{t('settings.schedules.settings')}<Icon name="chevronRight" size={14} /></a>{/if}
											<Button variant="primary" type="submit" busy={busy === s.kind} disabled={busy === s.kind}>{t('settings.schedules.save')}</Button>
											<Button variant="plain" type="button" onclick={() => closeEditor(s)}>{t('settings.schedules.cancel')}</Button>
										</div>
									</form>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			</section>
		{/each}
	</div>

	{#if loops.length}
		<section class="sched-group" aria-labelledby="sg-loops" data-testid="loops">
			<h2 id="sg-loops" class="group-head">{t('settings.schedules.stages')}</h2>
			<p class="group-about"><Words key="settings.schedules.stagesNote" links={{ a: to('/settings/parts') }} /></p>
			<div class="s-card sched-card">
				<ul>
					{#each loops as l (l.key)}
						<li data-testid="loop" class="loop">
							<div class="line">
								<div class="name">
									<span class="dot {runTone(l.state) === 'ok' ? 'ok' : runTone(l.state) === 'fail' ? 'fail' : runTone(l.state) === 'wait' ? 'busy' : 'off'}" title={runWord(l)}><span class="sr-only">{runWord(l)}</span></span>
									<span class="name-copy">
										<b>{l.name}</b>
										{#if l.reason}<span class="about" title={l.reason}>{l.reason}</span>{/if}
									</span>
								</div>
								<div class="meta">
								<div class="cell"><span class="k">{t('settings.schedules.howOften')}</span><span>{l.every_s ? often(l.every_s) : t('settings.schedules.allTime')}</span></div>
								<div class="cell" class:empty={!l.last_pass}><span class="k">{t('settings.schedules.lastRun')}</span>{#if l.last_pass}<time datetime={l.last_pass} title={dateTime(l.last_pass)}>{ago(l.last_pass, now)}</time>{:else}<span class="faint">{t('settings.schedules.notYet')}</span>{/if}</div>
								<div class="cell" class:empty={!l.next_pass}><span class="k">{t('settings.schedules.nextRun')}</span>{#if l.next_pass}<time datetime={l.next_pass} title={dateTime(l.next_pass)}>{until(l.next_pass, now)}</time>{:else}<span class="faint">{t('settings.schedules.notYet')}</span>{/if}</div>
								</div>
								<div class="acts"><span class="chip {runTone(l.state)}">{runWord(l)}</span></div>
							</div>
						</li>
					{/each}
				</ul>
			</div>
		</section>
	{/if}
{/if}

<style>
	/* A grouped list, like a phone's Settings app: a small heading over each rounded card, compact rows with a status dot, and columns on wide screens. */
	.groups { display: grid; gap: 22px; }
	.sched-group + .sched-group, .groups + .sched-group { margin-top: 22px; }
	.group-head { padding: 0 6px 6px; font: var(--t-footnote); font-weight: 600; letter-spacing: 0.01em; color: var(--text-muted); }
	.group-about { margin: -2px 6px 8px; color: var(--text-muted); font: var(--t-footnote); font-weight: 400; }
	.group-about :global(a) { color: var(--accent-text); font-weight: 600; }
	.s-card { margin: 0; padding: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.sched-card { container-type: inline-size; overflow: hidden; }
	.sched-card ul { margin: 0; padding: 0; list-style: none; }

	.now { margin-bottom: 22px; }
	.now-card { padding: 4px 0; list-style: none; box-shadow: 0 0 0 1px color-mix(in srgb, var(--wait) 40%, var(--line)); }
	.now-card li { display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 16px; }
	.now-card li + li { border-top: 1px solid var(--line); }
	.now-card b { flex: 1; min-width: 0; font: var(--t-callout); font-weight: 600; }
	.now-state { color: var(--wait-text); font: var(--t-footnote); }

	/* Columns: name, how often, last, next, actions. */
	.cols, .line {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 128px 96px 112px 172px;
		align-items: center;
		column-gap: 12px;
	}
	.cols { padding: 9px 16px 7px; border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-caption); letter-spacing: 0.01em; }
	li { position: relative; }
	li + li::before { content: ''; position: absolute; top: 0; left: 36px; right: 0; border-top: 1px solid var(--line); }
	.line { min-height: 52px; padding: 7px 12px 7px 16px; }
	.name { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.name-copy { display: grid; min-width: 0; }
	.name-copy b { font: var(--t-callout); font-weight: 600; color: var(--text); }
	.about { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; overflow-wrap: anywhere; }
	.cell { min-width: 0; font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cell time { text-decoration: underline dotted color-mix(in srgb, var(--text-muted) 50%, transparent); text-underline-offset: 3px; cursor: help; }
	.k { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.faint { color: var(--text-muted); }
	.meta { display: contents; }
	.acts { display: flex; align-items: center; justify-content: flex-end; gap: 4px; }

	.dot { position: relative; flex: none; width: 10px; height: 10px; border-radius: 50%; background: var(--text-faint); }
	.dot.ok { background: var(--ok); }
	.dot.fail { background: var(--fail); }
	.dot.off { background: transparent; box-shadow: inset 0 0 0 1.5px var(--text-faint); }
	.dot.busy { background: var(--wait); }
	.dot.busy::after { content: ''; position: absolute; inset: -4px; border-radius: 50%; border: 2px solid var(--wait); opacity: 0; animation: ping 1.6s var(--ease-out) infinite; }
	@keyframes ping { 0% { transform: scale(0.6); opacity: 0.7; } 100% { transform: scale(1.4); opacity: 0; } }

	.failed, .stale-note { display: flex; gap: 6px; align-items: flex-start; margin: -2px 16px 10px 36px; font: var(--t-footnote); font-weight: 400; overflow-wrap: anywhere; }
	.failed { color: var(--fail-text); }
	.failed :global(.icon) { flex: none; margin-top: 1px; }
	.stale-note { padding: 8px 10px; border-radius: 10px; background: color-mix(in srgb, var(--wait) 10%, transparent); color: var(--text); }

	/* The interval editor opens under its row, like an inset sheet. */
	li.open { background: color-mix(in srgb, var(--surface-2) 55%, transparent); }
	.editor { display: grid; gap: 12px; margin: 0 12px 12px 36px; padding: 14px 16px; border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), 0 6px 20px -12px rgb(0 0 0 / 30%); animation: open var(--dur-fast, 160ms) var(--ease-out); }
	@keyframes open { from { opacity: 0; transform: translateY(-4px); } }
	.ed-row { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; justify-content: space-between; }
	.ed-row + .ed-row { padding-top: 12px; border-top: 1px solid var(--line); }
	.ed-copy { display: grid; gap: 2px; min-width: 0; }
	.ed-about { margin-bottom: 6px; color: var(--text-muted); font: var(--t-footnote); font-weight: 400; }
	.ed-copy label, .ed-copy b { font: var(--t-callout); font-weight: 600; }
	.ed-row .input { width: 110px; flex: none; }
	.ed-acts { display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: wrap; }
	.its { display: inline-flex; align-items: center; gap: 2px; margin-right: auto; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }

	/* Narrow cards (phones, small windows): name and actions on top, the times on one line under them with their labels showing. */
	@container (max-width: 640px) {
		.cols { display: none; }
		.line { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'name' 'meta' 'acts'; row-gap: 4px; padding: 10px 12px 10px 14px; }
		.name { grid-area: name; }
		.acts { grid-area: acts; justify-content: flex-start; padding-left: 14px; margin-top: 2px; }
		.acts:empty { display: none; }
		.cell.empty { display: none; }
		.loop .line { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'name acts' 'meta meta'; }
		.loop .acts { padding-left: 0; margin-top: 0; }
		.meta { grid-area: meta; display: flex; flex-wrap: wrap; gap: 2px 14px; padding-left: 20px; }
		.cell { font: var(--t-footnote); font-weight: 400; color: var(--text-muted); }
		.cell time { color: var(--text); }
		.k { position: static; width: auto; height: auto; clip: auto; }
		.k::after { content: ' '; }
		.meta .cell:first-child .k { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
		.about { display: none; }
		li + li::before { left: 34px; }
		.failed, .stale-note, .editor { margin-left: 34px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.dot.busy::after, .editor { animation: none; }
	}
</style>
