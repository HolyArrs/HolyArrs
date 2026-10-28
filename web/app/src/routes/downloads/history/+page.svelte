<script lang="ts">
	// History: everything HolyArrs did, day by day. The tab bar (All · Came in
	// · Problems · Changes · Routine, with sub-tabs, counts and a title finder)
	// keeps its place in the address (&show=). On wide screens a picked row's
	// whole story shows in a details panel that can be hidden; the poster size
	// and the panel are remembered per member, like the Library's art size.
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { api } from '#lib/api.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { filterParams, readFilter, type HistoryFilter } from '#lib/history.ts';
	import type { TitleView } from '#lib/types.ts';
	import LookBar from '#lib/history-looks/LookBar.svelte';
	import LookDays from '#lib/history-looks/LookDays.svelte';
	import { TabCounts } from '#lib/history-looks/feed.svelte.ts';
	import { ALL, artLabels, loadArtSize, loadDetailsOpen, lookTabs, saveArtSize, saveDetailsOpen, showOf, tabKinds, type ArtSize } from '#lib/history-looks.ts';

	const start = showOf(page.url.searchParams.get('show'));
	let show = $state(start.show);
	let sub = $state(start.sub);
	const kinds = $derived(tabKinds(show, sub));
	const tabNow = $derived(lookTabs.find((t) => t.key === show) ?? lookTabs[0]);

	let filter = $state<HistoryFilter>(readFilter(page.url.searchParams, 50));
	// Every kind is read through the tabs; the old ?kinds= filter does not apply.
	const shownFilter = $derived({ ...filter, groups: [] });
	const tabEmpty = $derived.by(() => {
		const s = tabNow.subs.find((x) => x.key === sub);
		return { title: filter.title || filter.from || filter.to ? 'Nothing matches' : tabNow.empty, text: s?.hint ?? tabNow.hint };
	});
	function showAll() {
		show = ALL;
		sub = '';
		const u = new URL(page.url.href);
		u.searchParams.delete('show');
		replaceState(u, page.state);
	}

	// The title and dates live in the address, so a view can be shared and Back works.
	function apply(change: Partial<HistoryFilter>) {
		filter = { ...filter, offset: 0, ...change };
		const keep = new URLSearchParams(filterParams(filter));
		const v = page.url.searchParams.get('show');
		if (v) keep.set('show', v);
		const q = keep.toString();
		void goto(to('/downloads/history') + (q ? '?' + q : ''), { replace: true, reset: false });
	}

	const counts = new TabCounts();
	$effect(() => {
		void counts.load(shownFilter, show);
	});

	let titles = $state<TitleView[]>([]);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	let episodeName = $state('');
	const titleName = $derived(titles.find((t) => t.id === filter.title)?.name);

	// Poster size and the details panel: remembered per member.
	let myId = $state<number | undefined>();
	let size = $state<ArtSize>('m');
	let panel = $state(true);
	let ready = false;
	$effect(() => {
		const me = myId;
		size = loadArtSize(me);
		panel = loadDetailsOpen(me);
		ready = true;
	});
	function setSize(z: ArtSize) {
		size = z;
		saveArtSize(myId, z);
	}
	$effect(() => {
		const open = panel;
		if (ready) saveDetailsOpen(myId, open);
	});
	const sizeItems = $derived<MenuItem[]>([{ kind: 'heading', label: 'Poster size' }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: artLabels[z], checked: size === z, onselect: () => setSize(z) }))]);
	const panelId = 'history-details';

	// The details panel needs room: from 1024px wide; narrower screens use a sheet.
	let wide = $state(false);
	onMount(() => {
		const mq = window.matchMedia('(min-width: 1024px)');
		const fit = () => (wide = mq.matches);
		fit();
		mq.addEventListener('change', fit);
		void api
			.get<{ titles: TitleView[] }>('/titles')
			.then((r) => (titles = r.titles))
			.catch(() => {});
		void api
			.get<{ id?: number }>('/auth/me')
			.then((me) => (myId = me.id))
			.catch(() => {});
		return () => mq.removeEventListener('change', fit);
	});
</script>

<svelte:head><title>History · HolyArrs</title></svelte:head>

<PageHeader title="History" subtitle="Everything HolyArrs did, newest first: what it added, searched, chose, sent, imported, renamed and deleted, and why." />

<LookBar bind:show bind:sub counts={counts.tabs} subCounts={counts.subs} filter={shownFilter} {titles} {episodeName} onfilter={apply}>
	{#snippet tools()}
		<Menu label="Art size" variant="button" text={artLabels[size].replace(' posters', '')} icon="chevronDown" items={sizeItems} width={200} align="start" />
		{#if wide}<Button variant="secondary" size="md" icon="sidebar" aria-expanded={panel} aria-controls={panelId} data-testid="history-details-toggle" onclick={() => (panel = !panel)}>{panel ? 'Hide details' : 'Show details'}</Button>{/if}
	{/snippet}
</LookBar>

{#if filter.episode}
	<p class="episode" data-testid="episode-filter">
		<span>Only {episodeName || 'one episode'}{titleName ? ` of ${titleName}` : ''}.</span>
		<Button variant="plain" size="sm" onclick={() => apply({ episode: undefined })}>All of the show</Button>
	</p>
{/if}

<LookDays filter={shownFilter} {kinds} {titleById} empty={tabEmpty} onall={show !== ALL ? showAll : undefined} {size} {wide} bind:panel {panelId} bind:episodeName />

<style>
	.episode { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin: -6px 0 12px; color: var(--text-muted); font: var(--t-callout); }
</style>
