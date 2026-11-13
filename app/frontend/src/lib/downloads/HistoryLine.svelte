<script lang="ts">
	// Compact History row: small poster, title and time, then the step and one extra piece. Used by the Search view and the title pages.
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { artOf } from './history-kinds.ts';
	import { agoShort, partOf, rowExtra, rowStep } from './history-search.ts';
	import { rowText, type LineContext } from './history-lines.ts';
	import type { HistoryRow } from './history.ts';
	import { dateTime } from '../core/format.ts';
	import type { EventRecord, TitleView } from '../core/types.ts';

	let {
		row,
		ctx,
		titleById,
		who = () => '',
		on = false,
		controls,
		time = agoShort,
		onclick
	}: {
		row: HistoryRow;
		ctx: LineContext;
		titleById: Map<number, TitleView>;
		who?: (e: EventRecord) => string;
		on?: boolean;
		controls?: string;
		/** Short and relative by default; day groups pass the clock time. */
		time?: (at: string) => string;
		onclick: () => void;
	} = $props();

	const e = $derived(row.head);
	const w = $derived(rowText(row, ctx));
	const t = $derived(titleById.get(e.TitleID ?? 0));
	const s = $derived(rowStep(row, w.tag.word));
	const x = $derived(serverWords(rowExtra(row, s, who, w.line)));
	const part = $derived(partOf(row));
	// Shows the Version (HD, 4K) when the title has more than one, so its rows tell them apart.
	const version = $derived(w.bits.find((b) => b.kind === 'version')?.text ?? '');
</script>

<button type="button" class="hit c" class:bad={s.failed} class:on data-key={row.key} data-testid="history-row" data-kind={e.Kind} data-steps={row.steps.length ? row.events.length : undefined} aria-current={on ? 'true' : undefined} aria-controls={controls} {onclick}>
	<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={12} /></span>{/if}</span>
	<span class="cl1"><span class="what"><span class="nm" data-testid="history-title">{w.name}</span>{#if part}<span class="bit">{` · ${part}`}</span>{/if}{#if version}<span class="bit" data-testid="event-version">{' · '}<span data-testid="history-version">{version}</span></span>{/if}</span><time class="ago" datetime={e.At} title={dateTime(e.At)}>{time(e.At)}</time></span>
	<span class="cl2"><span class="stp"><Icon name={s.icon} size={14} />{s.label}</span>{#if x}<span class="ex">{` · ${x}`}</span>{/if}</span>
</button>

<style>
	.hit { position: relative; width: 100%; display: grid; grid-template-columns: 32px minmax(0, 1fr); grid-template-rows: auto auto; align-items: center; column-gap: 12px; row-gap: 2px; min-height: 56px; padding: 8px 12px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: var(--surface-2); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.post { grid-row: 1 / 3; width: 32px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 4px; }
	.sys { display: grid; place-items: center; width: 32px; aspect-ratio: 2 / 3; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); }
	.cl1 { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
	.what { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-callout); }
	.nm { color: var(--text); font-weight: 600; }
	.bit { font-variant-numeric: tabular-nums; }
	.ago { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.cl2 { display: flex; align-items: center; min-width: 0; overflow: hidden; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.stp { flex: none; display: inline-flex; align-items: center; gap: 6px; color: var(--text); font-weight: 500; }
	.ex { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: pre; }
	.hit.bad { box-shadow: inset 2px 0 0 var(--fail); }
	.hit.bad .stp, .hit.bad .ex { color: var(--fail-text); }
	.hit.bad .stp :global(svg) { color: var(--fail); }
	.hit.bad:focus-visible { box-shadow: inset 2px 0 0 var(--fail), var(--focus-ring); }
	.hit.on { background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line-strong); }
	.hit.bad.on { box-shadow: inset 2px 0 0 var(--fail), inset 0 0 0 1px var(--line-strong); }
	.hit.on:focus-visible { box-shadow: inset 0 0 0 1px var(--line-strong), var(--focus-ring); }
	:global(.quiet) > .hit .nm { color: var(--text-muted); font-weight: 500; }
	@media (max-width: 767px) { .hit { column-gap: 10px; padding: 8px 10px; } }
	@media (prefers-reduced-motion: reduce) { .hit { transition: none; } }
</style>
