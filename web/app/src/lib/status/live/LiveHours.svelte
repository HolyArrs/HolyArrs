<script lang="ts">
	// View "Hours": today as 24 hour cells in two rows of twelve. Past hours
	// are tinted by how their runs ended, coming hours are outlined in the tone
	// of the job planned there, and the current hour carries a marker at the
	// exact minute. Under the grid: the next run today and the job running now.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	type Ran = { name: string; state: string; at: number };
	type Planned = { name: string; tone: StatusRow['tone'] | 'info'; at: number };
	type Hour = { h: number; ran: Ran[]; coming: Planned[]; worst: '' | 'failed' | 'cancelled' | 'done' };

	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
	const word = (s: string) => (s === 'done' ? 'OK' : s === 'failed' ? 'failed' : s === 'cancelled' ? 'cancelled' : s);
	const rank: Record<string, number> = { failed: 3, cancelled: 2, done: 1 };
	const toneRank: Record<string, number> = { fail: 4, wait: 3, ok: 2, info: 1, idle: 0 };

	const dayStart = $derived.by(() => {
		const d = new Date(lj.tick);
		d.setHours(0, 0, 0, 0);
		return d.getTime();
	});
	const dayEnd = $derived(dayStart + 86_400_000);
	const nowHour = $derived(new Date(lj.tick).getHours());
	const nowMin = $derived(new Date(lj.tick).getMinutes());

	const hours = $derived.by((): Hour[] => {
		const out: Hour[] = Array.from({ length: 24 }, (_, h) => ({ h, ran: [], coming: [], worst: '' }));
		for (const j of lj.finished) {
			const at = t(j.EndedAt);
			if (!(at >= dayStart && at < dayEnd)) continue;
			const c = out[new Date(at).getHours()];
			c.ran.push({ name: lj.nameOf(j.Kind), state: j.State, at });
			if ((rank[j.State] ?? 0) > (rank[c.worst] ?? 0)) c.worst = j.State as Hour['worst'];
		}
		for (const r of upcoming) {
			const at = t(r.nextRun);
			if (!(at >= lj.tick && at < dayEnd)) continue;
			out[new Date(at).getHours()].coming.push({ name: r.name, tone: r.tone, at });
		}
		for (const c of out) {
			c.ran.sort((a, b) => a.at - b.at);
			c.coming.sort((a, b) => a.at - b.at);
		}
		return out;
	});

	// The strongest tone among an hour's planned runs, so a coming check that is failing stands out.
	const comingTone = (c: Hour) => c.coming.reduce<string>((w, p) => ((toneRank[p.tone] ?? 0) > (toneRank[w] ?? -1) ? p.tone : w), '');

	const ranCount = $derived(hours.reduce((n, c) => n + c.ran.length, 0));
	const comingCount = $derived(hours.reduce((n, c) => n + c.coming.length, 0));
	const next = $derived(lj.coming(upcoming, upcoming.length).find((r) => t(r.nextRun) >= lj.tick && t(r.nextRun) < dayEnd));
	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);

	function label(c: Hour): string {
		const span = `${String(c.h).padStart(2, '0')}:00 to ${String(c.h).padStart(2, '0')}:59`;
		const parts = [span];
		if (c.ran.length) parts.push(`Ran: ${c.ran.map((r) => `${r.name} at ${hm(r.at)} (${word(r.state)})`).join(', ')}`);
		if (c.coming.length) parts.push(`Coming: ${c.coming.map((p) => `${p.name} at ${hm(p.at)}`).join(', ')}`);
		if (!c.ran.length && !c.coming.length) parts.push(c.h < nowHour ? 'Nothing ran' : c.h === nowHour ? 'Nothing yet' : 'Nothing planned');
		return parts.join(' · ');
	}
</script>

