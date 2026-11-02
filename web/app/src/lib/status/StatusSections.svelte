<script lang="ts">
	// The Status page body, laid out the way the *arr status pages are: titled
	// sections, in three tabs. Status: Health, Parts (two columns), Storage and
	// About side by side. Jobs: Coming up and Recent runs side by side, then
	// every job. Releases: the releases list, Documentation and Support.
	import { onMount } from 'svelte';
	import { api } from '../api.ts';
	import { ago, bytes, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import PartGroups from './PartGroups.svelte';
	import Releases from './Releases.svelte';
	import { needsAttention, type StatusGroup, type StatusRow } from './status.ts';

	let {
		groups,
		tab,
		ontab,
		canAct,
		busy,
		oncheck,
		labelOf
	}: {
		groups: StatusGroup[];
		tab: string;
		ontab: (t: string) => void;
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
	} = $props();

	type Disk = { path: string; kind: string; label: string; free_bytes: number; total_bytes: number };
	type Update = { on: boolean; version: string; latest?: string; newer: boolean; link?: string; notes?: string; checked_at?: string; problem?: string; no_release?: boolean };

	let disks = $state<Disk[] | null>(null);
	let upd = $state<Update | null>(null);

	const parts = $derived(groups.filter((g) => g.id !== 'jobs'));
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	const problems = $derived(needsAttention(groups.flatMap((g) => g.rows)));
	const upcoming = $derived(jobs.filter((j) => j.nextRun).sort((a, b) => Date.parse(a.nextRun!) - Date.parse(b.nextRun!)));
	const recent = $derived(jobs.filter((j) => j.lastRun).sort((a, b) => Date.parse(b.lastRun!) - Date.parse(a.lastRun!)));
	const linked = $derived(parts.reduce((n, g) => n + g.rows.length, 0));
	const kindWord: Record<string, string> = { movie: 'Movies', show: 'Shows', music: 'Music' };
	/** usedPct is how full a disk is, 0..100, or null when unknown. */
	const usedPct = (d: Disk) => (d.total_bytes > 0 && d.free_bytes >= 0 ? Math.round(((d.total_bytes - d.free_bytes) / d.total_bytes) * 100) : null);

	const icon: Record<string, string> = {
		sources: 'M4 6h12M4 10h12M4 14h8',
		downloaders: 'M10 3v10m0 0-4-4m4 4 4-4M4 16h12',
		library: 'M3 6l2-2h4l2 2h6v9H3z',
		media: 'M3 4h14v9H3zM7 16h6',
		subtitles: 'M3 5h14v10H3zM6 11h3m2 0h3',
		lists: 'M6 5h10M6 10h10M6 15h10M3 5h.01M3 10h.01M3 15h.01',
		alerts: 'M10 3a5 5 0 0 0-5 5v3l-2 3h14l-2-3V8a5 5 0 0 0-5-5zM8 16a2 2 0 0 0 4 0',
		metadata: 'M10 3c4 0 7 1.3 7 3s-3 3-7 3-7-1.3-7-3 3-3 7-3zM3 6v8c0 1.7 3 3 7 3s7-1.3 7-3V6'
	};

	onMount(() => {
		api.get<{ disks: Disk[] }>('/health/disks').then((d) => (disks = d.disks), () => (disks = []));
		api.get<Update>('/app/update').then((u) => (upd = u), () => {});
	});
</script>

<div class="secs" data-testid="status-sections">
	<div class="seg" role="tablist" aria-label="Status view">
		<button type="button" role="tab" aria-selected={tab === 'parts'} onclick={() => ontab('parts')}>Status</button>
		<button type="button" role="tab" aria-selected={tab === 'jobs'} onclick={() => ontab('jobs')}>
			Jobs{#if jobs.some((j) => j.tone === 'fail')}<i class="badge" aria-label="has failures"></i>{/if}
		</button>
		<button type="button" role="tab" aria-selected={tab === 'releases'} onclick={() => ontab('releases')}>Releases</button>
	</div>

	{#if tab === 'parts'}
		<section aria-labelledby="s-health">
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

		<section aria-labelledby="s-parts">
			<h2 id="s-parts">Parts</h2>
			<PartGroups {parts} {icon} {canAct} {busy} {oncheck} />
		</section>

		<div class="pair">
			<section aria-labelledby="s-disk">
				<h2 id="s-disk">Storage</h2>
				{#if disks === null}
					<p class="none">Loading…</p>
				{:else if !disks.length}
					<p class="none">No library folders yet.</p>
				{:else}
					<ul class="disks">
						{#each disks as d (d.path)}
							{@const p = usedPct(d)}
							<li class="disk">
								<div class="dtop">
									<span class="dname">{d.label || kindWord[d.kind] || 'Library'}</span>
									<span class="dnum">
										{#if d.free_bytes >= 0}<b>{bytes(d.free_bytes)}</b> free{/if}{#if d.total_bytes > 0} of {bytes(d.total_bytes)}{/if}
									</span>
								</div>
								<div class="dbar" class:warn={p !== null && p >= 85} class:full={p !== null && p >= 95} role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={p ?? undefined} aria-label="{d.label || d.path} used">
									<i style:width="{p ?? 0}%"></i>
								</div>
								<div class="dfoot"><span class="mono" title={d.path}>{d.path}</span>{#if p !== null}<span>{p}% used</span>{/if}</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>

			<section aria-labelledby="s-about">
				<h2 id="s-about">About</h2>
				<dl class="kv">
					<dt>Version</dt>
					<dd>
						<code>{upd?.version ?? '…'}</code>
						{#if upd?.newer}<span class="pill warn">Update available</span>{:else if upd?.on && upd.latest}<span class="pill good">Up to date</span>{/if}
					</dd>
					<dt>Update check</dt>
					<dd>{#if !upd}…{:else if !upd.on}Off{:else if upd.problem}{upd.problem}{:else if upd.checked_at}Checked {ago(upd.checked_at)}{:else}Not checked yet{/if}</dd>
					<dt>Library folders</dt><dd>{disks?.length ?? '…'}</dd>
					<dt>Linked parts</dt><dd>{linked}</dd>
					<dt>Background jobs</dt><dd>{jobs.length}</dd>
					<dt>Needs attention</dt><dd>{#if problems.length}<span class="bad">{problems.length}</span>{:else}None{/if}</dd>
				</dl>
			</section>
		</div>
	{:else if tab === 'jobs'}
		<div class="pair">
			<section aria-labelledby="s-next">
				<h2 id="s-next">Coming up</h2>
				<ol class="tl">
					{#each upcoming.slice(0, 5) as r (r.key)}
						<li><span class="when">{until(r.nextRun)}</span><i class="d {r.tone}" aria-hidden="true"></i><span class="what">{r.name}</span></li>
					{:else}<li class="none">No runs planned.</li>{/each}
				</ol>
			</section>
			<section aria-labelledby="s-last">
				<h2 id="s-last">Recent runs</h2>
				<ol class="tl">
					{#each recent.slice(0, 5) as r (r.key)}
						<li><span class="when"><time datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun)}</time></span><i class="d {r.tone}" aria-hidden="true"></i><span class="what">{r.name}</span><span class="res {r.tone}">{r.word}</span></li>
					{:else}<li class="none">Nothing has run yet.</li>{/each}
				</ol>
			</section>
		</div>

		<section aria-labelledby="s-jobs">
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
	{:else}
		<Releases {upd} />

		<div class="pair">
			<section aria-labelledby="s-docs">
				<h2 id="s-docs">Documentation</h2>
				<dl class="kv">
					<dt>Guides</dt><dd class="soon">Arrives with the public release</dd>
					<dt>Questions</dt><dd class="soon">Arrives with the public release</dd>
					<dt>Report a problem</dt><dd class="soon">Arrives with the public release</dd>
				</dl>
			</section>
			<section aria-labelledby="s-support">
				<h2 id="s-support">Support HolyArrs</h2>
				<dl class="kv">
					<dt>Donations</dt><dd class="soon">Arrives with the public release</dd>
					<dt>Spread the word</dt><dd class="soon">Arrives with the public release</dd>
				</dl>
			</section>
		</div>
	{/if}
</div>

<style>
	.secs { display: flex; flex-direction: column; gap: 22px; }
	ul, ol { list-style: none; margin: 0; padding: 0; }
	/* Every section is a raised plate: lit top edge, soft drop, hairline divider under the title */
	section {
		position: relative; min-width: 0; padding: 18px 20px 20px; border-radius: 20px; border: 1px solid var(--line);
		background: linear-gradient(180deg, color-mix(in srgb, var(--surface-1) 94%, #fff 6%), color-mix(in srgb, var(--surface-1) 88%, transparent));
		-webkit-backdrop-filter: blur(14px) saturate(1.2); backdrop-filter: blur(14px) saturate(1.2);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.09), 0 1px 2px rgb(0 0 0 / 0.05), 0 18px 40px -26px rgb(0 0 0 / 0.40);
	}
	h2 {
		display: flex; align-items: center; gap: 10px; margin: 0 -20px 16px; padding: 0 20px 14px;
		border-bottom: 1px solid var(--line); box-shadow: 0 1px 0 rgb(255 255 255 / 0.05);
		font: 600 18px/1.2 var(--font-ui); color: var(--text);
	}
	h2::before { content: ''; width: 4px; height: 16px; border-radius: 2px; background: linear-gradient(var(--accent, var(--text)), color-mix(in srgb, var(--accent, var(--text)) 30%, transparent)); }
	.pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; align-items: stretch; }
	.seg { display: inline-flex; align-self: flex-start; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.seg button { position: relative; min-height: 40px; padding: 0 22px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.seg button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
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
	.disks { display: flex; flex-direction: column; gap: 16px; }
	.dtop { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
	.dname { font-weight: 600; font-size: 14.5px; color: var(--text); }
	.dnum { font-size: 13px; color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.dnum b { color: var(--text); font-weight: 600; }
	.dbar { height: 8px; margin-top: 8px; border-radius: 4px; overflow: hidden; background: color-mix(in srgb, var(--text) 10%, transparent); box-shadow: inset 0 1px 2px rgb(0 0 0 / 0.15); }
	.dbar i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, color-mix(in srgb, var(--text) 40%, transparent), color-mix(in srgb, var(--text) 62%, transparent)); }
	.dbar.warn i { background: var(--wait); }
	.dbar.full i { background: var(--fail); }
	.dfoot { display: flex; justify-content: space-between; gap: 12px; margin-top: 6px; font-size: 12px; color: var(--text-muted); }
	.dfoot span:last-child { flex: none; }
	.dfoot .mono { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	/* Coming up / Recent runs */
	.tl { position: relative; }
	.tl::before { content: ''; position: absolute; left: 96px; top: 8px; bottom: 8px; width: 1px; background: var(--line); }
	.tl li { position: relative; display: grid; grid-template-columns: 84px 9px minmax(0, 1fr) auto; align-items: center; gap: 12px; min-height: 40px; font-size: 14px; }
	.tl .when { font-size: 12.5px; color: var(--text-muted); text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
	.tl .d { width: 9px; height: 9px; box-shadow: 0 0 0 3px var(--surface-1); }
	.tl .what { color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tl .res { font-size: 12.5px; color: var(--text-muted); }
	.tl .res.fail { color: var(--fail-text); font-weight: 600; }
	.tl .none { display: block; }

	/* Tables */
	.tbl { width: 100%; border-collapse: collapse; font-size: 14px; }
	.tbl th { padding: 8px 12px; text-align: left; font-weight: 600; font-size: 13px; color: var(--text-muted); border-bottom: 1px solid var(--line); }
	.tbl td { padding: 10px 12px; border-bottom: 1px solid var(--line); color: var(--text); vertical-align: middle; }
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
	.pill { padding: 3px 9px; border-radius: var(--r-pill); font-size: 12px; font-weight: 600; }
	.pill.good { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--text); }
	.pill.warn { background: color-mix(in srgb, var(--wait) 22%, transparent); color: var(--text); }

	@media (max-width: 900px) { .pair { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		section { padding: 14px 14px 16px; border-radius: 16px; }
		h2 { margin: 0 -14px 12px; padding: 0 14px 12px; }
		.seg { align-self: stretch; }
		.seg button { flex: 1; padding: 0 12px; }
		.hrow { grid-template-columns: 22px minmax(0, 1fr); }
		.hat { grid-column: 2; }
		.hacts { grid-column: 2; margin-left: -10px; }
		.kv { grid-template-columns: 130px minmax(0, 1fr); }
		.tl::before { left: 76px; }
		.tl li { grid-template-columns: 64px 9px minmax(0, 1fr) auto; }
		.tbl.jobs thead { display: none; }
		.tbl.jobs tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; padding: 8px 0; border-bottom: 1px solid var(--line); }
		.tbl.jobs td { border: 0; padding: 2px 4px; font-size: 13px; color: var(--text-muted); }
		.tbl.jobs td.jn { font-size: 14px; color: var(--text); }
		.tbl.jobs td.num { grid-column: 2; grid-row: 1 / span 2; align-self: center; }
	}
</style>
