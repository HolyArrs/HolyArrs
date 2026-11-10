<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// The Status page body: four tabs of plates (Status, Jobs, Updates, About).
	import '#lib/ui/plate.css';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import PartsRail from './PartsRail.svelte';
	import RunsTimeline from './RunsTimeline.svelte';
	import WhyNote from './WhyNote.svelte';
	import { nextRun, until } from './schedules.ts';
	import type { Health } from './types.ts';
	import { atVerb, everyKind, needsAttention, nothingLinked, stopped, type StatusGroup, type StatusRow } from './rows.ts';
	import { api } from '../core/api.ts';
	import { ago, bytes, dateTime } from '../core/format.ts';
	import { to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import type { HealthCheck, UpdateInfo } from '../core/types.ts';
	import { onMount, untrack } from 'svelte';
	import type { Snippet } from 'svelte';

	type Disk = { path: string; kind: string; label: string; free_bytes: number; total_bytes: number };
	type Update = UpdateInfo & { notes?: string };
	type About = { version: string; go: string; os: string; docker: boolean; sqlite: string; migration: number; config_dir: string; start_dir: string; uptime_s: number; started_at: string };

	// Library kind to the plural shown beside each disk.
	const kindWord = (k: string): string => (k === 'movie' ? t('health.sections.movies') : k === 'show' ? t('health.sections.shows') : k === 'music' ? t('health.sections.music') : '');
	const tabs = $derived([
		{ id: 'parts', label: t('health.sections.tabStatus') },
		{ id: 'jobs', label: t('health.sections.tabJobs') },
		{ id: 'updates', label: t('health.sections.tabUpdates') },
		{ id: 'about', label: t('health.sections.tabAbout') }
	]);
	// Names of reads an account may not do, mapped to the kind they remove.
	const hiddenKind: Record<string, string> = { 'outside lists': 'lists', alerts: 'alerts' };

	let {
		groups,
		tab,
		ontab,
		canAct,
		busy,
		oncheck,
		labelOf,
		reader,
		failed = [],
		hidden = [],
		checkRuns = 0,
		updates
	}: {
		groups: StatusGroup[];
		tab: string;
		ontab: (t: string) => void;
		canAct: boolean;
		busy: (key: string) => boolean;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
		reader: HealthCheck | undefined;
		failed?: string[];
		// Reads this account may not do; their kinds are left out.
		hidden?: string[];
		// Goes up after every check, so Storage reloads its disks.
		checkRuns?: number;
		// The page draws the Updates tab, so health does not depend on updates.
		updates: Snippet<[{ upd: Update | null; docker: boolean | null; reader: HealthCheck | undefined; checksKnown: boolean; canAct: boolean; onupd: (u: Update) => void }]>;
	} = $props();

	let disks = $state<Disk[] | null>(null);
	let upd = $state<Update | null>(null);
	let updFailed = $state(false);
	let srv = $state<Health | null>(null);
	let about = $state<About | null>(null);
	let aboutAt = $state(0);
	let aboutFailed = $state(false);
	let aboutDenied = $state(false);
	// One clock for every relative time, so "in 5 min" keeps counting.
	let now = $state(Date.now());
	let why = $state<ReturnType<typeof WhyNote>>();
	let tablist = $state<HTMLElement>();
	// Tab changes go through the page address and drop focus; this puts it back after a key press.
	let refocus = false;
	let disksAt = 0;
	let disksFor = '';

	const parts = $derived(groups.filter((g) => g.id !== 'jobs'));
	const kinds = $derived(everyKind(parts).filter((k) => !hidden.some((h) => hiddenKind[h] === k.id)));
	// Library rows are folder and disk checks, so they show in Storage beside each disk.
	const libRows = $derived(parts.find((g) => g.id === 'library')?.rows ?? []);
	const backups = $derived(libRows.find((r) => r.key.startsWith('backups')));
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	const jobsTone = $derived(jobs.some((j) => j.tone === 'fail') ? 'fail' : jobs.some((j) => j.tone === 'wait') ? 'wait' : null);
	const allRows = $derived(groups.flatMap((g) => g.rows));
	const problems = $derived(needsAttention(allRows));
	// A fresh install has nothing to check yet, so say that instead of "no problems".
	const empty = $derived(!failed.includes('parts') && nothingLinked(allRows));
	const upcoming = $derived(jobs.filter((j) => j.nextRun && !stopped(j)).sort((a, b) => Date.parse(a.nextRun!) - Date.parse(b.nextRun!)));
	const recent = $derived(jobs.filter((j) => j.lastRun).sort((a, b) => Date.parse(b.lastRun!) - Date.parse(a.lastRun!)));

	// Reads disks only on the Status tab; groups is read so a live push counts as a change.
	$effect(() => {
		void groups;
		const why = `${tab}:${checkRuns}`;
		if (tab === 'parts') untrack(() => loadDisks(why));
		else disksFor = '';
	});

	// Keeps keyboard focus on the picked tab after the address changes.
	$effect(() => {
		const t = tab;
		if (!refocus) return;
		refocus = false;
		requestAnimationFrame(() => tablist?.querySelector<HTMLElement>(`#st-tab-${t}`)?.focus());
	});

	function tabKey(e: KeyboardEvent) {
		const i = tabs.findIndex((tb) => tb.id === tab);
		const n = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : e.key === 'Home' ? 0 : e.key === 'End' ? tabs.length - 1 : -1;
		if (n < 0 || n >= tabs.length || n === i) return;
		e.preventDefault();
		refocus = true;
		tablist?.querySelector<HTMLElement>(`#st-tab-${tabs[n].id}`)?.focus();
		ontab(tabs[n].id);
	}

	// Free space hardly changes: reload on entering Storage and after a check, at most once a minute on live pushes.
	function loadDisks(why: string) {
		if (why === disksFor && Date.now() - disksAt < 60_000) return;
		disksFor = why;
		disksAt = Date.now();
		api.get<{ disks: Disk[] }>('/health/disks').then((d) => (disks = d.disks), () => (disks ??= []));
	}
	function loadUpdate() {
		updFailed = false;
		api.get<Update>('/app/update').then((u) => (upd = u), () => (updFailed = !upd));
	}
	// /health holds the live server numbers; /app/about needs Settings, so a 403 means version only.
	function loadAbout() {
		aboutFailed = false;
		api.get<Health>('/health').then((h) => (srv = h), () => {});
		api.get<About>('/app/about').then(
			(a) => {
				about = a;
				aboutAt = Date.now();
			},
			(e: unknown) => {
				if ((e as { status?: number })?.status === 403) aboutDenied = true;
				else aboutFailed = !about;
			}
		);
	}

	onMount(() => {
		loadUpdate();
		loadAbout();
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	// Running clock like "0d 00:27:14".
	function uptime(s: number): string {
		const t = Math.floor(Math.max(0, s));
		const two = (n: number) => String(n).padStart(2, '0');
		return `${Math.floor(t / 86400)}d ${two(Math.floor(t / 3600) % 24)}:${two(Math.floor(t / 60) % 60)}:${two(t % 60)}`;
	}
	const folderRows = (d: Disk) => libRows.filter((r) => r.name === `${t('health.check.folder')} ${d.path}` || r.name === `${t('health.check.freeSpace')} ${d.path}`);
	const worst = (rs: StatusRow[]) => rs.find((r) => r.tone === 'fail') ?? rs.find((r) => r.tone === 'wait');
	// Null when the disk size is unknown.
	const usedPct = (d: Disk) => (d.total_bytes > 0 && d.free_bytes >= 0 ? Math.round(((d.total_bytes - d.free_bytes) / d.total_bytes) * 100) : null);
	// A run that should have started already says how late it is.
	function nextWord(j: StatusRow): string {
		if (!j.nextRun || stopped(j) || Number.isNaN(Date.parse(j.nextRun))) return t('health.sections.notPlanned');
		return j.word === t('health.job.running') ? until(j.nextRun, now) : nextRun(j.nextRun, now);
	}
	// The verb keeps "3 d ago" from reading as "not checked for 3 days".
	const when = (r: StatusRow) => (atVerb[r.group] ? t('health.rows.since', { verb: atVerb[r.group] ?? '', when: ago(r.at, now) }) : ago(r.at, now));
</script>

<div class="secs" data-testid="status-sections">
	<div class="seg" role="tablist" aria-label={t('health.sections.view')} tabindex="-1" bind:this={tablist} onkeydown={tabKey}>
		{#each tabs as tb (tb.id)}
			<button type="button" role="tab" id="st-tab-{tb.id}" aria-selected={tab === tb.id} aria-controls={tab === tb.id ? `st-panel-${tb.id}` : undefined} tabindex={tab === tb.id ? 0 : -1} onclick={() => ontab(tb.id)}>
				{tb.label}{#if tb.id === 'jobs' && jobsTone}<i class="badge {jobsTone}" aria-hidden="true"></i><span class="sr">{jobsTone === 'fail' ? t('health.sections.hasFailures') : t('health.sections.needsAttention')}</span>{:else if tb.id === 'updates' && upd?.newer}<i class="badge wait" aria-hidden="true"></i><span class="sr">{t('health.sections.updateAvailable')}</span>{/if}
			</button>
		{/each}
	</div>

	<div class="panel" class:tight={tab === 'jobs'} role="tabpanel" id="st-panel-{tab}" aria-labelledby="st-tab-{tab}">
	{#if tab === 'parts'}
		<section class="st-plate" aria-labelledby="s-health">
			<h2 id="s-health">{t('health.sections.health')}</h2>
			{#if problems.length}
				<ul class="health" data-testid="status-attention">
					{#each problems as r (r.key)}
						<li class="hrow" data-tone={r.tone} data-key={r.key}>
							<span class="hico {r.tone}" aria-hidden="true">!</span>
							<span class="hmsg">
								<b>{r.name}</b> <span class="hk">· {labelOf(r)}</span> <span class="chip {r.tone}">{r.word}</span>
								{#if r.reason}<span class="hr">{serverWords(r.reason)}</span>{/if}
							</span>
							{#if r.at}<time class="hat" datetime={r.at} title={dateTime(r.at)}>{when(r)}</time>{/if}
							{#if canAct}
								<span class="hacts">
									{#if r.check}<Button variant="plain" size="sm" disabled={busy(r.key)} onclick={() => oncheck(r)}>{r.group === 'jobs' ? t('health.sections.runNow') : t('health.rows.check')}</Button>{/if}
									{#if r.fix}<Button variant="plain" size="sm" href={to(r.fix)}>{t('health.rows.fix')}</Button>{/if}
								</span>
							{/if}
						</li>
					{/each}
				</ul>
			{:else if empty}
				<p class="ok-line"><i class="st-dot idle" aria-hidden="true"></i>{t('health.sections.nothingToCheck')}{#if canAct} <Button variant="plain" size="sm" href={to('/settings/sources')}>{t('health.sections.addSource')}</Button>{/if}</p>
			{:else if failed.includes('parts')}
				<p class="ok-line"><i class="st-dot idle" aria-hidden="true"></i>{t('health.headline.none')}.</p>
			{:else}
				<p class="ok-line"><i class="st-dot ok" aria-hidden="true"></i>{t('health.sections.allGood')}</p>
			{/if}
		</section>

		<PartsRail {kinds} {canAct} {busy} {oncheck} />

		<section class="st-plate" aria-labelledby="s-disk">
			<h2 id="s-disk">{t('health.groups.library')}</h2>
			{#if disks === null}
				<div class="dload" role="status"><span class="sr">{t('health.sections.loadingStorage')}</span><Skeleton lines={2} /></div>
			{:else if !disks.length}
				<p class="none">{t('health.sections.noFolders')}</p>
			{:else}
				<ul class="disks">
					{#each disks as d (d.path)}
						{@const p = usedPct(d)}
						{@const w = worst(folderRows(d))}
						<li class="disk {w?.tone ?? ''}" class:bad={!!w}>
							<div class="dtop">
								<span class="dname">{d.label || kindWord(d.kind) || t('health.sections.library')}</span>
								{#if w}<span class="chip {w.tone}">{w.word}</span>{:else}<span class="chip ok">{t('health.word.working')}</span>{/if}
								<span class="dnum">
									{#if d.free_bytes >= 0}<b>{bytes(d.free_bytes)}</b> {t('health.sections.free')}{/if}{#if d.total_bytes > 0}&nbsp;{t('health.sections.of')} {bytes(d.total_bytes)}{/if}
								</span>
							</div>
							<div class="dbar" class:warn={p !== null && p >= 85} class:full={p !== null && p >= 95} role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={p ?? undefined} aria-label={t('health.sections.used', { name: d.label || d.path })}>
								<i style:width="{p ?? 0}%"></i>
							</div>
							<div class="dfoot"><span class="mono" title={d.path}>{d.path}</span>{#if p !== null}<span>{t('health.sections.pctUsed', { pct: p })}</span>{/if}</div>
							{#if w?.reason}<p class="dwhy">{serverWords(w.reason)}{#if canAct && w.fix} <Button variant="plain" size="sm" href={to(w.fix)}>{t('health.rows.fix')}</Button>{/if}</p>{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if backups}
				<p class="bk">{t('health.kind.backups')} <span class="chip {backups.tone}">{backups.word}</span>{#if backups.at} · {t('health.sections.lastChecked')} <time datetime={backups.at} title={dateTime(backups.at)}>{ago(backups.at, now)}</time>{/if}{#if backups.reason && backups.tone !== 'ok'} · {serverWords(backups.reason)}{/if}</p>
			{/if}
		</section>
	{:else if tab === 'jobs'}
		<RunsTimeline {upcoming} {recent} {now} />

		<section class="st-plate" aria-labelledby="s-jobs">
			<h2 id="s-jobs">{t('health.sections.allJobs')}</h2>
			{#if !jobs.length}
				<p class="none">{failed.includes('jobs') ? t('health.sections.jobsFailed') : t('health.sections.noJobs')}</p>
			{:else}
			<table class="tbl jobs">
				<thead><tr><th>{t('health.sections.name')}</th><th>{t('health.sections.state')}</th><th>{t('health.sections.lastRun')}</th><th>{t('health.sections.nextRun')}</th>{#if canAct}<th><span class="sr">{t('health.sections.actions')}</span></th>{/if}</tr></thead>
				<tbody>
					{#each jobs as r (r.key)}
						<tr data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<td class="jn">{r.name}{#if r.tone === 'fail' && r.reason}<span class="jr">{serverWords(r.reason)}</span>{/if}</td>
							<td class="js"><span class="chip {r.tone}">{r.word}</span></td>
							<td class="jl"><span class="k">{t('health.sections.lastRun')}</span>{#if r.lastRun}<time datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun, now)}</time>{:else}{t('health.sections.notYet')}{/if}</td>
							<td class="jx"><span class="k">{t('health.sections.nextRun')}</span>{nextWord(r)}</td>
							{#if canAct}<td class="num"><Button variant="plain" size="sm" disabled={busy(r.key)} onclick={() => oncheck(r)}>{t('health.sections.runNow')}</Button></td>{/if}
						</tr>
					{/each}
				</tbody>
			</table>
			{/if}
		</section>
	{:else if tab === 'updates'}
		{#if updFailed && !upd}
			<section class="st-plate" aria-labelledby="s-upd">
				<h2 id="s-upd">{t('health.groups.app')}</h2>
				<p class="none fail-line">{t('health.sections.updFailed')} <Button variant="plain" size="sm" onclick={loadUpdate}>{t('health.sections.tryAgain')}</Button></p>
			</section>
		{:else}
			{@render updates({ upd, docker: about ? about.docker : null, reader, checksKnown: !failed.includes('health checks'), canAct, onupd: (u) => (upd = u) })}
		{/if}
	{:else}
		<section class="st-plate" aria-labelledby="s-about">
			<h2 id="s-about">{t('health.sections.tabAbout')}</h2>
			{#if aboutFailed && !about}
				<p class="none fail-line">{t('health.sections.aboutFailed')} <Button variant="plain" size="sm" onclick={loadAbout}>{t('health.sections.tryAgain')}</Button></p>
			{/if}
			<dl class="kv">
				<dt>{t('health.sections.version')}</dt>
				<dd>
					<code>{about?.version ?? upd?.version ?? (aboutFailed || aboutDenied ? '-' : '…')}</code>
					{#if upd?.newer}<span class="chip wait">{t('health.sections.updAvailableChip')}</span><Button variant="plain" size="sm" onclick={() => ontab('updates')}>{t('health.sections.seeUpdate')}</Button>{:else if upd?.on && upd.latest}<span class="chip ok">{t('updates.band.upToDate')}</span>{/if}
				</dd>
				{#if about}
					<dt>Go</dt><dd>{about.go.replace(/^go/, '')}</dd>
					<dt>{t('health.sections.system')}</dt><dd>{about.os}</dd>
					<dt>{t('health.sections.container')}</dt><dd>{about.docker ? t('health.sections.yes') : t('health.sections.no')}</dd>
					<dt>{t('health.sections.database')}</dt><dd>SQLite {about.sqlite}</dd>
					<dt>{t('health.sections.migration')}</dt><dd>{about.migration}</dd>
					<dt>{t('health.sections.configFolder')}</dt><dd><code>{about.config_dir}</code></dd>
					<dt>{t('health.sections.startFolder')}</dt><dd><code>{about.start_dir}</code></dd>
					<dt>{t('health.sections.uptime')}</dt><dd><time class="clock" datetime={about.started_at} title={t('health.sections.started', { when: dateTime(about.started_at) })}>{uptime(about.uptime_s + Math.max(0, now - aboutAt) / 1000)}</time></dd>
				{/if}
				{#if srv}
					<dt>{t('health.sections.writes')}</dt><dd>{t('health.sections.writesValue', { count: srv.writes, ms: srv.write_wait_p95_ms })}</dd>
					<dt>{t('health.sections.urlBase')}</dt><dd data-testid="url-base">{srv.url_base || t('health.sections.urlNone')}</dd>
					<dt>{t('health.sections.liveTabs')}</dt><dd>{srv.live_clients}</dd>
				{/if}
			</dl>
		</section>

		{#if session.can('users')}
			<section class="st-plate" aria-labelledby="s-help">
				<h2 id="s-help">{t('health.sections.help')}</h2>
				<p class="help-text">{t('health.sections.helpText')}</p>
				<AdminLinkButton href={to('/api/v1/support/bundle')} download data-testid="status-support-bundle">{t('health.sections.bundle')}</AdminLinkButton>
			</section>
		{/if}

		<section class="why-row" aria-labelledby="s-story">
			<div><h2 id="s-story">{t('health.why.title')}</h2><small>{t('health.sections.note')}</small></div>
			<button class="why-btn" type="button" aria-haspopup="dialog" onclick={() => why?.open()} data-testid="why-open"><span class="px" aria-hidden="true">&gt;_</span>{t('health.sections.readNote')}</button>
		</section>
		<WhyNote bind:this={why} />
	{/if}
	</div>
</div>

<style>
	.secs, .panel { display: flex; flex-direction: column; gap: var(--s-4); }
	ul { list-style: none; margin: 0; padding: 0; }
	/* Same pill track as the Downloads page. */
	.seg { display: inline-flex; align-self: flex-start; max-width: 100%; overflow-x: auto; scrollbar-width: none; gap: 2px; padding: 2px; border-radius: var(--r-pill); background: var(--surface-2); }
	.seg:focus { outline: none; }
	.seg button { position: relative; display: inline-flex; align-items: center; justify-content: center; height: 32px; padding: 0 16px; border: 0; border-radius: var(--r-pill); background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-subhead); white-space: nowrap; cursor: pointer; }
	.seg button:hover { color: var(--text); }
	.seg button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .seg button { height: 40px; } }
	/* Grows the tap area to 44px without changing the look. */
	@media (max-width: 767px) and (pointer: coarse) { .seg button::after { content: ''; position: absolute; inset-inline: 0; top: 50%; height: 44px; transform: translateY(-50%); } }
	.badge { position: absolute; top: 6px; right: 6px; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	.badge.wait { background: var(--wait); }
	.none { margin: 0; color: var(--text-muted); font-size: 14px; }
	.fail-line { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 8px; }
	.chip { vertical-align: middle; }

	.health { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; background: color-mix(in srgb, var(--fail) 4%, var(--surface-1)); box-shadow: inset 0 1px 2px rgb(0 0 0 / 0.06); }
	.hrow { display: grid; grid-template-columns: 22px minmax(0, 1fr) auto auto; align-items: center; gap: 12px; min-height: 48px; padding: 6px 14px; border-top: 1px solid var(--line); }
	.hrow:first-child { border-top: 0; }
	.hico { display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--fail); color: #fff; font: 700 12px/1 var(--font-ui); }
	.hico.wait { background: var(--wait); }
	.hmsg { min-width: 0; font-size: 14px; color: var(--text); }
	.hk { color: var(--text-muted); }
	.hr { display: block; font-size: 13px; color: var(--text-muted); overflow-wrap: anywhere; }
	.hat { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; }
	.hacts { display: flex; gap: 4px; }
	.ok-line { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; margin: 0; font-size: 14.5px; color: var(--text-muted); }
	.dload { max-width: 420px; }

	.dtop { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.dname { font-weight: 600; font-size: 14.5px; color: var(--text); }
	.dtop .dnum { margin-left: auto; }
	.disk.bad { padding: 10px 12px; margin: -10px -12px 0; border-radius: 10px; background: color-mix(in srgb, var(--fail) 6%, transparent); }
	.disk.bad.wait { background: color-mix(in srgb, var(--wait) 8%, transparent); }
	.dwhy { margin: 6px 0 0; font-size: 13px; color: var(--text-muted); overflow-wrap: anywhere; }
	.bk { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin: 18px 0 0; padding-top: 14px; border-top: 1px solid var(--line); font-size: 13.5px; color: var(--text-muted); }
	.disks { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 28px; }
	.dnum { font-size: 13px; color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.dnum b { color: var(--text); font-weight: 600; }
	.dbar { height: 8px; margin-top: 8px; border-radius: 4px; overflow: hidden; background: color-mix(in srgb, var(--text) 10%, transparent); box-shadow: inset 0 1px 2px rgb(0 0 0 / 0.15); }
	.dbar i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, color-mix(in srgb, var(--text) 40%, transparent), color-mix(in srgb, var(--text) 62%, transparent)); }
	.dbar.warn i { background: var(--wait); }
	.dbar.full i { background: var(--fail); }
	.dfoot { display: flex; justify-content: space-between; gap: 12px; margin-top: 6px; font-size: 12px; color: var(--text-muted); }
	.dfoot span:last-child { flex: none; }
	.dfoot .mono { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.tbl { width: 100%; border-collapse: collapse; font-size: 14px; }
	.tbl th { padding: 8px 12px; text-align: left; font-weight: 600; font-size: 13px; color: var(--text-muted); border-bottom: 1px solid var(--line); }
	.tbl td { padding: 10px 12px; border-bottom: 1px solid var(--line); color: var(--text); vertical-align: middle; }
	/* Tight on wide screens so the Jobs tab fits one screen. */
	@media (min-width: 768px) {
		.tight :global(.st-plate) { padding: 10px 18px 12px; }
		.tight :global(.st-plate > h2) { margin: 0 -18px 8px; padding: 0 18px 8px; font-size: 15px; }
		.tight :global(.tl li) { min-height: 28px; }
		.tight :global(.tl .nm) { font-size: 13.5px; }
		.tight :global(.tl .nowline) { min-height: 22px; }
	}
	.tbl.jobs { font-size: 13.5px; }
	.tbl.jobs th { padding: 2px 10px 6px; font-size: 12.5px; }
	.tbl.jobs td { padding: 0 10px; height: 34px; }
	.tbl.jobs tr:last-child td { border-bottom: 0; }
	.tbl .num { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.mono, code { font-family: var(--font-mono, ui-monospace, monospace); font-size: 12.5px; }
	code { padding: 3px 7px; border-radius: 6px; background: var(--surface-2, var(--surface-1)); }
	.jn { font-weight: 500; }
	.jr { display: block; font-weight: 400; font-size: 12.5px; color: var(--fail-text); }
	.sr, .k { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }

	.kv { display: grid; grid-template-columns: 160px minmax(0, 1fr); margin: 0; }
	.kv dt, .kv dd { margin: 0; padding: 11px 0; border-bottom: 1px solid var(--line); font-size: 14px; }
	.kv dt:last-of-type, .kv dd:last-of-type { border-bottom: 0; }
	.kv dt { font-weight: 600; color: var(--text-muted); }
	.kv dd { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; color: var(--text); min-width: 0; overflow-wrap: anywhere; }
	.muted { color: var(--text-muted); }
	.help-text { margin: 0 0 12px; max-width: 760px; font-size: 14px; line-height: 1.5; color: var(--text-muted); }

	@media (max-width: 900px) { .disks { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.seg { align-self: stretch; }
		.seg button { flex: 1 0 auto; padding: 0 10px; }
		.hrow { grid-template-columns: 22px minmax(0, 1fr); }
		.hat { grid-column: 2; }
		.hacts { grid-column: 2; margin-left: -12px; }
		.kv { grid-template-columns: 130px minmax(0, 1fr); }
		/* Stacks each job: name and state, then labelled times, button on the right. */
		.tbl.jobs thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
		.tbl.jobs tbody tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; column-gap: 8px; padding: 8px 0; border-bottom: 1px solid var(--line); }
		.tbl.jobs tbody tr:last-child { border-bottom: 0; }
		.tbl.jobs td { border: 0; padding: 2px 4px; height: auto; font-size: 13px; color: var(--text-muted); }
		.tbl.jobs td.jn { grid-column: 1; grid-row: 1; font-size: 14px; color: var(--text); }
		.tbl.jobs td.js { grid-column: 2; grid-row: 1; justify-self: end; }
		.tbl.jobs td.jl { grid-column: 1; grid-row: 2; }
		.tbl.jobs td.jx { grid-column: 1; grid-row: 3; }
		.tbl.jobs td.num { grid-column: 2; grid-row: 2 / span 2; justify-self: end; }
		.tbl.jobs .k { position: static; width: auto; height: auto; overflow: visible; clip: auto; color: var(--text-faint, var(--text-muted)); }
		.tbl.jobs .k::after { content: ' '; }
	}
	.clock { font-family: var(--font-mono, ui-monospace, monospace); font-variant-numeric: tabular-nums; }
	/* Colours match the website's dark and light themes. */
	.why-row { --coal: light-dark(#f4f1ea, #171717); --panel: light-dark(#fbfaf6, #242424); --cream: light-dark(#05080d, #f3f0e9); --muted: light-dark(#4a4f58, #b6bec3); --red: light-dark(#b4271e, #e2443a); --hard: light-dark(#05080d, #080a0c); --scan: light-dark(rgb(5 8 13 / 5%), rgb(0 0 0 / 20%)); --pixel: 'Silkscreen', var(--font-mono, ui-monospace), monospace;
		position: relative; overflow: hidden; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px; background: var(--coal); border: 2px solid var(--cream); box-shadow: 5px 5px 0 var(--hard); border-radius: 0; }
	.why-row::before { content: ''; position: absolute; inset: 0; pointer-events: none; background: repeating-linear-gradient(0deg, var(--scan) 0 1px, transparent 1px 3px); }
	.why-row > * { position: relative; }
	.why-row h2 { margin: 0; padding: 3px 0; font: 400 14px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; color: var(--cream); }
	.why-row small { display: block; margin-top: 6px; color: var(--muted); font-size: 13px; }
	.why-btn { all: unset; cursor: pointer; flex: none; display: inline-flex; align-items: center; gap: 10px; height: 40px; padding: 0 16px; background: var(--panel); color: var(--cream); border: 2px solid var(--cream); box-shadow: 4px 4px 0 var(--red); font: 400 12px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; transition: transform 120ms ease, box-shadow 120ms ease; }
	.why-btn .px { color: var(--red); letter-spacing: 0; }
	.why-btn:hover { transform: translate(-2px, -2px); box-shadow: 6px 6px 0 var(--red); }
	.why-btn:active { transform: translate(3px, 3px); box-shadow: 1px 1px 0 var(--red); }
	.why-btn:focus-visible { outline: 2px solid #ff9384; outline-offset: 3px; }
	@media (pointer: coarse) { .why-btn { height: 44px; } }
	@media (max-width: 520px) { .why-row { flex-direction: column; align-items: flex-start; } }
</style>
