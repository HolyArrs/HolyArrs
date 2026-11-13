<script lang="ts">
	// One History row: time, a dot on the rail, a small poster, the title with its part, and one line only when it adds something. Ends in the step mark.
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import VerbMark from './HistoryVerb.svelte';
	import { artOf, clock } from './history-kinds.ts';
	import { filterVerb, partOf, rowExtra, rowStep, sameWords } from './history-search.ts';
	import { sysSplit } from './history-panel.ts';
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
		count = 0,
		expanded,
		label,
		note,
		keyed = true,
		tabindex,
		onclick
	}: {
		row: HistoryRow;
		ctx: LineContext;
		titleById: Map<number, TitleView>;
		who?: (e: EventRecord) => string;
		on?: boolean;
		controls?: string;
		/** Count of a folded run of the same Event, shown after the name. */
		count?: number;
		/** Whether a folded run shows its times. */
		expanded?: boolean;
		/** Name for a folded run of routine work, instead of the head row's. */
		label?: string;
		note?: string;
		/** Carries the row key for arrow-key moves; a routine run's toggle has none. */
		keyed?: boolean;
		/** Only the picked row is a Tab stop on wide screens. */
		tabindex?: number;
		onclick: () => void;
	} = $props();

	const e = $derived(row.head);
	const w = $derived(rowText(row, ctx));
	// An ask with no title id yet borrows the poster of the Library title with the same name.
	const t = $derived(titleById.get(e.TitleID ?? 0) ?? (!e.TitleID && /ask_/.test(e.Kind) ? [...titleById.values()].find((x) => x.name === w.name) : undefined));
	const s = $derived(rowStep(row, w.tag.word));
	// A routine run is neutral: its own icon and no step word.
	const verb = $derived(label !== undefined ? { word: '', icon: 'settings' as const, tone: 'idle' as const } : filterVerb(row, w.tag));
	const extra = $derived(serverWords(rowExtra(row, s, who, w.line)));
	const part = $derived(partOf(row));
	const version = $derived(w.bits.find((b) => b.kind === 'version')?.text ?? '');
	// Rows with no title (sign-ins, health, backups) use the line as the name, with an icon instead of a poster. An ask keeps its title even when it is not in the Library yet.
	const bare = $derived(label !== undefined || (!e.title_name && w.name === 'HolyArrs'));
	const split = $derived(bare && label === undefined ? sysSplit(row, { ...ctx, titles: titleById }) : null);
	const name = $derived(label ?? (bare ? serverWords(split?.name || w.line) || extra || w.name : w.name));
	// The second line shows the Filter's step word when it differs, then the extra.
	const second = $derived.by(() => {
		if (note !== undefined) return note;
		if (bare) return serverWords(split?.line ?? '');
		const own = s.label && s.label !== verb.word && !sameWords.has(s.label) ? s.label : '';
		return [own, extra].filter(Boolean).join(' · ');
	});
</script>

