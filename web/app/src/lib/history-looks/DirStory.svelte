<script lang="ts">
	// Look C (?look=story): one card per title with its art and newest step.
	// Opening a card (?title=) shows that title's story from top to bottom:
	// requested, approved, searched, chosen, downloaded, imported, subtitles,
	// upgraded, read from the Events of that title alone.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, untrack } from 'svelte';
	import Icon from '../Icon.svelte';
	import Poster from '../Poster.svelte';
	import Button from '../ui/Button.svelte';
	import DirBar from './DirBar.svelte';
	import LookFrame from './LookFrame.svelte';
	import RowActions from './RowActions.svelte';
	import StateDot from './StateDot.svelte';
	import { EventFeed, TabCounts } from './feed.svelte.ts';
	import { ALL, lookTabs, tabKinds, whenShort } from '../history-looks.ts';
	import { groupDownloads } from '../history.ts';
	import { artOf, titleHref, verbOf } from '../history-views.ts';
	import { eventText, type LineContext } from '../history-lines.ts';
	import { dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import { dirQuery, readDir, storyOf, titleCards, whoMatches } from './dirs.ts';
	import type { TitleView } from '../types.ts';

	let { titles, titleById, people, me }: { titles: TitleView[]; titleById: Map<number, TitleView>; people: Map<number, string>; me?: number } = $props();
	const ctx = $derived<LineContext>({ titles: titleById, people, me });

	const st = $derived(readDir(page.url.searchParams));
	const one = $derived(st.filter.title ?? 0);
	// The cards follow the tabs; one title's story reads every kind it has.
	const kinds = $derived(one ? undefined : tabKinds(st.show, st.sub));
	const tab = $derived(lookTabs.find((t) => t.key === st.show) ?? lookTabs[0]);
	const empty = $derived({ title: st.filter.from || st.filter.to || st.who ? 'Nothing matches' : tab.empty, text: one ? 'Nothing happened to this title yet.' : tab.hint });

	const SIZE = 200;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(st.filter, kinds);
	$effect(() => {
		void [st.filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));
	const counts = new TabCounts();
	$effect(() => {
		void counts.load({ ...st.filter, title: undefined }, st.show);
	});

	const events = $derived(st.who ? feed.events.filter((e) => whoMatches(e, st.who, me)) : feed.events);
	const cards = $derived(titleCards(events));
	const story = $derived(storyOf(events));
	const rows = $derived(groupDownloads(events));
	const title = $derived(titleById.get(one));
	const name = $derived(title?.name ?? events[0]?.title_name ?? 'This title');
	const href = $derived(titleHref(one, titleById));
	const stuck = $derived(rows.find((r) => r.steps.some((s) => s.state === 'failed')) ?? rows.find((r) => r.steps.length));

	function open(id: number) {
		void goto(page.url.pathname + dirQuery(page.url.searchParams, { title: id }), {});
	}
	function back() {
		void goto(page.url.pathname + dirQuery(page.url.searchParams, { title: undefined, episode: undefined }));
	}
</script>

<DirBar counts={counts.tabs} {titles} />

{#if one}
	<div class="hd" data-testid="history-story">
		<Button variant="plain" size="sm" icon="chevronLeft" onclick={back}>Every title</Button>
		<div class="hero">
			<span class="art" class:square={title?.kind === 'artist'}><Poster src={artOf({ TitleID: one, artwork: events[0]?.artwork }, titleById)} name={name} /></span>
			<div class="hw">
				<h2 class="ht">{name}</h2>
				<p class="hs">{events.length} {events.length === 1 ? 'Event' : 'Events'}{events[0] ? ` · last ${whenShort(events[0].At)}` : ''}</p>
				<div class="ha">
					{#if stuck}<RowActions row={stuck} size="sm" />{/if}
					{#if href}<Button variant="secondary" size="sm" href={to(href)} iconAfter="chevronRight">Open {name}</Button>{/if}
				</div>
			</div>
		</div>
	</div>
	<LookFrame {feed} {empty} onretry={reload} onmore={() => feed.load(st.filter, kinds, feed.offset + SIZE, true)}>
		<ol class="story" aria-label="Story of {name}">
			{#each story as s (s.key)}
				<li class="step {s.state}" data-state={s.state}>
					<span class="dot" aria-hidden="true">{#if s.state === 'done'}<Icon name="check" size={13} />{:else if s.state === 'failed'}<Icon name="x" size={13} />{/if}</span>
					<div class="sb">
						<p class="sl"><b>{s.label}</b>{#if s.count > 1}<span class="sc">{s.count} times</span>{/if}<span class="sr-only">: {s.state === 'done' ? 'done' : s.state === 'failed' ? 'went wrong' : 'not yet'}</span></p>
						{#if s.last}
							<p class="st">{eventText(s.last, ctx).line}</p>
							<time class="sw" datetime={s.last.At} title={dateTime(s.last.At)}>{whenShort(s.last.At)}</time>
						{:else}
							<p class="st muted">Not yet</p>
						{/if}
					</div>
				</li>
			{/each}
		</ol>
	</LookFrame>
{:else}
	<LookFrame {feed} {empty} onall={st.show !== ALL ? () => void goto(page.url.pathname + dirQuery(page.url.searchParams, { show: undefined }), { replace: true }) : undefined} onretry={reload} onmore={() => feed.load(st.filter, kinds, feed.offset + SIZE, true)}>
		<ul class="grid" aria-label="Titles" data-testid="history-story-cards">
			{#each cards as c (c.id)}
				{@const t = titleById.get(c.id)}
				<li>
					<button type="button" class="card" class:problem={c.problem} onclick={() => open(c.id)}>
						<span class="cart" class:square={t?.kind === 'artist'}><Poster src={artOf(c.last, titleById)} name={c.name} /></span>
						<span class="cw">
							<span class="cn">{c.name}</span>
							<StateDot verb={verbOf(c.last.Kind)} />
							<span class="cl">{eventText(c.last, ctx).line}</span>
							<span class="cm"><time datetime={c.last.At}>{whenShort(c.last.At)}</time>{c.count > 1 ? ` · ${c.count} Events` : ''}</span>
						</span>
						<span class="chev" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>
					</button>
				</li>
			{/each}
		</ul>
	</LookFrame>
{/if}

<style>
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
	.card { display: flex; align-items: center; gap: 14px; width: 100%; min-height: 112px; padding: 10px 12px 10px 10px; box-sizing: border-box; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-1); color: var(--text); text-align: left; cursor: pointer; transition: background-color 160ms var(--ease-out), border-color 160ms var(--ease-out); }
	.card:hover { background: var(--surface-2); }
	.card:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.card.problem { border-color: color-mix(in srgb, var(--fail) 40%, var(--line)); }
	.cart { flex: none; width: 60px; aspect-ratio: 2 / 3; overflow: hidden; border-radius: 8px; background: var(--surface-2); }
	.cart.square, .art.square { aspect-ratio: 1; }
	.cw { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; flex: 1; min-width: 0; font: var(--t-footnote); }
	.cn { max-width: 100%; font: var(--t-headline); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cl { max-width: 100%; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.cm { color: var(--text-faint); }
	.chev { flex: none; color: var(--text-muted); }

	.hd { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; margin: 0 0 20px; }
	.hero { display: flex; align-items: flex-end; gap: 16px; width: 100%; }
	.art { flex: none; width: 96px; aspect-ratio: 2 / 3; overflow: hidden; border-radius: 10px; background: var(--surface-2); }
	.hw { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.ht { margin: 0; font: var(--t-headline); font-size: 22px; overflow-wrap: anywhere; }
	.hs { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.ha { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px; }

	.story { position: relative; margin: 0; padding: 0; list-style: none; max-width: 640px; }
	.step { position: relative; display: flex; gap: 14px; padding: 0 0 22px; }
	.step::before { content: ''; position: absolute; left: 13px; top: 28px; bottom: 0; width: 2px; background: var(--line); }
	.step:last-child::before { display: none; }
	.step.done::before { background: color-mix(in srgb, var(--ok) 45%, var(--line)); }
	.dot { flex: none; display: grid; place-items: center; width: 28px; height: 28px; border: 2px solid var(--line-strong); border-radius: 50%; box-sizing: border-box; background: var(--bg); color: var(--bg); }
	.done .dot { border-color: var(--ok); background: var(--ok); }
	.failed .dot { border-color: var(--fail); background: var(--fail); }
	.sb { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding-top: 4px; }
	.sl { display: flex; align-items: baseline; gap: 8px; margin: 0; font: var(--t-callout); }
	.waiting .sl { color: var(--text-muted); }
	.failed .sl b { color: var(--fail-text); }
	.sc { color: var(--text-faint); font: var(--t-footnote); }
	.st { margin: 0; color: var(--text); font: var(--t-footnote); overflow-wrap: anywhere; }
	.st.muted { color: var(--text-faint); }
	.sw { color: var(--text-muted); font: var(--t-caption); }
	@media (max-width: 767px) {
		.grid { grid-template-columns: minmax(0, 1fr); gap: 8px; }
		.card { min-height: 96px; }
		.cart { width: 52px; }
		.art { width: 76px; }
		.ht { font-size: 19px; }
	}
	@media (prefers-reduced-motion: reduce) { .card { transition: none; } }
</style>
