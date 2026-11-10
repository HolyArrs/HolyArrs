<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// History grouped by day. Each day has a sticky header with a tally, rows sit on a rail with their clock time, and runs of routine work fold into one line that opens in place. On wide screens the picked row shows in a details panel (arrow keys or j and k move); narrower screens open it in a sheet.
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EventSheet from './HistoryEventSheet.svelte';
	import VerbMark from './HistoryVerb.svelte';
	import StateDot from './HistoryDot.svelte';
	import DenseRow from './HistoryCard.svelte';
	import LookFrame from './HistoryFrame.svelte';
	import { EventFeed } from './history-feed.svelte.ts';
	import type { HistoryFilter, HistoryRow } from './history.ts';
	import { artOf, burstLine, byWhom, clock, dayLabel, rowTier } from './history-kinds.ts';
	import { byDay, dayItems, dayLine, type ArtSize } from './history-tabs.ts';
	import { dropAiTwins, filterVerb } from './history-search.ts';
	import { FACT_LABEL, glue, panelView, pathBits } from './history-panel.ts';
	import { rowText, type LineContext, type RowText } from './history-lines.ts';
	import { onMount, tick, untrack, type Snippet } from 'svelte';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import { to } from '../core/paths.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { reducedMotion } from '../core/motion.ts';
	import { DUR, easeIn, easeOut } from '../core/transitions.ts';
	import type { EventRecord, TitleView } from '../core/types.ts';

	let {
		filter,
		kinds,
		titleById,
		empty,
		onall,
		size,
		wide,
		panel = $bindable(true),
		panelId,
		episodeName = $bindable(''),
		people,
		me,
		keep,
		actions,
		compact = false
	}: {
		filter: HistoryFilter;
		kinds: string[] | undefined;
		titleById: Map<number, TitleView>;
		empty: { title: string; text: string };
		onall?: () => void;
		size: ArtSize;
		/** Wide enough for the details panel beside the days. */
		wide: boolean;
		/** The details panel is shown. */
		panel: boolean;
		panelId: string;
		/** With an episode filter, the episode name from the server. */
		episodeName?: string;
		/** Member names by id, for "Added to the Library by Maya". */
		people?: Map<number, string>;
		me?: number;
		/** Keeps only some Events, a filter the server does not do. */
		keep?: (e: EventRecord) => boolean;
		/** Extra buttons under a picked row's details. */
		actions?: Snippet<[HistoryRow]>;
		/** Compact rows: small poster, title and time, then the step and one extra piece. */
		compact?: boolean;
	} = $props();
	const who = (e: EventRecord) => {
		if (e.Actor?.kind === 'ai') return byWhom(e);
		const n = byWhom(e);
		return n !== 'A person' && e.Actor?.kind === 'user' ? n : e.Actor?.kind === 'user' ? (people?.get(e.Actor.id ?? 0) ?? '') : '';
	};
	const ctx = $derived<LineContext>({ titles: titleById, people, me });

	const SIZE = 200;
	// The first view fills the screen and no more. Earlier Events load 50 at a time until the rows reach the bottom of the window; rows past it wait for Show more.
	const FILL_STEP = 50;
	const FILL_PAGES = 5;
	const ROW = 42;
	let fit = $state(14);
	let capped = $state(true);
	onMount(() => {
		const size = () => (fit = window.innerWidth < 768 ? Math.max(5, Math.floor((window.innerHeight - 440) / 52)) : Math.max(6, Math.floor((window.innerHeight - 300) / ROW)));
		size();
		window.addEventListener('resize', size, { passive: true });
		return () => window.removeEventListener('resize', size);
	});
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	// One reload re-reads every page on screen, so the list never shrinks and grows back.
	const refresh = () => feed.load(filter, kinds, 0, false, Math.min(Math.max(SIZE, feed.events.length), SIZE * FILL_PAGES));
	$effect(() => {
		void [filter, kinds];
		capped = true;
		untrack(reload);
	});
	onMount(() => feed.watch(refresh));
	// A pinned day header gets a hairline and shadow so the rows under it keep a clean top edge.
	onMount(() => {
		let raf = 0;
		const check = () => {
			raf = 0;
			for (const h of list?.querySelectorAll<HTMLElement>('.dh') ?? []) {
				const pin = parseFloat(getComputedStyle(h).top) || 0;
				const day = h.parentElement?.getBoundingClientRect();
				h.classList.toggle('stuck', !!day && day.top < pin - 1 && day.bottom > pin + h.offsetHeight);
			}
		};
		const onScroll = () => { if (!raf) raf = requestAnimationFrame(check); };
		window.addEventListener('scroll', onScroll, { passive: true });
		window.addEventListener('resize', onScroll, { passive: true });
		onScroll();
		return () => { window.removeEventListener('scroll', onScroll); window.removeEventListener('resize', onScroll); cancelAnimationFrame(raf); };
	});
	$effect(() => {
		episodeName = feed.episode;
	});

	// A page of routine work only (the Routine tab) is not folded, because that is what was asked for.
	const days = $derived.by(() => {
		const all = dropAiTwins(feed.events);
		const list = byDay(keep ? all.filter(keep) : all);
		const allQuiet = list.every((d) => d.rows.every((r) => rowTier(r) === 'routine'));
		return list.map((d) => ({ ...d, items: foldSame(allQuiet ? d.rows.map((row) => ({ type: 'row' as const, key: row.key, row, tier: 'routine' as const })) : dayItems(d.rows)) }));
	});
	// The days cut to the rows that fit the screen, until Show more. A day header counts as one row.
	const allRows = $derived(days.reduce((n, d) => n + d.items.length + 1, 0));
	const visible = $derived.by(() => {
		if (!capped || feed.manual || allRows <= fit) return days;
		let n = fit;
		const out: typeof days = [];
		for (const d of days) {
			// A day starts only when its header and at least one row still fit.
			if (n < 2) break;
			n -= 1;
			out.push({ ...d, items: d.items.slice(0, n) });
			n -= Math.min(n, d.items.length);
		}
		return out;
	});
	const held = $derived(visible !== days);
	// Back-to-back repeats of one Event (12 sign-ins in a row) fold into one row with a count.
	type Item = ReturnType<typeof dayItems>[number] | { type: 'same'; key: string; rows: HistoryRow[] };
	const sameKey = (r: HistoryRow) => (r.steps.length || r.events.length > 1 ? '' : [r.head.Kind, r.head.TitleID ?? 0, r.head.Actor?.kind ?? '', r.head.Actor?.id ?? 0, rowText(r, ctx).line].join('|'));
	function foldSame(items: ReturnType<typeof dayItems>): Item[] {
		if (!compact) return items;
		const out: Item[] = [];
		for (const it of items) {
			const last = out[out.length - 1];
			if (it.type !== 'row') { out.push(it); continue; }
			const k = sameKey(it.row);
			if (k && last?.type === 'same' && sameKey(last.rows[0]) === k) {
				last.rows.push(it.row);
				// Keyed by the oldest row, so a new repeat on top keeps an open run open.
				last.key = `s${it.row.key}`;
				continue;
			}
			if (k && last?.type === 'row' && sameKey(last.row) === k) {
				out[out.length - 1] = { type: 'same', key: `s${it.row.key}`, rows: [last.row, it.row] };
				continue;
			}
			out.push(it);
		}
		return out;
	}
	// Counts by the Filter's steps, with the mark each row shows, so counts match the list.
	function tallyOf(rows: HistoryRow[]) {
		const n = new Map<string, { verb: ReturnType<typeof filterVerb>; count: number }>();
		let total = 0;
		for (const r of rows) {
			const v = filterVerb(r, rowText(r, ctx).tag);
			if (!v.word) continue;
			total++;
			const c = n.get(v.word);
			if (c) c.count++;
			else n.set(v.word, { verb: v, count: 1 });
		}
		return { total, steps: [...n.values()].sort((a, b) => b.count - a.count) };
	}
	function headLine(rows: HistoryRow[]): string {
		const tl = tallyOf(rows);
		const bad = tl.steps.filter((x) => x.verb.word === t('downloads.hk.needsYou') || x.verb.word === t('downloads.hk.failed')).map((x) => `${x.verb.word} ${x.count}`);
		return [t(tl.total === 1 ? 'downloads.hdays.event' : 'downloads.hdays.events', { count: tl.total }), ...bad].join(' · ');
	}
	const today = $derived.by(() => {
		const d = days.find((x) => x.rows.some((r) => r.key === picked?.key));
		if (!d) return null;
		// The last day on the page may continue past it, shown as "200+".
		const more = d === days[days.length - 1] && feed.events.length < feed.total;
		return { key: d.key, more, ...tallyOf(d.rows) };
	});
	$effect(() => {
		const rows = allRows;
		if (!feed.loaded || feed.busy || feed.error || feed.manual) return;
		if (rows >= fit || feed.events.length >= feed.total || feed.events.length >= SIZE * FILL_PAGES) return;
		untrack(() => feed.load(filter, kinds, feed.next, true, FILL_STEP));
	});
	let open = $state<Record<string, boolean>>({});
	let sheet = $state<HistoryRow | null>(null);
	const split = $derived(wide && panel);

	// Rows reachable by keyboard, in order; folded runs only once opened.
	const shown = $derived(visible.flatMap((d) => d.items.flatMap((it) => (it.type === 'row' ? [it.row] : it.type === 'same' ? (open[it.key] || it.rows.some((r) => r.key === pickedKey && r !== it.rows[0]) ? it.rows : [it.rows[0]]) : open[it.key] ? it.rows : []))));
	let pickedKey = $state('');
	// The first row is picked until the person picks another or it leaves the page.
	const picked = $derived(shown.find((r) => r.key === pickedKey) ?? shown[0] ?? null);
	let list = $state<HTMLElement>();

	function choose(r: HistoryRow) {
		pickedKey = r.key;
		if (!wide) sheet = r;
		else if (!panel) panel = true;
	}
	async function move(by: number) {
		const i = shown.findIndex((r) => r.key === picked?.key);
		const next = shown[Math.max(0, Math.min(shown.length - 1, i + by))];
		if (!next) return;
		pickedKey = next.key;
		await tick();
		list?.querySelector<HTMLElement>(`[data-key="${next.key}"]`)?.focus();
	}
	function key(e: KeyboardEvent) {
		if (!split || e.altKey || e.ctrlKey || e.metaKey) return;
		if (e.key === 'ArrowDown' || e.key === 'j') {
			e.preventDefault();
			void move(1);
		} else if (e.key === 'ArrowUp' || e.key === 'k') {
			e.preventDefault();
			void move(-1);
		}
	}

	const view = $derived(picked && wide ? panelView(picked, { titles: titleById, people, me }) : null);
	async function copy(path: string) {
		try {
			await navigator.clipboard.writeText(path);
			toasts.push(t('downloads.hes.copied'), 'ok');
		} catch {
			toasts.push(t('downloads.hes.copyFailed'), 'fail', { body: t('updates.goBack.copyFailedBody') });
		}
	}

	function unfold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function fold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	function paneIn(node: Element): TransitionConfig {
		return fade(node, { duration: reducedMotion() ? 0 : DUR.fast, easing: easeOut });
	}