<button type="button" class="hit {verb.tone}" {tabindex} class:on class:two={!!second} data-key={keyed ? row.key : undefined} data-testid="history-row" data-kind={e.Kind} data-steps={row.steps.length ? row.events.length : undefined} aria-current={on ? 'true' : undefined} aria-controls={controls} aria-expanded={expanded} {onclick}>
	<time class="clk" datetime={e.At} title={dateTime(e.At)}>{clock(e.At)}</time>
	<span class="node" aria-hidden="true"></span>
	<span class="art" class:square={t?.kind === 'artist'}>{#if bare}<span class="sysi"><Icon name={verb.tone === 'fail' ? 'alert' : 'settings'} size={13} /></span>{:else}<Poster src={artOf(e, titleById) ?? t?.artwork?.[0]} name={e.title_name ?? w.name} />{/if}</span>
	<span class="tx">
		<span class="l1"><span class="nm" data-testid="history-title">{name}</span>{#if !bare && part}<span class="sub">{` · ${part}`}</span>{/if}{#if !bare && version}<span class="sub" data-testid="event-version">{' · '}<span data-testid="history-version">{version}</span></span>{/if}{#if count > 1}<span class="cnt" data-testid="history-count-run" aria-hidden="true">{count}</span><span class="vh">, {count} {label !== undefined ? 'updates' : 'times'}</span>{/if}</span>
		{#if second || (!bare && part)}<span class="l2" class:ponly={!second}>{#if !bare && part}<span class="pp">{part}{second ? ' · ' : ''}</span>{/if}{second}</span>{/if}
	</span>
	<span class="mark">{#if verb.word}<VerbMark {verb} size="sm" />{/if}</span>
	<!-- Every row keeps the chevron's slot so step marks share a right edge. -->
	<span class="chev" class:open={expanded} aria-hidden="true">{#if expanded !== undefined}<Icon name="chevronDown" size={14} />{/if}</span>
</button>

<style>
	.hit { --clk: 58px; --art: 32px; position: relative; width: 100%; height: 42px; display: grid; grid-template-columns: var(--clk) 7px var(--art) minmax(0, 1fr) auto 14px; align-items: center; column-gap: 10px; padding: 0 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hit.on { background: color-mix(in srgb, var(--text) 9%, transparent); box-shadow: inset 0 0 0 1px var(--line); }
	.hit.on:focus-visible { box-shadow: inset 0 0 0 1px var(--line), var(--focus-ring); }
	.hit.on :is(.clk, .sub, .l2, .cnt), .hit:hover :is(.clk, .sub, .l2, .cnt) { color: color-mix(in srgb, var(--text) 35%, var(--text-muted)); }
	.fail.hit.on .l2, .fail.hit:hover .l2 { color: var(--fail-text); }
	.vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	.clk { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; white-space: nowrap; }
	.node { width: 7px; height: 7px; border-radius: 50%; background: var(--info); box-shadow: 0 0 0 3px var(--sheet-a, var(--surface-1)); }
	.ok .node { background: var(--ok); }
	.fail .node { background: var(--fail); }
	.wait .node { background: var(--wait); }
	.idle .node { width: 5px; height: 5px; margin-left: 1px; background: var(--text-faint); }
	.art { display: grid; place-items: center; width: var(--art); height: 32px; --poster-ratio: 2 / 3; }
	.art :global(.poster) { width: auto; height: 32px; border-radius: 4px; box-shadow: 0 0 0 1px var(--line); }
	.art.square { --poster-ratio: 1; }
	.art.square :global(.poster) { border-radius: 5px; }
	.sysi { display: grid; place-items: center; width: 22px; height: 22px; border-radius: 7px; background: var(--well, color-mix(in srgb, var(--text) 5%, transparent)); color: var(--text-muted); }
	.fail .sysi { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail); }
	.tx { display: flex; flex-direction: column; justify-content: center; gap: 1px; min-width: 0; }
	.l1 { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.nm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: 600 14px/1.3 var(--font-ui); }
	.sub { flex: 0 10 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: pre; margin-left: -8px; color: var(--text-muted); font: 400 14px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.nm:has(+ .sub) { flex: 0 1 auto; }
	.cnt { flex: none; min-width: 22px; height: 18px; padding: 0 6px; display: inline-grid; place-items: center; border-radius: var(--r-pill); background: var(--well, color-mix(in srgb, var(--text) 5%, transparent)); box-shadow: inset 0 0 0 1px var(--line); color: var(--text-muted); font: 600 11px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.l2 { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: 400 12px/1.3 var(--font-ui); }
	.fail .l2 { color: var(--fail-text); }
	.mark { display: inline-flex; align-items: center; justify-content: flex-end; gap: 6px; min-width: 112px; }
	.chev { display: grid; width: 14px; margin-left: -4px; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.open { transform: rotate(180deg); }
	@media (max-width: 1279px) { .mark { min-width: 0; } }
	.pp { display: none; }
	.l2.ponly { display: none; }
	@media (max-width: 767px) {
		.hit { --clk: 56px; height: 52px; column-gap: 8px; padding: 0 8px; }
		/* Phones: the name gets the line; the part moves under it and the step shows as its mark only (still read out). */
		.l1 .sub { display: none; }
		.pp { display: inline; }
		.l2.ponly { display: block; }
		.mark { min-width: 0; }
		.mark :global(.w) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	}
	@media (prefers-reduced-motion: reduce) { .hit, .chev { transition: none; } }
</style>
