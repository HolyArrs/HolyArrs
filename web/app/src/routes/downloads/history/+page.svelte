<script lang="ts">
	import { libraryTitles, ownedLibrary } from '#lib/owned.ts';
	import { session } from '#lib/session.svelte.ts';
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
	import DlTurnedDown from '#lib/downloads/DlTurnedDown.svelte';
	import { ALL, artLabels, loadArtSize, loadDetailsOpen, lookTabs, saveArtSize, saveDetailsOpen, showOf, tabKinds, type ArtSize } from '#lib/history-looks.ts';

	// Two views: the history feed, and the copies HolyArrs turned down (?tab=turned-down).
	let view = $state<'feed' | 'turned-down'>(page.url.searchParams.get('tab') === 'turned-down' ? 'turned-down' : 'feed');
	function pickView(v: 'feed' | 'turned-down') {
		view = v;
		const u = new URL(page.url.href);
		if (v === 'feed') u.searchParams.delete('tab');
		else u.searchParams.set('tab', v);
		replaceState(u, page.state);
	}

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
	let people = $state<Map<number, string>>(new Map());
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
		void libraryTitles()
			.then((r) => (titles = r))
			.catch(() => {});
		myId = session.s.id;
		// Member names for "Added to the Library by Maya" (only admins may list them).
		void api
			.get<{ users: { id: number; name: string }[] }>('/users')
			.then((r) => (people = new Map(r.users.map((u) => [u.id, u.name]))))
			.catch(() => {});
		return () => mq.removeEventListener('change', fit);
	});
</script>

<svelte:head><title>History · HolyArrs</title></svelte:head>

<PageHeader title="History" />

<div class="views" role="tablist" aria-label="History views" data-testid="history-views">
	<button type="button" role="tab" class="vw" aria-selected={view === 'feed'} onclick={() => pickView('feed')} data-testid="history-view-feed">Everything</button>
	<button type="button" role="tab" class="vw" aria-selected={view === 'turned-down'} onclick={() => pickView('turned-down')} data-testid="history-view-turned-down">Turned down</button>
</div>

{#if view === 'turned-down'}
<DlTurnedDown embedded />
{:else}

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

<LookDays filter={shownFilter} {kinds} {titleById} empty={tabEmpty} onall={show !== ALL ? showAll : undefined} {size} {wide} bind:panel {panelId} bind:episodeName {people} me={myId} />

{/if}

<style>
	.episode { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin: -6px 0 12px; color: var(--text-muted); font: var(--t-callout); }
	.views { display: inline-flex; gap: 2px; margin: 0 0 12px; padding: 2px; border-radius: var(--r-pill); background: var(--surface-2); }
	.vw { height: 28px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.vw:hover { color: var(--text); }
	.vw[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.vw:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (prefers-reduced-motion: reduce) { .vw { transition: none; } }
</style>
