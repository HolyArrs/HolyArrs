<script lang="ts">
	// What an ask is for in one line ("Seasons 1–2 · 2 of 4 seasons, 22
	// episodes", "2h 13m · out now"), its season pills (tapping one lists that
	// season's episodes), and "Not out yet · S6 airs Mar 3" when part of it
	// has not come out. Rows show the line alone; `pills` adds the pills.
	import { slide } from 'svelte/transition';
	import { DUR, easeOut } from '../ui/transitions.ts';
	import { askSummary, movieRelease, showNotOut, watchSeason } from './inbox.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import type { Ask } from '../types.ts';

	let { ctx, a, pills = false, notOut = true, dense = false }: { ctx: ApprovalsCtx; a: Ask; pills?: boolean; notOut?: boolean; dense?: boolean } = $props();

	$effect(() => ctx.want(a));
	const info = $derived(ctx.info(a));
	const sum = $derived(askSummary(a, info, ctx.details(a)?.runtime));
	const watch = $derived(watchSeason(a, info));
	const notOutLine = $derived.by(() => {
		if (!notOut) return '';
		if (a.kind === 'movie') return movieRelease(info).notOut;
		return watch ? showNotOut(watch, ctx.episodes(a.tmdb_id, watch)) : '';
	});
	let open = $state(0);
	const openEps = $derived(open ? ctx.episodes(a.tmdb_id, open) : null);
	const openPill = $derived(sum.pills.find((p) => p.season === open));
	const listed = $derived.by(() => {
		if (!openPill) return [];
		const want = openPill.episodes;
		const all = openEps ?? [];
		if (!want.length) return all.map((e) => ({ n: e.number, name: e.name ?? '', air: e.air_day ?? '' }));
		return want.map((n) => {
			const e = all.find((x) => x.number === n);
			return { n, name: e?.name ?? '', air: e?.air_day ?? '' };
		});
	});
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const today = new Date().toISOString().slice(0, 10);
</script>

<div class="sum" class:dense>
	{#if sum.line}<p class="line" data-testid="ask-summary">{sum.line}</p>{/if}
	{#if notOutLine}<p class="not-out" data-testid="ask-not-out">{notOutLine}</p>{/if}
	{#if pills && sum.pills.length}
		<div class="pills" role="group" aria-label="Seasons asked for">
			{#each sum.pills as p (p.season)}
				<button type="button" class="pill" class:on={open === p.season} aria-expanded={open === p.season} aria-controls="eps-{a.id}" onclick={() => (open = open === p.season ? 0 : p.season)}>{p.label}</button>
			{/each}
		</div>
		{#if open}
			<div class="eps" id="eps-{a.id}" transition:slide={{ duration: reduced() ? 0 : DUR.base, easing: easeOut }}>
				{#if openEps === null}
					<p class="muted">Loading season {open}…</p>
				{:else if !listed.length}
					<p class="muted">No episode list for season {open} yet.</p>
				{:else}
					<ol aria-label="Season {open} episodes">
						{#each listed as e (e.n)}
							<li class:later={!!e.air && e.air.slice(0, 10) > today}><span class="n">E{e.n}</span><span class="t">{e.name || `Episode ${e.n}`}</span>{#if e.air && e.air.slice(0, 10) > today}<span class="air">airs {e.air.slice(5, 10).replace('-', '/')}</span>{/if}</li>
					{/each}
				</ol>
			{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.sum { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.line { font: var(--t-callout); overflow-wrap: anywhere; }
	.dense .line { font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.not-out { align-self: flex-start; padding: 2px 8px; border-radius: var(--r-sm); background: color-mix(in srgb, var(--wait) 14%, transparent); color: var(--wait-text); font: var(--t-footnote); font-weight: 600; }
	.pills { display: flex; flex-wrap: wrap; gap: 6px; }
	.pill { min-height: 28px; padding: 0 10px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.pill:hover { background: var(--surface-3); }
	.pill.on { background: var(--accent-soft); color: var(--accent-text); }
	.pill:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.eps { overflow: hidden; }
	.eps ol { margin: 0; padding: 4px 0; list-style: none; max-height: 240px; overflow-y: auto; border-radius: var(--r-card); background: var(--surface-2); }
	.eps li { display: flex; align-items: baseline; gap: 10px; padding: 6px 12px; font: var(--t-footnote); }
	.eps li + li { border-top: 1px solid var(--line); }
	.eps .n { flex: none; width: 32px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.eps .t { flex: 1; min-width: 0; overflow-wrap: anywhere; }
	.eps .air { flex: none; color: var(--text-muted); }
	.eps .later .t { color: var(--text-muted); }
	.muted { color: var(--text-muted); font: var(--t-footnote); }
	@media (pointer: coarse) { .pill { min-height: 36px; } }
	@media (prefers-reduced-motion: reduce) { .pill { transition: none; } }
</style>
