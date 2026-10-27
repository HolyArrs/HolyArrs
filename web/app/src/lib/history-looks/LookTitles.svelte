<script lang="ts">
	// Look 3, By title: one row per movie, show or artist, newest first, with
	// its latest outcome and small counts of what happened to it. A row opens
	// in place to tell that title's story (newest first); each step opens the
	// side sheet, and "Only this title" keeps the whole page to it.
	import { onMount, untrack } from 'svelte';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import StateDot from './StateDot.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { to } from '../paths.ts';
	import type { HistoryFilter, HistoryRow } from '../history.ts';
	import { artOf, headVerb, titleHref } from '../history-views.ts';
	import { rowWhy, threads, whenShort } from '../history-looks.ts';
	import type { TitleView } from '../types.ts';

	let { filter, kinds, titleById, empty, onall, onfilter }: { filter: HistoryFilter; kinds: string[] | undefined; titleById: Map<number, TitleView>; empty: { title: string; text: string }; onall?: () => void; onfilter: (c: Partial<HistoryFilter>) => void } = $props();

	const SIZE = 300;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));

	const list = $derived(threads(feed.events));
	let sheet = $state<HistoryRow | null>(null);
	let open = $state<string | null>(null);
	// One title on the page: its story is open from the start.
	const only = $derived(list.length === 1 ? list[0].key : null);

	const TALLY: { key: string; label: string; tone: string }[] = [
		{ key: 'came', label: 'came in', tone: 'ok' },
		{ key: 'problems', label: 'problems', tone: 'fail' },
		{ key: 'changes', label: 'changes', tone: 'info' },
		{ key: 'routine', label: 'routine', tone: 'idle' }
	];

	function unfold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function fold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
