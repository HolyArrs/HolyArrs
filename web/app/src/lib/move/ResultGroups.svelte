<script lang="ts">
	import Button from '../ui/Button.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { groupLabels, needsGroups, needsLook, plural, resultWords, summary, tone, type Bucket, type Line, type Report } from '../move.ts';

	// The results of a test run or a move, one row per group, each with
	// "Show"; then what was not moved, the keys to type again and the apps'
	// notes. Used by Try it (before) and Done (after the move).
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
			hint: l.detail,
			pill: { word: resultWords(l.result, after, undo), tone: tone(l.result) },
			look: needsLook(l.result)
		};
	}
</script>

{#each groups as g (g.group)}
	<div class="srow" data-testid="result-group">
		<div class="txt"><span class="label">{g.label}</span><span class="hint">{summary(g.counts, after, undo)}</span></div>
		<div class="ctl"><Button onclick={() => onshow(g.label, g.lines, lineRow)} aria-label="Show {g.label}">Show</Button></div>
	</div>
{/each}
{#if report.not_moved.length > 0}
	<div class="srow">
		<div class="txt"><span class="label">Not moved</span><span class="hint">{plural(report.not_moved.length, 'setting')} with no match here</span></div>
		<div class="ctl"><Button onclick={() => onshow('Not moved', report.not_moved, (n, i) => ({ key: n.from + i, name: n.what, hint: `${groupLabels[n.group] ?? 'Other settings'} · ${n.why}` }))} aria-label="Show Not moved">Show</Button></div>
	</div>
{/if}
{#if needs.length > 0}
	<div class="srow">
		<div class="txt"><span class="label">Keys to type again</span><span class="hint">{plural(needs.length, 'key or password', 'keys and passwords')} never {needs.length === 1 ? 'comes' : 'come'} over</span></div>
		<div class="ctl"><Button onclick={() => onshow('Keys to type again', needs, (n) => n)} aria-label="Show Keys to type again">Show</Button></div>
	</div>
{/if}
{#if (report.notes?.length ?? 0) > 0}
	<div class="srow">
		<div class="txt"><span class="label">Notes from the apps</span><span class="hint">{plural(report.notes?.length ?? 0, 'note')}</span></div>
		<div class="ctl"><Button onclick={() => onshow('Notes from the apps', report.notes ?? [], (n, i) => ({ key: String(i), name: n }))} aria-label="Show Notes from the apps">Show</Button></div>
	</div>
{/if}
