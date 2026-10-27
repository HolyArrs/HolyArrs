<script lang="ts">
	// Look 5, Split: a slim list on the left and the picked row's whole story
	// on the right, for finding out why something happened without opening
	// and closing sheets: the reasons and paths, every step of a download, and
	// the latest Events for the same title. Arrow keys (or j and k) move
	// through the list. Phones show the list; a row opens the side sheet.
	import { onMount, tick, untrack } from 'svelte';
	import { fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import VerbMark from '../history-designs/VerbMark.svelte';
	import StateDot from './StateDot.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { api } from '../api.ts';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeOut } from '../ui/transitions.ts';
	import { details, groupDownloads, type HistoryFilter, type HistoryRow } from '../history.ts';
	import { artOf, byWhom, headVerb, rowTier, titleHref, verbOf } from '../history-views.ts';
	import { whenShort } from '../history-looks.ts';
	import type { EventPage, EventRecord, TitleView } from '../types.ts';

	let { filter, kinds, titleById, empty, onall, phone }: { filter: HistoryFilter; kinds: string[] | undefined; titleById: Map<number, TitleView>; empty: { title: string; text: string }; onall?: () => void; phone: boolean } = $props();

	const SIZE = 100;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));

	const rows = $derived(groupDownloads(feed.events));
	let pickedKey = $state('');
	// The first row is picked until a person picks another (or it leaves the page).
	const picked = $derived(rows.find((r) => r.key === pickedKey) ?? rows[0] ?? null);
	let sheet = $state<HistoryRow | null>(null);
	let list = $state<HTMLElement>();

	function choose(r: HistoryRow) {
		pickedKey = r.key;
		if (phone) sheet = r;
	}
	async function move(by: number) {
		const i = rows.findIndex((r) => r.key === picked?.key);
		const next = rows[Math.max(0, Math.min(rows.length - 1, i + by))];
		if (!next) return;
		pickedKey = next.key;
		await tick();
		list?.querySelector<HTMLElement>(`[data-key="${next.key}"]`)?.focus();
	}
	function key(e: KeyboardEvent) {
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
		// With the page kept to one title, the list already is its story.
		if (phone || filter.title || id === moreFor) return;
		moreFor = id;
		more = [];
		if (!id) return;
		api.get<EventPage>(`/events?title=${id}&limit=8`)
			.then((p) => {
				if (moreFor === id) more = p.events;
			})
			.catch(() => {});
	});
	const others = $derived(filter.title ? [] : more.filter((e) => !picked?.events.some((x) => x.ID === e.ID)).slice(0, 6));
	function paneIn(node: Element): TransitionConfig {
		return fade(node, { duration: reducedMotion() ? 0 : DUR.fast, easing: easeOut });
	}