</script>

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<ol class="threads" data-testid="look-threads">
		{#each list as th (th.key)}
			{@const head = th.rows[0]}
			{@const e = head.head}
			{@const v = headVerb(head)}
			{@const t = titleById.get(th.titleId ?? 0)}
			{@const on = open === th.key || only === th.key}
			{@const href = titleHref(th.titleId, titleById)}
			<li class="th {v.tone}" class:on>
				<button type="button" class="hit" aria-expanded={on} data-testid="look-row" onclick={() => (open = open === th.key ? null : th.key)}>
					<span class="post" class:square={t?.kind === 'artist'}>{#if th.titleId}<Poster src={artOf(e, titleById)} name={th.name} />{:else}<span class="sys"><Icon name="settings" size={16} /></span>{/if}</span>
					<span class="tx">
						<span class="nm">{th.name}</span>
						<span class="last"><StateDot verb={v} /><span class="sum">{rowWhy(head) || e.Summary}</span></span>
					</span>
					<span class="tally" aria-label="On this page">
						{#each TALLY as k (k.key)}
							{#if th.tally[k.key]}<span class="n {k.tone}" title="{th.tally[k.key]} {k.label}"><i aria-hidden="true"></i>{th.tally[k.key]}<span class="sr-only"> {k.label}</span></span>{/if}
						{/each}
					</span>
					<time class="at" datetime={e.At}>{whenShort(e.At)}</time>
					<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
				</button>
				{#if on}
					<div class="story" in:unfold out:fold>
						<ol class="steps">
							{#each th.rows as r (r.key)}
								{@const rv = headVerb(r)}
								<li class={rv.tone}>
									<button type="button" class="step" onclick={() => (sheet = r)}>
										<span class="node" aria-hidden="true"><i></i></span>
										<StateDot verb={rv} />
										<span class="ssum">{r.head.episode ? `${r.head.episode}: ` : ''}{rowWhy(r) || r.head.Summary}</span>
										<time datetime={r.head.At}>{whenShort(r.head.At)}</time>
									</button>
								</li>
							{/each}
						</ol>
						{#if th.titleId}
							<div class="acts">
								{#if filter.title !== th.titleId}<button type="button" class="link" onclick={() => onfilter({ title: th.titleId ?? undefined, episode: undefined })}>Only this title</button>{/if}
								{#if href}<a class="link" href={to(href)}>Open {th.name}</a>{/if}
							</div>
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ol>
</LookFrame>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.threads { margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.th { border-radius: var(--r-btn); }
	.th + .th { box-shadow: 0 -1px 0 var(--line); }
	.th.fail { box-shadow: inset 3px 0 0 var(--fail); }
	.th + .th.fail { box-shadow: inset 3px 0 0 var(--fail), 0 -1px 0 var(--line); }
	.hit { width: 100%; display: grid; grid-template-columns: 36px minmax(0, 1fr) auto 140px 20px; align-items: center; gap: 12px; min-height: 64px; padding: 8px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: var(--surface-2); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.post { width: 36px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 5px; }
	.sys { display: grid; place-items: center; width: 36px; aspect-ratio: 2 / 3; border-radius: 5px; background: var(--surface-2); color: var(--text-muted); }
	.tx { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.nm { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.last { display: flex; align-items: center; gap: 8px; min-width: 0; font: var(--t-footnote); }
	.last :global(.st) { flex: none; }
	.sum { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); }
	.tally { display: flex; gap: 6px; justify-content: flex-end; }
	.n { display: inline-flex; align-items: center; gap: 5px; height: 22px; padding: 0 8px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.n i { width: 6px; height: 6px; border-radius: 50%; background: var(--info); }
	.n.ok i { background: var(--ok); }
	.n.fail i { background: var(--fail); }
	.n.idle i { background: var(--text-faint); }
	.at { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; white-space: nowrap; }
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.on .chev { transform: rotate(180deg); }
	.story { padding: 0 10px 10px 58px; }
	.steps { position: relative; margin: 0; padding: 0; list-style: none; }
	.steps::before { content: ''; position: absolute; top: 18px; bottom: 18px; left: 9px; width: 2px; border-radius: 1px; background: var(--line); }
	.step { position: relative; width: 100%; display: grid; grid-template-columns: 20px auto minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 36px; padding: 0 8px 0 0; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-footnote); cursor: pointer; }
	.step:hover { background: var(--surface-2); }
	.step:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.node { display: grid; place-items: center; }
	.node i { width: 8px; height: 8px; border-radius: 50%; background: var(--info); box-shadow: 0 0 0 3px var(--surface-1); }
	.ok .node i { background: var(--ok); }
	.fail .node i { background: var(--fail); }
	.wait .node i { background: var(--wait); }
	.idle .node i { width: 6px; height: 6px; background: var(--text-faint); }
	.ssum { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); }
	.step time { color: var(--text-faint); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.acts { display: flex; gap: 16px; padding: 6px 0 0 30px; }
	.link { display: inline-flex; align-items: center; min-height: 32px; padding: 0; border: 0; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 600; cursor: pointer; border-radius: 6px; }
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	@media (max-width: 960px) { .hit { grid-template-columns: 36px minmax(0, 1fr) auto 20px; } .at { display: none; } }
	@media (max-width: 767px) {
		.hit { grid-template-columns: 36px minmax(0, 1fr) 20px; grid-template-areas: 'p t c' 'p n c'; row-gap: 4px; }
		.post, .sys { grid-area: p; }
		.tx { grid-area: t; }
		.tally { grid-area: n; justify-content: flex-start; }
		.chev { grid-area: c; }
		.last { flex-direction: column; align-items: flex-start; gap: 1px; }
		.last .sum { max-width: 100%; }
		.story { padding: 0 8px 10px 8px; }
		.step { grid-template-columns: 20px minmax(0, 1fr) auto; grid-template-areas: 'd s w' 'd m m'; padding: 6px 8px 6px 0; row-gap: 2px; }
		.step .node { grid-area: d; align-self: start; padding-top: 4px; }
		.step :global(.st) { grid-area: s; }
		.step time { grid-area: w; }
		.ssum { grid-area: m; }
		.steps::before { top: 14px; }
		.acts { padding-left: 30px; }
		.link { min-height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) { .hit, .chev { transition: none; } }
</style>
