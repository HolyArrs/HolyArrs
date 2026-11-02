<script lang="ts">
	// One group's parts as ruled rows inside its own section: a dot, the name
	// and its word, and the reason plus Check now / Fix right under any part
	// that is not working. Problems come first; long lists show six rows
	// until "Show all".
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import type { StatusRow } from './status.ts';

	let { rows, canAct, busy, oncheck }: { rows: StatusRow[]; canAct: boolean; busy: string | null; oncheck: (r: StatusRow) => void } = $props();

	const MAX = 6;
	let all = $state(false);
	const rank = (r: StatusRow) => (r.tone === 'fail' ? 0 : r.tone === 'wait' ? 1 : 2);
	const ordered = $derived([...rows].sort((a, b) => rank(a) - rank(b)));
</script>

<ul class="rows">
	{#each all ? ordered : ordered.slice(0, MAX) as r (r.key)}
		{@const bad = r.tone === 'fail' || r.tone === 'wait'}
		<li class="row" class:bad data-testid="status-row" data-tone={r.tone} data-key={r.key}>
			<div class="line">
				<i class="d {r.tone}" aria-hidden="true"></i>
				<span class="nm" title={r.name}>{r.name}</span>
				{#if r.at && !bad}<time class="at" datetime={r.at} title={dateTime(r.at)}>{ago(r.at)}</time>{/if}
				<span class="wd {r.tone}">{r.word}</span>
			</div>
			{#if bad}
				{#if r.reason}<p class="why">{r.reason}</p>{/if}
				{#if canAct && (r.check || r.fix)}
					<div class="acts">
						{#if r.check}<button type="button" class="lnk" disabled={busy === r.key} onclick={() => oncheck(r)}>Check now</button>{/if}
						{#if r.fix}<a class="lnk" href={to(r.fix)}>Fix</a>{/if}
					</div>
				{/if}
			{/if}
		</li>
	{/each}
</ul>
{#if ordered.length > MAX}
	<button type="button" class="all" aria-expanded={all} onclick={() => (all = !all)}>{all ? 'Show fewer' : `Show all ${ordered.length}`}</button>
{/if}

<style>
	.rows { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 12px; overflow: hidden; background: var(--surface-1); }
	.row { padding: 0 14px; border-top: 1px solid var(--line); }
	.row:first-child { border-top: 0; }
	.row:nth-child(even):not(.bad) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 45%, transparent); }
	.row.bad { background: color-mix(in srgb, var(--fail) 6%, var(--surface-1)); padding-bottom: 10px; }
	.row.bad[data-tone='wait'] { background: color-mix(in srgb, var(--wait) 7%, var(--surface-1)); }
	.line { display: flex; align-items: center; gap: 10px; min-height: 44px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.at { flex: none; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.wd { flex: none; min-width: 64px; text-align: right; font-size: 12.5px; color: var(--text-muted); }
	.wd.ok { color: var(--text); }
	.wd.fail { color: var(--fail-text); font-weight: 600; }
	.wd.wait { font-weight: 600; color: var(--text); }
	.why { margin: -4px 0 0 17px; font-size: 13px; line-height: 1.45; color: var(--text-muted); overflow-wrap: anywhere; }
	.acts { display: flex; gap: 4px; margin: 6px 0 0 9px; }
	.lnk {
		display: inline-flex; align-items: center; min-height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: none;
		color: var(--accent, var(--text)); font: 600 13px/1 var(--font-ui); text-decoration: none; cursor: pointer;
	}
	.lnk:hover { background: var(--surface-2, var(--surface-1)); }
	.lnk:focus-visible, .all:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.lnk:disabled { opacity: 0.5; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.d.idle { background: var(--text-faint); }
	.all { width: 100%; min-height: 40px; margin-top: 8px; border: 1px dashed var(--line); border-radius: 10px; background: none; color: var(--text-muted); font: 500 13px/1 var(--font-ui); cursor: pointer; }
	.all:hover { color: var(--text); }
	@media (max-width: 767px) { .at { display: none; } }
</style>
