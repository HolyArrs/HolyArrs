<script lang="ts">
	// Parts as ledger cards in two columns: a tinted header per group, one
	// ruled row per part, and the reason plus Fix right under any part that is
	// not working. Long groups show six rows until "Show all".
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import { groupCounts, type StatusGroup, type StatusRow } from './status.ts';

	let {
		parts,
		icon,
		canAct,
		busy,
		oncheck
	}: { parts: StatusGroup[]; icon: Record<string, string>; canAct: boolean; busy: string | null; oncheck: (r: StatusRow) => void } = $props();

	const MAX = 6;
	let all = $state<Record<string, boolean>>({});
	const toneOf = (g: StatusGroup) => {
		const c = groupCounts(g);
		return c.bad ? 'fail' : c.ok ? 'ok' : 'idle';
	};
	// Problems first inside each group, so they are never folded away.
	const ordered = (g: StatusGroup) => [...g.rows].sort((a, b) => rank(a) - rank(b));
	const rank = (r: StatusRow) => (r.tone === 'fail' ? 0 : r.tone === 'wait' ? 1 : 2);
	const newest = (g: StatusGroup) => g.rows.map((r) => r.at).filter(Boolean).sort().pop();
</script>

<ul class="groups">
	{#each parts as g (g.id)}
		{@const c = groupCounts(g)}
		{@const rows = ordered(g)}
		{@const at = newest(g)}
		<li class="pg {toneOf(g)}">
			<header class="band">
				<span class="ico" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id] ?? icon.lists} /></svg></span>
				<span class="ttl">{g.label}</span>
				<span class="chip">{c.ok}/{g.rows.length}</span>
			</header>
			<ul class="rows">
				{#each all[g.id] ? rows : rows.slice(0, MAX) as r (r.key)}
					{@const bad = r.tone === 'fail' || r.tone === 'wait'}
					<li class="row" class:bad data-testid="status-row" data-tone={r.tone} data-key={r.key}>
						<div class="line">
							<i class="d {r.tone}" aria-hidden="true"></i>
							<span class="nm" title={r.name}>{r.name}</span>
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
			{#if rows.length > MAX}
				<button type="button" class="all" aria-expanded={!!all[g.id]} onclick={() => (all[g.id] = !all[g.id])}>
					{all[g.id] ? 'Show fewer' : `Show all ${rows.length}`}
				</button>
			{/if}
			<footer class="foot">{#if at}Checked <time datetime={at} title={dateTime(at)}>{ago(at)}</time>{:else}Not checked yet{/if}</footer>
		</li>
	{/each}
</ul>

<style>
	ul { list-style: none; margin: 0; padding: 0; }
	.groups { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; align-items: start; }
	.pg {
		--tint: var(--ok);
		display: flex; flex-direction: column; overflow: hidden; border-radius: 14px; border: 1px solid var(--line); background: var(--surface-1);
		box-shadow: 0 1px 0 rgb(0 0 0 / 0.04), 0 10px 24px -16px rgb(0 0 0 / 0.30);
	}
	.pg.fail { --tint: var(--fail); border-color: color-mix(in srgb, var(--fail) 35%, var(--line)); }
	.pg.idle { --tint: var(--text-faint); }
	.band {
		display: flex; align-items: center; gap: 10px; padding: 12px 14px;
		background: linear-gradient(90deg, color-mix(in srgb, var(--tint) 18%, var(--surface-1)), color-mix(in srgb, var(--tint) 5%, var(--surface-1)));
		border-bottom: 1px solid color-mix(in srgb, var(--tint) 28%, var(--line));
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08);
	}
	.ico { display: grid; place-items: center; flex: none; width: 28px; height: 28px; border-radius: 8px; background: var(--surface-1); color: var(--tint); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.ico svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
	.ttl { flex: 1; font: 600 14px/1.2 var(--font-ui); color: var(--text); }
	.chip { padding: 3px 9px; border-radius: var(--r-pill); background: var(--surface-1); font: 600 12px/1.2 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; box-shadow: inset 0 0 0 1px var(--line); }
	.row { padding: 0 14px; border-top: 1px solid var(--line); }
	.row:first-child { border-top: 0; }
	.row:nth-child(even):not(.bad) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 45%, transparent); }
	.row.bad { background: color-mix(in srgb, var(--fail) 6%, var(--surface-1)); padding-bottom: 10px; }
	.row.bad[data-tone='wait'] { background: color-mix(in srgb, var(--wait) 7%, var(--surface-1)); }
	.line { display: flex; align-items: center; gap: 10px; min-height: 42px; }
	.nm { flex: 1; min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.wd { flex: none; font-size: 12.5px; color: var(--text-muted); }
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
	.all { min-height: 40px; border: 0; border-top: 1px dashed var(--line); background: none; color: var(--text-muted); font: 500 13px/1 var(--font-ui); cursor: pointer; }
	.all:hover { color: var(--text); }
	.foot { margin-top: auto; padding: 9px 14px; border-top: 1px solid var(--line); background: var(--surface-2, var(--surface-1)); font-size: 12px; color: var(--text-muted); }
	@media (max-width: 900px) { .groups { grid-template-columns: minmax(0, 1fr); } }
</style>
