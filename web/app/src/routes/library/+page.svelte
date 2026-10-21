<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
	import { bytes, date } from '#lib/format.ts';
	import { exceptionChip, filters, posterStatus, firstOfLetter, kinds, letterOf, letters, matchesTitleQuery, missingEpisodes, ofKind, titleQuery, type Filter, type Kind } from '#lib/library.ts';
	import {
		activeCount, columnSort, sortName, filterCounts, loadColumns, loadSaved, noFilter, passes, progressOf, releaseDate, saveColumns, saveSaved, sortLabels, sortTitles, sortsFor, startDir, columnsFor,
		type Column, type FilterContext, type FilterState, type Kept, type LibKind, type SavedFilter, type SortDir, type SortKey
	} from '#lib/library-table.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import { episodeLabel, showProgress } from '#lib/shows.ts';
	import { artistProgress } from '#lib/music.ts';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import BannerArt from '#lib/ui/BannerArt.svelte';
	import ColumnChooser from '#lib/ui/ColumnChooser.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { artName, slotKind } from '#lib/title-page.ts';
	import { peopleNames, plainReason } from '#lib/plain.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { audienceOf, audienceStatus } from '#lib/title-status.ts';
	import type { EpisodeView, TitleView } from '#lib/types.ts';
	import type { MovieCollection, CollectionDetail } from '#lib/collections.ts';
	import { ownedMark, type ExploreCard } from '#lib/explore.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { deleteBody } from '#lib/organise.ts';
	import { bannerHeight, bannerWidth, densityLabels, loadDensity, loadPosterSize, posterLabels, posterMin, saveDensity, savePosterSize, type Density, type PosterSize } from '#lib/library-art.ts';

	type View = 'table' | 'posters' | 'overview';

	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let searchError = $state('');
	let indexedIds = $state<Set<number>>(new Set());
	let mounted = $state(false);
	let loadRevision = $state(0);
	let searchRun = 0;
	let searchPending = $state(false);
	let restoreScroll = $state<{ x: number; y: number } | null>(null);
	let lastOpenedId = $state<number | null>(null);
	let myId = $state<number | undefined>();

	// ── URL state: kind (from the sidebar), filters, sort, search, grouping ──
	function params() {
		return (page.shallow?.url ?? page.url).searchParams;
	}
	function readKind(v: string | null): Kind {
		return kinds.some(([k]) => k === v) ? (v as Kind) : 'all';
	}
	function readFilter(p: Pick<URLSearchParams, 'get'>): FilterState {
		const status = p.get('show') as Filter;
		const kept = p.get('kept') as Kept;
		const asker = p.get('requester');
		const series = Number(p.get('series'));
		return {
			status: filters.some(([f]) => f === status) ? status : 'all',
			kept: kept === 'on' || kept === 'off' ? kept : 'any',
			asker: asker === 'me' || (asker && /^[1-9]\d*$/.test(asker)) ? asker : 'anyone',
			series: Number.isSafeInteger(series) && series > 0 ? series : 0
		};
	}
	function readSort(p: Pick<URLSearchParams, 'get'>, k: Kind): { key: SortKey; dir: SortDir } {
		const key = p.get('sort') as SortKey;
		const ok = sortsFor(k === 'all' ? 'all' : k).includes(key) ? key : 'name';
		const dir = p.get('dir');
		return { key: ok, dir: dir === 'asc' || dir === 'desc' ? dir : startDir(ok) };
	}
	let kind = $state<Kind>(readKind(page.url.searchParams.get('kind')));
	let filter = $state<FilterState>(readFilter(page.url.searchParams));
	let sort = $state(readSort(page.url.searchParams, readKind(page.url.searchParams.get('kind'))));
	let q = $state(page.url.searchParams.get('q')?.slice(0, 256) ?? '');
	let groupSeries = $state(page.url.searchParams.get('group') === 'series');
	$effect(() => {
		const p = params();
		kind = readKind(p.get('kind'));
		filter = readFilter(p);
		sort = readSort(p, kind);
		q = p.get('q')?.slice(0, 256) ?? '';
		groupSeries = p.get('group') === 'series';
	});
	function syncUrl() {
		const url = new URL((page.shallow?.url ?? page.url).href);
		const set = (k: string, v: string | null) => (v ? url.searchParams.set(k, v) : url.searchParams.delete(k));
		set('kind', kind === 'all' ? null : kind);
		set('show', filter.status === 'all' ? null : filter.status);
		set('kept', filter.kept === 'any' ? null : filter.kept);
		set('requester', filter.asker === 'anyone' ? null : filter.asker);
		set('series', filter.series ? String(filter.series) : null);
		set('q', q || null);
		set('sort', sort.key === 'name' ? null : sort.key);
		set('dir', sort.dir === startDir(sort.key) ? null : sort.dir);
		set('group', groupSeries ? 'series' : null);
		url.searchParams.delete('layout');
		void goto(url, { replace: true, shallow: true, reset: false });
	}
	const libKind = $derived<LibKind>(kind === 'all' ? 'movie' : (kind as LibKind));
	const query = $derived(titleQuery(q));
	const who = $derived(audienceOf((p) => session.can(p)));
	const canManage = $derived(session.can('manage_library') && !session.kid);
	const moviesOn = $derived(partsNow.answer?.parts.find((part) => part.part === 'movies')?.on !== false);
	const kindLabel: Record<string, string> = { movie: 'Movie', show: 'Show', artist: 'Artist' };
	const kindTitle = $derived(kind === 'movie' ? 'Movies' : kind === 'show' ? 'TV shows' : kind === 'artist' ? 'Music' : 'Library');

	// ── View: remembered per library; ?layout= from older links still works ──
	function readView(k: Kind, fromUrl = false): View {
		const asked = fromUrl ? page.url.searchParams.get('layout') : null;
		if (asked === 'list' || asked === 'table') return 'table';
		if (asked === 'grid' || asked === 'posters') return 'posters';
		try {
			const v = localStorage.getItem(`holyarrs.library-layout.${k}`);
			return v === 'posters' || v === 'overview' ? v : 'table';
		} catch {
			return 'table';
		}
	}
	let view = $state<View>(untrack(() => readView(kind, true)));
	let viewKind = untrack(() => kind);
	$effect(() => {
		if (kind !== viewKind) {
			viewKind = kind;
			view = readView(kind);
		}
	});
	function setView(v: View) {
		view = v;
		try {
			localStorage.setItem(`holyarrs.library-layout.${kind}`, v);
		} catch { /* Not remembered in a private window. */ }
	}

	// ── Columns: per member and library ──
	let listWidth = $state(1000);
	let columns = $state<Column[]>(columnsFor('movie'));
	$effect(() => {
		const k = libKind;
		const me = myId;
		columns = loadColumns(me, k);
	});
	const chosenCols = $derived(columns.filter((c) => c.on));
	// Columns that do not fit the width wait until the window is wider (the
	// banner and the title always stay); the header says how many wait.
	// The art is the hero of each row: the banner column takes its share of
	// the width (by density) and its 1000x185 shape sets the row height.
	// Columns that do not fit scroll sideways rather than squeeze the art.
	let density = $state<Density>('large');
	let posterSize = $state<PosterSize>('m');
	$effect(() => {
		const me = myId;
		density = loadDensity(me);
		posterSize = loadPosterSize(me);
	});
	function setDensity(d: Density) {
		density = d;
		saveDensity(myId, d);
	}
	function setPosterSize(z: PosterSize) {
		posterSize = z;
		savePosterSize(myId, z);
	}
	const bannerW = $derived(bannerWidth(listWidth, density));
	const bannerH = $derived(bannerHeight(bannerW));
	const shownCols = $derived(chosenCols);
	const gridCols = $derived(`${bannerW}px 28px minmax(220px, 1fr) ${kind === 'all' ? '' : shownCols.map((c) => c.width).join(' ')}`);
	const sizeItems = $derived<MenuItem[]>(
		view === 'posters'
			? [{ kind: 'heading', label: 'Poster size' }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: posterLabels[z], checked: posterSize === z, onselect: () => setPosterSize(z) }))]
			: [{ kind: 'heading', label: 'Banner size' }, ...(['large', 'comfortable', 'compact'] as const).map((d) => ({ label: densityLabels[d], checked: density === d, onselect: () => setDensity(d) }))]
	);
	function setColumns(c: Column[]) {
		columns = c;
		saveColumns(myId, libKind, c);
	}

	// ── Movie series: which titles belong to which series ──
	let series = $state<MovieCollection[]>([]);
	let seriesOfTitle = $state<Map<number, number>>(new Map());
	let seriesLoaded = false;
	async function loadSeries() {
		if (seriesLoaded || !moviesOn) return;
		seriesLoaded = true;
		try {
			const list = (await api.get<{ collections: MovieCollection[] }>('/collections')).collections;
			const details = await Promise.all(list.map((c) => api.get<CollectionDetail>(`/collections/${c.id}`).catch(() => null)));
			const map = new Map<number, number>();
			for (const d of details) for (const t of d?.titles ?? []) map.set(t.id, d!.id);
			series = list;
			seriesOfTitle = map;
		} catch {
			seriesLoaded = false;
		}
	}
	$effect(() => {
		if (mounted && (kind === 'movie' || kind === 'all')) void loadSeries();
	});
	const seriesName = (id: number) => series.find((s) => s.id === id)?.name ?? 'Movie series';

	// ── Filtering and sorting ──
	function linkedPeople(t: TitleView) {
		const people = new Map<number, { id: number; name: string }>();
		for (const ask of t.asks ?? []) {
			if (ask.title_id !== t.id) continue;
			for (const person of [ask.asked_by, ...ask.people]) if (person.name.trim()) people.set(person.id, person);
		}
		return [...people.values()];
	}
	const ctx = $derived<FilterContext>({ askers: (t) => linkedPeople(t).map((p) => p.id), me: myId, seriesOf: (t) => seriesOfTitle.get(t.id) });
	const ofThisKind = $derived(titles.filter((t) => ofKind(t, kind) && matchesTitleQuery(t, query, indexedIds)));
	const counts = $derived(filterCounts(ofThisKind, filter, ctx));
	const shown = $derived(sortTitles(ofThisKind.filter((t) => passes(t, filter, ctx)), sort.key, sort.dir));
	const askers = $derived([...new Map(ofThisKind.flatMap((t) => linkedPeople(t)).map((p) => [p.id, p])).values()].sort((a, b) => a.name.localeCompare(b.name)));
	const filtersOn = $derived(activeCount(filter));

	function setFilter(patch: Partial<FilterState>) {
		filter = { ...filter, ...patch };
		syncUrl();
	}
	function clearFilters() {
		filter = { ...noFilter };
		q = '';
		syncUrl();
	}
	function setSort(key: SortKey, dir?: SortDir) {
		sort = { key, dir: dir ?? (sort.key === key ? (sort.dir === 'asc' ? 'desc' : 'asc') : startDir(key)) };
		syncUrl();
	}

	// Saved filters, per member and library.
	let saved = $state<SavedFilter[]>([]);
	$effect(() => {
		saved = loadSaved(myId, kind);
	});
	let saveDialog = $state<HTMLDialogElement | null>(null);
	let saveName = $state('');
	function saveCurrent() {
		const name = saveName.trim().slice(0, 60);
		if (!name) return;
		saved = [...saved.filter((s) => s.name !== name), { name, filter: { ...filter } }];
		saveSaved(myId, kind, saved);
		saveDialog?.close();
		toasts.push(`Saved the filter "${name}".`, 'ok');
	}
	function forget(name: string) {
		saved = saved.filter((s) => s.name !== name);
		saveSaved(myId, kind, saved);
	}
	const sameFilter = (a: FilterState, b: FilterState) => a.status === b.status && a.kept === b.kept && a.asker === b.asker && a.series === b.series;

	const filterItems = $derived.by<MenuItem[]>(() => {
		const items: MenuItem[] = [{ kind: 'heading', label: 'Status' }];
		for (const [f, label] of filters) items.push({ label, hint: String(counts.status[f]), checked: filter.status === f, onselect: () => setFilter({ status: f }) });
		items.push({ kind: 'separator' }, { kind: 'heading', label: 'Keep up to date' });
		items.push({ label: 'Kept up to date', hint: String(counts.kept.on), checked: filter.kept === 'on', onselect: () => setFilter({ kept: filter.kept === 'on' ? 'any' : 'on' }) });
		items.push({ label: 'Not kept up to date', hint: String(counts.kept.off), checked: filter.kept === 'off', onselect: () => setFilter({ kept: filter.kept === 'off' ? 'any' : 'off' }) });
		if (askers.length || myId) {
			items.push({ kind: 'separator' }, { kind: 'heading', label: 'Asked by' });
			items.push({ label: 'Anyone', checked: filter.asker === 'anyone', onselect: () => setFilter({ asker: 'anyone' }) });
			if (myId) items.push({ label: 'Me', hint: String(counts.askers.get(myId) ?? 0), checked: filter.asker === 'me', onselect: () => setFilter({ asker: 'me' }) });
			for (const p of askers) if (p.id !== myId) items.push({ label: p.name, hint: String(counts.askers.get(p.id) ?? 0), checked: filter.asker === String(p.id), onselect: () => setFilter({ asker: String(p.id) }) });
		}
		if ((kind === 'movie' || kind === 'all') && series.length) {
			items.push({ kind: 'separator' }, { kind: 'heading', label: 'Movie series' });
			items.push({ label: 'Every movie', checked: !filter.series, onselect: () => setFilter({ series: 0 }) });
			for (const s of series.toSorted((a, b) => a.name.localeCompare(b.name))) items.push({ label: s.name, hint: String(counts.series.get(s.id) ?? 0), checked: filter.series === s.id, onselect: () => setFilter({ series: s.id }) });
		}
		items.push({ kind: 'separator' }, { kind: 'heading', label: 'Saved filters' });
		for (const s of saved) items.push({ label: s.name, icon: 'star', checked: sameFilter(s.filter, filter), onselect: () => setFilter(s.filter) });
		items.push({ label: 'Save this filter…', icon: 'plus', disabled: !filtersOn, onselect: () => { saveName = ''; saveDialog?.showModal(); } });
		if (saved.length) items.push({ label: 'Forget saved filters', icon: 'trash', onselect: () => { for (const s of saved) forget(s.name); } });
		return items;
	});
	const sortItems = $derived<MenuItem[]>([
		...sortsFor(kind === 'all' ? 'all' : kind).map((k): MenuItem => ({ label: sortLabels[k], checked: sort.key === k, onselect: () => setSort(k, startDir(k)) })),
		{ kind: 'separator' },
		{ label: 'Ascending', icon: 'chevronUpDown', checked: sort.dir === 'asc', onselect: () => setSort(sort.key, 'asc') },
		{ label: 'Descending', icon: 'chevronUpDown', checked: sort.dir === 'desc', onselect: () => setSort(sort.key, 'desc') },
		...(kind === 'movie' ? ([{ kind: 'separator' }, { label: 'Group by series', icon: 'grid', checked: groupSeries, onselect: () => { groupSeries = !groupSeries; syncUrl(); } }] as MenuItem[]) : [])
	]);
	const addItems = $derived<MenuItem[]>([
		...(partOn('movies') ? ([{ label: 'Add a movie', icon: 'film', href: to('/search') }] as MenuItem[]) : []),
		...(partOn('shows') ? ([{ label: 'Add a show', icon: 'tv', href: to('/search?kind=show') }] as MenuItem[]) : []),
		...(partOn('music') ? ([{ label: 'Add an artist', icon: 'music', href: to('/search?kind=music') }] as MenuItem[]) : []),
		{ kind: 'separator' },
		{ kind: 'heading', label: 'Library tools' },
		{ label: 'People', href: to('/people') },
		{ label: 'Held files', href: to('/library/held') },
		{ label: 'Turned down releases', href: to('/library/turned-down') },
		...(partOn('subtitles') ? ([{ label: 'Missing subtitles', href: to('/library/missing-subtitles') }] as MenuItem[]) : []),
		{ label: 'File codecs', href: to('/library/copies') },
		{ label: 'Rename files', href: to('/library/rename') },
		{ label: 'Import a library', href: to('/library/import') }
	]);
	function partOn(name: string) {
		return partsNow.answer?.parts.find((part) => part.part === name)?.on !== false;
	}

	// ── Entries: rows, with series headers when grouped ──
	type Entry = { type: 'head'; key: string; id: number; name: string; count: number } | { type: 'row'; key: string; t: TitleView };
	const grouped = $derived(groupSeries && kind === 'movie');
	const entries = $derived.by<Entry[]>(() => {
		if (!grouped) return shown.map((t) => ({ type: 'row', key: `r${t.id}`, t }));
		const bySeries = new Map<number, TitleView[]>();
		const loose: TitleView[] = [];
		for (const t of shown) {
			const s = seriesOfTitle.get(t.id);
			if (s) bySeries.set(s, [...(bySeries.get(s) ?? []), t]);
			else loose.push(t);
		}
		const out: Entry[] = [];
		for (const [id, ts] of [...bySeries].sort((a, b) => seriesName(a[0]).localeCompare(seriesName(b[0])))) {
			out.push({ type: 'head', key: `h${id}`, id, name: seriesName(id), count: ts.length });
			for (const t of ts) out.push({ type: 'row', key: `r${t.id}`, t });
		}
		if (loose.length) {
			out.push({ type: 'head', key: 'h0', id: 0, name: 'Not in a series', count: loose.length });
			for (const t of loose) out.push({ type: 'row', key: `r${t.id}`, t });
		}
		return out;
	});
	const rowEntries = $derived(entries.filter((e): e is Extract<Entry, { type: 'row' }> => e.type === 'row'));

	// ── Virtual list: only rows near the screen are drawn ──
	let listEl = $state<HTMLElement | null>(null);
	let scrollY = $state(0);
	let viewH = $state(900);
	let listTop = $state(0);
	const heights = new SvelteMap<string, number>();
	const phone = $derived(listWidth < 640);
	const guess = $derived(view === 'overview' ? 168 : phone ? Math.round((listWidth - 24) * 185 / 1000) + 92 : bannerH);
	const offsets = $derived.by(() => {
		const out = new Array<number>(entries.length + 1);
		out[0] = 0;
		for (let i = 0; i < entries.length; i++) out[i + 1] = out[i] + (heights.get(entries[i].key) ?? (entries[i].type === 'head' ? 52 : guess));
		return out;
	});
	const virtual = $derived(entries.length > 80);
	function indexAt(y: number) {
		let lo = 0;
		let hi = entries.length;
		while (lo < hi) {
			const mid = (lo + hi) >> 1;
			if (offsets[mid + 1] <= y) lo = mid + 1;
			else hi = mid;
		}
		return lo;
	}
	const range = $derived.by(() => {
		if (!virtual) return { start: 0, end: entries.length };
		const top = scrollY - listTop;
		return { start: Math.max(0, indexAt(top - 800)), end: Math.min(entries.length, indexAt(top + viewH + 800) + 1) };
	});
	const slice = $derived(entries.slice(range.start, range.end));
	function measure(node: HTMLElement, key: string) {
		let k = key;
		const ro = new ResizeObserver(() => {
			const h = node.offsetHeight;
			if (h && heights.get(k) !== h) heights.set(k, h);
		});
		ro.observe(node);
		return {
			update(next: string) {
				k = next;
			},
			destroy() {
				ro.disconnect();
			}
		};
	}
	function readTop() {
		if (listEl) listTop = listEl.getBoundingClientRect().top + window.scrollY;
	}
	$effect(() => {
		listEl;
		view;
		void tick().then(readTop);
	});

	// ── Keyboard: arrows move, Space selects, Enter opens ──
	let active = $state(0);
	async function focusRow(i: number) {
		const n = rowEntries.length;
		if (!n) return;
		active = Math.max(0, Math.min(n - 1, i));
		const key = rowEntries[active].key;
		let el = listEl?.querySelector<HTMLElement>(`[data-key="${key}"]`);
		if (!el) {
			const at = entries.findIndex((e) => e.key === key);
			window.scrollTo({ top: listTop + offsets[at] - 120, behavior: 'instant' });
			scrollY = window.scrollY;
			await tick();
			el = listEl?.querySelector<HTMLElement>(`[data-key="${key}"]`);
		}
		el?.focus({ preventScroll: false });
		el?.scrollIntoView({ block: 'nearest' });
	}
	function rowKey(e: KeyboardEvent, t: TitleView) {
		if (e.target !== e.currentTarget) return;
		const i = rowEntries.findIndex((r) => r.t.id === t.id);
		if (e.key === 'ArrowDown') { e.preventDefault(); void focusRow(i + 1); }
		else if (e.key === 'ArrowUp') { e.preventDefault(); void focusRow(i - 1); }
		else if (e.key === 'Home') { e.preventDefault(); void focusRow(0); }
		else if (e.key === 'End') { e.preventDefault(); void focusRow(rowEntries.length - 1); }
		else if (e.key === ' ' && canManage) {
			e.preventDefault();
			if (!selecting) startSelecting(t.id);
			else togglePick(t.id);
		} else if (e.key === 'Enter') { e.preventDefault(); void goto(titleLink(t.id)); }
	}
	function rowClick(e: MouseEvent, t: TitleView) {
		const target = e.target as HTMLElement;
		if (target.closest('a, button, input, label, [role="menu"]')) return;
		if (selecting) togglePick(t.id);
		else void goto(titleLink(t.id));
	}

	// ── Selecting and the bulk bar ──
	let selecting = $state(false);
	let picked = $state<Set<number>>(new Set());
	let busy = $state(false);
	let deleteIds = $state<number[]>([]);
	let deleteDialog = $state<HTMLDialogElement | null>(null);
	let rules = $state<{ id: number; name: string; kinds: string[] }[]>([]);
	const pickedShown = $derived(shown.filter((t) => picked.has(t.id)));
	const allPicked = $derived(shown.length > 0 && pickedShown.length === shown.length);
	const deleteWord = $derived(deleteIds.length === 1 ? (titles.find((t) => t.id === deleteIds[0])?.name ?? '1 title') : `${deleteIds.length} titles`);
	function togglePick(id: number) {
		const next = new Set(picked);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		picked = next;
	}
	function startSelecting(id?: number) {
		selecting = true;
		picked = new Set(id === undefined ? [] : [id]);
		if (!rules.length) void api.get<{ pick_rules: { id: number; name: string; kinds: string[] }[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
	}
	function stopSelecting() {
		selecting = false;
		picked = new Set();
	}
	function pickAll() {
		picked = allPicked ? new Set() : new Set(shown.map((t) => t.id));
	}
	async function eachTitle(ids: number[], run: (id: number) => Promise<unknown>): Promise<number> {
		let failed = 0;
		for (const id of ids) {
			try {
				await run(id);
			} catch {
				failed++;
			}
		}
		return failed;
	}
	function report(done: string, failed: number, total: number) {
		if (failed) toasts.push(`${total - failed} of ${total} ${done}. ${failed} could not be changed.`, 'fail');
		else toasts.push(`${total} ${total === 1 ? 'title' : 'titles'} ${done}.`, 'ok');
	}
	async function setKeep(ids: number[], keep: boolean) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.put(`/titles/${id}`, { keep_up_to_date: keep }));
		busy = false;
		report(keep ? 'kept up to date' : 'no longer kept up to date', failed, ids.length);
		await load();
	}
	async function setRules(ids: number[], rule: { id: number; name: string }) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.put(`/titles/${id}`, { pick_rules_id: rule.id }));
		busy = false;
		report(`now use ${rule.name}`, failed, ids.length);
		await load();
	}
	async function searchTitles(ids: number[]) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.post(`/titles/${id}/search`, {}));
		busy = false;
		report('searching now', failed, ids.length);
	}
	async function refreshTitle(t: TitleView) {
		try {
			await api.post(`/titles/${t.id}/refresh`, {});
			toasts.push(`Refreshing ${t.name}.`, 'ok');
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		}
	}
	function askDelete(ids: number[]) {
		if (!ids.length) return;
		deleteIds = ids;
		deleteDialog?.showModal();
	}
	async function confirmDelete() {
		const ids = deleteIds;
		busy = true;
		// Files stay on disk; the title page offers the file and Never-add choices.
		const failed = await eachTitle(ids, (id) => api.post(`/titles/${id}/delete`, deleteBody({}, false, false, true)));
		busy = false;
		deleteDialog?.close();
		report('removed, files kept on disk', failed, ids.length);
		stopSelecting();
		await load();
	}
	const rulesItems = $derived<MenuItem[]>(
		rules.filter((r) => !r.kinds.length || kind === 'all' || r.kinds.includes(kind)).map((r): MenuItem => ({ label: r.name, onselect: () => setRules(pickedShown.map((t) => t.id), r) }))
	);
	function selectKey(e: KeyboardEvent) {
		if (selecting && e.key === 'Escape' && !deleteDialog?.open) stopSelecting();
	}

	// ── A to Z: only while sorted by name ──
	// Letters follow the sort, which skips a leading The, A or An.
	const jumps = $derived(firstOfLetter(shown.map((t) => ({ id: t.id, name: sortName(t.name) }))));
	const railOn = $derived(sort.key === 'name' && !grouped && shown.length > 12);
	let railFocus = $state('A');
	let rail = $state<HTMLElement | null>(null);
	async function jump(letter: string) {
		let id = jumps.get(letter);
		if (id === undefined) return;
		if (sort.dir === 'desc') id = [...shown].reverse().find((t) => letterOf(sortName(t.name)) === letter)?.id ?? id;
		railFocus = letter;
		const at = entries.findIndex((e) => e.type === 'row' && e.t.id === id);
		if (at < 0) return;
		if (view === 'posters') {
			document.querySelector<HTMLElement>(`[data-title-id="${id}"]`)?.scrollIntoView({ block: 'start', behavior: reducedMotion() ? 'auto' : 'smooth' });
			return;
		}
		window.scrollTo({ top: listTop + offsets[at] - 120, behavior: reducedMotion() || virtual ? 'instant' : 'smooth' });
		scrollY = window.scrollY;
		active = rowEntries.findIndex((r) => r.t.id === id);
	}
	function railKey(e: KeyboardEvent) {
		const on = letters.filter((l) => jumps.has(l));
		const i = on.indexOf(railFocus);
		let next = '';
		if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next = on[Math.min(on.length - 1, i + 1)];
		else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') next = on[Math.max(0, i - 1)];
		else if (e.key === 'Home') next = on[0];
		else if (e.key === 'End') next = on[on.length - 1];
		if (!next) return;
		e.preventDefault();
		railFocus = next;
		rail?.querySelector<HTMLElement>(`[data-letter="${next === '#' ? 'num' : next}"]`)?.focus();
	}
	$effect(() => {
		if (!jumps.has(railFocus)) railFocus = letters.find((l) => jumps.has(l)) ?? 'A';
	});

	// ── Cells ──
	function titleLink(id: number): string {
		const url = page.shallow?.url ?? page.url;
		return to(`/library/${id}?returnTo=${encodeURIComponent(url.pathname + url.search)}`);
	}
	function requestedBy(t: TitleView): string {
		return peopleNames(linkedPeople(t).map((p) => p.name));
	}
	function progressText(t: TitleView): string {
		if (t.kind === 'show') return showProgress(t.show);
		if (t.kind === 'artist') return artistProgress(t.artist);
		return '';
	}
	/** Bar colour: all in, some missing, nothing yet, or only future ones. */
	function tone(t: TitleView): 'full' | 'gap' | 'none' | 'later' {
		const p = progressOf(t);
		const s = t.kind === 'show' ? t.show : t.kind === 'artist' ? t.artist : undefined;
		if (p.total === 0) return 'later';
		if (p.have >= p.total) return 'full';
		if (!t.keep_up_to_date && p.have === 0) return 'none';
		return s && s.missing === 0 && p.have < p.total ? 'later' : 'gap';
	}
	function movieOut(t: TitleView): string {
		const now = Date.now();
		if (t.digital_date) return Date.parse(t.digital_date) <= now ? `Out ${date(t.digital_date)}` : `Home release ${date(t.digital_date)}`;
		if (t.cinema_date) return Date.parse(t.cinema_date) <= now ? `In cinemas ${date(t.cinema_date)}` : `Cinemas ${date(t.cinema_date)}`;
		return date(releaseDate(t)) || 'Not known';
	}
	function cell(t: TitleView, c: Column): string {
		switch (c.id) {
			case 'network': return t.show?.network ?? '';
			case 'rules': return t.pick_rules_name ?? '';
			case 'next': return t.show?.next_air_date ? date(t.show.next_air_date) : t.show?.status === 'ended' ? 'Ended' : '';
			case 'last': return date(t.show?.last_air_date);
			case 'seasons': return t.show?.season_count ? String(t.show.season_count) : '';
			case 'added': return date(t.added_at);
			case 'year': return t.year ? String(t.year) : '';
			case 'size': return t.size_bytes ? bytes(t.size_bytes) : '';
			case 'genres': return (t.genres ?? []).slice(0, 3).join(', ');
			case 'rating': return t.rating ?? '';
			case 'path': return t.folder_path ?? '';
			case 'release': return t.kind === 'movie' ? movieOut(t) : '';
			case 'runtime': return t.runtime_minutes ? `${Math.floor(t.runtime_minutes / 60)}h ${t.runtime_minutes % 60}m` : '';
			case 'series': {
				const s = seriesOfTitle.get(t.id);
				return s ? seriesName(s) : '';
			}
			case 'albums': return t.artist ? `${t.artist.included}/${t.artist.albums}` : '';
			case 'latest': return date(t.artist?.next_release);
			default: return '';
		}
	}
	function posterLines(t: TitleView): string[] {
		const lines = [t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'];
		if (t.pick_rules_name) lines.push(t.pick_rules_name);
		if (t.kind === 'show' && t.show?.next_air_date) lines.push(`Next ${date(t.show.next_air_date)}`);
		else if (t.added_at) lines.push(`Added ${date(t.added_at)}`);
		return lines;
	}
	function markOf(t: TitleView) {
		return ownedMark({ key: String(t.id), kind: t.kind === 'show' ? 'show' : t.kind === 'artist' ? 'artist' : 'movie', name: t.name, titleId: t.id } as ExploreCard, t, who);
	}
	const isProgressCol = (c: Column) => c.id === 'episodes' || c.id === 'tracks';

	// Missing view: a show folds its missing episodes under one row.
	let foldOpen = $state<number | null>(null);
	let missingOf = $state<Record<number, EpisodeView[] | 'loading' | 'error'>>({});
	async function toggleMissing(t: TitleView) {
		foldOpen = foldOpen === t.id ? null : t.id;
		if (foldOpen !== t.id || Array.isArray(missingOf[t.id])) return;
		missingOf[t.id] = 'loading';
		try {
			const r = await api.get<{ episodes: EpisodeView[] }>(`/titles/${t.id}/episodes`);
			missingOf[t.id] = missingEpisodes(r.episodes);
		} catch {
			missingOf[t.id] = 'error';
		}
	}

	// ── Loading, search and scroll memory ──
	$effect(() => {
		loadRevision;
		if (!mounted) return;
		const run = ++searchRun;
		indexedIds = new Set();
		searchError = '';
		searchPending = !!query;
		if (!query) return;
		const controller = new AbortController();
		const timer = setTimeout(async () => {
			try {
				const found = await api.get<{ titles: TitleView[] }>(`/titles?q=${encodeURIComponent(query)}`, controller.signal);
				if (run === searchRun) indexedIds = new Set(found.titles.map((t) => t.id));
			} catch (e) {
				if (!controller.signal.aborted && run === searchRun) searchError = message(e);
			} finally {
				if (run === searchRun) searchPending = false;
			}
		}, 200);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	});
	function scrollKey() {
		const url = page.shallow?.url ?? page.url;
		return `${url.pathname}${url.search}|${view}`;
	}
	beforeNavigate(({ to: target }) => {
		if (!target || target.url.pathname === page.url.pathname) return;
		try {
			const suffix = target.url.pathname.slice(page.url.pathname.length + 1);
			const titleId = target.url.pathname.startsWith(`${page.url.pathname}/`) && /^\d+$/.test(suffix) ? Number(suffix) : lastOpenedId;
			const saved = { key: scrollKey(), x: window.scrollX, y: window.scrollY, titleId };
			sessionStorage.setItem('holyarrs.preview.library-scroll', JSON.stringify(saved));
			const url = page.shallow?.url ?? page.url;
			const views = JSON.parse(sessionStorage.getItem('holyarrs.preview.library-views') ?? '{}');
			views[kind] = { ...saved, href: url.pathname + url.search };
			sessionStorage.setItem('holyarrs.preview.library-views', JSON.stringify(views));
		} catch { /* Scroll memory is optional when storage is unavailable. */ }
	});
	$effect(() => {
		if (!loaded || searchPending || !restoreScroll) return;
		const position = restoreScroll;
		restoreScroll = null;
		void tick().then(() => requestAnimationFrame(() => window.scrollTo({ left: position.x, top: position.y, behavior: 'instant' })));
	});
	async function load() {
		try {
			const [library, me] = await Promise.all([api.get<{ titles: TitleView[] }>('/titles'), api.get<{ id?: number }>('/auth/me')]);
			titles = library.titles;
			myId = me.id;
			error = '';
			loadRevision++;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	async function retryLoad() {
		loaded = false;
		await load();
	}
	onMount(() => {
		mounted = true;
		try {
			const views = JSON.parse(sessionStorage.getItem('holyarrs.preview.library-views') ?? '{}');
			const saved = views[kind] ?? JSON.parse(sessionStorage.getItem('holyarrs.preview.library-scroll') ?? 'null');
			if (saved?.key === scrollKey() && Number.isFinite(saved.x) && Number.isFinite(saved.y) && saved.x >= 0 && saved.y >= 0) {
				restoreScroll = { x: saved.x, y: saved.y };
				lastOpenedId = Number.isSafeInteger(saved.titleId) && saved.titleId > 0 ? saved.titleId : null;
			}
		} catch { /* Start normally if a saved position is unavailable. */ }
		void load();
		const onResize = () => {
			viewH = window.innerHeight;
			readTop();
		};
		onResize();
		window.addEventListener('resize', onResize);
		const offs = [live.on('title', load), live.on('slot', load), live.on('resync', load)];
		return () => {
			window.removeEventListener('resize', onResize);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>{kindTitle} · HolyArrs</title></svelte:head>
<svelte:window onkeydown={selectKey} bind:scrollY />

<div class="library" class:selecting>
	<PageHeader title={kindTitle} subtitle={loaded && !error ? `${shown.length} ${shown.length === 1 ? 'title' : 'titles'}${filtersOn || q ? ' match' : ''}` : undefined}>
		{#snippet actions()}
			{#if session.can('manage_library')}<Menu label="Add and Library tools" variant="button" text="Add" icon="plus" items={addItems} width={250} />{/if}
		{/snippet}
	</PageHeader>

	<div class="toolbar" role="toolbar" aria-label="Library view">
		<SegmentedControl label="View" value={view} options={[{ value: 'table', label: 'Banners', icon: 'menu' }, { value: 'posters', label: 'Posters', icon: 'grid' }, { value: 'overview', label: 'Overview', icon: 'info' }]} onchange={(v) => setView(v)} />
		{#if view !== 'overview' && !(view === 'table' && phone)}<Menu label="Art size" variant="button" text={view === 'posters' ? posterLabels[posterSize].replace(' posters', '') : densityLabels[density]} icon="chevronDown" items={sizeItems} width={200} align="start" />{/if}
		<Menu label="Sort titles" variant="button" text="{sortLabels[sort.key]} {sort.dir === 'asc' ? '↑' : '↓'}" icon="chevronUpDown" items={sortItems} width={220} align="start" />
		<span class="filter-btn" class:on={filtersOn > 0}>
			<Menu label="Filter" variant="button" text={filtersOn ? `Filter · ${filtersOn}` : 'Filter'} icon="chevronDown" items={filterItems} width={280} align="start" />
		</span>
		<div class="find-box">
			<span class="find-ic" aria-hidden="true"><Icon name="search" size={16} /></span>
			<label class="sr-only" for="find">Find in the Library</label>
			<input id="find" class="input find" type="search" bind:value={q} oninput={(event) => { q = event.currentTarget.value; syncUrl(); }} placeholder="Search {kindTitle === 'Library' ? 'the Library' : kindTitle}" autocomplete="off" maxlength="256" />
		</div>
		<span class="tool-gap"></span>
		{#if view === 'table' && kind !== 'all' && !phone}<ColumnChooser {columns} onchange={setColumns} onreset={() => setColumns(columnsFor(libKind))} />{/if}
		{#if canManage && shown.length}<Button variant={selecting ? 'primary' : 'secondary'} size="md" icon="check" aria-pressed={selecting} onclick={() => (selecting ? stopSelecting() : startSelecting())} data-testid="library-select">{selecting ? 'Done' : 'Select'}</Button>{/if}
	</div>

	{#if filtersOn || q}
		<div class="chips" role="group" aria-label="Active filters">
			{#if filter.status !== 'all'}<Chip label={filters.find(([f]) => f === filter.status)?.[1] ?? ''} selected onclick={() => setFilter({ status: 'all' })} onremove={() => setFilter({ status: 'all' })} />{/if}
			{#if filter.kept !== 'any'}<Chip label={filter.kept === 'on' ? 'Kept up to date' : 'Not kept up to date'} selected onclick={() => setFilter({ kept: 'any' })} onremove={() => setFilter({ kept: 'any' })} />{/if}
			{#if filter.asker !== 'anyone'}<Chip label="Asked by {filter.asker === 'me' ? 'me' : (askers.find((p) => String(p.id) === filter.asker)?.name ?? 'someone')}" selected onclick={() => setFilter({ asker: 'anyone' })} onremove={() => setFilter({ asker: 'anyone' })} />{/if}
			{#if filter.series}<Chip label="Movie series: {seriesName(filter.series)}" selected onclick={() => setFilter({ series: 0 })} onremove={() => setFilter({ series: 0 })} />{/if}
			{#if q}<Chip label="Search: {q}" selected onclick={() => { q = ''; syncUrl(); }} onremove={() => { q = ''; syncUrl(); }} />{/if}
			<Button variant="plain" size="md" onclick={clearFilters}>Clear</Button>
		</div>
	{/if}
	{#if searchError && !error}<p class="search-error" role="alert"><Icon name="alert" size={16} />{plainReason(searchError).text}</p>{/if}

	<div class:has-rail={railOn} class="with-rail">
		<div class="content">
			{#if !loaded}
				{#if view === 'posters'}
					<ul class="grid" style="--poster-min: {posterMin[posterSize]}px" aria-hidden="true">{#each Array(12) as _, i (i)}<li><Skeleton shape="poster" /><Skeleton width="70%" /></li>{/each}</ul>
				{:else}
					<div class="rows" style="--bw: {bannerW}px" aria-hidden="true">{#each Array(8) as _, i (i)}<div class="sk-row"><span class="sk-banner"><Skeleton shape="block" height="100%" /></span><Skeleton width="40%" /><Skeleton width="60%" /></div>{/each}</div>
				{/if}
			{:else if error}
				<LoadError hasData={false} message={plainReason(error).text} onretry={retryLoad} />
			{:else if titles.length === 0}
				<EmptyState icon="library" title="Your Library is empty" text={session.can('manage_library') ? 'Find a movie, a show or an artist on the Search page and add it. HolyArrs then keeps it up to date.' : 'Nothing has been added yet. Look for a movie, a show or an artist on the Search page.'} action={{ label: 'Search for a movie', href: to('/search'), icon: 'search' }} />
				<div class="empty-more">
					<Button href={to('/search?kind=show')}>Search for a show</Button>
					<Button href={to('/search?kind=music')}>Search for an artist</Button>
					{#if session.can('manage_library')}<Button href={to('/library/import')}>Import a library</Button>{/if}
					{#if session.can('settings')}<Button href={to('/settings/library-folders')}>Library folders</Button>{/if}
				</div>
			{:else if shown.length === 0}
				<EmptyState compact icon="search" title="Nothing matches this filter." action={filtersOn || q ? { label: 'Clear', onclick: clearFilters } : undefined} />
			{:else if view === 'posters'}
				{#if grouped}<p class="group-note">Grouped by series in the Banners and Overview views.</p>{/if}
				<ul class="grid" style="--poster-min: {posterMin[posterSize]}px" aria-label="{kindTitle} titles">
					{#each shown as t, i (t.id)}
						{@const p = progressOf(t)}
						{@const requester = requestedBy(t)}
						<li class="item rise" data-testid="library-card" data-title-id={t.id} data-letter={letterOf(sortName(t.name))} class:selected={picked.has(t.id)} class:last-opened={lastOpenedId === t.id} style="--i: {Math.min(i, 8)}">
							<PosterCard title={t.name} href={titleLink(t.id)} image={t.artwork[0]} subtitle={[t.year, t.kind === 'movie' ? '' : kindLabel[t.kind]].filter(Boolean).join(' · ')} kind={t.kind === 'artist' ? 'artist' : t.kind === 'show' ? 'show' : 'movie'} owned={markOf(t)} vtName={artName(t.id)} />
							<span class="meter {tone(t)}" role="img" aria-label={progressText(t) || (p.have ? 'In the Library' : 'Not in the Library yet')}><i style="width: {p.total ? (p.have / p.total) * 100 : 0}%"></i></span>
							<ul class="lines">{#each posterLines(t) as line, j (j)}<li class:kept={j === 0 && t.keep_up_to_date}>{line}</li>{/each}</ul>
							{#if selecting}
								<button type="button" class="pick" aria-pressed={picked.has(t.id)} aria-label="Select {t.name}" onclick={() => togglePick(t.id)} data-testid="library-pick"><span class="pick-dot" aria-hidden="true">{#if picked.has(t.id)}<Icon name="check" size={14} stroke={3} />{/if}</span></button>
							{/if}
							{#if lastOpenedId === t.id || requester}
								<div class="card-notes">
									{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}
									{#if requester}<span class="requester" data-testid="requested-by">Asked by {requester}</span>{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<div class="rows {view}" class:has-select={selecting} style="--cols: {gridCols}" bind:this={listEl} bind:clientWidth={listWidth} role="list" aria-label="{kindTitle} titles">
					{#if view === 'table' && !phone}
						<div class="row head" role="presentation">
							<span class="h banner-h">Banner</span>
							<span></span>
							<button type="button" class="h sort" aria-label="Sort by name" aria-pressed={sort.key === 'name'} onclick={() => setSort('name')}>Title{#if sort.key === 'name'}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
							{#if kind !== 'all'}
								{#each shownCols as c (c.id)}
									{@const sk = columnSort[c.id]}
									{#if sk && sortsFor(libKind).includes(sk)}
										<button type="button" class="h sort" class:num={c.numeric} aria-label="Sort by {c.label}" aria-pressed={sort.key === sk} onclick={() => setSort(sk)}>{c.label}{#if sort.key === sk}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
									{:else}
										<span class="h" class:num={c.numeric}>{c.label}</span>
									{/if}
								{/each}
							{/if}
						</div>
					{/if}
					<div class="body" style:height={virtual ? `${offsets[entries.length]}px` : undefined}>
						<div style:transform={virtual ? `translateY(${offsets[range.start]}px)` : undefined}>
							{#each slice as e (e.key)}
								{#if e.type === 'head'}
									<div class="series-head" use:measure={e.key} data-key={e.key}>
										<h3>{e.name}</h3>
										<span>{e.count} in the Library</span>
										{#if e.id}<a href={to(`/library/collections/${e.id}`)}>Open series</a>{/if}
									</div>
								{:else}
									{@const t = e.t}
									{@const st = exceptionChip(filter.status, posterStatus(t, who))}
									{@const p = progressOf(t)}
									{@const requester = requestedBy(t)}
									{@const fold = filter.status === 'missing' && t.kind === 'show' && (t.show?.missing ?? 0) > 0}
									{@const ri = rowEntries.findIndex((r) => r.t.id === t.id)}
									<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
									<div
										class="entry"
										role="listitem"
										tabindex={ri === active ? 0 : -1}
										aria-label="{t.name}{t.year ? `, ${t.year}` : ''}{picked.has(t.id) ? ', selected' : ''}"
										data-key={e.key}
										data-testid="library-row"
										data-title-id={t.id}
										data-letter={letterOf(sortName(t.name))}
										class:selected={picked.has(t.id)}
										class:last-opened={lastOpenedId === t.id}
										use:measure={e.key}
										onkeydown={(ev) => rowKey(ev, t)}
										onfocus={() => (active = ri)}
										onclick={(ev) => rowClick(ev, t)}
									>
										{#if view === 'table'}
											<div class="row" style:--row-h="{bannerH}px">
												<span class="art"><BannerArt title={t} width={phone ? listWidth : bannerW} /></span>
												<span class="state" title={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'}>
													{#if selecting}<input type="checkbox" class="pick-box" checked={picked.has(t.id)} aria-label="Select {t.name}" tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />
													{:else}<span class="keep" class:on={t.keep_up_to_date} aria-label={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'} role="img"><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={16} /></span>{/if}
												</span>
												<span class="name">
													<a href={titleLink(t.id)} tabindex="-1">{t.name}</a>
													<span class="sub">
														{#if kind === 'all'}{kindLabel[t.kind]}{#if t.year} · {t.year}{/if}{/if}
														{#if st}<StatusPill kind={st.kind} label={st.label} />{/if}
														{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<StatusPill kind="approved" testid="request-approved" />{/if}
													</span>
													{#if phone}<span class="facts">{chosenCols.slice(0, 3).map((c) => (isProgressCol(c) ? `${p.have}/${p.total}` : cell(t, c))).filter(Boolean).join(' · ')}</span>{/if}
												</span>
												{#if kind !== 'all'}
													{#each shownCols as c (c.id)}
														{#if isProgressCol(c)}
															<span class="c prog">
																<span class="meter {tone(t)}"><i style="width: {p.total ? (p.have / p.total) * 100 : 0}%"></i></span>
																<span class="num-t">{p.have}/{p.total}</span>
																<span class="sr-only">{progressText(t)}</span>
															</span>
														{:else}
															<span class="c" class:num={c.numeric} class:path={c.id === 'path'} title={c.id === 'path' ? cell(t, c) : undefined}>{cell(t, c) || '—'}</span>
														{/if}
													{/each}
												{/if}
												<span class="acts">
													{#if canManage && !selecting}
														<IconButton icon="search" size="sm" tipSide="below-end" label="Search now for {t.name}" onclick={() => searchTitles([t.id])} />
														<IconButton icon="refresh" size="sm" tipSide="below-end" label="Refresh {t.name}" onclick={() => refreshTitle(t)} />
														<IconButton icon="tools" size="sm" tipSide="below-end" label="Edit {t.name}" href={titleLink(t.id)} />
													{/if}
												</span>
												{#if kind === 'all' || phone}<span class="sr-only">{progressText(t)}</span>{/if}
											</div>
										{:else}
											<div class="ov">
												<a class="ov-poster" href={titleLink(t.id)} tabindex="-1"><Poster src={t.artwork[0]} name={t.name} /></a>
												<div class="ov-main">
													<h3><a href={titleLink(t.id)} tabindex="-1">{t.name}</a>{#if t.year}<span class="yr">{t.year}</span>{/if}</h3>
													<p class="ov-text">{t.overview || 'No description yet.'}</p>
													<span class="meter {tone(t)}" aria-hidden="true"><i style="width: {p.total ? (p.have / p.total) * 100 : 0}%"></i></span>
													<span class="sr-only">{progressText(t)}</span>
												</div>
												<dl class="ov-facts">
													<dt>Kept up to date</dt><dd>{t.keep_up_to_date ? 'Yes' : 'No'}</dd>
													{#each shownCols.filter((c) => !isProgressCol(c)).slice(0, 4) as c (c.id)}{#if cell(t, c)}<dt>{c.label}</dt><dd>{cell(t, c)}</dd>{/if}{/each}
													{#if st}<dt>Status</dt><dd><StatusPill kind={st.kind} label={st.label} /></dd>{/if}
												</dl>
												{#if selecting}<input type="checkbox" class="pick-box ov-pick" checked={picked.has(t.id)} aria-label="Select {t.name}" tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />{/if}
											</div>
										{/if}
										{#if fold}
											{@const eps = missingOf[t.id]}
											<div class="fold">
												<Button variant="plain" size="sm" icon={foldOpen === t.id ? 'chevronDown' : 'chevronRight'} aria-expanded={foldOpen === t.id} aria-controls="missing-{t.id}" onclick={() => toggleMissing(t)} data-testid="missing-fold">{t.show?.missing} {t.show?.missing === 1 ? 'episode' : 'episodes'} missing</Button>
												{#if foldOpen === t.id}
													<div id="missing-{t.id}" class="fold-body" data-testid="missing-episodes">
														{#if eps === 'loading' || eps === undefined}<Skeleton lines={2} />
														{:else if eps === 'error'}<p class="fold-note">The episodes could not be loaded. <a href={titleLink(t.id)}>Open {t.name}</a></p>
														{:else if eps.length === 0}<p class="fold-note">Nothing missing any more.</p>
														{:else}
															<ul>
																{#each eps as ep (ep.id)}
																	<li data-testid="missing-episode"><span class="ep-code">{episodeLabel(ep, t.show?.episode_order ?? '')}</span> <span class="ep-name">{ep.name || 'Name not known yet'}</span>{#if ep.air_date}<span class="ep-date"> · {date(ep.air_date)}</span>{/if}{#if ep.slots[0] && ep.slots[0].state !== 'wanted'}{' '}{@const es = audienceStatus(slotKind(ep.slots[0].state), who, { tracked: true })}<StatusPill kind={es.kind} label={who === 'staff' ? ep.slots[0].state_label : es.label} />{/if}</li>
																{/each}
															</ul>
														{/if}
													</div>
												{/if}
											</div>
										{/if}
										{#if lastOpenedId === t.id || requester}
											<p class="row-notes">
												{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}
												{#if requester}<span class="requester" data-testid="requested-by">Asked by {requester}</span>{/if}
											</p>
										{/if}
									</div>
								{/if}
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</div>
		{#if railOn && loaded && !error}{@render letterRail()}{/if}
	</div>

	{#if selecting}
		<div class="select-bar" role="toolbar" aria-label="Selected titles" data-testid="select-bar">
			<span class="count" aria-live="polite">{pickedShown.length} selected</span>
			<Button variant="plain" size="md" onclick={pickAll}>{allPicked ? 'Select none' : `Select all ${shown.length}`}</Button>
			<span class="bar-gap"></span>
			<Button size="md" icon="bellOn" disabled={busy || !pickedShown.length} onclick={() => setKeep(pickedShown.map((t) => t.id), true)}>Keep up to date</Button>
			<Button size="md" icon="bell" disabled={busy || !pickedShown.length} onclick={() => setKeep(pickedShown.map((t) => t.id), false)}>Stop keeping</Button>
			{#if rulesItems.length}<Menu label="Pick rules for the selected titles" variant="button" text="Pick rules" icon="star" items={rulesItems} width={220} placement="above" />{/if}
			<Button size="md" icon="search" disabled={busy || !pickedShown.length} onclick={() => searchTitles(pickedShown.map((t) => t.id))}>Search now</Button>
			<Button variant="destructive" size="md" icon="trash" disabled={busy || !pickedShown.length} onclick={() => askDelete(pickedShown.map((t) => t.id))} data-testid="select-delete">Remove {pickedShown.length || ''}</Button>
			<IconButton icon="x" label="Stop selecting" onclick={stopSelecting} />
		</div>
	{/if}
</div>

<dialog bind:this={deleteDialog} aria-labelledby="bulk-delete-title" class="modal" data-testid="bulk-delete">
	<h2 id="bulk-delete-title" class="h2">Remove {deleteWord}?</h2>
	<p class="small muted">{deleteIds.length === 1 ? 'It leaves' : `All ${deleteIds.length} leave`} the Library. Files stay on disk. To also delete files, open the title and use Delete there.</p>
	<div class="modal-actions">
		<Button onclick={() => deleteDialog?.close()} disabled={busy}>Cancel</Button>
		<Button variant="destructive" icon="trash" disabled={busy} onclick={confirmDelete} data-testid="bulk-delete-confirm">Remove {deleteIds.length} {deleteIds.length === 1 ? 'title' : 'titles'}</Button>
	</div>
</dialog>

<dialog bind:this={saveDialog} aria-labelledby="save-filter-title" class="modal">
	<form method="dialog" onsubmit={(e) => { e.preventDefault(); saveCurrent(); }}>
		<h2 id="save-filter-title" class="h2">Save this filter</h2>
		<label class="small muted" for="save-filter-name">Name</label>
		<input id="save-filter-name" class="input" bind:value={saveName} maxlength="60" placeholder="For example: Missing, kept up to date" />
		<div class="modal-actions">
			<Button onclick={() => saveDialog?.close()}>Cancel</Button>
			<Button variant="primary" type="submit" disabled={!saveName.trim()}>Save</Button>
		</div>
	</form>
</dialog>

{#snippet letterRail()}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<nav class="rail" aria-label="Jump to a letter" data-testid="letter-rail" bind:this={rail} onkeydown={railKey}>
		{#each letters as l (l)}
			{@const on = jumps.has(l)}
			<button type="button" class="rail-l" data-letter={l === '#' ? 'num' : l} disabled={!on} tabindex={on && l === railFocus ? 0 : -1} aria-label={l === '#' ? '# Jump to titles that start with a number or sign' : `${l}, jump to titles under ${l}`} title={l === '#' ? 'Numbers and signs' : l} onclick={() => jump(l)}>{l}</button>
		{/each}
	</nav>
{/snippet}

<style>
	.library { display: flex; flex-direction: column; gap: 14px; padding-bottom: 48px; min-width: 0; }
	.library.selecting { padding-bottom: 112px; }

	/* One toolbar: view, sort, filter, search, columns, select. */
	.toolbar { position: sticky; top: var(--topbar, 0px); z-index: 6; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 8px 0; background: var(--bg); }
	.find-box { position: relative; flex: 1 1 220px; max-width: 360px; min-width: 0; }
	.find-ic { position: absolute; left: 11px; top: 50%; translate: 0 -50%; color: var(--text-muted); display: grid; pointer-events: none; }
	.find { width: 100%; min-height: 40px; padding-left: 34px; border-radius: var(--r-btn); background: var(--surface-2); border-color: transparent; }
	.tool-gap { flex: 1 1 auto; }
	.filter-btn.on :global(button[aria-haspopup]) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent); }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.search-error { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--fail); font: var(--t-callout); }
	.content { min-width: 0; }
	.with-rail { min-width: 0; }
	.with-rail.has-rail { display: grid; grid-template-columns: minmax(0, 1fr) 26px; gap: 12px; align-items: start; }
	.group-note { margin: 0 0 12px; color: var(--text-muted); font: var(--t-footnote); }

	/* Banner table */
	.rows { min-width: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); overflow: clip; }
	.rows.table { overflow-x: auto; }
	.row { display: grid; grid-template-columns: var(--cols); align-items: center; gap: 0 16px; min-height: var(--row-h, 100px); padding: 0 20px 0 0; min-width: max-content; }
	.row.head { position: sticky; top: 0; z-index: 2; min-height: 38px; padding-top: 0; padding-bottom: 0; padding-left: 0; background: var(--surface-1); border-bottom: 1px solid var(--line); }
	.h { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; text-align: left; white-space: nowrap; }
	.h.num, .c.num { text-align: right; justify-self: end; }
	.banner-h { padding-left: 14px; }
	.sort { display: inline-flex; align-items: center; gap: 4px; padding: 6px 0; border: 0; background: none; cursor: pointer; }
	.sort.num { justify-content: flex-end; }
	.sort[aria-pressed='true'] { color: var(--text); }
	.sort:hover { color: var(--text); }
	.sort:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.dir { color: var(--accent); }
	.entry { position: relative; outline: none; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.entry + .entry, .series-head + .entry { border-top: 1px solid var(--line); }
	.entry:hover { background: color-mix(in srgb, var(--surface-2) 70%, transparent); }
	.entry:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
	.entry.selected { background: color-mix(in srgb, var(--accent) 12%, var(--surface-1)); }
	.entry.last-opened { box-shadow: inset 3px 0 0 var(--text-muted); }
	.state { display: grid; place-items: center; }
	.keep { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; color: var(--text-muted); }
	.keep.on { color: var(--ok, #2fa36b); background: color-mix(in srgb, var(--ok, #2fa36b) 14%, transparent); }
	.art { display: block; min-width: 0; align-self: stretch; --banner-r: 0; }
	.art :global(img:not(.logo)) { transition: opacity 240ms var(--ease-out), filter var(--dur-fast) var(--ease-out), scale 400ms var(--ease-out); }
	.entry:hover .art :global(img:not(.logo)) { filter: brightness(1.12) saturate(1.05); }
	@media (prefers-reduced-motion: no-preference) { .entry:hover .art :global(img:not(.logo)) { scale: 1.02; } }
	.name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.name a { color: var(--text); font: var(--t-title3, var(--t-headline)); font-weight: 600; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.sub:empty { display: none; }
	.facts { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.c { min-width: 0; color: var(--text); font: var(--t-callout); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.c.path { color: var(--text-muted); font: var(--t-footnote); font-family: var(--font-mono, ui-monospace, monospace); direction: rtl; text-align: left; }
	.prog { display: grid; grid-template-columns: 1fr auto; align-items: center; gap: 8px; }
	.num-t { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.acts { position: absolute; right: 10px; top: calc(var(--row-h, 100px) / 2); translate: 0 -50%; z-index: 1; display: flex; gap: 2px; padding: 4px; border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 88%, transparent); box-shadow: var(--shadow-card); backdrop-filter: blur(8px); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.acts:empty { display: none; }
	.entry:hover .acts, .entry:focus-within .acts { opacity: 1; }
	@media (hover: none) { .acts { opacity: 1; } }

	/* Progress meter, coloured by state */
	.meter { position: relative; display: block; height: 5px; border-radius: 99px; background: var(--surface-3); overflow: hidden; }
	.meter i { position: absolute; inset: 0 auto 0 0; border-radius: inherit; background: var(--text-muted); }
	.meter.full i { background: var(--ok, #2fa36b); }
	.meter.gap i { background: var(--warn, #d9822b); }
	.meter.gap { background: color-mix(in srgb, var(--fail, #d64545) 28%, var(--surface-3)); }
	.meter.later i { background: var(--info, #3b82f6); }
	.meter.none i { background: var(--text-muted); }

	.series-head { display: flex; align-items: baseline; gap: 12px; padding: 18px 16px 10px; background: var(--surface-2); border-top: 1px solid var(--line); }
	.series-head h3 { margin: 0; font: var(--t-headline); }
	.series-head span { color: var(--text-muted); font: var(--t-footnote); }
	.series-head a { margin-left: auto; color: var(--accent); font: var(--t-footnote); font-weight: 600; }

	/* Overview list */
	.ov { display: grid; grid-template-columns: 92px minmax(0, 1fr) 240px; gap: 18px; padding: 14px 16px; }
	.ov-poster { display: block; border-radius: 8px; overflow: hidden; }
	.ov-main { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.ov h3 { margin: 0; font: var(--t-title3); display: flex; align-items: baseline; gap: 10px; }
	.ov h3 a { color: var(--text); text-decoration: none; }
	.yr { color: var(--text-muted); font: var(--t-callout); }
	.ov-text { margin: 0; color: var(--text-muted); font: var(--t-callout); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.ov .meter { max-width: 280px; margin-top: auto; }
	.ov-facts { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 6px 12px; margin: 0; align-content: start; font: var(--t-footnote); }
	.ov-facts dt { color: var(--text-muted); }
	.ov-facts dd { margin: 0; text-align: right; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ov-pick { position: absolute; right: 14px; top: 14px; }

	/* Loading rows match the banner shape */
	.sk-row { display: grid; grid-template-columns: var(--bw, 420px) 1fr 1fr; align-items: center; gap: 16px 60px; padding: 0 20px 0 0; }
	.sk-row + .sk-row { border-top: 1px solid var(--line); }
	.sk-banner { display: block; aspect-ratio: 1000 / 185; }
	.sk-banner :global(.ui-sk) { height: 100%; border-radius: 0; }

	/* Posters */
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--poster-min, 172px), 1fr)); gap: 26px 18px; margin: 0; padding: 0; list-style: none; }
	.grid > li { position: relative; min-width: 0; display: flex; flex-direction: column; gap: 6px; content-visibility: auto; contain-intrinsic-size: auto 340px; scroll-margin-top: 120px; }
	.grid .meter { height: 3px; }
	.lines { display: flex; flex-direction: column; gap: 1px; margin: 0; padding: 0; list-style: none; color: var(--text-muted); font: var(--t-footnote); }
	.lines li { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lines li.kept { color: var(--ok-text, var(--ok, #2fa36b)); }
	.grid > li.last-opened { outline: 2px solid var(--text-muted); outline-offset: 4px; border-radius: var(--r-card); }
	.grid > li.selected { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: var(--r-card); }
	.pick { position: absolute; inset: 0 0 auto 0; aspect-ratio: 2 / 3; z-index: 3; display: block; padding: 0; border: 0; border-radius: var(--r-card); background: transparent; cursor: pointer; }
	.pick[aria-pressed='true'] { background: rgb(0 0 0 / 30%); box-shadow: inset 0 0 0 3px var(--accent); }
	.pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pick-dot { position: absolute; left: 8px; top: 8px; width: 24px; height: 24px; display: grid; place-items: center; border-radius: 50%; border: 2px solid #fff; background: rgb(0 0 0 / 35%); color: #fff; }
	.pick[aria-pressed='true'] .pick-dot { background: var(--accent-fill); border-color: var(--accent-fill); color: var(--accent-ink, #fff); }
	.pick-box { width: 18px; height: 18px; accent-color: var(--accent); }
	.card-notes { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; min-width: 0; }
	.requester { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.last-opened-label { font: var(--t-footnote); font-weight: 600; color: var(--text); }
	.row-notes { display: flex; flex-wrap: wrap; gap: 4px 12px; margin: -6px 0 0; padding: 0 12px 10px 54px; }

	/* Bulk bar */
	.select-bar { position: sticky; bottom: 16px; z-index: 20; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 8px; padding: 10px 12px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card), 0 8px 28px rgb(0 0 0 / 25%); }
	.select-bar .count { padding: 0 8px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.bar-gap { flex: 1 1 auto; }
	.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
	.modal .input { width: 100%; margin-top: 6px; }

	/* A to Z */
	.rail { position: sticky; top: calc(var(--topbar, 0px) + 72px); display: flex; flex-direction: column; align-items: stretch; padding: 4px 0; border-radius: 999px; background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rail-l { display: grid; place-items: center; min-height: 22px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text); font: 600 11px/1 var(--font-ui); cursor: pointer; }
	.rail-l:hover:not(:disabled) { background: var(--surface-2); color: var(--accent); }
	.rail-l:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rail-l:disabled { color: var(--text-muted); opacity: .45; cursor: default; }

	/* Missing fold */
	.fold { padding: 0 12px 10px 54px; }
	.fold-body { margin-top: 6px; padding: 8px 12px; border-radius: var(--r-btn); background: var(--surface-2); }
	.fold-body ul { display: flex; flex-direction: column; gap: 6px; margin: 0; padding: 0; list-style: none; font: var(--t-callout); }
	.fold-body li { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 6px; min-width: 0; }
	.ep-code { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.ep-name { overflow-wrap: anywhere; }
	.ep-date { color: var(--text-muted); }
	.fold-note { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.empty-more { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; }

	
	/* Phones: each row becomes a banner card; posters stay a grid. */
	@media (max-width: 639px) {
		.toolbar { position: static; }
		.find-box { flex-basis: 100%; max-width: none; order: 5; }
		.tool-gap { display: none; }
		.with-rail.has-rail { grid-template-columns: minmax(0, 1fr) 20px; gap: 6px; }
		.rail { top: 64px; padding: 2px 0; }
		.rail-l { font-size: 10px; min-height: 18px; }
		.rows.table { overflow: clip; }
		.row { grid-template-columns: 1fr auto; grid-template-areas: 'art art' 'name acts'; gap: 10px 8px; min-width: 0; padding: 0 12px 12px; }
		.row > .art { margin: 0 -12px; }
		.rows.table .row { min-height: 0; }
		.row > .state { position: absolute; right: 20px; top: 20px; z-index: 1; }
		.row > .state .keep { background: rgb(0 0 0 / 55%); color: #fff; }
		.row > .state .keep.on { background: color-mix(in srgb, var(--ok, #2fa36b) 85%, #000); color: #fff; }
		.row > .art { grid-area: art; }
		.row > .name { grid-area: name; }
		.row > .acts { grid-area: acts; position: static; translate: none; padding: 0; background: none; box-shadow: none; backdrop-filter: none; opacity: 1; }
		.row > .c { display: none; }
		.ov { grid-template-columns: 72px minmax(0, 1fr); }
		.ov-facts { grid-column: 1 / -1; }
		.grid { grid-template-columns: repeat(auto-fill, minmax(calc(var(--poster-min, 172px) * 0.66), 1fr)); gap: 18px 12px; }
		.fold, .row-notes { padding-left: 12px; }
		.sk-row { grid-template-columns: 1fr; padding: 0 0 12px; }
		.sk-row > :global(.stack), .sk-row > :global(.line) { display: none; }
	}
</style>
