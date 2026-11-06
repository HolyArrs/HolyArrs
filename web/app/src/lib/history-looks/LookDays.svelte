<script lang="ts">
	// History, day by day. Each day has a sticky header with what it held
	// ("2 came in · 1 problem · 40 routine"); rows sit on a rail with their
	// clock time, and runs of routine work fold into one quiet line that
	// opens in place. On wide screens the picked row's whole story shows in
	// a details panel on the right (reasons, paths, every step of a
	// download, the latest Events for the same title); arrow keys or j and k
	// move through the rows. The panel can be hidden; picking a row shows it
	// again. Narrower screens open the row in a sheet instead.
	import { onMount, tick, untrack, type Snippet } from 'svelte';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import VerbMark from '../history-designs/VerbMark.svelte';
	import StateDot from './StateDot.svelte';
	import DenseRow from './DenseRow.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { api } from '../api.ts';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { details, type HistoryFilter, type HistoryRow } from '../history.ts';
	import { artOf, burstLine, byWhom, clock, dayLabel, rowTier, titleHref } from '../history-views.ts';
	import { byDay, dayItems, dayLine, whenShort, type ArtSize } from '../history-looks.ts';
	import { dropAiTwins, eventStep, filterVerb, isRoutine, stepVerb } from './dirs.ts';
	import { eventText, rowText, type LineContext, type RowText } from '../history-lines.ts';
	import type { EventPage, EventRecord, TitleView } from '../types.ts';

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
		/** The details panel is shown (wide screens). */
		panel: boolean;
		panelId: string;
		/** With an episode filter: the episode, named by the server. */
		episodeName?: string;
		/** Member names by id, for "Added to the Library by Maya". */
		people?: Map<number, string>;
		/** The signed-in member. */
		me?: number;
		/** Keeps only some Events (a filter the server does not do). */
		keep?: (e: EventRecord) => boolean;
		/** Extra buttons under a picked row's details. */
		actions?: Snippet<[HistoryRow]>;
		/** The Search looks' row: small poster, title and time, then the step and one extra piece. */
		compact?: boolean;
	} = $props();
	const who = (e: EventRecord) => {
		if (e.Actor?.kind === 'ai') return byWhom(e);
		const n = byWhom(e);
		return n !== 'A person' && e.Actor?.kind === 'user' ? n : e.Actor?.kind === 'user' ? (people?.get(e.Actor.id ?? 0) ?? '') : '';
	};
	const ctx = $derived<LineContext>({ titles: titleById, people, me });

	const SIZE = 200;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));
	// A day header that is pinned under the page's top row gets a hairline and a soft shadow, so the list
	// under it keeps a visible top edge instead of rows being cut off mid-row.
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

	// A page of routine only (the Routine tab) is not folded: it is what was asked for.
	const days = $derived.by(() => {
		const all = dropAiTwins(feed.events);
		const list = byDay(keep ? all.filter(keep) : all);
		const allQuiet = list.every((d) => d.rows.every((r) => rowTier(r) === 'routine'));
		return list.map((d) => ({ ...d, items: foldSame(allQuiet ? d.rows.map((row) => ({ type: 'row' as const, key: row.key, row, tier: 'routine' as const })) : dayItems(d.rows)) }));
	});
	// Back-to-back repeats of the same single Event (12 sign-ins in a row) fold into one row with a count.
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
				// Named after its oldest row, so a new repeat on top keeps an open run open.
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
	// This day: the picked row's day, counted by the Filter's steps.
	const today = $derived.by(() => {
		const d = days.find((x) => x.rows.some((r) => r.key === picked?.key));
		if (!d) return null;
		// Every Event counts on its own (a grouped download is several steps).
		const n = new Map<string, { verb: ReturnType<typeof filterVerb>; count: number }>();
		let total = 0;
		for (const r of d.rows)
			for (const e of r.events) {
				const one = { key: String(e.ID), head: e, events: [e], steps: [] };
				const v = filterVerb(one, eventText(e, ctx).tag);
				if (!v.word) continue;
				total++;
				const c = n.get(v.word);
				if (c) c.count++;
				else n.set(v.word, { verb: v, count: 1 });
			}
		// The last day on the page may go on past it: "200+".
		const more = d === days[days.length - 1] && feed.events.length < feed.total;
		return { key: d.key, total, more, steps: [...n.values()].sort((a, b) => b.count - a.count) };
	});
	let open = $state<Record<string, boolean>>({});
	let sheet = $state<HistoryRow | null>(null);
	const split = $derived(wide && panel);

	// The rows a person can move through, in order: folded runs only once opened.
	const shown = $derived(days.flatMap((d) => d.items.flatMap((it) => (it.type === 'row' ? [it.row] : it.type === 'same' ? (open[it.key] || it.rows.some((r) => r.key === pickedKey && r !== it.rows[0]) ? it.rows : [it.rows[0]]) : open[it.key] ? it.rows : []))));
	let pickedKey = $state('');
	// The first row is picked until a person picks another (or it leaves the page).
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

	// The latest Events for the picked title, every kind.
	let more = $state<EventRecord[]>([]);
	let moreFor = 0;
	$effect(() => {
		const id = picked?.head.TitleID ?? 0;
		// With the page kept to one title, the rows already are its story.
		if (!split || filter.title || id === moreFor) return;
		moreFor = id;
		more = [];
		if (!id) return;
		api.get<EventPage>(`/events?title=${id}&limit=30`)
			.then((p) => {
				if (moreFor === id) more = p.events;
			})
			.catch(() => {});
	});
	const others = $derived(filter.title ? [] : more.filter((e) => !isRoutine(e.Kind) && !picked?.events.some((x) => x.ID === e.ID)).slice(0, 6));

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
	<span class="l2">{w.line}</span>
{/snippet}

