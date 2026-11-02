<script lang="ts">
	// The Status page body, laid out the way the *arr status pages are: frosted
	// plates in four tabs. Status: Health, the parts (PartsRail), Storage.
	// Jobs: the timeline, then every job. Updates: where this HolyArrs stands,
	// how to update, bundled tools, every release. About: this HolyArrs, its
	// story, Documentation and Donations.
	import './plate.css';
	import { onMount } from 'svelte';
	import { api } from '../api.ts';
	import { ago, bytes, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import PartsRail from './PartsRail.svelte';
	import RunsTimeline from './RunsTimeline.svelte';
	import Updates from './Updates.svelte';
	import type { HealthCheck } from '../types.ts';
	import { everyKind, needsAttention, type StatusGroup, type StatusRow } from './status.ts';

	let {
		groups,
		tab,
		ontab,
		canAct,
		busy,
		oncheck,
		labelOf,
		ytdlp
	}: {
		groups: StatusGroup[];
		tab: string;
		ontab: (t: string) => void;
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
		/** The trailers health check, which names yt-dlp's version (Updates tab). */
		ytdlp: HealthCheck | undefined;
	} = $props();

	type Disk = { path: string; kind: string; label: string; free_bytes: number; total_bytes: number };
	type Update = { on: boolean; version: string; latest?: string; newer: boolean; link?: string; notes?: string; checked_at?: string; problem?: string; no_release?: boolean };

	let disks = $state<Disk[] | null>(null);
	let upd = $state<Update | null>(null);
	type About = { version: string; go: string; os: string; docker: boolean; sqlite: string; migration: number; config_dir: string; start_dir: string; uptime_s: number; started_at: string };
	let about = $state<About | null>(null);

	const parts = $derived(groups.filter((g) => g.id !== 'jobs'));
	// Library rows are folder and disk checks: they live in Storage, beside each disk.
	const kinds = $derived(everyKind(parts));
	const libRows = $derived(parts.find((g) => g.id === 'library')?.rows ?? []);
	const folderRows = (d: Disk) => libRows.filter((r) => r.name.endsWith(d.path));
	const backups = $derived(libRows.find((r) => r.key.startsWith('backups')));
	const worst = (rs: StatusRow[]) => rs.find((r) => r.tone === 'fail') ?? rs.find((r) => r.tone === 'wait');
	/** uptime says how long HolyArrs has run: "3 d 4 h", "5 h 12 min", "8 min". */
	function uptime(sec: number): string {
		const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
		return d ? `${d} d ${h} h` : h ? `${h} h ${m} min` : `${m} min`;
	}
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	const problems = $derived(needsAttention(groups.flatMap((g) => g.rows)));
	const upcoming = $derived(jobs.filter((j) => j.nextRun).sort((a, b) => Date.parse(a.nextRun!) - Date.parse(b.nextRun!)));
	const recent = $derived(jobs.filter((j) => j.lastRun).sort((a, b) => Date.parse(b.lastRun!) - Date.parse(a.lastRun!)));
	const linked = $derived(parts.reduce((n, g) => n + g.rows.length, 0));
	const kindWord: Record<string, string> = { movie: 'Movies', show: 'Shows', music: 'Music' };
	/** usedPct is how full a disk is, 0..100, or null when unknown. */
	const usedPct = (d: Disk) => (d.total_bytes > 0 && d.free_bytes >= 0 ? Math.round(((d.total_bytes - d.free_bytes) / d.total_bytes) * 100) : null);


	onMount(() => {
		api.get<{ disks: Disk[] }>('/health/disks').then((d) => (disks = d.disks), () => (disks = []));
		api.get<Update>('/app/update').then((u) => (upd = u), () => {});
		api.get<About>('/app/about').then((a) => (about = a), () => {});
	});
</script>

<div class="secs" data-testid="status-sections">
	<div class="seg" role="tablist" aria-label="Status view">
		<button type="button" role="tab" aria-selected={tab === 'parts'} onclick={() => ontab('parts')}>Status</button>
		<button type="button" role="tab" aria-selected={tab === 'jobs'} onclick={() => ontab('jobs')}>
			Jobs{#if jobs.some((j) => j.tone === 'fail')}<i class="badge" aria-label="has failures"></i>{/if}
		</button>
		<button type="button" role="tab" aria-selected={tab === 'updates'} onclick={() => ontab('updates')}>
			Updates{#if upd?.newer}<i class="badge new" aria-label="update available"></i>{/if}
		</button>
		<button type="button" role="tab" aria-selected={tab === 'about'} onclick={() => ontab('about')}>About</button>
	</div>

	{#if tab === 'parts'}
		<section class="st-plate" aria-labelledby="s-health">
			<h2 id="s-health">Health</h2>
			{#if problems.length}
				<ul class="health" data-testid="status-attention">
					{#each problems as r (r.key)}
						<li class="hrow" data-tone={r.tone} data-key={r.key}>
							<span class="hico {r.tone}" aria-hidden="true">!</span>
							<span class="hmsg">
								<b>{r.name}</b> <span class="hk">· {labelOf(r)} · {r.word}</span>
								{#if r.reason}<span class="hr">{r.reason}</span>{/if}
							</span>
							{#if r.at}<time class="hat" datetime={r.at} title={dateTime(r.at)}>{ago(r.at)}</time>{/if}
							{#if canAct}
								<span class="hacts">
									{#if r.check}<button type="button" class="lnk" disabled={busy === r.key} onclick={() => oncheck(r)}>{r.group === 'jobs' ? 'Run now' : 'Check now'}</button>{/if}
									{#if r.fix}<a class="lnk" href={to(r.fix)}>Fix</a>{/if}
								</span>
							{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="ok-line"><span class="okdot" aria-hidden="true"></span>No problems. Every part and job is working.</p>
			{/if}
		</section>

		<PartsRail {kinds} {canAct} {busy} {oncheck} />

		<section class="st-plate" aria-labelledby="s-disk">
			<h2 id="s-disk">Storage</h2>
			{#if disks === null}
				<p class="none">Loading…</p>
			{:else if !disks.length}
				<p class="none">No library folders yet.</p>
			{:else}
				<ul class="disks">
					{#each disks as d (d.path)}
						{@const p = usedPct(d)}
						{@const w = worst(folderRows(d))}
						<li class="disk" class:bad={!!w}>
							<div class="dtop">
								<span class="dname">{d.label || kindWord[d.kind] || 'Library'}</span>
								{#if w}<span class="dstate {w.tone}">{w.word}</span>{:else}<span class="dstate ok">Working</span>{/if}
								<span class="dnum">
									{#if d.free_bytes >= 0}<b>{bytes(d.free_bytes)}</b> free{/if}{#if d.total_bytes > 0}&nbsp;of {bytes(d.total_bytes)}{/if}
								</span>
							</div>
							<div class="dbar" class:warn={p !== null && p >= 85} class:full={p !== null && p >= 95} role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={p ?? undefined} aria-label="{d.label || d.path} used">
								<i style:width="{p ?? 0}%"></i>
							</div>
							<div class="dfoot"><span class="mono" title={d.path}>{d.path}</span>{#if p !== null}<span>{p}% used</span>{/if}</div>
							{#if w?.reason}<p class="dwhy">{w.reason}{#if canAct && w.fix} <a class="lnk" href={to(w.fix)}>Fix</a>{/if}</p>{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if backups}
				<p class="bk"><i class="d {backups.tone}" aria-hidden="true"></i>Backups · <b>{backups.word}</b>{#if backups.at} · last checked <time datetime={backups.at} title={dateTime(backups.at)}>{ago(backups.at)}</time>{/if}{#if backups.reason && backups.tone !== 'ok'} · {backups.reason}{/if}</p>
			{/if}
		</section>
	{:else if tab === 'jobs'}
		<div class="jobs-grid">
		<RunsTimeline {upcoming} {recent} now={Date.now()} />

		<section class="st-plate" aria-labelledby="s-jobs">
			<h2 id="s-jobs">All jobs</h2>
			<table class="tbl jobs">
				<thead><tr><th>Name</th><th>State</th><th>Last run</th><th>Next run</th>{#if canAct}<th><span class="sr">Actions</span></th>{/if}</tr></thead>
				<tbody>
					{#each jobs as r (r.key)}
						<tr data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<td class="jn">{r.name}{#if r.tone === 'fail' && r.reason}<span class="jr">{r.reason}</span>{/if}</td>
							<td><span class="st {r.tone}"><i class="d {r.tone}" aria-hidden="true"></i>{r.word}</span></td>
							<td>{#if r.lastRun}<time datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun)}</time>{:else}Never{/if}</td>
							<td>{until(r.nextRun)}</td>
							{#if canAct}<td class="num"><button type="button" class="lnk" disabled={busy === r.key} onclick={() => oncheck(r)}>Run now</button></td>{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</section>
		</div>
	{:else if tab === 'updates'}
		<Updates {upd} docker={about ? about.docker : null} {ytdlp} {canAct} onupd={(u) => (upd = u)} />
	{:else}
		<section class="st-plate" aria-labelledby="s-about">
			<h2 id="s-about">About</h2>
			<dl class="kv">
				<dt>Version</dt>
				<dd>
					<code>{about?.version ?? upd?.version ?? '…'}</code>
					{#if upd?.newer}<button type="button" class="pill warn" onclick={() => ontab('updates')}>Update available</button>{:else if upd?.on && upd.latest}<span class="pill good">Up to date</span>{/if}
				</dd>
				{#if about}
					<dt>Go</dt><dd>{about.go.replace(/^go/, '')}</dd>
					<dt>System</dt><dd>{about.os}</dd>
					<dt>Docker</dt><dd>{about.docker ? 'Yes' : 'No'}</dd>
					<dt>Database</dt><dd>SQLite {about.sqlite}</dd>
					<dt>Database migration</dt><dd>{about.migration}</dd>
					<dt>Config folder</dt><dd><code>{about.config_dir}</code></dd>
					<dt>Start folder</dt><dd><code>{about.start_dir}</code></dd>
					<dt>Uptime</dt><dd>{uptime(about.uptime_s)} <span class="muted">· since {dateTime(about.started_at)}</span></dd>
				{/if}
			</dl>
		</section>

		<section class="st-plate" aria-labelledby="s-story">
			<h2 id="s-story">Why HolyArrs</h2>
			<div class="story">
				<p>Running a media server meant running six apps next to your download client: one each for shows, movies, music, subtitles, indexers and requests. Each had its own settings, its own logins and its own idea of what was going on.</p>
				<p>HolyArrs puts the whole trip in one place, from someone asking for a title to it showing up on the media server.</p>
				<p class="draft">Draft. Luis writes the final version.</p>
			</div>
		</section>

		<div class="pair">
			<section class="st-plate" aria-labelledby="s-docs">
				<h2 id="s-docs">Documentation</h2>
				<dl class="kv">
					<dt>Guides</dt><dd class="soon">Arrives with the public release</dd>
					<dt>Questions</dt><dd class="soon">Arrives with the public release</dd>
					<dt>Report a problem</dt><dd class="soon">Arrives with the public release</dd>
				</dl>
			</section>
			<section class="st-plate give" aria-labelledby="s-support">
				<h2 id="s-support">Donations</h2>
				<p class="gl">HolyArrs is free. If it saves you time, you can help keep it going.</p>
				<dl class="kv">
					<dt>Sponsor</dt><dd class="soon">Arrives with the public release</dd>
					<dt>One-time</dt><dd class="soon">Arrives with the public release</dd>
				</dl>
			</section>
		</div>
	{/if}
</div>

<style>
	.secs { display: flex; flex-direction: column; gap: 22px; }
	ul { list-style: none; margin: 0; padding: 0; }
	.pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; align-items: stretch; }
	.seg { display: inline-flex; align-self: flex-start; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.seg button { position: relative; min-height: 40px; padding: 0 22px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.seg button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.badge.new { background: var(--wait); }
	.badge { position: absolute; top: 8px; right: 10px; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	.none { margin: 0; color: var(--text-muted); font-size: 14px; }

	/* Health */
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
	.lnk {
		display: inline-flex; align-items: center; min-height: 36px; padding: 0 10px; border: 0; border-radius: 8px; background: none;
		color: var(--accent, var(--text)); font: 500 13.5px/1 var(--font-ui); text-decoration: none; cursor: pointer; white-space: nowrap;
	}
	.lnk:hover { background: var(--surface-2, var(--surface-1)); }
	.lnk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.lnk:disabled { opacity: 0.5; }
	.ok-line { display: flex; align-items: center; gap: 10px; margin: 0; font-size: 14.5px; color: var(--text-muted); }
	.okdot { width: 9px; height: 9px; border-radius: 50%; background: var(--ok); }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }

	/* Storage */
	.dtop { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
	.dname { font-weight: 600; font-size: 14.5px; color: var(--text); }
	.dtop .dnum { margin-left: auto; }
	.dstate { font-size: 12px; font-weight: 600; color: var(--text-muted); }
	.dstate.ok { color: var(--text-muted); font-weight: 500; }
	.dstate.fail { color: var(--fail-text); }
	.disk.bad { padding: 10px 12px; margin: -10px -12px 0; border-radius: 10px; background: color-mix(in srgb, var(--fail) 6%, transparent); }
	.dwhy { margin: 6px 0 0; font-size: 13px; color: var(--text-muted); overflow-wrap: anywhere; }
	.bk { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin: 18px 0 0; padding-top: 14px; border-top: 1px solid var(--line); font-size: 13.5px; color: var(--text-muted); }
	.bk b { color: var(--text); font-weight: 600; }
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

	/* Tables */
	.tbl { width: 100%; border-collapse: collapse; font-size: 14px; }
	.tbl th { padding: 8px 12px; text-align: left; font-weight: 600; font-size: 13px; color: var(--text-muted); border-bottom: 1px solid var(--line); }
	.tbl td { padding: 10px 12px; border-bottom: 1px solid var(--line); color: var(--text); vertical-align: middle; }
	/* Jobs: the timeline beside every job, so the tab fits a screen; rows kept tight */
	.jobs-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr); gap: 22px; align-items: start; }
	.tbl.jobs th { padding: 6px 10px 8px; }
	.tbl.jobs td { padding: 0 10px; height: 40px; font-size: 13.5px; }
	.tbl.jobs tr:last-child td { border-bottom: 0; }
	.tbl.jobs .lnk { min-height: 32px; }
	.tbl .num { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.mono, code { font-family: var(--font-mono, ui-monospace, monospace); font-size: 12.5px; }
	code { padding: 3px 7px; border-radius: 6px; background: var(--surface-2, var(--surface-1)); }
	.jn { font-weight: 500; }
	.jr { display: block; font-weight: 400; font-size: 12.5px; color: var(--fail-text); }
	.st { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
	.st.fail { color: var(--fail-text); font-weight: 500; }
	.st.idle { color: var(--text-muted); }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }

	/* About, Documentation, Support */
	.kv { display: grid; grid-template-columns: 160px minmax(0, 1fr); margin: 0; }
	.kv dt, .kv dd { margin: 0; padding: 11px 0; border-bottom: 1px solid var(--line); font-size: 14px; }
	.kv dt:last-of-type, .kv dd:last-of-type { border-bottom: 0; }
	.kv dt { font-weight: 600; color: var(--text-muted); }
	.kv dd { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; color: var(--text); min-width: 0; overflow-wrap: anywhere; }
	.kv .bad { color: var(--fail-text); font-weight: 600; }
	.soon { color: var(--text-muted); font-style: italic; }
	.muted { color: var(--text-muted); }
	.story { max-width: 760px; }
	.story p { margin: 0 0 12px; font-size: 15px; line-height: 1.65; color: var(--text); }
	.story .draft { margin: 4px 0 0; font-size: 12.5px; font-style: italic; color: var(--text-muted); }
	.give { background: linear-gradient(160deg, color-mix(in srgb, var(--accent, var(--text)) 7%, var(--surface-1)), var(--surface-1) 60%); }
	.gl { margin: 0 0 8px; font-size: 14.5px; color: var(--text); }
	button.pill { border: 0; cursor: pointer; font-family: inherit; }
	.pill { padding: 3px 9px; border-radius: var(--r-pill); font-size: 12px; font-weight: 600; }
	.pill.good { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--text); }
	.pill.warn { background: color-mix(in srgb, var(--wait) 22%, transparent); color: var(--text); }

	@media (max-width: 1180px) { .jobs-grid { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 900px) { .pair, .disks { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.seg { align-self: stretch; }
		.seg button { flex: 1; padding: 0 12px; }
		.hrow { grid-template-columns: 22px minmax(0, 1fr); }
		.hat { grid-column: 2; }
		.hacts { grid-column: 2; margin-left: -10px; }
		.kv { grid-template-columns: 130px minmax(0, 1fr); }
		.tbl.jobs thead { display: none; }
		.tbl.jobs tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; padding: 8px 0; border-bottom: 1px solid var(--line); }
		.tbl.jobs td { border: 0; padding: 2px 4px; height: auto; font-size: 13px; color: var(--text-muted); }
		.tbl.jobs td.jn { font-size: 14px; color: var(--text); }
		.tbl.jobs td.num { grid-column: 2; grid-row: 1 / span 2; align-self: center; }
	}
</style>
