<script lang="ts">
	import type { Snippet } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Poster from '#lib/Poster.svelte';
	import VerbMark from './VerbMark.svelte';
	import { to } from '#lib/paths.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import { details, type HistoryRow } from '#lib/history.ts';
	import { artOf, byWhom, titleHref } from '#lib/history-views.ts';
	import { eventText, rowText, type LineContext } from '#lib/history-lines.ts';
	import type { TitleView } from '#lib/types.ts';

	// Everything about one History row in a side sheet (a bottom sheet on
	// phones): the full text, who did it, the reasons and paths, every step
	// of a download, and the way to the title.
	let { row = $bindable(null), titleById, people, me, actions }: { row: HistoryRow | null; titleById: Map<number, TitleView>; people?: Map<number, string>; me?: number; /** Extra buttons for the row (History preview looks). */ actions?: Snippet<[HistoryRow]> } = $props();
	const ctx = $derived<LineContext>({ titles: titleById, people, me });
	let open = $state(false);
	$effect(() => {
		open = !!row;
	});
	const e = $derived(row?.head);
	const words = $derived(row ? rowText(row, ctx) : null);
	const href = $derived(titleHref(e?.TitleID, titleById));
	const title = $derived(e?.TitleID ? titleById.get(e.TitleID) : undefined);
</script>

<Sheet bind:open title={words?.name ?? 'Event'} placement="right" size="md" onclose={() => (row = null)}>
	{#if row && e && words}
		<div class="es" data-testid="hv-sheet">
			<div class="head">
				{#if e.TitleID && e.title_name}
					<span class="art" class:square={title?.kind === 'artist'}><Poster src={artOf(e, titleById)} name={e.title_name} /></span>
				{/if}
				<div class="hw">
					<VerbMark verb={words.tag} />
					{#if words.bits.length}<p class="sub">{words.bits.map((b) => b.text).join(' · ')}</p>{/if}
					<p class="sum">{words.line}</p>
				</div>
			</div>
			<dl class="facts">
				<dt>When</dt><dd><time datetime={e.At}>{dateTime(e.At)}</time> <span class="muted">({ago(e.At)})</span></dd>
				<dt>By</dt><dd>{byWhom(e)}</dd>
				{#each row.steps.length ? [] : details(e) as [k, v], i (i)}<dt>{k}</dt><dd class="mono">{v}</dd>{/each}
			</dl>
			{#if actions}<div class="acts">{@render actions(row)}</div>{/if}
			{#if row.events.length > 1}
				<h3 class="h">Every step</h3>
				<ol class="trail">
					{#each row.events as step (step.ID)}
						{@const why = details(step)}
						{@const sw = eventText(step, ctx)}
						<li>
							<div class="t-line"><VerbMark verb={sw.tag} size="sm" /><time datetime={step.At} title={dateTime(step.At)}>{ago(step.At)}</time></div>
							<p class="t-sum">{sw.line}</p>
							{#if why.length}<dl class="facts small">{#each why as [k, v], i (i)}<dt>{k}</dt><dd class="mono">{v}</dd>{/each}</dl>{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		{#if href && e?.title_name}<Button variant="primary" href={to(href)} iconAfter="chevronRight">Open {e.title_name}</Button>{/if}
	{/snippet}
</Sheet>

<style>
	.acts { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0; }
	.es { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
	.head { display: flex; gap: 14px; align-items: flex-start; }
	.art { flex: none; width: 72px; --poster-ratio: 2 / 3; border-radius: var(--r-sm); overflow: hidden; }
	.art.square { --poster-ratio: 1; }
	.hw { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.sum { font: var(--t-body); overflow-wrap: anywhere; }
	.sub, .muted { color: var(--text-muted); font: var(--t-footnote); }
	.facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 8px 14px; margin: 0; padding: 14px; border-radius: var(--r-card); background: var(--surface-2); font: var(--t-footnote); }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; overflow-wrap: anywhere; font-family: var(--font-ui); }
	.facts dd.mono { font-family: var(--font-mono); }
	.mono { font-family: var(--font-mono); }
	.facts.small { margin-top: 8px; padding: 10px; background: var(--surface-1); }
	.h { font: var(--t-headline); }
	.trail { display: flex; flex-direction: column; gap: 12px; margin: 0; padding: 0; list-style: none; }
	.trail li { padding: 12px; border-radius: var(--r-card); background: var(--surface-2); }
	.t-line { display: flex; justify-content: space-between; align-items: center; gap: 8px; color: var(--text-muted); font: var(--t-footnote); }
	.t-sum { margin-top: 6px; font: var(--t-callout); overflow-wrap: anywhere; }
	@media (max-width: 480px) { .facts { grid-template-columns: minmax(0, 1fr); gap: 2px; } .facts dd + dt { margin-top: 8px; } }
</style>