{#snippet line(r: HistoryRow)}
	{@const e = r.head}
	{@const w = rowText(r, ctx)}
	{@const t = titleById.get(e.TitleID ?? 0)}
	{@const on = split && picked?.key === r.key}
	{#if compact}
		<DenseRow row={r} {ctx} {titleById} {who} {on} controls={wide ? panelId : undefined} onclick={() => choose(r)} />
	{:else}
	<button type="button" class="hit {w.tag.tone}" class:on data-key={r.key} data-testid="history-row" data-kind={e.Kind} data-steps={r.steps.length ? r.events.length : undefined} aria-current={on ? 'true' : undefined} aria-controls={wide ? panelId : undefined} onclick={() => choose(r)}>
		<time class="clk" datetime={e.At}>{clock(e.At)}</time>
		<span class="node" aria-hidden="true"><i></i></span>
		<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
		<span class="tx">{@render words(w)}</span>
	</button>
	{/if}
{/snippet}

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<div class="split size-{size}" class:compact class:on={split} class:wide data-testid="look-days" data-size={size} data-panel={wide ? (panel ? 'open' : 'hidden') : undefined}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="days" bind:this={list} onkeydown={key}>
			{#each days as d (d.key)}
				<section class="day" aria-labelledby="day-{d.key}">
					<header class="dh">
						<h2 id="day-{d.key}">{dayLabel(d.key)}</h2>
						<p class="dl">{dayLine(d.tally)}</p>
					</header>
					<ol class="rail">
						{#each d.items as it (it.key)}
							{#if it.type === 'row'}
								<li class="item" class:quiet={it.tier === 'routine'}>{@render line(it.row)}</li>
							{:else if it.type === 'same'}
								{@const on = !!open[it.key]}
								{@const head = it.rows[0]}
								<li class="item fold">
									<DenseRow row={head} {ctx} {titleById} {who} on={split && picked?.key === head.key} count={it.rows.length} expanded={on} controls={wide ? panelId : undefined} onclick={() => { open = { ...open, [it.key]: !on }; if (wide) choose(head); }} />
									{#if on}
										<ol class="inner" in:unfold out:fold>
											{#each it.rows.slice(wide ? 1 : 0) as r (r.key)}<li class="item quiet">{@render line(r)}</li>{/each}
										</ol>
									{/if}
								</li>
							{:else}
								{@const on = !!open[it.key]}
								<li class="item fold">
									{#if compact}
									<DenseRow row={it.rows[0]} {ctx} {titleById} {who} count={it.rows.length} expanded={on} label="Routine work" keyed={false} note={burstLine(it.rows.flatMap((r) => r.events), titleById)} onclick={() => (open = { ...open, [it.key]: !on })} />
									{:else}
									<button type="button" class="hit q" aria-expanded={on} onclick={() => (open = { ...open, [it.key]: !on })}>
										<time class="clk" datetime={it.rows[0].head.At}>{clock(it.rows[0].head.At)}</time>
										<span class="node" aria-hidden="true"><i></i></span>
										<span class="qn">{it.rows.length}</span>
										<span class="tx">
											<span class="l1"><span class="what"><span class="nm">Routine work</span><span class="bit">{` · ${it.rows.length} updates`}</span></span></span>
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
				<p class="none miss" data-testid="history-no-match">No Events on this page match.{feed.events.length < feed.total ? ' Show more to look further back.' : ''}</p>
			{/each}
		</div>

		{#if wide}
			<aside class="pane" id={panelId} aria-label="Details" data-testid="look-pane" hidden={!panel ? true : undefined} inert={!panel}>
				{#if picked}
					{@const e = picked.head}
					{@const w = rowText(picked, ctx)}
					{@const t = titleById.get(e.TitleID ?? 0)}
					{@const href = titleHref(e.TitleID, titleById)}
					<div class="card">
					{#key picked.key}
						<div class="pin" in:paneIn>
							<div class="ph">
								{#if e.title_name}<span class="part" class:square={t?.kind === 'artist'}><Poster src={artOf(e, titleById)} name={e.title_name} /></span>{/if}
								<div class="pw">
									<VerbMark verb={compact ? stepVerb(picked, w.tag) : w.tag} />
									<h2 class="pt">{w.name}</h2>
									{#if w.bits.length}<p class="psub">{w.bits.map((b) => b.text).join(' · ')}</p>{/if}
									<p class="psum">{w.line}</p>
								</div>
							</div>
							{#if picked.steps.length}
								<ol class="steps" aria-label="Steps of this download" data-testid="history-steps">
									{#each picked.steps as st (st.key)}<li class={st.state} data-state={st.state}><span class="pip" aria-hidden="true"></span>{st.label}<span class="sr-only">: {st.state === 'done' ? 'done' : st.state === 'failed' ? 'failed' : 'not yet'}</span></li>{/each}
								</ol>
							{/if}
							<dl class="facts">
								<dt>When</dt><dd><time datetime={e.At}>{dateTime(e.At)}</time> <span class="ago">({ago(e.At)})</span></dd>
								<dt>By</dt><dd>{who(e) || byWhom(e)}</dd>
								{#each picked.steps.length ? [] : details(e) as [k, val], i (i)}<dt>{k}</dt><dd class="mono">{val}</dd>{/each}
							</dl>
							{#if !picked.steps.length && details(e).length === 0}<p class="none">No reasons or paths were kept for this Event.</p>{/if}
							{#if picked.events.length > 1}
								<h3 class="h">Every step</h3>
								<ol class="trail" data-testid="history-trail">
									{#each picked.events as s (s.ID)}
										{@const why = details(s)}
										{@const sw = eventText(s, ctx)}
										<li>
											<div class="tl"><VerbMark verb={eventStep(s, sw.tag)} size="sm" /><time datetime={s.At} title={dateTime(s.At)}>{whenShort(s.At)}</time></div>
											<p class="ts">{sw.line}</p>
											{#if why.length}<dl class="facts small">{#each why as [k, val], i (i)}<dt>{k}</dt><dd class="mono">{val}</dd>{/each}</dl>{/if}
										</li>
									{/each}
								</ol>
							{/if}
							{#if others.length}
								<h3 class="h">Also for {e.title_name}</h3>
								<ol class="also">
									{#each others as o (o.ID)}
										{@const ow = eventText(o, ctx)}
										<li><VerbMark verb={eventStep(o, ow.tag)} size="sm" /><span class="as">{ow.line}</span><time datetime={o.At}>{whenShort(o.At)}</time></li>
									{/each}
								</ol>
							{/if}
							{#if actions}<div class="pact">{@render actions(picked)}</div>{/if}
							{#if !actions && href && e.title_name}<div class="pact"><Button variant="secondary" href={to(href)} iconAfter="chevronRight">Open {e.title_name}</Button></div>{/if}
						</div>
					{/key}
					</div>
					{#if compact && today}
						<section class="card day-card" aria-label="This day" data-testid="history-this-day">
							<div class="dch"><h3 class="h">This day</h3><span class="dcn">{today.total}{today.more ? '+' : ''} {today.total === 1 && !today.more ? 'event' : 'events'}</span></div>
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
	/* Poster size: one set of measures per size, used by every row. */
	.split { --clk: 64px; --art: 56px; --row: 96px; --pane: 0px; --gap: 0px; }
	.size-s { --art: 26px; --row: 52px; }
	.size-l { --art: 88px; --row: 140px; }
	.split.wide.on { --pane: 380px; --gap: 16px; }
	.split { display: grid; grid-template-columns: minmax(0, 1fr) var(--pane); column-gap: var(--gap); align-items: start; transition: grid-template-columns var(--dur-base) var(--ease-out), column-gap var(--dur-base) var(--ease-out); }

	.days { display: flex; flex-direction: column; gap: 20px; min-width: 0; }
	.dh { position: sticky; top: var(--topbar); z-index: 2; display: flex; align-items: baseline; justify-content: space-between; gap: 4px 12px; flex-wrap: wrap; margin-inline: -12px; padding: 10px 12px 8px; background: var(--bg); transition: top var(--bar-glide); }
	/* The pinned header's background reaches 12px past the list each side (text unmoved), so the list's outline and shadow never peek out beside it. */
	.dh::after { content: ''; position: absolute; left: 0; right: 0; bottom: 0; height: 1px; background: var(--line-strong); box-shadow: 0 6px 16px 2px light-dark(rgb(20 24 40 / .10), rgb(0 0 0 / .55)); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); pointer-events: none; }
	.dh:global(.stuck)::after { opacity: 1; }
	.dh h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.dl { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.rail { position: relative; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	/* The rail line runs through every dot. */
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
	/* Two lines at every size: what it is (with its tag), then what happened. */
	.tx { display: flex; flex-direction: column; gap: 2px; min-width: 0; grid-column: 4 / -1; }
	.l1 { display: flex; align-items: center; gap: 10px; min-width: 0; font: var(--t-footnote); }
	.what { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-callout); }
	.nm { color: var(--text); font-weight: 600; }
	.bit { font-variant-numeric: tabular-nums; }
	.l1 :global(.st) { flex: none; }
	.l2 { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.quiet .nm { color: var(--text-muted); font-weight: 500; }
	/* Medium and Large: roomier rows, bigger words. */
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
	/* A folded run: the count where the poster goes, the line, a chevron. */
	.hit.q { min-height: var(--row); }
	.hit.q .tx { grid-column: 4; }
	.qn { display: grid; place-items: center; justify-self: center; min-width: 22px; height: 22px; padding: 0 4px; border-radius: 6px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.on { transform: rotate(180deg); }
	.inner { margin: 0 0 4px; padding: 0; list-style: none; }
	.inner .item { box-shadow: 0 -1px 0 var(--line); }

	/* The details panel: slides out to the right edge when hidden. */
	/* The panel's top lines up with the first row (under the day header, 45px), at rest and when pinned. */
	.pane { position: sticky; top: calc(var(--topbar) + var(--pane-drop, 45px)); margin-top: var(--pane-drop, 45px); min-width: 0; max-height: calc(100dvh - var(--topbar) - 65px); overflow-y: auto; overscroll-behavior: contain; display: flex; flex-direction: column; gap: 12px; transition: opacity var(--dur-base) var(--ease-out), translate var(--dur-base) var(--ease-out), visibility 0s linear 0s; }
	.pane[hidden] { display: block; visibility: hidden; opacity: 0; translate: 24px 0; padding-inline: 0; overflow: hidden; transition: opacity var(--dur-fast) var(--ease-in), translate var(--dur-fast) var(--ease-in), padding var(--dur-base) var(--ease-out), visibility 0s linear var(--dur-fast); }
	.card { padding: 20px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.pin { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	/* This day: the picked row's day, counted by the Filter's steps. */
	.day-card { padding: 16px 18px; }
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
	.ph { display: flex; gap: 16px; align-items: flex-start; }
	.part { flex: none; width: 88px; --poster-ratio: 2 / 3; }
	.part.square { --poster-ratio: 1; }
	.part :global(.poster) { border-radius: 8px; }
	.pw { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; min-width: 0; }
	.pt { font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; }
	.psub { color: var(--text-muted); font: var(--t-footnote); }
	.psum { font: var(--t-callout); overflow-wrap: anywhere; }
	.steps { display: flex; flex-wrap: wrap; align-items: center; margin: 0; padding: 0; list-style: none; font: var(--t-footnote); }
	.steps li { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); }
	.steps li + li::before { content: ''; width: 18px; height: 1px; margin: 0 8px; background: var(--line); }
	.pip { width: 8px; height: 8px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--text-muted); }
	.steps .done { color: var(--text); }
	.steps .done .pip { background: var(--ok); box-shadow: none; }
	.steps .failed { color: var(--fail-text); font-weight: 600; }
	.steps .failed .pip { background: var(--fail); box-shadow: none; }
	.facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 8px 14px; margin: 0; padding: 14px; border-radius: var(--r-card); background: var(--surface-2); font: var(--t-footnote); }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; overflow-wrap: anywhere; }
	.facts dd.mono { font-family: var(--font-mono); }
	.facts .ago { color: var(--text-muted); }
	.facts.small { margin-top: 8px; padding: 10px; background: var(--surface-1); }
	.h { font: var(--t-headline); }
	.trail, .also { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
	.trail { gap: 8px; }
	.trail li { padding: 10px 12px; border-radius: var(--r-card); background: var(--surface-2); }
	.tl { display: flex; justify-content: space-between; align-items: center; gap: 8px; font: var(--t-footnote); }
	.tl time { color: var(--text-muted); white-space: nowrap; }
	.ts { margin-top: 4px; font: var(--t-footnote); overflow-wrap: anywhere; }
	.also li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 36px; font: var(--t-footnote); }
	.also li + li { border-top: 1px solid var(--line); }
	.as { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); }
	.also time { color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.pact { display: flex; flex-wrap: wrap; gap: 8px; }
	/* Muted, not faint: faint text fails contrast on the panel in both themes. */
	.none { color: var(--text-muted); font: var(--t-footnote); }
	.miss { margin: 0; padding: 24px 0; color: var(--text-muted); text-align: center; }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	@media (max-width: 1279px) { .split.wide.on { --pane: 340px; } }
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

	/* History's compact cards: the list is the sheet Home's tiles and the Requests cards are made of (lit from above,
	   a hairline edge, one long soft shadow), with a 1px rail through the row dots; the details cards match. */
	.compact { --sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a); --well: light-dark(rgb(20 24 40 / 4%), rgb(255 255 255 / 5%)); }
	.compact .rail, .compact .card { outline: 0; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%)), 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.compact .rail::before { top: 22px; bottom: 22px; left: calc(4px + 10px + 58px + 10px + 3px); width: 1px; border-radius: 0; }
	.compact .inner { margin: 0; }
	/* The details cards' top lines up with the list card's top edge (just under the day header). */
	.compact { --pane-drop: 41px; }
	@media (max-width: 767px) { .compact .rail::before { left: calc(4px + 8px + 52px + 8px + 3px); } }
	@media (prefers-reduced-motion: reduce) {
		.split, .hit, .chev, .dh, .pane, .pane[hidden] { transition: none; }
		.pane[hidden] { translate: none; }
	}
</style>
