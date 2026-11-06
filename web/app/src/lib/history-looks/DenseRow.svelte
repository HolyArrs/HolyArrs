<script lang="ts">
	// History's row (the compact cards look): the time, a dot on the thin rail, a small poster,
	// the title with its part and, only when it says something new, one quiet line; the step
	// mark (the Filter's word) on the right. Back-to-back repeats come in with a count.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import VerbMark from '../history-designs/VerbMark.svelte';
	import { dateTime } from '../format.ts';
	import { artOf, clock } from '../history-views.ts';
	import { filterVerb, partOf, rowExtra, rowStep } from './dirs.ts';
	import { rowText, type LineContext } from '../history-lines.ts';
	import type { HistoryRow } from '../history.ts';
	import type { EventRecord, TitleView } from '../types.ts';

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
		onclick
	}: {
		row: HistoryRow;
		ctx: LineContext;
		titleById: Map<number, TitleView>;
		who?: (e: EventRecord) => string;
		on?: boolean;
		controls?: string;
		/** A folded run of the same Event: how many, shown as a count after the name. */
		count?: number;
		/** A folded run: whether its times are shown under it. */
		expanded?: boolean;
		/** A folded run of routine work: its own name and line instead of the head row's. */
		label?: string;
		note?: string;
		/** The row carries its row key (for moving with the arrow keys); a routine run's toggle does not. */
		keyed?: boolean;
		onclick: () => void;
	} = $props();

	const e = $derived(row.head);
	const w = $derived(rowText(row, ctx));
	const t = $derived(titleById.get(e.TitleID ?? 0));
	const s = $derived(rowStep(row, w.tag.word));
	// A routine run is neutral: its own icon, no step word, whatever its first row was.
	const verb = $derived(label !== undefined ? { word: '', icon: 'settings' as const, tone: 'idle' as const } : filterVerb(row, w.tag));
	const extra = $derived(rowExtra(row, s, who, w.line));
	const part = $derived(partOf(row));
	const version = $derived(w.bits.find((b) => b.kind === 'version')?.text ?? '');
	// No title (sign-ins, health, backups): the line itself is the name, with a small icon instead of a poster.
	const bare = $derived(label !== undefined || !e.title_name);
	const name = $derived(label ?? (bare ? w.line || extra || w.name : w.name));
	// The second line only when it adds something: the row's own step word when the Filter's differs ("Out now"), then the extra.
	const second = $derived.by(() => {
		if (note !== undefined) return note;
		if (bare) return '';
		const own = s.label && s.label !== verb.word ? s.label : '';
		return [own, extra].filter(Boolean).join(' · ');
	});
</script>

<button type="button" class="hit {verb.tone}" class:on class:two={!!second} data-key={keyed ? row.key : undefined} data-testid="history-row" data-kind={e.Kind} data-steps={row.steps.length ? row.events.length : undefined} aria-current={on ? 'true' : undefined} aria-controls={controls} aria-expanded={expanded} {onclick}>
	<time class="clk" datetime={e.At} title={dateTime(e.At)}>{clock(e.At)}</time>
	<span class="node" aria-hidden="true"></span>
	<span class="art" class:square={t?.kind === 'artist'}>{#if bare}<span class="sysi"><Icon name={verb.tone === 'fail' ? 'alert' : 'settings'} size={13} /></span>{:else}<Poster src={artOf(e, titleById)} name={e.title_name ?? ''} />{/if}</span>
	<span class="tx">
		<span class="l1"><span class="nm" data-testid="history-title">{name}</span>{#if !bare && part}<span class="sub">{` · ${part}`}</span>{/if}{#if !bare && version}<span class="sub" data-testid="event-version">{' · '}<span data-testid="history-version">{version}</span></span>{/if}{#if count > 1}<span class="cnt" data-testid="history-count-run" aria-hidden="true">{count}</span><span class="vh">, {count} {label !== undefined ? 'updates' : 'times'}</span>{/if}</span>
		{#if second || (!bare && part)}<span class="l2" class:ponly={!second}>{#if !bare && part}<span class="pp">{part}{second ? ' · ' : ''}</span>{/if}{second}</span>{/if}
	</span>
	<span class="mark">{#if verb.word}<VerbMark {verb} size="sm" />{/if}{#if expanded !== undefined}<span class="chev" class:open={expanded} aria-hidden="true"><Icon name="chevronDown" size={14} /></span>{/if}</span>
</button>

<style>
	/* One 42px line per Event (52px on phones); the second line only when it says something new. */
	.hit { --clk: 58px; --art: 32px; position: relative; width: 100%; height: 42px; display: grid; grid-template-columns: var(--clk) 7px var(--art) minmax(0, 1fr) auto; align-items: center; column-gap: 10px; padding: 0 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The picked row: a neutral lift with a hairline, never the red of a failure. */
	.hit.on { background: color-mix(in srgb, var(--text) 9%, transparent); box-shadow: inset 0 0 0 1px var(--line); }
	.hit.on:focus-visible { box-shadow: inset 0 0 0 1px var(--line), var(--focus-ring); }
	/* Muted words stay readable on the tinted picked and hovered rows (contrast in light). */
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
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.open { transform: rotate(180deg); }
	@media (max-width: 1279px) { .mark { min-width: 0; } }
	/* The part on the second line is for phones only (desktops show it after the name). */
	.pp { display: none; }
	.l2.ponly { display: none; }
	@media (max-width: 767px) {
		.hit { --clk: 52px; height: 52px; column-gap: 8px; padding: 0 8px; }
		/* Phones: the name gets the line; the part moves under it and the step shows as its mark only (still read out). */
		.l1 .sub { display: none; }
		.pp { display: inline; }
		.l2.ponly { display: block; }
		.mark { min-width: 0; }
		.mark :global(.w) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	}
	@media (prefers-reduced-motion: reduce) { .hit, .chev { transition: none; } }
</style>
