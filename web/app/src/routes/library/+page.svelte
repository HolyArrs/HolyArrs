<script lang="ts">
	import { libraryTitles, ownedLibrary } from '#lib/owned.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
	import { ago, bytes, date } from '#lib/format.ts';
	import { exceptionChip, filters, posterStatus, firstOfLetter, kinds, letterOf, letters, matchesTitleQuery, missingEpisodes, ofKind, titleQuery, type Filter, type Kind } from '#lib/library.ts';
	import {
		activeCount, columnSort, sortName, filterCounts, loadColumns, loadSaved, noFilter, passes, progressOf, releaseDate, saveColumns, saveSaved, sortLabel, sortTitles, sortsFor, startDir, columnsFor, defaultSort,
		type Column, type FilterContext, type FilterState, type Kept, type LibKind, type SavedFilter, type SortDir, type SortKey,
		tableMinWidth
	} from '#lib/library-table.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import { DUR, EASE, dropIn, dropOut, easeOut, moveMs } from '#lib/ui/transitions.ts';
	import { fade, slide } from 'svelte/transition';
	import { episodeLabel, showProgress } from '#lib/shows.ts';
	import { artistProgress } from '#lib/music/music.ts';
	import { artistHref } from '#lib/music/music-pages.ts';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import LibraryPoster from '#lib/ui/LibraryPoster.svelte';
	import { flip } from 'svelte/animate';
	import { barOf, detailLabels, flipFrom, loadPosterDetail, needsAttention, savePosterDetail, type PosterDetail } from '#lib/library-poster.ts';
	import BannerArt from '#lib/ui/BannerArt.svelte';
	import ColumnChooser from '#lib/ui/ColumnChooser.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import LibraryViewSwitch from '#lib/LibraryViewSwitch.svelte';
	import DlFind from '#lib/downloads/DlFind.svelte';
	import { searchShortcut } from '#lib/downloads/keys.ts';
	import { dlTop } from '#lib/dl-top.svelte.ts';
	import { rememberLibraryKind } from '#lib/library-last-kind.ts';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { artName, slotKind } from '#lib/title-page.ts';
	import { peopleNames, plainReason } from '#lib/plain.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { audienceOf, audienceStatus } from '#lib/title-status.ts';
	import { libraryState } from '#lib/library-state.ts';
	import { quality } from '#lib/explore.ts';
	import type { EpisodeView, TitleView } from '#lib/types.ts';
	import type { MovieCollection, CollectionDetail } from '#lib/collections.ts';
	import { ownedMark, type ExploreCard } from '#lib/explore.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { codecLabel, copyCodecs, languageName, type CopySearchPreview, type CopySearchResult } from '#lib/library-copies.ts';
	import { deleteBody } from '#lib/organise.ts';
	import { bannerHeight, bannerRatio, bannerWidth, densityLabels, loadDensity, loadPosterSize, posterLabels, posterMinFor, posterRatio, posterSrc, saveDensity, savePosterSize, squarePhoneMin, type Density, type PosterSize } from '#lib/library-art.ts';

	type View = 'rows' | 'table' | 'posters' | 'overview';

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
		// Codecs are known for movies only; subtitles for movies and TV shows (never Music).
		const k = p.get('kind');
		const codec = p.get('codec') ?? '';
		return {
			codec: k === 'movie' && codec !== 'all' && (copyCodecs as readonly string[]).includes(codec) ? codec : '',
			subs: (k === 'movie' || k === 'show') && p.get('subs') === 'missing',
			status: filters.some(([f]) => f === status) ? status : 'all',
			kept: kept === 'on' || kept === 'off' ? kept : 'any',
			asker: asker === 'me' || (asker && /^[1-9]\d*$/.test(asker)) ? asker : 'anyone',
			series: Number.isSafeInteger(series) && series > 0 ? series : 0
		};
	}
	function readSort(p: Pick<URLSearchParams, 'get'>, k: Kind): { key: SortKey; dir: SortDir } {
		const key = p.get('sort') as SortKey;
		const d = defaultSort(k === 'all' ? 'all' : k);
		const ok = sortsFor(k === 'all' ? 'all' : k).includes(key) ? key : d.key;
		const dir = p.get('dir');
		return { key: ok, dir: dir === 'asc' || dir === 'desc' ? dir : ok === d.key ? d.dir : startDir(ok) };
	}
	let kind = $state<Kind>(readKind(page.url.searchParams.get('kind')));
	let filter = $state<FilterState>(readFilter(page.url.searchParams));
	let sort = $state(readSort(page.url.searchParams, readKind(page.url.searchParams.get('kind'))));
	let q = $state(page.url.searchParams.get('q')?.slice(0, 256) ?? '');
	let groupSets = $state(page.url.searchParams.get('group') === 'series');
	$effect(() => {
		const p = params();
		kind = readKind(p.get('kind'));
		filter = readFilter(p);
		sort = readSort(p, kind);
		q = p.get('q')?.slice(0, 256) ?? '';
		groupSets = p.get('group') === 'series';
	});
	function syncUrl() {
		const url = new URL((page.shallow?.url ?? page.url).href);
		const set = (k: string, v: string | null) => (v ? url.searchParams.set(k, v) : url.searchParams.delete(k));
		set('kind', kind === 'all' ? null : kind);
		set('show', filter.status === 'all' ? null : filter.status);
		set('kept', filter.kept === 'any' ? null : filter.kept);
		set('requester', filter.asker === 'anyone' ? null : filter.asker);
		set('series', filter.series ? String(filter.series) : null);
		set('codec', filter.codec || null);
		set('subs', filter.subs ? 'missing' : null);
		set('q', q || null);
		const d = defaultSort(kind === 'all' ? 'all' : kind);
		set('sort', sort.key === d.key ? null : sort.key);
		set('dir', sort.dir === (sort.key === d.key ? d.dir : startDir(sort.key)) ? null : sort.dir);
		set('group', groupSets ? 'series' : null);
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
		if (asked === 'rows') return 'rows';
		if (asked === 'list' || asked === 'table' || asked === 'banners') return 'table';
		if (asked === 'grid' || asked === 'posters') return 'posters';
		if (asked === 'overview') return 'overview';
		// Rows is the view everyone starts on (Luis's pick, 2026-11-05); a view picked before that day is not carried over.
		try {
			const v = localStorage.getItem(`holyarrs.library-view.${k}`);
			return v === 'posters' || v === 'overview' || v === 'table' ? v : 'rows';
		} catch {
			return 'rows';
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
		if (v === view) return;
		view = v;
		// The new view fades up in one piece, like a Downloads tab; the posters' own rise still plays.
		void tick().then(() => {
			const el = view === 'posters' ? gridEl : listEl;
			if (el && !reducedMotion()) el.animate([{ opacity: 0, transform: 'translateY(4px)' }, { opacity: 1, transform: 'none' }], { duration: DUR.base, easing: EASE.out });
		});
		try {
			localStorage.setItem(`holyarrs.library-view.${kind}`, v);
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
	// the width (by density) and its shape sets the row height: 1000x185 for
	// movies and shows, 1000x400 for Music (square artist photos need height).
	// Columns that do not fit scroll sideways rather than squeeze the art.
	let density = $state<Density>('large');
	let posterSize = $state<PosterSize>('m');
	let posterDetail = $state<PosterDetail>('status');
	$effect(() => {
		const me = myId;
		density = loadDensity(me);
		posterSize = loadPosterSize(me);
		posterDetail = loadPosterDetail(me);
	});
	function setDensity(d: Density) {
		density = d;
		saveDensity(myId, d);
	}
	let gridEl = $state<HTMLElement>();
	/** Glide the posters on screen from where they were to where they land (size changes). */
	async function glide(change: () => void) {
		const items = gridEl && !reducedMotion() ? [...gridEl.children].filter((el): el is HTMLElement => el instanceof HTMLElement) : [];
		const vh = window.innerHeight;
		const before = new Map(items.map((el) => [el, el.getBoundingClientRect()] as const).filter(([, r]) => r.bottom > -200 && r.top < vh + 200));
		change();
		await tick();
		for (const [el, a] of before) {
			const b = el.getBoundingClientRect();
			const from = flipFrom(a, b);
			if (from) el.animate([{ transform: from, transformOrigin: '0 0' }, { transform: 'none', transformOrigin: '0 0' }], { duration: DUR.base, easing: EASE.out });
		}
	}
	function setPosterSize(z: PosterSize) {
		void glide(() => (posterSize = z));
		savePosterSize(myId, z);
	}
	function setPosterDetail(d: PosterDetail) {
		posterDetail = d;
		savePosterDetail(myId, d);
	}
	// The banner sits inset in the sheet (8px from its edge, 6px above and below).
	const ART_INSET = 8;
	const ART_GAP = 12;
	const bannerFloor: Record<Density, number> = { large: 340, comfortable: 260, compact: 180 };
	const wantW = $derived(bannerWidth(listWidth, density));
	let touchActs = $state(0);
	$effect(() => {
		touchActs = canManage && typeof matchMedia === 'function' && matchMedia('(hover: none)').matches ? 132 : 0;
	});
	const colsNeed = (cols: Column[]) => tableMinWidth(['0px', '28px', '180px', ...cols.map((c) => c.width)], 16, 20 + touchActs) + ART_INSET;
	// The table never needs a sideways scroll: the banner gives way first (down to its floor),
	// then the last columns step out until the rest fits.
	const shownCols = $derived.by(() => {
		if (phone || kind === 'all') return chosenCols;
		let out = chosenCols;
		while (out.length > 0 && colsNeed(out) + Math.min(wantW, bannerFloor[density]) > listWidth) out = out.slice(0, -1);
		return out;
	});
	const hiddenCols = $derived(chosenCols.length - shownCols.length);
	// As many facts as the list is wide enough for: three on a wide screen, down to one.
	// The Rows grid: poster 52, title >= 120, status 168, each fact 150, actions (wider on touch screens), 12 px gaps.
	const actsW = $derived(96 + (touchActs ? 48 : 0));
	const factCols = $derived(chosenCols.filter((c) => c.id !== 'status' && c.id !== 'kept'));
	/** colPx is a column's width for the Rows grid (a stretching column takes its floor). */
	const colPx = (c: Column) => Math.max(84, parseInt(c.width.replace(/^minmax\((\d+)px.*$/, '$1'), 10) || 150);
	const rowCols = $derived.by(() => {
		if (kind === 'all') return [];
		let room = listWidth - 10 - 52 - 120 - 168 - actsW - 12 * 3 - (selecting ? 28 + 12 : 0);
		const out: Column[] = [];
		for (const c of factCols) {
			if (out.length === 3 || (out.length && room < colPx(c) + 12)) break;
			out.push(c);
			room -= colPx(c) + 12;
		}
		return out;
	});
	const factsTpl = $derived(rowCols.map((c) => `${colPx(c)}px`).join(' ') || '0px');
	/** rowCell is a fact for the Rows view: counts for progress columns, "14 d ago" for dates that are recent by nature. */
	function rowCell(t: TitleView, c: Column, p: { have: number; total: number }): string {
		if (isProgressCol(c)) return p.total ? `${p.have}/${p.total}` : '';
		if (c.id === 'release' && t.kind === 'movie' && t.digital_date && Date.parse(t.digital_date) <= Date.now()) return date(t.digital_date);
		if (c.id === 'added') return ago(t.added_at);
		if (c.id === 'activity') return ago(t.last_activity);
		return cell(t, c);
	}
	const bannerW = $derived(listWidth < 640 ? wantW : Math.round(Math.max(Math.min(wantW, bannerFloor[density]), Math.min(wantW, listWidth - colsNeed(kind === 'all' ? [] : shownCols)))));
	/** A row's height for its title's kind: the banner plus its inset (Music rows are taller). */
	const rowH = (k: string) => bannerHeight(bannerW, k) + ART_GAP;
	// Music's square art packs a little tighter; the All view keeps the 2:3 columns.
	const gridMin = $derived(posterMinFor(posterSize, kind === 'artist' ? 'artist' : 'movie'));
	const colWidths = $derived([`${bannerW}px`, '28px', 'minmax(180px, 1fr)', ...(kind === 'all' ? [] : shownCols.map((c) => c.width))]);
	const gridCols = $derived(colWidths.join(' '));
	// One width for every row (header included), so columns line up.
	const tableMin = $derived(tableMinWidth(colWidths));
	const sizeItems = $derived<MenuItem[]>(
		view === 'posters'
			? [
					{ kind: 'heading', label: 'Poster size' }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: posterLabels[z], checked: posterSize === z, onselect: () => setPosterSize(z) })),
					{ kind: 'separator' }, { kind: 'heading', label: 'Poster details' }, ...(['title', 'status', 'full'] as const).map((d) => ({ label: detailLabels[d], checked: posterDetail === d, onselect: () => setPosterDetail(d) }))
				] as MenuItem[]
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
	async function loadSets() {
		if (seriesLoaded || !moviesOn) return;
		seriesLoaded = true;
		try {
			const list = (await api.get<{ collections: MovieCollection[] }>('/collections')).collections;
			// The list carries each series' movie ids; older servers need one read per series.
			const details = list.every((c) => c.title_ids) ? [] : await Promise.all(list.map((c) => api.get<CollectionDetail>(`/collections/${c.id}`).catch(() => null)));
			const map = new Map<number, number>();
			for (const c of list) for (const id of c.title_ids ?? []) map.set(id, c.id);
			for (const d of details) for (const t of d?.titles ?? []) map.set(t.id, d!.id);
			series = list;
			seriesOfTitle = map;
		} catch {
			seriesLoaded = false;
		}
	}
	$effect(() => {
		if (mounted && (kind === 'movie' || kind === 'all')) void loadSets();
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
	// Titles with subtitles still wanted (movies and TV shows), from the same list the subtitles search uses.
	let subsTitles = $state<Set<number> | null>(null);
	const ctx = $derived<FilterContext>({ askers: (t) => linkedPeople(t).map((p) => p.id), me: myId, seriesOf: (t) => seriesOfTitle.get(t.id), subsMissing: (t) => !!subsTitles?.has(t.id) });
	const ofThisKind = $derived(titles.filter((t) => ofKind(t, kind) && matchesTitleQuery(t, query, indexedIds)));
	const counts = $derived(filterCounts(ofThisKind, filter, ctx));
	const shown = $derived(sortTitles(ofThisKind.filter((t) => passes(t, filter, ctx)), sort.key, sort.dir));
	const askers = $derived([...new Map(ofThisKind.flatMap((t) => linkedPeople(t)).map((p) => [p.id, p])).values()].sort((a, b) => a.name.localeCompare(b.name)));
	const filtersOn = $derived(activeCount(filter));

	// ── Toolbar: the count where the title was, the top search field, the pinned bar ──
	const nouns: Record<string, [string, string]> = { movie: ['movie', 'movies'], show: ['TV show', 'TV shows'], artist: ['artist', 'artists'], all: ['title', 'titles'] };
	const kindTotal = $derived(titles.filter((t) => ofKind(t, kind)).length);
	const countText = $derived.by(() => {
		const [one, many] = nouns[kind] ?? nouns.all;
		if (filtersOn || q) return `${shown.length} of ${kindTotal} ${kindTotal === 1 ? one : many}`;
		return `${shown.length} ${shown.length === 1 ? one : many}`;
	});
	// The Library has no top bar: its own row carries the search (as on Downloads). Ctrl K and "/" focus it.
	const findWords = $derived(kind === 'movie' ? 'Search movies' : kind === 'show' ? 'Search TV shows' : kind === 'artist' ? 'Search music' : 'Search library');
	function setQ(v: string) {
		q = v.slice(0, 256);
		syncUrl();
	}
	function findKey(e: KeyboardEvent) {
		const t = e.target as HTMLElement | null;
		const inField = !!t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
		const box = '[role="dialog"], dialog, [popover]:popover-open';
		const inDialog = !!t?.closest?.(box) || !!document.querySelector('dialog[open]');
		if (!searchShortcut(e, inField, inDialog)) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		if (phone) {
			findOpen = true;
			void tick().then(() => document.getElementById('find')?.focus());
		} else {
			const f = document.querySelector<HTMLInputElement>('.library [data-dl-search]');
			f?.focus();
			f?.select();
		}
	}
	$effect(() => {
		window.addEventListener('keydown', findKey, true);
		return () => window.removeEventListener('keydown', findKey, true);
	});
	// A plain /library link opens the kind used last (+page.ts).
	$effect(() => rememberLibraryKind(kind));
	// Phones have no side menu: the count is also the Movies / TV shows / Music switch.
	const shelfItems = $derived<MenuItem[]>(
		(
			[
				['movie', 'Movies', 'film', 'movies'],
				['show', 'TV shows', 'tv', 'shows'],
				['artist', 'Music', 'music', 'music']
			] as const
		)
			.filter(([k, , , part]) => partsNow.answer?.parts.find((p) => p.part === part)?.on !== false && !(k === 'artist' && session.kid))
			.map(([k, label]): MenuItem => ({ label, checked: kind === k, onselect: () => void goto(to(`/library?kind=${k}`)) }))
	);
	let findOpen = $state(false);
	// Rows of chips and folds grow in; a fade under reduced motion.
	const grow = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.base, easing: easeOut }));
	const shrink = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.fast, easing: easeOut }));
	let sortOpen = $state(false);
	let toolbarEl = $state<HTMLElement>();
	let stuck = $state(false);
	function checkStuck() {
		if (!toolbarEl) return;
		const bar = parseFloat(getComputedStyle(toolbarEl).top) || 0;
		stuck = window.scrollY > 0 && getComputedStyle(toolbarEl).position === 'sticky' && toolbarEl.getBoundingClientRect().top <= bar + 0.5;
	}

	function setFilter(patch: Partial<FilterState>) {
		filter = { ...filter, ...patch };
		syncUrl();
	}
	function clearSearch() {
		q = '';
		findOpen = false;
		syncUrl();
	}
	function clearFilters() {
		filter = { ...noFilter };
		clearSearch();
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
		toasts.push(`Saved “${name}”.`, 'ok');
	}
	function forget(name: string) {
		saved = saved.filter((s) => s.name !== name);
		saveSaved(myId, kind, saved);
	}
	const sameFilter = (a: FilterState, b: FilterState) => a.status === b.status && a.kept === b.kept && a.asker === b.asker && a.series === b.series && (a.codec ?? '') === (b.codec ?? '') && !!a.subs === !!b.subs;

	// Counts stay blank until the Library has loaded, so a 0 only ever means 0.
	const countHint = (n: number) => (loaded ? String(n) : undefined);
	// Add only adds. Everything that looks after the Library is in Tools (people are found by the search,
	// movie series by Group by series, and held files are on Downloads).
	const addItems = $derived<MenuItem[]>([
		...(partOn('movies') ? ([{ label: 'Movie', icon: 'film', href: to('/search') }] as MenuItem[]) : []),
		...(partOn('shows') ? ([{ label: 'TV show', icon: 'tv', href: to('/search?kind=show') }] as MenuItem[]) : []),
		...(partOn('music') ? ([{ label: 'Artist', icon: 'music', href: to('/search?kind=music') }] as MenuItem[]) : [])
	]);
	// Recently deleted lives in Sort & filter: its count is read when the panel opens (at most once a minute).
	let deletedCount = $state<number | null>(null);
	let deletedAt = 0;
	$effect(() => {
		if (!sortOpen || !canManage || Date.now() - deletedAt < 60_000) return;
		deletedAt = Date.now();
		void api.get<{ files: unknown[] }>('/holding').then((r) => (deletedCount = r.files?.length ?? 0), () => {});
	});
	function partOn(name: string) {
		return partsNow.answer?.parts.find((part) => part.part === name)?.on !== false;
	}

	// ── Entries: rows, with series headers when grouped ──
	type Entry = { type: 'head'; key: string; id: number; name: string; count: number } | { type: 'row'; key: string; t: TitleView };
	const grouped = $derived(groupSets && kind === 'movie');
	const entries = $derived.by<Entry[]>(() => {
		if (!grouped) return shown.map((t) => ({ type: 'row', key: `r${t.id}`, t }));
		const bySet = new Map<number, TitleView[]>();
		const loose: TitleView[] = [];
		for (const t of shown) {
			const s = seriesOfTitle.get(t.id);
			if (s) bySet.set(s, [...(bySet.get(s) ?? []), t]);
			else loose.push(t);
		}
		const out: Entry[] = [];
		for (const [id, ts] of [...bySet].sort((a, b) => seriesName(a[0]).localeCompare(seriesName(b[0])))) {
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
	// A row's height belongs to the view it was measured in: start over when the view changes.
	let heightsView = untrack(() => view);
	$effect(() => {
		if (view !== heightsView) {
			heightsView = view;
			untrack(() => heights.clear());
		}
	});
	const phone = $derived(listWidth < 640);
	/** Height guess for a row not yet measured, from its kind's banner shape. */
	const guessFor = (k: string) => (view === 'rows' ? (phone ? 60 : 54) : view === 'overview' ? 168 : phone ? Math.round((listWidth - 24) / bannerRatio(k)) + 92 : rowH(k));
	const offsets = $derived.by(() => {
		const out = new Array<number>(entries.length + 1);
		out[0] = 0;
		for (let i = 0; i < entries.length; i++) out[i + 1] = out[i] + (heights.get(entries[i].key) ?? (entries[i].type === 'head' ? 52 : guessFor((entries[i] as Extract<Entry, { type: 'row' }>).t.kind)));
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
		} else if (e.key === 'Enter') { e.preventDefault(); void goto(openLink(t)); }
	}
	function rowClick(e: MouseEvent, t: TitleView) {
		const target = e.target as HTMLElement;
		if (selecting) {
			// While selecting, the whole row selects: its title link too.
			if (target.closest('button, input, label, [role="menu"]')) return;
			e.preventDefault();
			togglePick(t.id);
			return;
		}
		if (target.closest('a, button, input, label, [role="menu"]')) return;
		void goto(openLink(t));
	}

	// ── Selecting and the bulk bar ──
	let selecting = $state(false);
	let picked = $state<Set<number>>(new Set());
	let busy = $state(false);
	let deleteIds = $state<number[]>([]);
	let deleteDialog = $state<HTMLDialogElement | null>(null);
	let rules = $state<{ id: number; name: string; kinds: string[] }[]>([]);
	let folders = $state<{ id: number; path: string; kind: string; label: string }[]>([]);
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
		if (!folders.length) void api.get<{ library_folders: typeof folders }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
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
		report('being looked for now', failed, ids.length);
	}
	// Find another copy (movies): the selected movies' main files, searched again at their Quality
	// after a review. Protected files are left out.
	const copyPicks = $derived(pickedShown.flatMap((t) => (t.copy && !t.copy.protected ? [{ file_id: t.copy.file_id, version_id: t.copy.version_id }] : [])));
	let copyDialog = $state<HTMLDialogElement | null>(null);
	let copyPreview = $state<CopySearchPreview | null>(null);
	async function reviewCopies() {
		busy = true;
		try {
			copyPreview = await api.post<CopySearchPreview>('/library/copies/search/preview', { selections: copyPicks });
			copyDialog?.showModal();
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		} finally {
			busy = false;
		}
	}
	async function findCopies() {
		if (!copyPreview) return;
		busy = true;
		try {
			const r = await api.post<CopySearchResult>('/library/copies/search', { selections: copyPicks, review_token: copyPreview.review_token });
			toasts.push(r.job_ids.length === 1 ? 'Searching for 1 copy.' : `Searching for ${r.job_ids.length} copies.`, 'ok');
			copyDialog?.close();
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		} finally {
			busy = false;
		}
	}
	// Subtitles: which titles still want some (read at most once a minute); the search covers the shown kind.
	let subsAt = 0;
	let subsSearching = $state(false);
	$effect(() => {
		if (!canManage || !(kind === 'movie' || kind === 'show') || !partOn('subtitles')) return;
		if (Date.now() - subsAt < 60_000) return;
		subsAt = Date.now();
		void api.get<{ titles: { id: number }[] }>('/subtitles/missing?limit=1').then((r) => (subsTitles = new Set((r.titles ?? []).map((x) => x.id))), () => {});
	});
	async function searchMissingSubs() {
		subsSearching = true;
		try {
			const r = await api.post<{ queued: number }>('/subtitles/missing/search', { kind: kind === 'movie' ? 'movie' : 'episode' });
			toasts.push(r.queued === 1 ? 'Searching subtitles for 1 file.' : `Searching subtitles for ${r.queued} files.`, 'ok');
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		} finally {
			subsSearching = false;
		}
	}
	async function refreshTitle(t: TitleView) {
		try {
			await api.post(`/titles/${t.id}/refresh`, {});
			toasts.push(`Refreshing ${t.name}.`, 'ok');
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		}
	}
	// Missing list: one search for every missing title of this kind, after a confirm.
	let missingDialog = $state<HTMLDialogElement | null>(null);
	let searchingAll = $state(false);
	const missingCount = $derived(titles.filter((t) => ofKind(t, kind) && passes(t, { ...noFilter, status: 'missing' }, ctx)).length);
	const missingWhat = $derived(kind === 'movie' ? ' movies' : kind === 'show' ? ' shows' : kind === 'artist' ? ' music' : '');
	async function searchAllMissing() {
		searchingAll = true;
		try {
			await api.post('/search/missing', kind === 'all' ? undefined : { kind });
			toasts.push(`Searching ${missingCount} missing ${missingCount === 1 ? 'title' : 'titles'}.`, 'ok');
			missingDialog?.close();
		} catch (e) {
			toasts.push(plainReason(message(e)).text, 'fail');
		} finally {
			searchingAll = false;
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
	// Move the selected titles' files to another folder of their kind; mixed kinds get no choice.
	async function moveTitles(ids: number[], folder: { id: number; label: string; path: string }) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.post(`/titles/${id}/move`, { library_folder_id: folder.id, apply: true }));
		busy = false;
		report(`moving to ${folder.label || folder.path}`, failed, ids.length);
		stopSelecting();
		await load();
	}
	const moveItems = $derived.by<MenuItem[]>(() => {
		const kinds = new Set(pickedShown.map((t) => t.kind));
		if (kinds.size !== 1) return [];
		// An artist lives in a music folder.
		const k = [...kinds][0] === 'artist' ? 'music' : [...kinds][0];
		return folders.filter((f) => f.kind === k).map((f): MenuItem => ({ label: f.label || f.path, disabled: busy || !pickedShown.length, onselect: () => moveTitles(pickedShown.map((t) => t.id), f) }));
	});
	// Only rules that fit every ticked title (Music rules say "music", artist titles "artist").
	const rulesItems = $derived.by<MenuItem[]>(() => {
		const need = new Set((pickedShown.length ? pickedShown.map((t) => t.kind) : kind === 'all' ? [] : [kind]).map((k) => (k === 'artist' ? 'music' : k)));
		return rules.filter((r) => !r.kinds.length || [...need].every((k) => r.kinds.includes(k))).map((r): MenuItem => ({ label: r.name, disabled: busy || !pickedShown.length, onselect: () => setRules(pickedShown.map((t) => t.id), r) }));
	});
	// Phones get the bar's actions as one menu (Downloads does the same).
	const phoneActs = $derived<MenuItem[]>([
		{ label: allPicked ? 'Select none' : `Select all ${shown.length}`, icon: 'check', onselect: pickAll },
		{ kind: 'separator' },
		{ label: 'Keep up to date', icon: 'bellOn', disabled: busy || !pickedShown.length, onselect: () => setKeep(pickedShown.map((t) => t.id), true) },
		{ label: 'Stop keeping', icon: 'bell', disabled: busy || !pickedShown.length, onselect: () => setKeep(pickedShown.map((t) => t.id), false) },
		{ label: 'Search now', icon: 'search', disabled: busy || !pickedShown.length, onselect: () => searchTitles(pickedShown.map((t) => t.id)) },
		...(kind === 'movie' ? ([{ label: 'Find another copy', icon: 'copy', disabled: busy || !copyPicks.length, onselect: reviewCopies }] as MenuItem[]) : []),
		...(rulesItems.length ? ([{ kind: 'separator' }, { kind: 'heading', label: 'Quality' }, ...rulesItems] as MenuItem[]) : []),
		...(moveItems.length > 1 ? ([{ kind: 'separator' }, { kind: 'heading', label: 'Move to' }, ...moveItems] as MenuItem[]) : [])
	]);
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
	// Where a title opens: artists go to their music page from every view, the rest to the title page.
	function openLink(t: TitleView): string {
		return t.kind === 'artist' && t.ids?.musicbrainz ? artistHref(t.ids.musicbrainz) : titleLink(t.id);
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
			case 'codec': return t.copy && t.copy.codec !== 'unknown' ? codecLabel(t.copy.codec) : '';
			case 'audio': return (t.copy?.audio_languages ?? []).map(languageName).join(', ');
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
			case 'status': return libraryState(t)?.label ?? '';
			case 'requested': return requestedBy(t);
			case 'quality': {
				const have = quality(t);
				const want = t.versions?.[0]?.label;
				return have || want ? `${have ?? '—'} → ${want ?? '—'}` : '';
			}
			case 'kept': return t.keep_up_to_date ? 'Yes' : 'No';
			case 'showstate': return t.show?.status === 'ended' ? 'Ended' : t.show?.status ? 'Continuing' : '';
			case 'activity': return date(t.last_activity);
			default: return '';
		}
	}
	/** The "Everything" line under a poster: next airing for shows, else rules and quality. */
	function posterExtra(t: TitleView): string {
		if (t.kind === 'show' && t.show?.next_air_date) return `Next ${date(t.show.next_air_date)}`;
		return [t.pick_rules_name, quality(t)].filter(Boolean).join(' · ');
	}
	/** The hover panel: quality and pick rules, next airing, added, who asked. */
	function posterFacts(t: TitleView): string[] {
		const asked = requestedBy(t);
		return [
			[quality(t), t.pick_rules_name].filter(Boolean).join(' · '),
			t.kind === 'show' && t.show?.next_air_date ? `Next ${date(t.show.next_air_date)}` : '',
			t.added_at ? `Added ${date(t.added_at)}` : '',
			asked ? `Asked by ${asked}` : ''
		].filter(Boolean);
	}
	function markOf(t: TitleView) {
		const m = ownedMark({ key: String(t.id), kind: t.kind === 'show' ? 'show' : t.kind === 'artist' ? 'artist' : 'movie', name: t.name, titleId: t.id } as ExploreCard, t, who);
		const ls = who === 'staff' ? libraryState(t) : null;
		return m && ls ? { ...m, label: ls.label } : m;
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
			// My id comes from the session (the layout already read /auth/me).
			titles = await libraryTitles();
			myId = session.s.id;
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
<svelte:window onkeydown={selectKey} bind:scrollY onscroll={checkStuck} onresize={checkStuck} />

<div class="library" class:selecting>
	<h1 class="sr-only">{kindTitle}</h1>
	<div class="toolbar" class:stuck class:topbar={!phone} role="toolbar" aria-label="Library view" bind:this={toolbarEl}>
		{#if phone}
			{#if shelfItems.length > 1}
				<span class="count shelf"><Menu label="Show Movies, TV shows or Music. Now: {kindTitle}" variant="button" icon={kind === 'show' ? 'tv' : kind === 'artist' ? 'music' : 'film'} text={loaded && !error ? countText : kindTitle} items={shelfItems} width={200} align="start" /></span>
			{:else}
				<p class="count" aria-live="polite">{#if loaded && !error}{countText}{/if}</p>
			{/if}
		{:else}
			{#if dlTop.collapsed}<span class="cap"><IconButton icon="sidebar" label="Show sidebar" onclick={() => dlTop.toggle()} data-testid="library-side-btn" /></span>{/if}
			<DlFind bind:term={() => q, setQ} shown={shown.length} total={kindTotal} searchKey={dlTop.searchKey} placeholder={findWords} note={loaded && !error ? countText : ''} testid="library-find" />
		{/if}
		<span class="t-view cap"><LibraryViewSwitch value={view} options={[{ value: 'rows', label: 'Rows', icon: 'menu' }, { value: 'table', label: 'Banners', icon: 'image' }, { value: 'posters', label: 'Posters', icon: 'grid' }, { value: 'overview', label: 'Overview', icon: 'info' }]} onchange={(v) => setView(v)} sizeItems={!phone && (view === 'table' || view === 'posters') ? sizeItems : null} sizeLabel={view === 'posters' ? posterLabels[posterSize] : densityLabels[density]} compact={phone || listWidth < 1100} /></span>
		<span class="filter-btn cap" class:on={filtersOn > 0}>
			{#if phone}
				<Button size="md" icon="sliders" aria-haspopup="dialog" onclick={() => (sortOpen = true)} data-testid="library-sort-filter">{filtersOn ? `Sort & filter · ${filtersOn}` : 'Sort & filter'}</Button>
			{:else}
				<Popover label="Sort and filter" variant="button" icon="sliders" text={filtersOn ? `Sort & filter · ${filtersOn}` : 'Sort & filter'} width={620} bind:open={sortOpen}>
					{#snippet children()}{@render sortFilter()}{/snippet}
				</Popover>
			{/if}
		</span>
		{#if phone}
			<span class="t-find"><IconButton icon="search" label={findWords} aria-pressed={findOpen || !!q} onclick={() => { findOpen = !findOpen || !!q; if (findOpen) void tick().then(() => document.getElementById('find')?.focus()); }} /></span>
		{/if}
		<span class="tool-gap"></span>
		<span class="t-tools cap">
		{#if view !== 'posters' && kind !== 'all' && !phone}<span class="t-cols"><ColumnChooser {columns} onchange={setColumns} onreset={() => setColumns(columnsFor(libKind))} /></span>{/if}
		{#if canManage && shown.length}<span class="t-sel">{#if phone}<IconButton icon="check" variant={selecting ? 'filled' : 'tinted'} label={selecting ? 'Done' : 'Select'} onclick={() => (selecting ? stopSelecting() : startSelecting())} data-testid="library-select" />{:else}<Button variant={selecting ? 'primary' : 'secondary'} size="md" icon="check" onclick={() => (selecting ? stopSelecting() : startSelecting())} data-testid="library-select">{selecting ? 'Done' : 'Select'}</Button>{/if}</span>{/if}
		{#if canManage}
			<span class="t-add"><Menu label="Add to the Library" variant={phone ? 'icon' : 'button'} text={phone ? undefined : 'Add'} icon="plus" items={addItems} width={190} align="end" /></span>
		{/if}
		</span>
		{#if phone && (findOpen || q)}
			<div class="find-box rise">
				<span class="find-ic" aria-hidden="true"><Icon name="search" size={16} /></span>
				<label class="sr-only" for="find">{findWords}</label>
				<input id="find" class="input find" type="search" bind:value={q} oninput={(event) => { q = event.currentTarget.value; syncUrl(); }} onblur={() => { if (!q) findOpen = false; }} placeholder={findWords} autocomplete="off" maxlength="256" />
			</div>
		{/if}
	</div>
	{#if phone}
		<Sheet bind:open={sortOpen} title="Sort and filter" placement="bottom">{@render sortFilter()}{#snippet footer()}<div class="sf-foot sf-foot-sheet">{@render sfFoot()}</div>{/snippet}</Sheet>
	{/if}

	{#if filtersOn || q || grouped}
		<div class="chips" role="group" aria-label="Active filters" in:grow out:shrink>
			{#if filter.status !== 'all'}<Chip label={filters.find(([f]) => f === filter.status)?.[1] ?? ''} selected onclick={() => setFilter({ status: 'all' })} onremove={() => setFilter({ status: 'all' })} />{/if}
			{#if filter.kept !== 'any'}<Chip label={filter.kept === 'on' ? 'Kept' : 'Not kept'} selected onclick={() => setFilter({ kept: 'any' })} onremove={() => setFilter({ kept: 'any' })} />{/if}
			{#if filter.asker !== 'anyone'}<Chip label="Asked by {filter.asker === 'me' ? 'me' : (askers.find((p) => String(p.id) === filter.asker)?.name ?? 'someone')}" selected onclick={() => setFilter({ asker: 'anyone' })} onremove={() => setFilter({ asker: 'anyone' })} />{/if}
			{#if filter.series}<Chip label="Movie series: {seriesName(filter.series)}" selected onclick={() => setFilter({ series: 0 })} onremove={() => setFilter({ series: 0 })} />{/if}
			{#if filter.codec}<Chip label={codecLabel(filter.codec)} selected onclick={() => setFilter({ codec: '' })} onremove={() => setFilter({ codec: '' })} />{/if}
			{#if filter.subs}<Chip label="Missing subtitles" selected onclick={() => setFilter({ subs: false })} onremove={() => setFilter({ subs: false })} />{/if}
			{#if q}<Chip label="Search: {q}" selected onclick={clearSearch} onremove={clearSearch} />{/if}
			{#if grouped}<Chip label="Grouped by series" selected onclick={() => { groupSets = false; syncUrl(); }} onremove={() => { groupSets = false; syncUrl(); }} />{/if}
			<Button variant="plain" size="md" onclick={clearFilters}>Clear</Button>
			{#if canManage && filter.subs && shown.length}<span class="t-miss"><Button size="md" icon="search" busy={subsSearching} onclick={searchMissingSubs} data-testid="search-missing-subtitles">Search subtitles</Button></span>{/if}
			{#if canManage && filter.status === 'missing' && missingCount}<span class="t-miss"><Button size="md" icon="search" disabled={searchingAll} onclick={() => missingDialog?.showModal()} data-testid="search-all-missing">Search all missing{missingWhat}</Button></span>{/if}
		</div>
	{/if}
	{#if searchError && !error}<p class="search-error" role="alert"><Icon name="alert" size={16} />{plainReason(searchError).text}</p>{/if}

	<!-- The rail column is kept while loading too, so the cards do not narrow when it appears. -->
	<div class:has-rail={railOn || (!loaded && sort.key === 'name' && !grouped)} class="with-rail">
		<div class="content">
			<PageLoader loading={!loaded} label="Loading the Library" onretry={retryLoad}>
			{#if error}
				<LoadError hasData={false} message={plainReason(error).text} onretry={retryLoad} />
			{:else if titles.length === 0}
				<EmptyState icon="library" title="Nothing here yet" action={{ label: 'Add a movie', href: to('/search'), icon: 'search' }} />
				<div class="empty-more">
					{#if partOn('shows')}<Button href={to('/search?kind=show')}>Add a show</Button>{/if}
					{#if partOn('music')}<Button href={to('/search?kind=music')}>Add an artist</Button>{/if}
					{#if session.can('settings')}<Button href={to('/settings/library-folders')}>Folders</Button>{/if}
				</div>
			{:else if shown.length === 0}
				<EmptyState compact icon="search" title={q && !filtersOn ? `Nothing for “${q}”` : 'Nothing matches'} action={filtersOn || q ? { label: 'Clear filters', onclick: clearFilters } : undefined} />
			{:else if view === 'posters'}
				<ul class="grid" class:selecting style="--poster-min: {gridMin}px" style:--poster-min-phone={kind === 'artist' ? `${squarePhoneMin[posterSize]}px` : undefined} aria-label="{kindTitle} titles" bind:this={gridEl}>
					{#each shown as t (t.id)}
						{@const p = progressOf(t)}
						{@const ls = who === 'staff' ? libraryState(t) : null}
						<li class="item" style:--poster-ratio={posterRatio(t.kind)} animate:flip={{ duration: moveMs, easing: easeOut }} data-testid="library-card" data-title-id={t.id} data-letter={letterOf(sortName(t.name))} class:selected={picked.has(t.id)} class:last-opened={lastOpenedId === t.id}>
							<LibraryPoster title={t.name} year={t.year} href={openLink(t)} image={posterSrc(t.artwork[0], gridMin)} kind={t.kind === 'artist' ? 'artist' : t.kind === 'show' ? 'show' : 'movie'} vtName={artName(t.id)} status={ls} attention={needsAttention(ls)} bar={barOf(ls, p.have, p.total)} detail={posterDetail} extra={posterExtra(t)} facts={posterFacts(t)} kept={t.keep_up_to_date} {canManage} {selecting} selected={picked.has(t.id)} onsearch={() => searchTitles([t.id])} onkeep={(k) => setKeep([t.id], k)} />
							{#if selecting}
								<button type="button" class="pick" aria-pressed={picked.has(t.id)} aria-label="Select {t.name}" onclick={() => togglePick(t.id)} data-testid="library-pick"><span class="pick-dot" aria-hidden="true">{#if picked.has(t.id)}<Icon name="check" size={14} stroke={3} />{/if}</span></button>
							{/if}
							{#if lastOpenedId === t.id}<span class="sr-only" data-testid="last-opened">Last opened</span>{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<div class="rows {view}" class:v-rows={view === 'rows'} class:has-select={selecting} style="--acts-w: {actsW}px; --cols: {gridCols}; --table-min: {tableMin}px; --banner-w: {bannerW}px; --row-pad: {20 + touchActs}px" bind:this={listEl} bind:clientWidth={listWidth}>
					{#if view === 'rows' && !phone}
						<div class="lh" role="presentation" style="--facts: {factsTpl}">
							{#if selecting}<span></span>{/if}
							<span></span>
							<button type="button" class="h sort" aria-label={sort.key === 'name' ? `Title, sorted ${sort.dir === 'asc' ? 'low to high' : 'high to low'}` : 'Title'} aria-pressed={sort.key === 'name'} onclick={() => setSort('name')}>Title{#if sort.key === 'name'}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
							<span class="h">Status</span>
							{#each rowCols as c (c.id)}
								{@const sk = columnSort[c.id]}
								{#if sk && sortsFor(kind === 'all' ? 'all' : libKind).includes(sk)}
									<button type="button" class="h sort" aria-label={sort.key === sk ? `${c.label}, sorted ${sort.dir === 'asc' ? 'low to high' : 'high to low'}` : c.label} aria-pressed={sort.key === sk} onclick={() => setSort(sk)}>{c.label}{#if sort.key === sk}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
								{:else}
									<span class="h">{c.label}</span>
								{/if}
							{/each}
							<span></span>
						</div>
					{/if}
					{#if view === 'table' && !phone}
						<div class="row head" role="presentation">
							<span class="h banner-h">Banner</span>
							<span></span>
							<button type="button" class="h sort" aria-label={sort.key === 'name' ? `Title, sorted ${sort.dir === 'asc' ? 'low to high' : 'high to low'}` : 'Title'} aria-pressed={sort.key === 'name'} onclick={() => setSort('name')}>Title{#if sort.key === 'name'}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
							{#if kind !== 'all'}
								{#each shownCols as c (c.id)}
									{@const sk = columnSort[c.id]}
									{#if sk && sortsFor(libKind).includes(sk)}
										<button type="button" class="h sort" class:num={c.numeric} aria-label={sort.key === sk ? `${c.label}, sorted ${sort.dir === 'asc' ? 'low to high' : 'high to low'}` : c.label} aria-pressed={sort.key === sk} onclick={() => setSort(sk)}>{c.label}{#if sort.key === sk}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
									{:else}
										<span class="h" class:num={c.numeric}>{c.label}</span>
									{/if}
								{/each}
							{/if}
							{#if hiddenCols > 0}<span class="h more" title="{hiddenCols} more {hiddenCols === 1 ? 'column shows' : 'columns show'} when there is room">+{hiddenCols}</span>{/if}
						</div>
					{/if}
					<div class="body" style:height={virtual ? `${offsets[entries.length]}px` : undefined}>
						<div style:padding-top={virtual ? `${offsets[range.start]}px` : undefined} role="list" aria-label="{kindTitle} titles">
							{#each slice as e (e.key)}
								{#if e.type === 'head'}
									<div class="series-head" use:measure={e.key} data-key={e.key}>
										<h2>{e.name}</h2>
										<span>{e.count} in the Library</span>
										{#if e.id}<a href={to(`/library/collections/${e.id}`)}>Open series</a>{/if}
									</div>
								{:else}
									{@const t = e.t}
									{@const st = exceptionChip(filter.status, posterStatus(t, who))}
									{@const ls = libraryState(t)}
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
										class:first={e.key === entries[0]?.key}
										use:measure={e.key}
										onkeydown={(ev) => rowKey(ev, t)}
										onfocus={() => (active = ri)}
										onclick={(ev) => rowClick(ev, t)}
									>
										{#if view === 'rows'}
											<div class="lr" class:ph={phone} style="--facts: {factsTpl}; --row-h: {phone ? 60 : 54}px">
												{#if selecting}<span class="lsel"><input type="checkbox" class="pick-box lpick" checked={picked.has(t.id)} aria-label="Select {t.name}" tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" /></span>{/if}
												<span class="lp" style:--poster-ratio={posterRatio(t.kind)} class:round={t.kind === 'artist'}>
													<Poster src={t.artwork[0]} name={t.name} />
												</span>
												<span class="lt">
													<a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>
													{#if !phone && t.year}<small>{t.year}</small>{/if}
													{#if phone}{@const fc = rowCols.find((c) => !isProgressCol(c)) ?? rowCols[0]}<span class="lsub">{[ls?.label, fc ? rowCell(t, fc, p) : ''].filter(Boolean).join(' · ')}</span>{/if}
												</span>
												{#if !phone}
													<span class="ls {ls?.tone ?? 'idle'}" data-testid="library-status" data-state={ls?.key}><i class="dot" aria-hidden="true"></i>{ls?.label ?? '—'}</span>
													{#each rowCols as c (c.id)}<span class="lc" title={c.id === 'path' ? cell(t, c) : undefined}>{rowCell(t, c, p) || '—'}</span>{/each}
													<span class="acts">
														{#if canManage && !selecting}
															<IconButton icon="search" size="sm" tipSide="below-end" label="Search now for {t.name}" tabindex={ri === active ? 0 : -1} onclick={() => searchTitles([t.id])} />
															<IconButton icon="refresh" size="sm" tipSide="below-end" label="Refresh {t.name}" tabindex={ri === active ? 0 : -1} onclick={() => refreshTitle(t)} />
															<IconButton icon="tools" size="sm" tipSide="below-end" label="Edit {t.name}" tabindex={ri === active ? 0 : -1} href={titleLink(t.id)} />
														{/if}
													</span>
												{:else}
													<small class="lyr">{t.year ?? ''}</small>
												{/if}
												<span class="sr-only">{progressText(t)}</span>
											</div>
										{:else if view === 'table'}
											<div class="row" style:--row-h="{rowH(t.kind)}px">
												<span class="art"><BannerArt title={t} width={phone ? listWidth : bannerW} /></span>
												<span class="state" title={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'}>
													{#if selecting}<input type="checkbox" class="pick-box" checked={picked.has(t.id)} aria-label="Select {t.name}" tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />
													{:else}<span class="keep" class:on={t.keep_up_to_date} aria-label={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'} role="img"><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={16} /></span>{/if}
												</span>
												<span class="name">
													<a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>
													<span class="sub">
														{#if kind === 'all'}{kindLabel[t.kind]}{#if t.year} · {t.year}{/if}{/if}
														{#if ls && (kind === 'all' || phone || !shownCols.some((c) => c.id === 'status'))}<span class="ls {ls.tone}" data-testid="library-status" data-state={ls.key}><i class="dot" aria-hidden="true"></i>{ls.label}</span>{/if}
													</span>
													{#if phone}<span class="facts">{factCols.slice(0, 3).map((c) => (isProgressCol(c) ? `${p.have}/${p.total}` : cell(t, c))).filter(Boolean).join(' · ')}</span>{/if}
												</span>
												{#if kind !== 'all'}
													{#each shownCols as c (c.id)}
														{#if c.id === 'status'}
															<span class="c lstate {ls?.tone ?? 'idle'}" data-testid="library-status" data-state={ls?.key}>
																{#if ls}
																	<span class="ls-line ls {ls.tone}"><i class="dot" aria-hidden="true"></i><span class="ls-t">{ls.label}</span></span>
																	{#if ls.percent !== undefined}<span class="meter ls-bar" aria-hidden="true"><i style="width: {ls.percent}%"></i></span>{/if}
																{:else}—{/if}
															</span>
														{:else if c.id === 'kept'}
															<span class="c keep-c" title={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'}><span class="keep" class:on={t.keep_up_to_date} role="img" aria-label={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'}><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={16} /></span></span>
														{:else if isProgressCol(c)}
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
														<IconButton icon="search" size="sm" tipSide="below-end" label="Search now for {t.name}" tabindex={ri === active ? 0 : -1} onclick={() => searchTitles([t.id])} />
														<IconButton icon="refresh" size="sm" tipSide="below-end" label="Refresh {t.name}" tabindex={ri === active ? 0 : -1} onclick={() => refreshTitle(t)} />
														<IconButton icon="tools" size="sm" tipSide="below-end" label="Edit {t.name}" tabindex={ri === active ? 0 : -1} href={titleLink(t.id)} />
													{/if}
												</span>
												{#if kind === 'all' || phone}<span class="sr-only">{progressText(t)}</span>{/if}
											</div>
										{:else}
											<div class="ov" class:music={t.kind === 'artist'}>
												<a class="ov-poster" style:--poster-ratio={posterRatio(t.kind)} href={openLink(t)} tabindex="-1" aria-hidden="true"><Poster src={t.artwork[0]} name={t.name} /></a>
												<div class="ov-main">
													<h2><a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>{#if t.year}<span class="yr">{t.year}</span>{/if}<span class="keep ov-keep" class:on={t.keep_up_to_date} role="img" aria-label={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'} title={t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date'}><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={15} /></span></h2>
													{#if t.kind === 'artist'}
														{#if t.artist?.bio}<p class="ov-text ov-bio">{t.artist.bio}</p>{/if}
													{:else}
														<p class="ov-text">{t.overview || 'No description yet.'}</p>
													{/if}
													{#if t.kind !== 'movie'}
														<span class="ov-prog">
															<span class="meter {tone(t)}" aria-hidden="true"><i style="width: {p.total ? (p.have / p.total) * 100 : 0}%"></i></span>
															{#if p.total}<span class="ov-count" aria-hidden="true">{p.have}/{p.total}</span>{/if}
														</span>
													{/if}
													<span class="sr-only">{progressText(t)}</span>
												</div>
												<!-- The status leads on its own line (never cut short); under it only facts that say something. -->
												<dl class="ov-facts">
													{#if ls}<dt class="sr-only">Status</dt><dd class="ov-state"><span class="ls {ls.tone}"><i class="dot" aria-hidden="true"></i>{ls.label}</span></dd>{/if}
													{#each chosenCols.filter((c) => !isProgressCol(c) && c.id !== 'status' && c.id !== 'kept').slice(0, 4) as c (c.id)}{#if cell(t, c)}<dt>{c.label}</dt><dd title={cell(t, c)}>{cell(t, c)}</dd>{/if}{/each}
												</dl>
												{#if selecting}<input type="checkbox" class="pick-box ov-pick" checked={picked.has(t.id)} aria-label="Select {t.name}" tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />{/if}
											</div>
										{/if}
										{#if fold}
											{@const eps = missingOf[t.id]}
											<div class="fold">
												<Button variant="plain" size="sm" icon="chevronRight" class="fold-btn" aria-expanded={foldOpen === t.id} aria-controls="missing-{t.id}" onclick={() => toggleMissing(t)} data-testid="missing-fold">{t.show?.missing} {t.show?.missing === 1 ? 'episode' : 'episodes'} missing</Button>
												{#if foldOpen === t.id}
													<div id="missing-{t.id}" class="fold-body" data-testid="missing-episodes" in:grow out:shrink>
														{#if eps === 'loading' || eps === undefined}<Skeleton lines={2} />
														{:else if eps === 'error'}<p class="fold-note">Episodes didn’t load. <a href={titleLink(t.id)}>Open {t.name}</a></p>
														{:else if eps.length === 0}<p class="fold-note">Nothing missing.</p>
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
										{#if lastOpenedId === t.id}<span class="sr-only" data-testid="last-opened">Last opened</span>{/if}
										{#if requester && view !== 'rows'}
											<p class="row-notes">
												<span class="requester" data-testid="requested-by">Asked by {requester}</span>
											</p>
										{/if}
									</div>
								{/if}
							{/each}
						</div>
					</div>
				</div>
			{/if}
			</PageLoader>
		</div>
		{#if railOn && loaded && !error}{@render letterRail()}{/if}
	</div>

	{#if selecting}
		<div class="select-bar" role="toolbar" aria-label="Selected titles" data-testid="select-bar" in:dropIn out:dropOut>
			<span class="count" aria-live="polite">{pickedShown.length} selected</span>
			{#if !phone}<Button variant="plain" size="md" onclick={pickAll}>{allPicked ? 'Select none' : `Select all ${shown.length}`}</Button>{/if}
			<span class="bar-gap"></span>
			{#if phone}
				<Menu label="Actions for the selected titles" icon="more" items={phoneActs} width={240} placement="above" align="end" />
			{:else}
				<Button size="md" icon="bellOn" disabled={busy || !pickedShown.length} onclick={() => setKeep(pickedShown.map((t) => t.id), true)}>Keep up to date</Button>
				<Button size="md" icon="bell" disabled={busy || !pickedShown.length} onclick={() => setKeep(pickedShown.map((t) => t.id), false)}>Stop keeping</Button>
				{#if moveItems.length > 1}<Menu label="Move the selected titles to another folder" variant="button" text="Move" icon="folder" items={moveItems} width={260} placement="above" />{/if}
				{#if rulesItems.length}<Menu label="Quality for the selected titles" variant="button" text="Quality" icon="star" items={rulesItems} width={220} placement="above" />{/if}
				<Button size="md" icon="search" disabled={busy || !pickedShown.length} onclick={() => searchTitles(pickedShown.map((t) => t.id))}>Search now</Button>
				{#if kind === 'movie'}<Button size="md" icon="copy" disabled={busy || !copyPicks.length} onclick={reviewCopies} data-testid="select-find-copy">Find another copy</Button>{/if}
			{/if}
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

<dialog bind:this={copyDialog} aria-labelledby="find-copy-title" class="modal" data-testid="find-copy" onclose={() => (copyPreview = null)}>
	{#if copyPreview}
		<h2 id="find-copy-title" class="h2">Find another copy of {copyPreview.copies.length} {copyPreview.copies.length === 1 ? 'movie' : 'movies'}?</h2>
		<p class="small muted">{copyPreview.slot_count} {copyPreview.slot_count === 1 ? 'search' : 'searches'} at each movie's Quality. Nothing downloads on its own.</p>
		<ul class="copy-list">{#each copyPreview.copies as c (`${c.file_id}:${c.version_id}`)}<li><b>{c.title}</b><span>{c.version} · {codecLabel(c.codec)}</span></li>{/each}</ul>
	{/if}
	<div class="modal-actions">
		<Button onclick={() => copyDialog?.close()} disabled={busy}>Cancel</Button>
		<Button variant="primary" icon="search" {busy} onclick={findCopies} data-testid="find-copy-confirm">Search</Button>
	</div>
</dialog>

<dialog bind:this={missingDialog} aria-labelledby="search-missing-title" class="modal" data-testid="search-missing">
	<h2 id="search-missing-title" class="h2">Search for {missingCount} missing {missingCount === 1 ? 'title' : 'titles'}?</h2>
	<p class="small muted">Kept titles only, a few at a time. Progress on Home.</p>
	<div class="modal-actions">
		<Button onclick={() => missingDialog?.close()} disabled={searchingAll}>Cancel</Button>
		<Button variant="primary" icon="search" busy={searchingAll} onclick={searchAllMissing} data-testid="search-missing-confirm">Search</Button>
	</div>
</dialog>

<dialog bind:this={saveDialog} aria-labelledby="save-filter-title" class="modal">
	<form method="dialog" onsubmit={(e) => { e.preventDefault(); saveCurrent(); }}>
		<h2 id="save-filter-title" class="h2">Save this filter</h2>
		<label class="small muted" for="save-filter-name">Name</label>
		<input id="save-filter-name" class="input" bind:value={saveName} maxlength="60" />
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

{#snippet sfRow(label: string, on: boolean, pick: () => void, hint = '', extra = '')}
	<button type="button" class="sf-o" class:on aria-pressed={on} onclick={pick}>
		<span class="sf-tick" aria-hidden="true">{#if on}<Icon name="check" size={14} stroke={2.4} />{/if}</span><span class="sf-l">{label}</span>{#if extra}<span class="sf-x" aria-hidden="true">{extra}</span>{/if}{#if hint}<span class="sf-n">{hint}</span>{/if}
	</button>
{/snippet}

{#snippet sortFilter()}
	<div class="sf">
		<section class="sf-col sf-sort" aria-label="Sort">
			<h3 class="sf-h">Sort by</h3>
			{#each sortsFor(kind === 'all' ? 'all' : kind) as k (k)}
				{@render sfRow(sortLabel(k, kind === 'all' ? 'all' : kind), sort.key === k, () => setSort(k, sort.key === k ? (sort.dir === 'asc' ? 'desc' : 'asc') : startDir(k)), '', sort.key === k ? (sort.dir === 'asc' ? '↑' : '↓') : '')}
			{/each}
			<div class="sf-dir"><SegmentedControl label="Order" size="sm" value={sort.dir} options={[{ value: 'asc', label: 'Ascending' }, { value: 'desc', label: 'Descending' }]} onchange={(d) => setSort(sort.key, d)} /></div>
			{#if kind === 'movie'}
				<span class="sf-sep" aria-hidden="true"></span>
				{@render sfRow('Group by series', groupSets, () => { groupSets = !groupSets; syncUrl(); })}
			{/if}
		</section>
		<section class="sf-col sf-filter" aria-label="Filter">
			<h3 class="sf-h">Status</h3>
			<div class="sf-chips">
				{#each filters as [f, label] (f)}<span class="sf-chip t-{f}"><Chip {label} count={loaded ? counts.status[f] : undefined} selected={filter.status === f} onclick={() => setFilter({ status: f })} /></span>{/each}
			</div>
			<h3 class="sf-h">Keep up to date</h3>
			<div class="sf-chips">
				<Chip label="Kept" icon="bellOn" count={loaded ? counts.kept.on : undefined} selected={filter.kept === 'on'} onclick={() => setFilter({ kept: filter.kept === 'on' ? 'any' : 'on' })} />
				<Chip label="Not kept" icon="bell" count={loaded ? counts.kept.off : undefined} selected={filter.kept === 'off'} onclick={() => setFilter({ kept: filter.kept === 'off' ? 'any' : 'off' })} />
			</div>
			{#if askers.length || myId}
				<h3 class="sf-h">Asked by</h3>
				<div class="sf-chips">
					<Chip label="Anyone" selected={filter.asker === 'anyone'} onclick={() => setFilter({ asker: 'anyone' })} />
					{#if myId}<Chip label="Me" count={loaded ? (counts.askers.get(myId) ?? 0) : undefined} selected={filter.asker === 'me'} onclick={() => setFilter({ asker: 'me' })} />{/if}
					{#each askers.filter((p) => p.id !== myId) as p (p.id)}<Chip label={p.name} count={loaded ? (counts.askers.get(p.id) ?? 0) : undefined} selected={filter.asker === String(p.id)} onclick={() => setFilter({ asker: String(p.id) })} />{/each}
				</div>
			{/if}
			{#if kind === 'movie' && counts.codecs.size}
				<h3 class="sf-h">Codec</h3>
				<div class="sf-chips">
					{#each [...counts.codecs].toSorted((a, b) => b[1] - a[1]) as [c, n] (c)}<Chip label={codecLabel(c)} count={n} selected={filter.codec === c} onclick={() => setFilter({ codec: filter.codec === c ? '' : c })} />{/each}
				</div>
			{/if}
			{#if (kind === 'movie' || kind === 'show') && subsTitles && (counts.subs || filter.subs)}
				<h3 class="sf-h">Subtitles</h3>
				<div class="sf-chips">
					<Chip label="Missing" icon="subtitle" count={counts.subs} selected={!!filter.subs} onclick={() => setFilter({ subs: !filter.subs })} />
				</div>
			{/if}
			{#if (kind === 'movie' || kind === 'all') && series.length}
				<h3 class="sf-h">Movie series</h3>
				<div class="sf-well">
					{@render sfRow('Every movie', !filter.series, () => setFilter({ series: 0 }))}
					{#each series.toSorted((x, y) => x.name.localeCompare(y.name)) as sr (sr.id)}
						{@render sfRow(sr.name, filter.series === sr.id, () => setFilter({ series: sr.id }), countHint(counts.series.get(sr.id) ?? 0))}
					{/each}
				</div>
			{/if}
			{#if canManage && (partOn('movies') || partOn('shows') || partOn('music'))}
				<span class="sf-sep" aria-hidden="true"></span>
				<a class="sf-o sf-link" href={to('/library/deleted')} onclick={() => (sortOpen = false)} data-testid="library-deleted"><span class="sf-tick" aria-hidden="true"><Icon name="trash" size={14} /></span><span class="sf-l">Recently deleted</span>{#if deletedCount}<span class="sf-n">{deletedCount}</span>{/if}</a>
			{/if}
		</section>
		{#if phone && view === 'posters'}
			<section class="sf-col sf-size" aria-label="Posters">
				{#each sizeItems as it, i (i)}
					{#if it.kind === 'heading'}<h3 class="sf-h">{it.label}</h3>
					{:else if it.kind !== 'separator'}{@render sfRow(it.label, !!it.checked, () => it.onselect?.())}{/if}
				{/each}
			</section>
		{/if}
		{#if !phone}<footer class="sf-foot">{@render sfFoot()}</footer>{/if}
	</div>
{/snippet}

{#snippet sfFoot()}
			<div class="sf-saved">
				{#each saved as sv (sv.name)}<Chip label={sv.name} icon="star" selected={sameFilter(sv.filter, filter)} onclick={() => setFilter(sv.filter)} onremove={() => forget(sv.name)} />{/each}
				<Button variant="plain" size="sm" icon="plus" disabled={!filtersOn} onclick={() => { sortOpen = false; saveName = ''; saveDialog?.showModal(); }}>Save this filter</Button>
			</div>
			<Button variant="plain" size="sm" disabled={!filtersOn} onclick={clearFilters}>Clear filters</Button>
{/snippet}

<style>
	.library { display: flex; flex-direction: column; gap: 14px; padding-bottom: 48px; min-width: 0;
		/* The sheet Home's tiles and the Requests cards are made of: lit from above, a hairline, one long soft shadow. */
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.library.selecting { padding-bottom: 112px; }

	/* One row, and the page has no top bar above it (as on Downloads): search, view and size, sort and filter,
	   columns, select, add. Each group sits in the same glass capsule the Downloads row uses. Pinned, it is one solid band. */
	.toolbar { position: sticky; top: 0; z-index: 6; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: calc(-1 * var(--s-6, 24px)) calc(-1 * var(--gutter, 0px)) 0; padding: 10px var(--gutter, 0px); background: var(--bg); border-bottom: 1px solid transparent; transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.toolbar.stuck { border-bottom-color: var(--line); box-shadow: 0 10px 24px rgb(0 0 0 / .08); }
	.toolbar.topbar .cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.toolbar.topbar .cap:empty { display: none; }
	.toolbar.topbar .tool-gap { display: none; }
	/* Inside a capsule every control is flat and 44px tall; the capsule is its frame. */
	.toolbar.topbar .cap :global(.ui-btn.secondary), .toolbar.topbar .cap :global(.trigger.button) { height: 44px; min-height: 44px; padding-inline: 12px; border-radius: 10px; background: none; box-shadow: none; }
	.toolbar.topbar .cap :global(.ui-btn.primary) { height: 44px; min-height: 44px; border-radius: 10px; }
	.toolbar.topbar .cap :global(.ui-btn.secondary:hover), .toolbar.topbar .cap :global(.trigger.button:hover), .toolbar.topbar .cap :global(.trigger.button[aria-expanded='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.toolbar.topbar .cap :global(.ui-ibtn) { width: 44px; height: 44px; border-radius: 10px; }
	.toolbar.topbar .t-view { --vs-bg: transparent; }
	.toolbar.topbar .t-view :global(.vs) { padding: 0; gap: 2px; }
	.toolbar.topbar .t-view :global(.vs button[role='radio']) { min-height: 44px; min-width: 44px; }
	.toolbar.topbar .t-view :global(.vs .seg) { border-radius: 10px; }
	.toolbar.topbar .filter-btn.on :global(.trigger.button) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.t-tools, .t-sel, .t-add, .t-cols { display: contents; }
	.chips .t-miss { display: inline-flex; margin-left: auto; }
	.toolbar.topbar .t-tools { display: flex; margin-left: auto; }
	/* A narrower page keeps the one row: first Columns and Select fold to their icons, then Tools and Add too.
	   A folded name is only hidden from the eye, so the button keeps its name for screen readers. */
	.toolbar.topbar { container-type: inline-size; }
	.toolbar.topbar :global(.dlfind) { flex-basis: 200px; min-width: 180px; }
	@container (max-width: 1000px) {
		.toolbar.topbar .t-cols :global(.trigger.button > span), .toolbar.topbar .t-cols :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.toolbar.topbar .t-sel :global(.ui-btn) { font-size: 0; gap: 0; }
		.toolbar.topbar .t-sel :global(.ui-btn), .toolbar.topbar .t-cols :global(.trigger.button) { width: 44px; padding-inline: 0; justify-content: center; }
	}
	@container (max-width: 780px) {
		.toolbar.topbar :global(.dlfind) { flex-basis: 150px; min-width: 150px; }
		.toolbar.topbar :global(.dlfind .kb) { display: none; }
	}
	@container (max-width: 1000px) {
		.toolbar.topbar .t-tools :global(.trigger.button > span), .toolbar.topbar .t-tools :global(.trigger.button > svg:last-child), .toolbar.topbar .filter-btn :global(.trigger.button > span), .toolbar.topbar .filter-btn :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.toolbar.topbar .t-tools :global(.trigger.button), .toolbar.topbar .filter-btn :global(.trigger.button) { width: 44px; padding-inline: 0; justify-content: center; }
	}
	.toolbar .count { margin: 0 8px 0 0; min-width: 0; font: var(--t-headline); font-weight: 650; color: var(--text); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.toolbar .count:empty { display: none; }
	.toolbar .count.shelf { display: inline-flex; min-width: 0; }
	.count.shelf :global(button[aria-haspopup]) { min-width: 0; max-width: 100%; padding-inline: 0 8px; background: none; box-shadow: none; font: var(--t-headline); font-weight: 650; color: var(--text); }
	.count.shelf :global(button[aria-haspopup] span) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* Sort and filter: one panel in the app's own material. Sort is a ticked list (as in every menu),
	   filters are the same chips the page shows once they are on. */
	.sf { display: grid; grid-template-columns: 212px minmax(0, 1fr); padding: 6px; }
	.sf-col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.sf-sort { padding-right: 12px; }
	.sf-filter { padding-left: 16px; border-left: 1px solid var(--line); }
	.sf-h { margin: 10px 8px 6px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.sf-col > .sf-h:first-child { margin-top: 6px; }
	.sf-filter .sf-h { margin-inline: 2px; }
	.sf-sep { height: 1px; margin: 6px 8px; background: var(--line); }
	.sf-o { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 32px; padding: 0 8px; border-radius: 8px; color: var(--text); font: 500 13.5px/1.2 var(--font-ui); text-align: left; transition: background-color 120ms ease; }
	.sf-o + .sf-o { margin-top: 2px; }
	.sf-link { text-decoration: none; }
	.sf-link .sf-tick { color: var(--text-muted); }
	.sf-o:hover { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.sf-o:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sf-o.on { font-weight: 650; }
	.sf-tick { flex: none; display: grid; place-items: center; width: 14px; color: var(--accent-text, var(--accent)); }
	.sf-l { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sf-x { flex: none; color: var(--accent-text, var(--accent)); font-weight: 700; }
	.sf-n { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.sf-dir { margin: 10px 4px 4px; }
	.sf-dir :global(.ui-seg) { width: 100%; }
	.sf-chips { display: flex; flex-wrap: wrap; gap: 6px; }
	/* A status chip carries its status colour as a dot, the same colours the rows use. */
	.sf-chip { display: inline-flex; }
	.sf-chip:not(.t-all) :global(.main)::before { content: ''; flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--dot, var(--text-faint)); }
	.sf-chip.t-missing { --dot: var(--warn, #d9822b); }
	.sf-chip.t-not_out_yet { --dot: var(--text-muted); }
	.sf-chip.t-better { --dot: var(--info, #409cff); }
	.sf-chip.t-have { --dot: var(--ok, #2fa36b); }
	/* The series list is long: it scrolls inside its own well, and takes the height the Sort column leaves. */
	.sf-well { flex: 1 1 0; min-height: 104px; overflow-y: auto; overscroll-behavior: contain; padding: 4px; border-radius: 10px; background: color-mix(in srgb, var(--text) 4%, transparent); box-shadow: inset 0 0 0 1px var(--line); scrollbar-width: thin; scrollbar-color: var(--line-strong, var(--text-faint)) transparent; }
	/* Tools: tiles with the Settings pages' glossy icons, two across (one on phones). */
	/* A count sits on the icon's corner, so every tile keeps the same shape: red when it calls for action, grey when it only tells. */
	/* The footer stays at the panel's edge while the lists scroll under it (the panel has 6px padding). */
	.sf-foot { grid-column: 1 / -1; position: sticky; bottom: -6px; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: 8px -6px -6px; padding: 8px 8px 6px; background: var(--surface-1); border-top: 1px solid var(--line); }
	.sf-foot-sheet { position: static; margin: 0; padding: 0; border: 0; background: none; width: 100%; }
	.sf-saved { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
	@media (pointer: coarse) { .sf-o { min-height: 44px; } }
	.find-box { position: relative; flex: 1 1 220px; max-width: 360px; min-width: 0; }
	.find-ic { position: absolute; left: 11px; top: 50%; translate: 0 -50%; color: var(--text-muted); display: grid; pointer-events: none; }
	.find { width: 100%; min-height: 40px; padding-left: 34px; border-radius: var(--r-btn); background: var(--surface-2); border-color: transparent; }
	/* Touch phones get the full 44px tap height; desktop keeps the slimmer field. */
	@media (pointer: coarse) and (max-width: 767px) { .find { min-height: 44px; } }
	.tool-gap { flex: 1 1 auto; }
	.filter-btn.on :global(button[aria-haspopup]) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.copy-list { display: grid; gap: 6px; max-height: 240px; overflow: auto; margin: 0 0 14px; padding: 0; list-style: none; font: var(--t-callout); }
	.copy-list li { display: flex; justify-content: space-between; gap: 12px; }
	.copy-list span { color: var(--text-muted); white-space: nowrap; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.search-error { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--fail); font: var(--t-callout); }
	.content { min-width: 0; }
	.with-rail { min-width: 0; }
	.with-rail.has-rail { display: grid; grid-template-columns: minmax(0, 1fr) 26px; gap: 12px; align-items: start; }

	.toolbar.topbar .t-view { order: 2; }
	.toolbar.topbar .filter-btn { order: 3; }
	.toolbar.topbar .tool-gap { order: 4; }
	.toolbar.topbar .t-tools { order: 5; }

	/* Rows: a ledger. 54 px a title, a small poster, the name and year, the status with its colour dot, three facts, the actions. */
	.lh, .lr { display: grid; grid-template-columns: 52px minmax(120px, 1fr) 168px var(--facts, 150px) var(--acts-w, 96px); align-items: center; gap: 0 12px; }
	.rows.has-select .lh, .rows.has-select .lr { grid-template-columns: 28px 52px minmax(120px, 1fr) 168px var(--facts, 150px) var(--acts-w, 96px); }
	.lsel { display: grid; place-items: center; padding-left: 10px; }
	.lr { position: relative; }
	.lh { position: relative; height: 40px; padding-right: 10px; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); }
	.lr { height: var(--row-h, 54px); padding-right: 10px; }
	.rows.v-rows .entry::before { left: 0; right: 0; }
	.lp { position: relative; display: block; width: 32px; margin-left: 10px; border-radius: 5px; overflow: hidden; box-shadow: 0 0 0 1px var(--line); }
	.lp.round { border-radius: 50%; }
	.lpick { display: block; margin: 0 auto; }
	.lt { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
	.lt a { color: var(--text); font: 600 14px/1.25 var(--font-ui); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lt a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.lt small { flex: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); }
	.ls { display: inline-flex; align-items: center; gap: 7px; min-width: 0; color: var(--text); font: var(--t-callout); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ls .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
	.ls.ok .dot { background: var(--ok); }
	.ls.info .dot { background: var(--info); }
	.ls.wait .dot { background: var(--wait); }
	.ls.fail .dot { background: var(--fail); }
	.ls.fail { color: var(--fail-text); }
	.ls.idle { color: var(--text-muted); }
	.lc { min-width: 0; color: var(--text-muted); font: var(--t-callout); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lr .acts { top: 50%; }
	/* Phone rows (chosen by the list's own width): the poster, the name over "status · fact", the year at the right. */
	.lr.ph { grid-template-columns: 50px minmax(0, 1fr) auto; height: 60px; padding-right: 12px; }
	.rows.has-select .lr.ph { grid-template-columns: 28px 50px minmax(0, 1fr) auto; }
	.lr.ph .lp { width: 36px; margin-left: 8px; }
	.lr.ph .lt { flex-direction: column; align-items: flex-start; gap: 3px; }
	.lr.ph .lt a { max-width: 100%; }
	.lsub { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
	.lyr { color: var(--text-muted); font: 500 12px/1 var(--font-ui); }


	/* Banner table */
	.rows { position: relative; isolation: isolate; min-width: 0; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b) 420px); box-shadow: var(--edge), var(--drop); overflow: clip; }
	/* A whisper of grain keeps the flat colour from looking printed (as on Home's tiles). */
	.rows::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; opacity: .05; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E"); }
	.row { display: grid; grid-template-columns: var(--cols); align-items: center; gap: 0 16px; min-height: var(--row-h, 100px); padding: 0 var(--row-pad, 20px) 0 8px; width: 100%; box-sizing: border-box; }
	/* The virtual list moves its own rows (a top spacer); the browser's scroll
	   anchoring must not also correct for them, or the two chase each other. */
	.body { overflow-anchor: none; }
	.row.head { position: relative; min-height: 40px; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); }
	/* Sentence case, like Downloads' column headings: one voice on the page. */
	.h { color: color-mix(in srgb, var(--text) 78%, transparent); font: 600 12px/1 var(--font-ui); text-align: left; white-space: nowrap; }
	.h.more { position: absolute; right: 6px; top: 50%; translate: 0 -50%; padding: 3px 5px; border-radius: 6px; background: color-mix(in srgb, var(--text) 8%, transparent); letter-spacing: 0; cursor: help; }
	.h.num, .c.num { text-align: right; justify-self: end; }
	.sort { display: inline-flex; align-items: center; gap: 4px; padding: 6px 0; border: 0; background: none; cursor: pointer; }
	.sort.num { justify-content: flex-end; }
	.sort[aria-pressed='true'] { color: var(--text); }
	.sort:hover { color: var(--text); }
	.sort:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.dir { color: var(--accent); }
	.entry { position: relative; outline: none; cursor: pointer; scroll-margin-top: 84px; transition: background var(--dur-fast) var(--ease-out); }
	.library.selecting .entry, .library.selecting .grid > li { scroll-margin-bottom: 120px; }
	/* Every row carries its top line, so a row's height never changes when the
	   virtual window drops the row above it (that would nudge the list 1px). */
	/* The line between rows is drawn, not a border, and starts after the picture. */
	.entry::before { content: ''; position: absolute; top: 0; left: 20px; right: 20px; height: 1px; background: var(--line); pointer-events: none; }
	.rows.table .entry::before { left: calc(var(--banner-w, 0px) + 24px); }
	.rows.overview .entry::before { left: 16px; }
	.entry.first::before { display: none; }
	.entry:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.entry:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
	.entry.selected { background: color-mix(in srgb, var(--accent) 9%, transparent); }
	/* Coming back from a title, the one you opened is marked by a thin line that fades away by itself. */
	.entry.last-opened { animation: last-row 2.6s var(--ease-out) both; }
	@media (prefers-reduced-motion: reduce) {
		.entry.last-opened { animation: none; box-shadow: inset 2px 0 0 color-mix(in srgb, var(--text-muted) 70%, transparent); }
		.grid > li.last-opened { animation: none; outline-color: color-mix(in srgb, var(--text-muted) 60%, transparent); }
		.meter i { transition: none; }
	}
	@keyframes last-row { 0%, 65% { box-shadow: inset 2px 0 0 color-mix(in srgb, var(--text-muted) 70%, transparent); } 100% { box-shadow: inset 2px 0 0 transparent; } }
	@keyframes last-ring { 0%, 65% { outline-color: color-mix(in srgb, var(--text-muted) 60%, transparent); } 100% { outline-color: transparent; } }
	.state { display: grid; place-items: center; }
	.keep { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; color: var(--text-muted); }
	.keep.on { color: var(--ok, #2fa36b); background: color-mix(in srgb, var(--ok, #2fa36b) 14%, transparent); }
	/* The banner is a picture set into the sheet: rounded, lit on its top edge, a short shadow under it. */
	.art { position: relative; display: block; min-width: 0; align-self: center; --banner-r: 12px; border-radius: 12px; box-shadow: 0 1px 2px light-dark(rgb(20 24 40 / 12%), rgb(0 0 0 / 50%)), 0 10px 18px -12px light-dark(rgb(20 24 40 / 50%), rgb(0 0 0 / 90%)); transition: box-shadow var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
	.art::after { content: ''; position: absolute; inset: 0; z-index: 3; border-radius: inherit; box-shadow: inset 0 1px 0 rgb(255 255 255 / 16%), inset 0 0 0 1px rgb(255 255 255 / 7%); pointer-events: none; }
	.art :global(img:not(.logo)) { transition: opacity var(--dur) var(--ease-out), filter var(--dur-fast) var(--ease-out), scale var(--dur-slow) var(--ease-out); }
	.entry:hover .art :global(img:not(.logo)) { filter: brightness(1.12) saturate(1.05); }
	@media (prefers-reduced-motion: no-preference) { .entry:hover .art :global(img:not(.logo)) { scale: 1.02; } }
	.name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.name a { color: var(--text); font: var(--t-title3, var(--t-headline)); font-weight: 600; text-decoration: none; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
	.name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.sub:empty { display: none; }
	.facts { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.c { min-width: 0; color: var(--text); font: var(--t-callout); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.c.path { color: var(--text-muted); font: var(--t-footnote); font-family: var(--font-mono, ui-monospace, monospace); direction: rtl; text-align: left; }
	.prog { display: grid; grid-template-columns: 1fr auto; align-items: center; gap: 8px; }
	.lstate { display: flex; flex-direction: column; gap: 5px; }
	.ls-line { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
	.ls-t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lstate.ok .ls-line :global(svg) { color: var(--ok); }
	.lstate.info .ls-line :global(svg) { color: var(--info); }
	.lstate.wait .ls-line :global(svg) { color: var(--wait); }
	.lstate.fail { color: var(--fail-text); }
	.lstate.idle { color: var(--text-muted); }
	.ls-bar { max-width: 140px; }
	.lstate.info .ls-bar i { background: var(--info); }
	.lstate.fail .ls-bar i { background: var(--fail); }
	.lstate.idle .ls-bar i { background: var(--text-faint); }
	.keep-c { display: flex; }
	.num-t { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.acts { position: absolute; right: 10px; top: calc(var(--row-h, 100px) / 2); translate: 0 -50%; z-index: 1; display: flex; gap: 2px; padding: 4px; border-radius: 999px; background: color-mix(in srgb, var(--sheet-a) 86%, transparent); box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 6%); backdrop-filter: blur(8px); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.acts:empty { display: none; }
	.entry:hover .acts, .entry:focus-within .acts { opacity: 1; }
	@media (hover: none) { .acts { opacity: 1; } }

	/* Progress meter, coloured by state */
	.meter { position: relative; display: block; height: 5px; border-radius: 99px; background: var(--surface-3); overflow: hidden; }
	.meter i { position: absolute; inset: 0 auto 0 0; border-radius: inherit; background: var(--text-muted); transition: width var(--dur-slow) var(--ease-out); }
	.meter.full i { background: var(--ok, #2fa36b); }
	.meter.gap i { background: var(--warn, #d9822b); }
	.meter.gap { background: color-mix(in srgb, var(--fail, #d64545) 28%, var(--surface-3)); }
	.meter.later i { background: var(--info, #3b82f6); }
	.meter.none i { background: var(--text-muted); }

	.series-head { display: flex; align-items: baseline; gap: 12px; padding: 18px 16px 10px; background: var(--surface-2); border-top: 1px solid var(--line); }
	.series-head h2 { margin: 0; font: var(--t-headline); }
	.series-head span { color: var(--text-muted); font: var(--t-footnote); }
	.series-head a { margin-left: auto; color: var(--accent-text); font: var(--t-footnote); font-weight: 600; }

	/* Overview list */
	.ov { display: grid; grid-template-columns: 92px minmax(0, 1fr) minmax(200px, 264px); gap: 18px 24px; padding: 14px 20px 14px 16px; }
	.ov-poster { display: block; border-radius: 8px; overflow: hidden; }
	.ov-main { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.ov h2 { margin: 0; font: var(--t-title3); display: flex; align-items: baseline; gap: 10px; }
	.ov h2 a { color: var(--text); text-decoration: none; }
	.yr { color: var(--text-muted); font: var(--t-callout); }
	.ov-text { margin: 0; color: var(--text-muted); font: var(--t-callout); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	/* A description reads best at a book's line length; the rest of the row is the facts'. */
	.ov-text { max-width: 68ch; }
	.ov-prog { display: flex; align-items: center; gap: 10px; margin-top: auto; }
	.ov .meter { flex: 0 1 280px; }
	.ov-count { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.ov-keep { flex: none; align-self: center; width: 24px; height: 24px; }
	.ov-facts { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 6px 12px; margin: 0; align-content: start; font: var(--t-footnote); }
	.ov-facts dt { color: var(--text-muted); }
	.ov-facts dd { margin: 0; text-align: right; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ov-facts dd.ov-state { grid-column: 1 / -1; justify-self: end; max-width: 100%; margin-bottom: 4px; overflow: visible; }
	.ov-pick { position: absolute; right: 14px; top: 14px; }
	/* Music: a square photo that keeps its shape, and two or three lines of bio. */
	.ov.music .ov-poster { align-self: start; }

	/* Loading rows match the banner shape */

	/* Posters */
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--poster-min, 172px), 1fr)); gap: 26px 18px; margin: 0; padding: 0; list-style: none; }
	/* Posters: no paint containment here, so the lift, tilt and shadow are never clipped. */
	.grid > li { position: relative; min-width: 0; display: flex; flex-direction: column; gap: 4px; scroll-margin-top: 120px; }
	.grid > li:hover, .grid > li:focus-within { z-index: 2; }
	.grid > li.last-opened { outline: 1px solid transparent; outline-offset: 4px; border-radius: var(--r-card); animation: last-ring 2.6s var(--ease-out) both; }
	.grid > li.selected { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: var(--r-card); }
	.pick { position: absolute; inset: 0 0 auto 0; aspect-ratio: var(--poster-ratio, 2 / 3); z-index: 3; display: block; padding: 0; border: 0; border-radius: var(--r-card); background: transparent; cursor: pointer; }
	.pick[aria-pressed='true'] { background: rgb(0 0 0 / 30%); box-shadow: inset 0 0 0 3px var(--accent); }
	.pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pick-dot { position: absolute; left: 8px; top: 8px; width: 24px; height: 24px; display: grid; place-items: center; border-radius: 50%; border: 2px solid #fff; background: rgb(0 0 0 / 35%); color: #fff; }
	.pick[aria-pressed='true'] .pick-dot { background: var(--accent-fill); border-color: var(--accent-fill); color: var(--accent-ink, #fff); animation: pick-pop var(--dur) var(--ease-spring); }
	.grid .pick { animation: pick-in var(--dur-fast) var(--ease-out); }
	@keyframes pick-pop { from { transform: scale(0.6); } }
	@keyframes pick-in { from { opacity: 0; } }
	@media (prefers-reduced-motion: reduce) { .pick[aria-pressed='true'] .pick-dot, .grid .pick { animation: none; } }
	.pick-box { width: 18px; height: 18px; accent-color: var(--accent); }
	.requester { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.row-notes { display: flex; flex-wrap: wrap; gap: 4px 12px; margin: -6px 0 0; padding: 0 12px 10px 54px; }
	.rows.table .row-notes, .rows.table .fold { padding-left: calc(var(--banner-w, 0px) + 68px); }

	/* Bulk bar */
	.select-bar { position: sticky; bottom: 16px; z-index: 20; container-type: inline-size; display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 6px; margin-top: 8px; padding: 10px 12px; border-radius: var(--r-panel); background: color-mix(in srgb, var(--sheet-a) 90%, transparent); -webkit-backdrop-filter: blur(16px); backdrop-filter: blur(16px); box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 6%), 0 8px 28px rgb(0 0 0 / 25%); }
	/* Narrow: the bar's buttons fold to their icons (Remove keeps its word), the toolbar's own trick. */
	@container (max-width: 1000px) {
		.select-bar :global(.ui-btn.secondary > span), .select-bar :global(.trigger.button > span), .select-bar :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.select-bar :global(.ui-btn.secondary), .select-bar :global(.trigger.button) { width: 36px; padding-inline: 0; justify-content: center; }
	}
	/* Phones: the bar stays above the bottom tab bar and the safe area. */
	@media (max-width: 767px) { .select-bar { bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom, 0px) + 8px); } }
	@media (max-width: 639px) { .select-bar { flex-wrap: nowrap; } .select-bar .bar-gap { flex: 1 1 0; min-width: 0; } }
	.select-bar > :global(.ui-ibtn:last-child) { order: -1; }
	/* A little tighter, so the whole bar stays on one line beside an open side menu. */
	.select-bar :global(.ui-btn.md), .select-bar :global(.trigger.button) { padding-inline: 11px; }
	.select-bar :global(.ui-btn.plain) { padding-inline: 6px; }
	.select-bar .count { padding: 0 8px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.bar-gap { flex: 1 1 auto; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--s-6); }
	.modal .input { width: 100%; margin-top: 6px; }

	/* A to Z */
	.rail { position: sticky; top: 84px; display: flex; flex-direction: column; align-items: stretch; padding: 4px 0; border-radius: 999px; background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rail-l { display: grid; place-items: center; min-height: 24px; /* WCAG 2.5.8: each letter is at least a 24px target. */ padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text); font: 600 11px/1 var(--font-ui); cursor: pointer; }
	.rail-l:hover:not(:disabled) { background: var(--surface-2); color: var(--accent); }
	.rail-l:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rail-l:disabled { color: var(--text-muted); opacity: .45; cursor: default; }

	/* Missing fold */
	.fold { padding: 0 12px 10px 54px; }
	.fold-body { margin-top: 6px; padding: 8px 12px; border-radius: var(--r-btn); background: var(--surface-2); }
	:global(.fold-btn svg:first-child) { transition: rotate var(--dur-fast) var(--ease-out); }
	:global(.fold-btn[aria-expanded="true"] svg:first-child) { rotate: 90deg; }
	.fold-body ul { display: flex; flex-direction: column; gap: 6px; margin: 0; padding: 0; list-style: none; font: var(--t-callout); }
	.fold-body li { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 6px; min-width: 0; }
	.ep-code { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.ep-name { overflow-wrap: anywhere; }
	.ep-date { color: var(--text-muted); }
	.fold-note { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.empty-more { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; }

	
	/* Phones: each row becomes a banner card; posters stay a grid. */
	@media (max-width: 639px) {
		.toolbar { position: static; margin: 0; padding: 8px 0; }
		.t-sel, .t-add, .t-view, .filter-btn { display: inline-flex; }
			/* Two rows: count, search, select, add; then view and size, sort and filter. */
		.toolbar .count { order: 0; flex: 1 1 auto; margin: 0; overflow: hidden; text-overflow: ellipsis; }
		.t-find { order: 1; }
		.t-add { order: 3; }
		.tool-gap { display: block; order: 4; flex-basis: 100%; height: 0; }
		.t-view { order: 5; }
		.filter-btn { order: 6; }
		.t-sel { order: 2; }
		.chips .t-miss { flex-basis: 100%; }
		.chips .t-miss :global(button) { width: 100%; }
		.find-box { flex-basis: 100%; max-width: none; order: 4; }
		.sf { grid-template-columns: 1fr; padding: 0; }
		.sf-sort { padding: 0; }
		.sf-filter { margin-top: 10px; padding: 4px 0 0; border-left: 0; border-top: 1px solid var(--line); }
		.sf-well { flex: none; max-height: 232px; }
		.sf-size { margin-top: 10px; padding-top: 4px; border-top: 1px solid var(--line); }
		.sf-foot { flex-wrap: wrap; }
		.with-rail.has-rail { grid-template-columns: minmax(0, 1fr) 20px; gap: 6px; }
		.rail { top: 64px; padding: 2px 0; }
		.rail-l { font-size: 10px; min-height: 24px; }
		.rows.table { overflow: clip; }
		.rows.table > .body { min-width: 0; }
		.row { grid-template-columns: 1fr auto; grid-template-areas: 'art art' 'name acts'; gap: 10px 8px; min-width: 0; padding: 0 12px 12px; }
		.row > .art { margin: 0 -12px; --banner-r: 0; border-radius: 0; box-shadow: none; }
		.row > .art::after { display: none; }
		.rows.table .entry::before { left: 0; right: 0; }
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
		.ov-facts dd.ov-state { justify-self: start; }
		.ov-bio { -webkit-line-clamp: 2; line-clamp: 2; }
		.grid { grid-template-columns: repeat(auto-fill, minmax(var(--poster-min-phone, calc(var(--poster-min, 172px) * 0.66)), 1fr)); gap: 18px 12px; }
		.fold, .row-notes, .rows.table .fold, .rows.table .row-notes { padding-left: 12px; }
	}
</style>
