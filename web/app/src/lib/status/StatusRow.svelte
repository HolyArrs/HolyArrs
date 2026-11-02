<script lang="ts">
	// One Status row: a dot and its word, the name, a plain reason and when it
	// was last checked. Jobs also show last run and next run. Fix and Check now
	// appear only when asked for and when the row has them.
	import { ago, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import type { StatusRow } from './status.ts';

	let {
		row,
		actions = false,
		kind = '',
		busy = false,
		oncheck
	}: { row: StatusRow; actions?: boolean; kind?: string; busy?: boolean; oncheck?: (r: StatusRow) => void } = $props();
	const isJob = $derived(row.group === 'jobs');
</script>

<li class="row" data-testid="status-row" data-tone={row.tone} data-key={row.key}>
	<span class="st {row.tone}"><i aria-hidden="true"></i>{row.word}</span>
	<span class="main">
		<span class="name">{row.name}{#if kind}<span class="kind"> · {kind}</span>{/if}</span>
		{#if isJob}
			<span class="sub">
				Last run {#if row.lastRun}<time datetime={row.lastRun} title={dateTime(row.lastRun)}>{ago(row.lastRun)}</time>{:else}never{/if}
				· Next {until(row.nextRun)}
			</span>
		{/if}
		{#if row.reason}<span class="sub reason">{row.reason}</span>{/if}
	</span>
	{#if !isJob && row.at}<time class="at" datetime={row.at} title={dateTime(row.at)}>{ago(row.at)}</time>{/if}
	{#if actions && row.check && oncheck || actions && row.fix && (row.tone === 'fail' || row.tone === 'wait')}
		<span class="acts">
			{#if row.check && oncheck}
				<button type="button" class="btn" disabled={busy} aria-busy={busy} onclick={() => oncheck(row)}>{isJob ? 'Run now' : 'Check now'}</button>
			{/if}
			{#if row.fix && (row.tone === 'fail' || row.tone === 'wait')}<a class="btn" href={to(row.fix)}>Fix</a>{/if}
		</span>
	{/if}
</li>

<style>
	.row {
		display: grid; grid-template-columns: 120px minmax(0, 1fr) auto auto; align-items: center; gap: 4px 14px;
		min-height: 56px; padding: 8px 16px; border-top: 1px solid var(--line); box-sizing: border-box;
	}
	.row:first-child { border-top: 0; }
	.st { display: inline-flex; align-items: center; gap: 7px; font-weight: 500; font-size: 13.5px; color: var(--text); white-space: nowrap; }
	.st i { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.st.ok i { background: var(--ok); }
	.st.wait i { background: var(--wait); }
	.st.fail { color: var(--fail-text); }
	.st.fail i { background: var(--fail); }
	.st.idle { color: var(--text-muted); }
	.st.idle i { background: var(--text-faint); }
	.main { display: flex; flex-direction: column; min-width: 0; gap: 2px; }
	.name { font-weight: 500; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.kind { color: var(--text-muted); font-weight: 400; }
	.sub { font-size: 13px; color: var(--text-muted); }
	.reason { overflow-wrap: anywhere; }
	.at { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.acts { display: flex; gap: 8px; }
	.btn {
		display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 44px; padding: 0 14px;
		border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); color: var(--text);
		font: 500 13.5px/1 var(--font-ui); text-decoration: none; cursor: pointer; box-sizing: border-box;
	}
	.btn:hover { border-color: var(--line-strong); }
	.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled { opacity: 0.6; cursor: default; }
	@media (max-width: 767px) {
		.row { grid-template-columns: minmax(0, 1fr) auto; padding: 10px 14px; }
		.st { grid-column: 2; grid-row: 1; }
		.main { grid-column: 1; grid-row: 1 / span 2; }
		.at { grid-column: 2; grid-row: 2; justify-self: end; }
		.acts { grid-column: 1 / -1; }
	}
</style>
