<script lang="ts">
	// The Board look: a strip of group tiles across the top, problems as wide
	// cards, then every group as its own card in a grid beside a jobs column.
	// Uses the full width on desktop and stacks to one column on phones.
	import { ago, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import { groupCounts, type StatusGroup, type StatusRow } from './status.ts';

	let {
		groups,
		problems,
		canAct,
		busy,
		oncheck,
		labelOf
	}: {
		groups: StatusGroup[];
		problems: StatusRow[];
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
	} = $props();

	const parts = $derived(groups.filter((g) => g.id !== 'jobs'));
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	// Jobs by next run, soonest first; ones with no next run go last.
	const upcoming = $derived(
		[...jobs].sort((a, b) => (a.nextRun ? Date.parse(a.nextRun) : Infinity) - (b.nextRun ? Date.parse(b.nextRun) : Infinity))
	);
	const jobCounts = $derived(groupCounts({ id: 'jobs', label: 'Jobs', rows: jobs }));
	const toneOf = (c: { bad: number; ok: number }) => (c.bad ? 'fail' : c.ok ? 'ok' : 'idle');
</script>

{#snippet act(r: StatusRow)}
	{#if canAct && r.check}
		<button type="button" class="btn" disabled={busy === r.key} aria-busy={busy === r.key} onclick={() => oncheck(r)}>{r.group === 'jobs' ? 'Run now' : 'Check now'}</button>
	{/if}
	{#if canAct && r.fix && (r.tone === 'fail' || r.tone === 'wait')}<a class="btn" href={to(r.fix)}>Fix</a>{/if}
{/snippet}

<div class="board" data-testid="status-board">
	<ul class="tiles" aria-label="Summary">
		{#each parts as g (g.id)}
			{@const c = groupCounts(g)}
			<li>
				<a class="tile {toneOf(c)}" href="#grp-{g.id}">
					<span class="tl">{g.label}</span>
					<span class="tn"><b>{c.ok}</b>/{g.rows.length}</span>
					<span class="meter" aria-hidden="true">
						<i class="ok" style:flex-grow={c.ok}></i><i class="fail" style:flex-grow={c.bad}></i><i class="idle" style:flex-grow={c.off}></i>
					</span>
					<span class="ts">{c.bad ? `${c.bad} need${c.bad === 1 ? 's' : ''} attention` : c.off ? `${c.off} off` : 'All working'}</span>
				</a>
			</li>
		{/each}
		{#if jobs.length}
			<li>
				<a class="tile {toneOf(jobCounts)}" href="#grp-jobs">
					<span class="tl">Jobs</span>
					<span class="tn"><b>{jobCounts.ok}</b>/{jobs.length}</span>
					<span class="meter" aria-hidden="true">
						<i class="ok" style:flex-grow={jobCounts.ok}></i><i class="fail" style:flex-grow={jobCounts.bad}></i><i class="idle" style:flex-grow={jobCounts.off}></i>
					</span>
					<span class="ts">{jobCounts.bad ? `${jobCounts.bad} need${jobCounts.bad === 1 ? 's' : ''} attention` : 'All on time'}</span>
				</a>
			</li>
		{/if}
	</ul>

	{#if problems.length}
		<section class="attn" aria-labelledby="b-attn" data-testid="status-attention">
			<h2 id="b-attn">Needs attention · {problems.length}</h2>
			<ul class="pcards">
				{#each problems as r (r.key)}
					<li class="pcard {r.tone}" data-testid="status-row" data-tone={r.tone} data-key={r.key}>
						<div class="ph">
							<span class="st {r.tone}"><i aria-hidden="true"></i>{r.word}</span>
							<span class="kind">{labelOf(r)}</span>
						</div>
						<p class="pn">{r.name}</p>
						{#if r.reason}<p class="pr">{r.reason}</p>{/if}
						<div class="pf">
							{#if r.at}<time datetime={r.at} title={dateTime(r.at)}>{ago(r.at)}</time>{:else}<span></span>{/if}
							<span class="acts">{@render act(r)}</span>
						</div>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<div class="cols">
		<div class="grid">
			{#each parts as g (g.id)}
				<section class="card" id="grp-{g.id}" aria-labelledby="h-{g.id}">
					<h2 id="h-{g.id}">{g.label}<span class="cnt">{g.rows.length}</span></h2>
					<ul class="items">
						{#each g.rows as r (r.key)}
							<li class="item" data-testid="status-row" data-tone={r.tone} data-key={r.key}>
								<span class="dot {r.tone}" aria-hidden="true"></span>
								<span class="im">
									<span class="in">{r.name}</span>
									<span class="iw">
										<span class="w {r.tone}">{r.word}</span>{#if r.at} · <time datetime={r.at} title={dateTime(r.at)}>{ago(r.at)}</time>{/if}
									</span>
								</span>
								{#if canAct && r.check}
									<button type="button" class="ibtn" disabled={busy === r.key} aria-busy={busy === r.key} aria-label="Check {r.name} now" title="Check now" onclick={() => oncheck(r)}>
										<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M16 10a6 6 0 1 1-1.8-4.3M16 3.5v3.2h-3.2" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" /></svg>
									</button>
								{/if}
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		</div>

		{#if jobs.length}
			<section class="card jobs" id="grp-jobs" aria-labelledby="h-jobs">
				<h2 id="h-jobs">Jobs<span class="cnt">{jobs.length}</span></h2>
				<ul class="items">
					{#each upcoming as r (r.key)}
						<li class="job" data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<span class="dot {r.tone}" aria-hidden="true"></span>
							<span class="im">
								<span class="in">{r.name}</span>
								<span class="iw">
									{#if r.tone === 'ok' || r.tone === 'idle'}
										Last {#if r.lastRun}<time datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun)}</time>{:else}never{/if}
									{:else}<span class="w {r.tone}">{r.word}</span>{/if}
								</span>
							</span>
							<span class="next" title="Next run">{until(r.nextRun)}</span>
							{#if canAct && r.check}
								<button type="button" class="ibtn" disabled={busy === r.key} aria-busy={busy === r.key} aria-label="Run {r.name} now" title="Run now" onclick={() => oncheck(r)}>
									<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7 5.5v9l7-4.5z" fill="currentColor" /></svg>
								</button>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	</div>
</div>

<style>
	.board { display: flex; flex-direction: column; gap: 20px; }
	ul { list-style: none; margin: 0; padding: 0; }
	h2 {
		display: flex; align-items: center; gap: 8px; margin: 0; padding: 14px 16px 8px;
		font: 600 13px/1.2 var(--font-ui); letter-spacing: 0.02em; text-transform: uppercase; color: var(--text-muted);
	}
	.cnt { padding: 2px 7px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; letter-spacing: 0; color: var(--text-muted); }

	/* Summary tiles */
	.tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 12px; }
	.tile {
		display: grid; gap: 6px; padding: 14px 16px; border: 1px solid var(--line); border-radius: var(--r-card, 16px);
		background: var(--surface-1); color: var(--text); text-decoration: none; transition: border-color 160ms var(--ease-out);
	}
	.tile:hover { border-color: var(--line-strong); }
	.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile.fail { border-color: color-mix(in srgb, var(--fail) 45%, var(--line)); }
	.tl { font-size: 13px; font-weight: 500; color: var(--text-muted); }
	.tn { font: 500 15px/1 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.tn b { font-size: 28px; font-weight: 600; color: var(--text); }
	.ts { font-size: 12.5px; color: var(--text-muted); }
	.tile.fail .ts { color: var(--fail-text); }
	.meter { display: flex; gap: 2px; height: 5px; border-radius: 3px; overflow: hidden; background: var(--surface-2, var(--line)); }
	.meter i { flex: 0 1 0; min-width: 0; }
	.meter .ok { background: var(--ok); }
	.meter .fail { background: var(--fail); }
	.meter .idle { background: var(--text-faint); }

	/* Problems */
	.attn h2 { padding: 0 2px 10px; color: var(--fail-text); }
	.pcards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr)); gap: 12px; }
	.pcard {
		display: flex; flex-direction: column; gap: 6px; padding: 14px 16px; border-radius: var(--r-card, 16px);
		border: 1px solid color-mix(in srgb, var(--fail) 40%, var(--line));
		background: color-mix(in srgb, var(--fail) 6%, var(--surface-1));
	}
	.pcard.wait { border-color: color-mix(in srgb, var(--wait) 40%, var(--line)); background: color-mix(in srgb, var(--wait) 6%, var(--surface-1)); }
	.ph { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.kind { font-size: 12.5px; color: var(--text-muted); }
	.pn { margin: 0; font-weight: 600; font-size: 16px; color: var(--text); }
	.pr { margin: 0; font-size: 13.5px; color: var(--text-muted); overflow-wrap: anywhere; }
	.pf { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: auto; padding-top: 4px; }
	.pf time { font-size: 12.5px; color: var(--text-muted); }
	.acts { display: flex; gap: 8px; }
	.st { display: inline-flex; align-items: center; gap: 7px; font-weight: 600; font-size: 13px; }
	.st i { width: 8px; height: 8px; border-radius: 50%; background: var(--fail); }
	.st.fail { color: var(--fail-text); }
	.st.wait i { background: var(--wait); }

	/* Groups + jobs */
	.cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(300px, 380px); gap: 16px; align-items: start; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(280px, 100%), 1fr)); gap: 16px; align-items: start; }
	.card { border: 1px solid var(--line); border-radius: var(--r-card, 16px); background: var(--surface-1); overflow: hidden; scroll-margin-top: calc(var(--topbar, 0px) + 90px); }
	.jobs { position: sticky; top: calc(var(--topbar, 0px) + 90px); max-height: calc(100vh - var(--topbar, 0px) - 110px); overflow-y: auto; }
	.item, .job {
		display: grid; grid-template-columns: 10px minmax(0, 1fr) auto; align-items: center; gap: 12px;
		min-height: 52px; padding: 6px 10px 6px 16px; border-top: 1px solid var(--line);
	}
	.job { grid-template-columns: 10px minmax(0, 1fr) auto auto; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.dot.ok { background: var(--ok); }
	.dot.wait { background: var(--wait); }
	.dot.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 20%, transparent); }
	.dot.idle { background: var(--text-faint); }
	.im { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.in { font-weight: 500; font-size: 14.5px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.iw { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.w.fail { color: var(--fail-text); font-weight: 500; }
	.w.wait { color: var(--wait-text, var(--text)); font-weight: 500; }
	.next { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.btn, .ibtn {
		display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 44px; box-sizing: border-box;
		border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); color: var(--text);
		font: 500 13.5px/1 var(--font-ui); text-decoration: none; cursor: pointer;
	}
	.btn { padding: 0 14px; }
	.ibtn { width: 44px; padding: 0; border-color: transparent; background: none; color: var(--text-muted); }
	.ibtn svg { width: 18px; height: 18px; }
	.btn:hover, .ibtn:hover { border-color: var(--line-strong); color: var(--text); }
	.btn:focus-visible, .ibtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled, .ibtn:disabled { opacity: 0.5; cursor: default; }

	@media (max-width: 1100px) {
		.cols { grid-template-columns: minmax(0, 1fr); }
		.jobs { position: static; max-height: none; }
	}
	@media (max-width: 767px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
		.tile { padding: 12px; }
		.tn b { font-size: 24px; }
		.pcards, .grid { grid-template-columns: minmax(0, 1fr); }
	}
</style>
