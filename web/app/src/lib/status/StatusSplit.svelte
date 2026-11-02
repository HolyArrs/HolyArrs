<script lang="ts">
	// The Split look: every part and job in one list on the left, the picked
	// one in full on the right. Problems sit at the top of the list. On phones
	// the detail opens over the list with a Back button.
	import { ago, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import { groupCounts, needsAttention, type StatusGroup, type StatusRow } from './status.ts';

	let {
		groups,
		canAct,
		busy,
		oncheck,
		labelOf
	}: {
		groups: StatusGroup[];
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
	} = $props();

	const all = $derived(groups.flatMap((g) => g.rows));
	const problems = $derived(needsAttention(all));
	let picked = $state<string | null>(null);
	let shown = $state(false); // phones: detail open over the list
	const sel = $derived(all.find((r) => r.key === picked) ?? problems[0] ?? all[0]);

	function pick(r: StatusRow) {
		picked = r.key;
		shown = true;
	}
</script>

{#snippet item(r: StatusRow, withKind = false)}
	<li>
		<button type="button" class="it" class:on={sel?.key === r.key} aria-current={sel?.key === r.key} data-testid="status-row" data-tone={r.tone} data-key={r.key} onclick={() => pick(r)}>
			<span class="dot {r.tone}" aria-hidden="true"></span>
			<span class="in">{r.name}{#if withKind}<span class="k"> · {labelOf(r)}</span>{/if}</span>
			<span class="w {r.tone}">{r.word}</span>
		</button>
	</li>
{/snippet}

<div class="split" class:shown data-testid="status-split">
	<nav class="list" aria-label="Parts and jobs">
		{#if problems.length}
			<section>
				<h2 class="bad">Needs attention<span class="cnt">{problems.length}</span></h2>
				<ul>{#each problems as r (r.key)}{@render item(r, true)}{/each}</ul>
			</section>
		{/if}
		{#each groups as g (g.id)}
			{@const c = groupCounts(g)}
			<section>
				<h2>{g.label}<span class="cnt">{c.ok}/{g.rows.length}</span></h2>
				<ul>{#each g.rows as r (r.key)}{@render item(r)}{/each}</ul>
			</section>
		{/each}
	</nav>

	{#if sel}
		<article class="detail" aria-labelledby="sd-name">
			<button type="button" class="back" onclick={() => (shown = false)}>‹ All parts</button>
			<p class="kind">{labelOf(sel)}</p>
			<h2 id="sd-name" class="dn">{sel.name}</h2>
			<p class="big {sel.tone}"><i aria-hidden="true"></i>{sel.word}</p>
			{#if sel.reason}<p class="reason" class:bad={sel.tone === 'fail'}>{sel.reason}</p>{/if}

			<dl class="facts">
				{#if sel.group === 'jobs'}
					<div><dt>Last run</dt><dd>{#if sel.lastRun}<time datetime={sel.lastRun} title={dateTime(sel.lastRun)}>{ago(sel.lastRun)}</time>{:else}Never{/if}</dd></div>
					<div><dt>Next run</dt><dd>{until(sel.nextRun)}</dd></div>
				{/if}
				{#if sel.at}
					<div><dt>{sel.group === 'jobs' ? 'Last finished' : 'Since'}</dt><dd><time datetime={sel.at}>{dateTime(sel.at)}</time> · {ago(sel.at)}</dd></div>
				{/if}
				<div><dt>Group</dt><dd>{labelOf(sel)}</dd></div>
			</dl>

			{#if canAct && (sel.check || sel.fix)}
				<div class="acts">
					{#if sel.check}
						<button type="button" class="btn pri" disabled={busy === sel.key} aria-busy={busy === sel.key} onclick={() => oncheck(sel)}>{sel.group === 'jobs' ? 'Run now' : 'Check now'}</button>
					{/if}
					{#if sel.fix}<a class="btn" href={to(sel.fix)}>{sel.tone === 'fail' || sel.tone === 'wait' ? 'Fix in Settings' : 'Open in Settings'}</a>{/if}
				</div>
			{/if}
		</article>
	{/if}
</div>

<style>
	.split {
		display: grid; grid-template-columns: minmax(300px, 400px) minmax(0, 1fr); gap: 16px; align-items: start;
	}
	ul { list-style: none; margin: 0; padding: 0; }
	.list, .detail { border: 1px solid var(--line); border-radius: var(--r-card, 16px); background: var(--surface-1); }
	.list {
		position: sticky; top: calc(var(--topbar, 0px) + 100px); max-height: calc(100vh - var(--topbar, 0px) - 130px);
		overflow-y: auto; padding: 6px 0 10px;
	}
	h2 {
		display: flex; align-items: center; justify-content: space-between; margin: 0; padding: 12px 16px 6px;
		font: 600 12.5px/1.2 var(--font-ui); letter-spacing: 0.02em; text-transform: uppercase; color: var(--text-muted);
	}
	h2.bad { color: var(--fail-text); }
	.cnt { font-size: 12px; letter-spacing: 0; font-variant-numeric: tabular-nums; }
	.it {
		display: grid; grid-template-columns: 10px minmax(0, 1fr) auto; align-items: center; gap: 10px; width: 100%;
		min-height: 44px; padding: 6px 16px; border: 0; background: none; color: var(--text); font: inherit; text-align: left; cursor: pointer;
	}
	.it:hover { background: var(--surface-2, var(--surface-1)); }
	.it.on { background: var(--surface-2, var(--surface-1)); box-shadow: inset 3px 0 0 var(--line-strong); }
	.it:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.in { font-size: 14.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.k { color: var(--text-muted); }
	.w { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; }
	.w.fail { color: var(--fail-text); font-weight: 500; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.dot.ok { background: var(--ok); }
	.dot.wait { background: var(--wait); }
	.dot.fail { background: var(--fail); }
	.dot.idle { background: var(--text-faint); }

	.detail { padding: 24px 28px 28px; min-height: 420px; }
	.back { display: none; }
	.kind { margin: 0; font-size: 13px; color: var(--text-muted); }
	.dn { margin: 4px 0 12px; padding: 0; font: 600 28px/1.2 var(--font-ui); text-transform: none; letter-spacing: 0; color: var(--text); display: block; }
	.big { display: inline-flex; align-items: center; gap: 9px; margin: 0; font: 600 17px/1 var(--font-ui); color: var(--text); }
	.big i { width: 11px; height: 11px; border-radius: 50%; background: var(--info); }
	.big.ok i { background: var(--ok); }
	.big.wait i { background: var(--wait); }
	.big.fail { color: var(--fail-text); }
	.big.fail i { background: var(--fail); }
	.big.idle { color: var(--text-muted); }
	.big.idle i { background: var(--text-faint); }
	.reason {
		margin: 16px 0 0; padding: 12px 14px; border-radius: 12px; background: var(--surface-2, var(--surface-1));
		font-size: 14.5px; color: var(--text); overflow-wrap: anywhere;
	}
	.reason.bad { background: color-mix(in srgb, var(--fail) 8%, var(--surface-1)); border: 1px solid color-mix(in srgb, var(--fail) 35%, var(--line)); }
	.facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 16px; margin: 24px 0 0; }
	.facts div { display: flex; flex-direction: column; gap: 3px; }
	dt { font-size: 12.5px; color: var(--text-muted); }
	dd { margin: 0; font-size: 15px; color: var(--text); }
	.acts { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 28px; }
	.btn {
		display: inline-flex; align-items: center; justify-content: center; min-height: 44px; padding: 0 18px; box-sizing: border-box;
		border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); color: var(--text);
		font: 500 14px/1 var(--font-ui); text-decoration: none; cursor: pointer;
	}
	.btn.pri { border-color: var(--line-strong); }
	.btn:hover { border-color: var(--line-strong); }
	.btn:focus-visible, .back:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled { opacity: 0.6; cursor: default; }

	@media (max-width: 767px) {
		.split { grid-template-columns: minmax(0, 1fr); }
		.list { position: static; max-height: none; }
		.detail { display: none; padding: 16px; min-height: 0; }
		.split.shown .list { display: none; }
		.split.shown .detail { display: block; }
		.back {
			display: inline-flex; align-items: center; min-height: 44px; margin: -8px 0 8px -4px; padding: 0 8px; border: 0;
			background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer;
		}
		.dn { font-size: 22px; }
	}
</style>