</script>

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<div class="split" data-testid="look-split">
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<ol class="list" bind:this={list} aria-label="Events" onkeydown={key}>
			{#each rows as r (r.key)}
				{@const e = r.head}
				{@const v = headVerb(r)}
				{@const t = titleById.get(e.TitleID ?? 0)}
				{@const on = !phone && picked?.key === r.key}
				<li class="li {v.tone}" class:on class:quiet={rowTier(r) === 'routine'}>
					<button type="button" class="hit" data-key={r.key} data-testid="look-row" aria-current={on ? 'true' : undefined} tabindex={phone || on ? 0 : -1} onclick={() => choose(r)}>
						<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
						<span class="tx">
							<span class="nm">{e.title_name ?? 'HolyArrs'}{#if e.episode}<span class="ep">{` ${e.episode}`}</span>{/if}</span>
							<span class="ln"><StateDot verb={v} /></span>
						</span>
						<time datetime={e.At}>{ago(e.At)}</time>
					</button>
				</li>
			{/each}
		</ol>

		{#if !phone && picked}
			{@const e = picked.head}
			{@const v = headVerb(picked)}
			{@const t = titleById.get(e.TitleID ?? 0)}
			{@const href = titleHref(e.TitleID, titleById)}
			<aside class="pane" aria-label="Details" data-testid="look-pane">
				{#key picked.key}
					<div class="pin" in:paneIn>
						<div class="ph">
							{#if e.title_name}<span class="part" class:square={t?.kind === 'artist'}><Poster src={artOf(e, titleById)} name={e.title_name} /></span>{/if}
							<div class="pw">
								<VerbMark verb={v} />
								<h2 class="pt">{e.title_name ?? 'HolyArrs'}</h2>
								{#if e.episode || e.version}<p class="psub">{[e.episode, e.version].filter(Boolean).join(' · ')}</p>{/if}
								<p class="psum">{e.Summary}</p>
							</div>
						</div>
						{#if picked.steps.length}
							<ol class="steps" aria-label="Steps of this download">
								{#each picked.steps as st (st.key)}<li class={st.state}><span class="pip" aria-hidden="true"></span>{st.label}<span class="sr-only">: {st.state === 'done' ? 'done' : st.state === 'failed' ? 'failed' : 'not yet'}</span></li>{/each}
							</ol>
						{/if}
						<dl class="facts">
							<dt>When</dt><dd><time datetime={e.At}>{dateTime(e.At)}</time></dd>
							<dt>By</dt><dd>{byWhom(e)}</dd>
							{#each picked.steps.length ? [] : details(e) as [k, val], i (i)}<dt>{k}</dt><dd class="mono">{val}</dd>{/each}
						</dl>
						{#if picked.events.length > 1}
							<h3 class="h">Every step</h3>
							<ol class="trail">
								{#each picked.events as s (s.ID)}
									{@const why = details(s)}
									<li>
										<div class="tl"><StateDot verb={verbOf(s.Kind)} /><time datetime={s.At} title={dateTime(s.At)}>{whenShort(s.At)}</time></div>
										<p class="ts">{s.Summary}</p>
										{#if why.length}<dl class="facts small">{#each why as [k, val], i (i)}<dt>{k}</dt><dd class="mono">{val}</dd>{/each}</dl>{/if}
									</li>
								{/each}
							</ol>
						{/if}
						{#if others.length}
							<h3 class="h">Also for {e.title_name}</h3>
							<ol class="also">
								{#each others as o (o.ID)}
									<li><StateDot verb={verbOf(o.Kind)} /><span class="as">{o.Summary}</span><time datetime={o.At}>{whenShort(o.At)}</time></li>
								{/each}
							</ol>
						{/if}
						{#if href && e.title_name}<div class="pact"><Button variant="secondary" href={to(href)} iconAfter="chevronRight">Open {e.title_name}</Button></div>{/if}
						{#if !picked.steps.length && details(e).length === 0}<p class="none">No reasons or paths were kept for this Event.</p>{/if}
					</div>
				{/key}
			</aside>
		{/if}
	</div>
</LookFrame>

{#if phone}<EventSheet bind:row={sheet} {titleById} />{/if}

<style>
	.split { display: grid; grid-template-columns: minmax(300px, 380px) minmax(0, 1fr); gap: 16px; align-items: start; }
	.list { position: sticky; top: calc(var(--topbar) + 12px); max-height: calc(100dvh - var(--topbar) - 120px); overflow-y: auto; overscroll-behavior: contain; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.li { border-radius: var(--r-btn); }
	.li + .li { box-shadow: 0 -1px 0 var(--line); }
	.hit { width: 100%; display: grid; grid-template-columns: 26px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 52px; padding: 6px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: var(--surface-2); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.li.on .hit { background: var(--accent-soft); }
	.li.fail .hit { box-shadow: inset 3px 0 0 var(--fail); }
	.li.fail .hit:focus-visible { box-shadow: inset 3px 0 0 var(--fail), var(--focus-ring); }
	.post { width: 26px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 4px; }
	.sys { display: grid; place-items: center; width: 26px; aspect-ratio: 2 / 3; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); }
	.tx { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.nm { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.quiet .nm { font-weight: 500; color: var(--text-muted); }
	.ep { color: var(--text-muted); font-weight: 400; }
	.ln { display: flex; min-width: 0; font: var(--t-footnote); }
	.hit time { align-self: start; padding-top: 2px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }

	.pane { position: sticky; top: calc(var(--topbar) + 12px); min-width: 0; max-height: calc(100dvh - var(--topbar) - 120px); overflow-y: auto; padding: 20px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.pin { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
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
	.also time { color: var(--text-faint); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.pact { display: flex; }
	.none { color: var(--text-faint); font: var(--t-footnote); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	@media (max-width: 1099px) { .split { grid-template-columns: minmax(260px, 320px) minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.split { grid-template-columns: minmax(0, 1fr); }
		.list { position: static; max-height: none; overflow: visible; }
		.hit { min-height: 60px; }
	}
	@media (prefers-reduced-motion: reduce) { .hit { transition: none; } }
</style>