<div class="hrs">
	<h3>
		Today
		<span class="st" class:on={ranCount + comingCount > 0}>{ranCount} ran · {comingCount} coming</span>
	</h3>

	{#each [0, 12] as base (base)}
		<div class="half">
			<ol class="grid" aria-label={base === 0 ? 'Hours 0 to 11' : 'Hours 12 to 23'}>
				{#each hours.slice(base, base + 12) as c (c.h)}
					{@const past = c.h < nowHour}
					{@const tone = comingTone(c)}
					<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
					<li
						class="cell {c.worst ? `r-${c.worst}` : ''} {tone && !past ? `c-${tone}` : ''}"
						class:past
						class:now={c.h === nowHour}
						class:empty={!c.ran.length && !c.coming.length}
						tabindex={c.ran.length || c.coming.length || c.h === nowHour ? 0 : -1}
						title={label(c)}
						aria-label={label(c)}
					>
						{#if c.ran.length > 1}<span class="n">{c.ran.length}</span>{/if}
						{#if c.h === nowHour}<i class="mk" style:left="{(nowMin / 60) * 100}%" aria-hidden="true"></i>{/if}
					</li>
				{/each}
			</ol>
			<div class="nums" aria-hidden="true">
				{#each Array.from({ length: 12 }, (_, i) => base + i) as h (h)}
					<span>{h % 3 === 0 ? h : ''}</span>
				{/each}
			</div>
		</div>
	{/each}

	<div class="key" aria-hidden="true">
		<span><i class="sw r-done"></i>OK</span>
		<span><i class="sw r-failed"></i>Failed</span>
		<span><i class="sw r-cancelled"></i>Cancelled</span>
		<span><i class="sw plan"></i>Coming</span>
	</div>

	<div class="foot">
		<p class="nx">
			{#if next}
				<span class="k">Next</span><span class="nm">{next.name}</span><span class="tm">at {hm(t(next.nextRun))}</span>
			{:else}
				<span class="nm muted">Nothing more today</span>
			{/if}
		</p>
		{#if run}
			<p class="nx">
				<i class="d" aria-hidden="true"></i><span class="nm">{[run.label, pct > 0 ? `${pct}%` : '', run.note || 'Working'].filter(Boolean).join(' · ')}</span>
			</p>
			<div class="prog" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label={run.label}>
				<i style:width="{pct > 0 ? pct : 100}%"></i>
			</div>
		{/if}
	</div>
</div>

<style>
	.hrs { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 12px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.half + .half { margin-top: 8px; }
	.grid, .nums { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 4px; }
	.grid { list-style: none; margin: 0; padding: 0; }
	.cell {
		position: relative; height: 32px; border-radius: 5px; box-sizing: border-box;
		display: flex; align-items: center; justify-content: center;
		box-shadow: inset 0 0 0 1px var(--line);
		transition: background-color 0.8s linear, box-shadow 0.8s linear;
	}
	.cell.past.empty { background: color-mix(in srgb, var(--text) 5%, transparent); box-shadow: none; }
	.cell.r-done { background: color-mix(in srgb, var(--ok) 44%, transparent); box-shadow: none; }
	.cell.r-cancelled { background: color-mix(in srgb, var(--text-faint) 42%, transparent); box-shadow: none; }
	.cell.r-failed { background: color-mix(in srgb, var(--fail) 66%, transparent); box-shadow: none; }
	.cell.c-ok { box-shadow: inset 0 0 0 1.5px var(--ok); }
	.cell.c-wait { box-shadow: inset 0 0 0 1.5px var(--wait); }
	.cell.c-fail { box-shadow: inset 0 0 0 1.5px var(--fail); }
	.cell.c-info { box-shadow: inset 0 0 0 1.5px var(--info); }
	.cell.c-idle { box-shadow: inset 0 0 0 1.5px var(--text-faint); }
	.cell.now { box-shadow: inset 0 0 0 2px var(--accent, var(--text)); }
	.cell:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.n { font: 600 10.5px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; opacity: 0.85; }
	.mk { position: absolute; top: 4px; bottom: 4px; width: 2px; margin-left: -1px; border-radius: 1px; background: var(--accent, var(--text)); transition: left 0.8s linear; }

	.nums { margin-top: 3px; }
	.nums span { font-size: 10.5px; line-height: 1.2; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: visible; }

	.key { display: flex; flex-wrap: wrap; gap: 4px 14px; margin: 10px 0 0; font-size: 11.5px; color: var(--text-muted); }
	.key span { display: inline-flex; align-items: center; gap: 6px; }
	.sw { width: 10px; height: 10px; border-radius: 2px; }
	.sw.r-done { background: color-mix(in srgb, var(--ok) 44%, transparent); }
	.sw.r-failed { background: color-mix(in srgb, var(--fail) 66%, transparent); }
	.sw.r-cancelled { background: color-mix(in srgb, var(--text-faint) 42%, transparent); }
	.sw.plan { box-shadow: inset 0 0 0 1.5px var(--text-muted); }

	.foot { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--line); }
	.nx { display: flex; align-items: baseline; gap: 8px; min-width: 0; margin: 0; min-height: 22px; font-size: 13px; }
	.nx + .nx { margin-top: 2px; }
	.k { flex: none; font-weight: 600; color: var(--text-muted); font-size: 12.5px; }
	.nm { flex: 0 1 auto; min-width: 0; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.nm.muted { color: var(--text-muted); }
	.tm { flex: none; color: var(--text-muted); font-size: 12.5px; font-variant-numeric: tabular-nums; white-space: nowrap; }
	.d { flex: none; align-self: center; width: 7px; height: 7px; border-radius: 50%; background: var(--accent, var(--text)); }
	.prog { height: 3px; margin: 5px 0 0 15px; border-radius: 2px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.prog i { display: block; height: 100%; border-radius: inherit; background: var(--accent, var(--text)); transition: width 0.8s linear; }

	@media (max-width: 420px) {
		.grid, .nums { gap: 3px; }
		.cell { height: 28px; border-radius: 4px; }
		.n { font-size: 10px; }
	}
	@media (prefers-reduced-motion: no-preference) {
		.d { animation: pulse 2.4s ease-out infinite; }
		.prog.indet i { width: 40% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 9px transparent; } }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.cell, .mk, .prog i { transition: none; }
		.prog.indet i { width: 100% !important; opacity: 0.45; }
	}
</style>
