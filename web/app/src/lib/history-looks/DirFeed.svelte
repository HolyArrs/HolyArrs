<script lang="ts">
	// Look A (?look=feed): the day-by-day feed with the Requests top bar. One
	// story per download, routine runs folded, Problems kept apart in their
	// own line above the days, and each row's details offer the next step.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Icon from '../Icon.svelte';
	import DirBar from './DirBar.svelte';
	import LookDays from './LookDays.svelte';
	import RowActions from './RowActions.svelte';
	import { TabCounts } from './feed.svelte.ts';
	import { ALL, lookTabs, tabKinds } from '../history-looks.ts';
	import { dirQuery, readDir, showChange, whoMatches } from './dirs.ts';
	import type { EventRecord, TitleView } from '../types.ts';

	let { titles, titleById, people, me }: { titles: TitleView[]; titleById: Map<number, TitleView>; people: Map<number, string>; me?: number } = $props();

	const st = $derived(readDir(page.url.searchParams));
	const kinds = $derived(tabKinds(st.show, st.sub));
	const tab = $derived(lookTabs.find((t) => t.key === st.show) ?? lookTabs[0]);
	const empty = $derived({ title: st.filter.title || st.filter.from || st.filter.to || st.who ? 'Nothing matches' : tab.empty, text: tab.subs.find((s) => s.key === st.sub)?.hint ?? tab.hint });
	const keep = $derived(st.who ? (e: EventRecord) => whoMatches(e, st.who, me) : undefined);

	const counts = new TabCounts();
	$effect(() => {
		void counts.load(st.filter, st.show);
	});
	const problems = $derived(counts.tabs.problems ?? 0);
	const go = (change: Record<string, string | undefined>) => void goto(page.url.pathname + dirQuery(page.url.searchParams, change), { replace: true, reset: false });

	let wide = $state(false);
	let panel = $state(true);
	onMount(() => {
		const mq = window.matchMedia('(min-width: 1024px)');
		const fit = () => (wide = mq.matches);
		fit();
		mq.addEventListener('change', fit);
		return () => mq.removeEventListener('change', fit);
	});
</script>

<DirBar counts={counts.tabs} {titles} />

{#if st.show === ALL && problems > 0}
	<button type="button" class="apart" data-testid="history-problems-apart" onclick={() => go(showChange('problems'))}>
		<span class="ai" aria-hidden="true"><Icon name="alert" size={16} /></span>
		<span class="at"><b>{problems} {problems === 1 ? 'problem' : 'problems'}</b><span>Kept apart from the feed. See what went wrong.</span></span>
		<Icon name="chevronRight" size={16} />
	</button>
{/if}

<LookDays filter={st.filter} {kinds} {titleById} {empty} onall={st.show !== ALL ? () => go({ show: undefined }) : undefined} size="m" {wide} bind:panel panelId="history-feed-details" {people} {me} {keep}>
	{#snippet actions(row)}<RowActions {row} />{/snippet}
</LookDays>

<style>
	.apart { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 52px; margin: 0 0 16px; padding: 8px 14px; box-sizing: border-box; border: 1px solid color-mix(in srgb, var(--fail) 40%, var(--line)); border-radius: 12px; background: color-mix(in srgb, var(--fail) 10%, var(--surface-1)); color: var(--text); text-align: left; cursor: pointer; transition: background-color 160ms var(--ease-out); }
	.apart:hover { background: color-mix(in srgb, var(--fail) 16%, var(--surface-1)); }
	.apart:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ai { display: grid; place-items: center; width: 28px; height: 28px; flex: none; border-radius: 50%; background: color-mix(in srgb, var(--fail) 18%, transparent); color: var(--fail-text); }
	.at { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; font: var(--t-callout); }
	.at span { color: var(--text-muted); font: var(--t-footnote); }
	@media (prefers-reduced-motion: reduce) { .apart { transition: none; } }
</style>
