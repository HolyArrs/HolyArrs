<script lang="ts">
	import type { ShowList } from './ItemsSheet.svelte';
	import { groupLabels, needsGroups, needsLook, plural, resultWords, summary, tone, type Bucket, type Line, type Report } from './move.ts';

	let {
		report,
		groups,
		after = false,
		onshow
	}: {
		report: Report;
		groups: Bucket[];
		after?: boolean;
		onshow: ShowList;
	} = $props();
	const undo = $derived(!!report.undo);
	const needs = $derived(needsGroups(report.needs_input ?? []).flatMap((g) => g.items.map((t, i) => ({ key: `${g.label}-${i}`, name: t, hint: g.label }))));

	function lineRow(l: Line, i: number) {
		return {
			key: l.key + ':' + i,
			name: l.name,
			hint: l.result === 'not_found' ? undefined : l.detail,
			pill: { word: resultWords(l.result, after, undo), tone: tone(l.result) },
			look: needsLook(l.result)
		};
	}
</script>

<ul class="rg">
	{#each groups as g (g.group)}
		<li class="rg-row" data-testid="result-group">
			<span class="rg-txt"><b>{g.label}</b><span class="rg-sub">{summary(g.counts, after, undo)}</span></span>
			<button type="button" class="rg-show" onclick={() => onshow(g.label, g.lines, lineRow)} aria-label="Show {g.label}">Show</button>
		</li>
	{/each}
	{#if report.not_moved.length > 0}
		<li class="rg-row">
			<span class="rg-txt"><b>Not moved</b><span class="rg-sub">{plural(report.not_moved.length, 'setting')}</span></span>
			<button type="button" class="rg-show" onclick={() => onshow('Not moved', report.not_moved, (n, i) => ({ key: n.from + i, name: n.what, hint: `${groupLabels[n.group] ?? 'Other settings'} · ${n.why}` }))} aria-label="Show Not moved">Show</button>
		</li>
	{/if}
	{#if needs.length > 0}
		<li class="rg-row">
			<span class="rg-txt"><b>Keys to add again</b><span class="rg-sub">{needs.length}</span></span>
			<button type="button" class="rg-show" onclick={() => onshow('Keys to add again', needs, (n) => n)} aria-label="Show Keys to add again">Show</button>
		</li>
	{/if}
	{#if (report.notes?.length ?? 0) > 0}
		<li class="rg-row">
			<span class="rg-txt"><b>Notes from the apps</b><span class="rg-sub">{report.notes?.length}</span></span>
			<button type="button" class="rg-show" onclick={() => onshow('Notes from the apps', report.notes ?? [], (n, i) => ({ key: String(i), name: n }))} aria-label="Show Notes from the apps">Show</button>
		</li>
	{/if}
</ul>

<style>
	.rg { list-style: none; margin: 0; padding: 0 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.rg-row { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 8px 0; border-top: 1px solid var(--line); }
	.rg-row:first-child { border-top: 0; }
	.rg-txt { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: var(--t-callout); color: var(--text); }
	.rg-txt b { font-weight: 600; }
	.rg-sub { margin-left: 10px; color: var(--text-muted); font: var(--t-footnote); }
	.rg-show { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.rg-show:hover { color: var(--text); }
	.rg-show:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	@media (max-width: 600px), (pointer: coarse) {
		.rg-show { min-height: 44px; padding: 0 4px; }
	}
</style>