</script>

{#snippet words(w: RowText)}
	<span class="l1">
		<span class="what"><span class="nm" data-testid="history-title">{w.name}</span>{#each w.bits as b, i (i)}<span class="bit" data-testid={b.kind === 'episode' && i === 0 ? 'history-episode' : b.kind === 'version' ? 'history-version' : undefined}>{` · ${b.text}`}</span>{/each}</span>
		<StateDot verb={w.tag} />
	</span>
	<span class="l2">{serverWords(w.line)}</span>
{/snippet}

{#snippet line(r: HistoryRow)}
	{@const e = r.head}
	{@const w = rowText(r, ctx)}
	{@const t = titleById.get(e.TitleID ?? 0)}
	{@const on = split && picked?.key === r.key}
	{#if compact}
		<DenseRow row={r} {ctx} {titleById} {who} {on} tabindex={split ? (picked?.key === r.key ? 0 : -1) : undefined} controls={wide ? panelId : undefined} onclick={() => choose(r)} />
	{:else}
	<button type="button" class="hit {w.tag.tone}" class:on data-key={r.key} data-testid="history-row" data-kind={e.Kind} data-steps={r.steps.length ? r.events.length : undefined} aria-current={on ? 'true' : undefined} aria-controls={wide ? panelId : undefined} onclick={() => choose(r)}>
		<time class="clk" datetime={e.At}>{clock(e.At)}</time>
		<span class="node" aria-hidden="true"><i></i></span>
		<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
		<span class="tx">{@render words(w)}</span>
	</button>
	{/if}
{/snippet}

<LookFrame {feed} {empty} {onall} count={days.length > 0} onretry={reload} held={held} onmore={() => { if (held) capped = false; else { feed.manual = true; void feed.load(filter, kinds, feed.next, true); } }}>
	<div class="split size-{size}" class:compact class:on={split} class:wide data-testid="look-days" data-size={size} data-panel={wide ? (panel ? 'open' : 'hidden') : undefined}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="days" bind:this={list} onkeydown={key}>
			{#each visible as d (d.key)}
				<section class="day" aria-labelledby="day-{d.key}">
					<header class="dh">
						<h2 id="day-{d.key}">{dayLabel(d.key)}</h2>
						<p class="dl">{compact ? headLine(d.rows) : dayLine(d.tally)}</p>
					</header>
					<ol class="rail">
						{#each d.items as it (it.key)}
							{#if it.type === 'row'}
								<li class="item" class:quiet={it.tier === 'routine'}>{@render line(it.row)}</li>
							{:else if it.type === 'same'}
								{@const on = !!open[it.key]}
								{@const head = it.rows[0]}
								<li class="item fold">
									<DenseRow row={head} {ctx} {titleById} {who} on={split && picked?.key === head.key} tabindex={split ? (picked?.key === head.key ? 0 : -1) : undefined} count={it.rows.length} expanded={on} controls={wide ? panelId : undefined} onclick={() => { open = { ...open, [it.key]: !on }; if (wide) choose(head); }} />
									{#if on}
										<ol class="inner" in:unfold out:fold>
											{#each it.rows.slice(1) as r (r.key)}<li class="item quiet">{@render line(r)}</li>{/each}
										</ol>
									{/if}
								</li>
							{:else}
								{@const on = !!open[it.key]}
								<li class="item fold">
									{#if compact}
									<DenseRow row={it.rows[0]} {ctx} {titleById} {who} count={it.rows.length} expanded={on} label={t('downloads.hdays.routine')} keyed={false} note={burstLine(it.rows.flatMap((r) => r.events), titleById)} onclick={() => (open = { ...open, [it.key]: !on })} />
									{:else}
									<button type="button" class="hit q" aria-expanded={on} onclick={() => (open = { ...open, [it.key]: !on })}>
										<time class="clk" datetime={it.rows[0].head.At}>{clock(it.rows[0].head.At)}</time>
										<span class="node" aria-hidden="true"><i></i></span>
										<span class="qn">{it.rows.length}</span>
										<span class="tx">
											<span class="l1"><span class="what"><span class="nm">{t('downloads.hdays.routine')}</span><span class="bit">{` · ${t('downloads.hdays.updates', { count: it.rows.length })}`}</span></span></span>
											<span class="l2">{burstLine(it.rows.flatMap((r) => r.events), titleById)}</span>
										</span>
										<span class="chev" class:on aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
									</button>
									{/if}
									{#if on}
										<ol class="inner" in:unfold out:fold>
											{#each it.rows as r (r.key)}<li class="item quiet">{@render line(r)}</li>{/each}
										</ol>
									{/if}
								</li>
							{/if}
						{/each}
					</ol>
				</section>
			{:else}
				<div class="none" data-testid="history-no-match">
					<span class="eic"><Icon name="search" size={22} stroke={2} /></span>
					<p class="et">{t('downloads.hdays.nothingMatches')}</p>
					{#if feed.events.length < feed.total}<p class="es">{t('downloads.hdays.showMore')}</p>{/if}
					{#if onall}<button type="button" class="elink" onclick={onall}>{t('downloads.bar.clear')}</button>{/if}
				</div>
			{/each}
		</div>

		{#if wide}
			<aside class="pane" id={panelId} aria-label={t('downloads.hdays.details')} data-testid="look-pane" hidden={!panel ? true : undefined} inert={!panel}>
					{#if picked && view}
						{@const v = view}
						<div class="card">
						{#key picked.key}
							<div class="pin" in:paneIn>
								<div class="ph">
									{#if v.art}<span class="part" class:square={v.art.square}><Poster src={v.art.src} name={v.art.name} /></span>{/if}
									<div class="pw">
										<VerbMark verb={v.verb} size="sm" />
										<h2 class="pt">{v.title}</h2>
										{#if v.meta}<p class="psub">{v.meta}</p>{/if}
									</div>
								</div>
								{#if v.steps.length}
									<ol class="steps" aria-label={t('downloads.hes.steps')} data-testid="history-steps">
										{#each v.steps as st (st.key)}<li class={st.state} data-state={st.state}><span class="pip" aria-hidden="true"></span>{st.label}<span class="sr-only">: {st.state === 'done' ? 'done' : st.state === 'failed' ? 'failed' : st.state === 'needs' ? 'waiting' : 'not yet'}</span></li>{/each}
									</ol>
								{/if}
								{#if v.line}<p class="psum" class:long={v.long}>{serverWords(v.line)}</p>{/if}
								<dl class="pf">
									{#each v.facts as f (f.label)}
										<dt>{f.label}</dt>
										<dd class:mono={f.mono} class:one={f.label === 'Release'} title={f.label === 'Release' ? f.value : undefined}>
											{#if f.at}<time datetime={f.at}>{f.value}</time>{#if f.ago}<span class="ago">{` · ${f.ago}`}</span>{/if}
											{:else if f.full}<span class="path"><span class="pv" title={f.full}>{#each pathBits(f.value) as b, i (i)}{glue(b)}<wbr>{/each}</span><button type="button" class="cp" aria-label={t('downloads.hes.copyPath')} title={t('downloads.hes.copyPath')} onclick={() => copy(f.full ?? '')}><Icon name="copy" size={14} /></button></span>
											{:else}{f.value}{/if}
										</dd>
									{/each}
								</dl>
								{#if actions ? v.find || v.actions.length : v.actions.some((a) => a.key === 'title')}
									<div class="pact">
										{#if actions}{@render actions(picked)}
										{:else}{@const t = v.actions.find((a) => a.key === 'title')}{#if t}<Button variant="secondary" size="sm" href={to(t.href)} iconAfter="chevronRight">{t.label}</Button>{/if}{/if}
									</div>
								{/if}
							</div>
						{/key}
						</div>
					{#if compact && today}
						<section class="card day-card" aria-label={t('downloads.hdays.thisDay')} data-testid="history-this-day">
							<div class="dch"><h3 class="h">{t('downloads.hdays.thisDay')}</h3>{#if today.steps.length > 1}<span class="dcn">{today.total}{today.more ? '+' : ''} {today.total === 1 && !today.more ? t('downloads.hdays.eventWord') : t('downloads.hdays.eventsWord')}</span>{/if}</div>
							<ul class="tally">
								{#each today.steps as x (x.verb.word)}<li class="tp {x.verb.tone}"><span class="td" aria-hidden="true"></span>{x.verb.word}<b>{x.count}</b></li>{/each}
							</ul>
						</section>
					{/if}
				{/if}
			</aside>
		{/if}
	</div>
</LookFrame>

{#if !wide}<EventSheet bind:row={sheet} {titleById} {people} {me} {actions} />{/if}

<style>
	.split { --clk: 64px; --art: 56px; --row: 96px; --pane: 0px; --gap: 0px; }
	.size-s { --art: 26px; --row: 52px; }
	.size-l { --art: 88px; --row: 140px; }
	.split.wide.on { --pane: 380px; --gap: 16px; }
	.split { display: grid; grid-template-columns: minmax(0, 1fr) var(--pane); column-gap: var(--gap); align-items: start; transition: grid-template-columns var(--dur-base) var(--ease-out), column-gap var(--dur-base) var(--ease-out); }

	.days { display: flex; flex-direction: column; gap: 20px; min-width: 0; }
	.dh { position: sticky; top: var(--topbar); z-index: 2; display: flex; align-items: baseline; justify-content: space-between; gap: 4px 12px; flex-wrap: wrap; margin-inline: 0; padding: 10px 16px 8px; background: color-mix(in srgb, var(--bg) 55%, transparent); -webkit-backdrop-filter: blur(18px); backdrop-filter: blur(18px); border-radius: 12px; transition: top var(--bar-glide); }
	/* The pinned header background reaches 12px past the list on each side so the list outline never shows beside it. */
	.dh::after { content: ''; position: absolute; left: 0; right: 0; bottom: 0; height: 1px; background: var(--line-strong); box-shadow: 0 6px 16px 2px light-dark(rgb(20 24 40 / .10), rgb(0 0 0 / .55)); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); pointer-events: none; }
	.dh:global(.stuck)::after { opacity: 1; }
	.dh h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.dl { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.rail { position: relative; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.rail::before { content: ''; position: absolute; top: 18px; bottom: 18px; left: calc(4px + 10px + var(--clk) + 10px + 5px); width: 2px; border-radius: 1px; background: var(--line); }
	.item + .item { box-shadow: 0 -1px 0 var(--line); }
	.hit { position: relative; width: 100%; display: grid; grid-template-columns: var(--clk) 12px var(--art) minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: var(--row); padding: 6px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: var(--surface-2); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hit.on { background: var(--accent-soft); }
	.clk { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; white-space: nowrap; }
	.node { display: grid; place-items: center; width: 12px; height: 12px; }
	.node i { width: 10px; height: 10px; border-radius: 50%; background: var(--info); box-shadow: 0 0 0 3px var(--surface-1); }
	.ok .node i { background: var(--ok); }
	.fail .node i { background: var(--fail); }
	.wait .node i { background: var(--wait); }
	.idle .node i, .q .node i { width: 6px; height: 6px; background: var(--text-faint); }
	.post { width: var(--art); --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 4px; }
	.sys { display: grid; place-items: center; width: var(--art); aspect-ratio: 2 / 3; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); }
	.tx { display: flex; flex-direction: column; gap: 2px; min-width: 0; grid-column: 4 / -1; }
	.l1 { display: flex; align-items: center; gap: 10px; min-width: 0; font: var(--t-footnote); }
	.what { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-callout); }
	.nm { color: var(--text); font-weight: 600; }
	.bit { font-variant-numeric: tabular-nums; }
	.l1 :global(.st) { flex: none; }
	.l2 { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.quiet .nm { color: var(--text-muted); font-weight: 500; }
	.size-m .hit, .size-l .hit { gap: 14px; padding: 8px 12px; }
	.size-m .rail::before, .size-l .rail::before { left: calc(4px + 12px + var(--clk) + 14px + 5px); }
	.size-m .post :global(.poster), .size-m .sys { border-radius: 6px; }
	.size-l .post :global(.poster), .size-l .sys { border-radius: 8px; }
	.size-m .tx { gap: 4px; }
	.size-m .what { font: var(--t-headline); }
	.size-m .l2 { font: var(--t-callout); }
	.size-l .tx { gap: 6px; }
	.size-l .what { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.size-l .l1 { font: var(--t-callout); }
	.size-l .l2 { font: var(--t-body); }
	.hit.q { min-height: var(--row); }
	.hit.q .tx { grid-column: 4; }
	.qn { display: grid; place-items: center; justify-self: center; min-width: 22px; height: 22px; padding: 0 4px; border-radius: 6px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.on { transform: rotate(180deg); }
	.inner { margin: 0 0 4px; padding: 0; list-style: none; }
	.inner .item { box-shadow: 0 -1px 0 var(--line); }

	/* The panel's top lines up with the first row, at rest and when pinned. */
	.pane { position: sticky; top: calc(var(--topbar) + var(--pane-drop, 45px)); margin-top: var(--pane-drop, 45px); min-width: 0; max-height: calc(100dvh - var(--topbar) - 65px); overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; gap: 12px; transition: opacity var(--dur-base) var(--ease-out), translate var(--dur-base) var(--ease-out), visibility 0s linear 0s; }
	.pane[hidden] { display: block; visibility: hidden; opacity: 0; translate: 24px 0; padding-inline: 0; overflow: hidden; transition: opacity var(--dur-fast) var(--ease-in), translate var(--dur-fast) var(--ease-in), padding var(--dur-base) var(--ease-out), visibility 0s linear var(--dur-fast); }
	.card { margin: 0; padding: 20px; border: 0; border-radius: 16px; background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.pin { display: flex; flex-direction: column; min-width: 0; }
	.dch { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	.dcn { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.tally { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
	.tp { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; border-radius: var(--r-pill); background: var(--well, color-mix(in srgb, var(--text) 5%, transparent)); box-shadow: inset 0 0 0 1px var(--line); color: var(--text); font: 500 12.5px/1 var(--font-ui); }
	.tp b { color: var(--text-muted); font-weight: 600; font-variant-numeric: tabular-nums; }
	.td { width: 6px; height: 6px; border-radius: 50%; background: var(--info); }
	.tp.ok .td { background: var(--ok); }
	.tp.wait .td { background: var(--wait); }
	.tp.idle .td { background: var(--text-faint); }
	.tp.fail .td { background: var(--fail); }
	.tp.fail { color: var(--fail-text); }
	.h { font: var(--t-headline); }
	.ph { display: flex; gap: 14px; align-items: start; min-width: 0; }
	.part { flex: none; width: 64px; --poster-ratio: 2 / 3; }
	.part.square { --poster-ratio: 1; }
	.part :global(.poster) { border-radius: 10px; box-shadow: 0 0 0 1px var(--line); }
	.pw { flex: 1; display: flex; flex-direction: column; align-items: flex-start; min-width: 0; }
	.pt { margin-top: 4px; color: var(--text); font: var(--t-title3); letter-spacing: var(--t-title3-track); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.psub { width: 100%; margin-top: 2px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.steps { display: flex; align-items: center; margin: 12px 0 0; padding: 0; list-style: none; font: var(--t-footnote); white-space: nowrap; }
	.steps li { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); }
	.steps li + li { flex: 1 1 auto; min-width: 0; }
	.steps li + li::before { content: ''; flex: 1 1 auto; min-width: 10px; height: 1px; margin: 0 4px 0 10px; background: var(--line); }
	.pip { flex: none; width: 8px; height: 8px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--text-muted); }
	.steps .done { color: var(--text); }
	.steps .done .pip { background: var(--ok); box-shadow: none; }
	.steps .failed { color: var(--fail-text); font-weight: 600; }
	.steps .failed .pip { background: var(--fail); box-shadow: none; }
	.steps .needs { color: var(--text); font-weight: 600; }
	.steps .needs .pip { background: var(--accent); box-shadow: none; }
	.psum { margin-top: 12px; color: var(--text); font: var(--t-subhead); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.psum.long { -webkit-line-clamp: 3; line-clamp: 3; }
	.pf { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 8px 12px; margin: 20px 0 0; padding: 0; font: var(--t-footnote); }
	.pf dt { color: var(--text-muted); }
	.pf dd { margin: 0; min-width: 0; color: var(--text); font: var(--t-footnote); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
	.pf dd.mono { font: var(--t-caption); font-family: var(--font-mono); font-weight: 400; line-height: 1.45; }
	.pf dd.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pf .ago { color: var(--text-muted); }
	.path { display: inline-flex; align-items: flex-start; gap: 4px; min-width: 0; max-width: 100%; }
	.pv { min-width: 0; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
	.cp { flex: none; display: grid; place-items: center; width: 24px; height: 24px; margin-top: -4px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.cp:hover { background: var(--well, var(--surface-2)); color: var(--text); }
	.cp:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pact { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
	.pact > :global(.ui-btn) { border-radius: var(--r-pill); }
	.none { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 40px 20px; text-align: center; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.eic { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 12px; background: var(--surface-2); color: var(--text-muted); }
	.et { font: var(--t-headline); }
	.es { color: var(--text-muted); font: var(--t-callout); max-width: 360px; }
	.elink { display: inline-flex; align-items: center; min-height: 44px; padding: 0 4px; border: 0; background: none; color: var(--accent-text); font: var(--t-callout); font-weight: 600; cursor: pointer; border-radius: 6px; }
	.elink:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	/* Under 1280px the list stays the wider column and step marks show only their icon (the word is still read out). */
	@media (max-width: 1279px) {
		.split.wide.on { --pane: 300px; }
		.compact.on .days :global(.mark .w) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	}
	@media (max-width: 767px) {
		.split { --clk: 54px; }
		.size-m { --art: 48px; --row: 84px; }
		.size-l { --art: 72px; --row: 116px; }
		.rail::before { left: calc(4px + 8px + var(--clk) + 8px + 5px); }
		.size-m .rail::before, .size-l .rail::before { left: calc(4px + 8px + var(--clk) + 10px + 5px); }
		.hit { gap: 8px; padding: 6px 8px; min-height: max(var(--row), 52px); }
		.size-m .hit, .size-l .hit { gap: 10px; padding: 8px; }
		.l1 { gap: 8px; }
		.size-l .what { font: var(--t-headline); }
		.size-l .l2 { font: var(--t-callout); }
	}

	.compact { --sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a); --well: light-dark(rgb(20 24 40 / 4%), rgb(255 255 255 / 5%)); }
	.compact .rail, .compact .card { outline: 0; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%)), 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.compact .rail::before { top: 22px; bottom: 22px; left: calc(4px + 10px + 58px + 10px + 3px); width: 1px; border-radius: 0; }
	.compact .inner { margin: 0; }
	.compact { --pane-drop: 41px; }
	@media (max-width: 767px) { .compact .rail::before { left: calc(4px + 8px + 56px + 8px + 3px); } }
	@media (prefers-reduced-motion: reduce) {
		.split, .hit, .chev, .dh, .pane, .pane[hidden] { transition: none; }
		.pane[hidden] { translate: none; }
	}
</style>
