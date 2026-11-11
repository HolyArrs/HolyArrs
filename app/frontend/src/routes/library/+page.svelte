<script lang="ts">
	import { LibraryDialogs, LibraryList, LibrarySavedFilters, LibrarySelectBar, LibrarySortFilter, LibraryToolbar, grow, linkedPeople, noFilter, partOff, shrink, type View } from '#lib/mediafiles/index.ts';
		import Icon from '#lib/ui/Icon.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { audienceOf } from '#lib/ui/title-status.ts';
	import { densityLabels, loadDensity, loadPosterSize, posterLabels, saveDensity, savePosterSize, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import { libraryTitles, filters, kinds, matchesTitleQuery, ofKind, titleQuery, type Filter, type Kind, activeCount, filterCounts, loadColumns, loadSaved, passes, saveColumns, saveSaved, sortTitles, sortsFor, startDir, chosenColumns, columnsFor, defaultSort, type Column, type FilterContext, type FilterState, type Kept, type LibKind, type SavedFilter, type SortDir, type SortKey, detailLabels, flipFrom, loadPosterDetail, savePosterDetail, type PosterDetail, rememberLibraryKind, partsNow, codecLabel, copyCodecs, type CopySearchPreview, type CopySearchResult, deleteBody } from '#lib/mediafiles/index.ts';
	import { episodeLabel, showProgress } from '#lib/shows/index.ts';
	import { artistProgress, artistHref } from '#lib/music/index.ts';
	import { DlFind, searchShortcut, dlTop } from '#lib/downloads/index.ts';
	import type { MovieCollection, CollectionDetail } from '#lib/mediafiles/index.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { date } from '#lib/core/format.ts';
	import { reducedMotion } from '#lib/core/motion.ts';
	import { DUR, EASE } from '#lib/core/transitions.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { TitleView } from '#lib/core/types.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';
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
	const kindTitle = $derived(kind === 'movie' ? tr('mediafiles.kinds.movie') : kind === 'show' ? tr('mediafiles.kinds.show') : kind === 'artist' ? tr('mediafiles.kinds.artist') : tr('library.page.library'));

	function readView(k: Kind, fromUrl = false): View {
		const asked = fromUrl ? page.url.searchParams.get('layout') : null;
		if (asked === 'rows') return 'rows';
		if (asked === 'list' || asked === 'table' || asked === 'banners') return 'table';
		if (asked === 'grid' || asked === 'posters') return 'posters';
		if (asked === 'overview') return 'overview';
		// Rows is the view everyone starts on; a view picked before that default was set is not carried over.
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

	let listWidth = $state(1000);
	let columns = $state<Column[]>(columnsFor('movie'));
	$effect(() => {
		const k = libKind;
		const me = myId;
		columns = loadColumns(me, k);
	});
	const chosenCols = $derived(chosenColumns(columns, sort.key));
	// Columns that do not fit the width wait until the window is wider (the banner and the title always stay); the header says how many wait. The art is the hero of each row, so the banner column takes its share of the width.
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
	const phone = $derived(listWidth < 640);
	let listEl = $state<HTMLElement | null>(null);
	const sizeItems = $derived<MenuItem[]>(
		view === 'posters'
			? [
					{ kind: 'heading', label: tr('library.page.posterSize') }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: posterLabels[z], checked: posterSize === z, onselect: () => setPosterSize(z) })),
					{ kind: 'separator' }, { kind: 'heading', label: tr('library.page.posterDetails') }, ...(['title', 'status', 'full'] as const).map((d) => ({ label: detailLabels[d], checked: posterDetail === d, onselect: () => setPosterDetail(d) }))
				] as MenuItem[]
			: [{ kind: 'heading', label: tr('library.page.bannerSize') }, ...(['large', 'comfortable', 'compact'] as const).map((d) => ({ label: densityLabels[d], checked: density === d, onselect: () => setDensity(d) }))]
	);
	function setColumns(c: Column[]) {
		columns = c;
		saveColumns(myId, libKind, c);
	}

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
	const seriesName = (id: number) => series.find((s) => s.id === id)?.name ?? tr('library.page.collection');

	// Titles with subtitles still wanted (movies and TV shows), from the same list the subtitles search uses.
	let subsTitles = $state<Set<number> | null>(null);
	const ctx = $derived<FilterContext>({ askers: (t) => linkedPeople(t).map((p) => p.id), me: myId, seriesOf: (t) => seriesOfTitle.get(t.id), subsMissing: (t) => !!subsTitles?.has(t.id) });
	const ofThisKind = $derived(titles.filter((t) => ofKind(t, kind) && matchesTitleQuery(t, query, indexedIds)));
	const counts = $derived(filterCounts(ofThisKind, filter, ctx));
	const shown = $derived(sortTitles(ofThisKind.filter((t) => passes(t, filter, ctx)), sort.key, sort.dir));
	const askers = $derived([...new Map(ofThisKind.flatMap((t) => linkedPeople(t)).map((p) => [p.id, p])).values()].sort((a, b) => a.name.localeCompare(b.name)));
	const filtersOn = $derived(activeCount(filter));

	const kindTotal = $derived(titles.filter((t) => ofKind(t, kind)).length);
	const countText = $derived.by(() => {
		const k = ['movie', 'show', 'artist'].includes(kind) ? kind : 'all';
		if (filtersOn || q) return tr(`library.count.${k}.${kindTotal === 1 ? 'ofOne' : 'ofOther'}`, { n: shown.length, total: kindTotal });
		return tr(`library.count.${k}.${shown.length === 1 ? 'one' : 'other'}`, { n: shown.length });
	});
	// The Library has no top bar: its own row carries the search (as on Downloads). Ctrl K and "/" focus it.
	const findWords = $derived(kind === 'movie' ? tr('library.page.findMovies') : kind === 'show' ? tr('library.page.findShows') : kind === 'artist' ? tr('library.page.findMusic') : tr('library.page.findLibrary'));
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
			.map(([k]): MenuItem => ({ label: tr(`mediafiles.kinds.${k}`), checked: kind === k, onselect: () => void goto(to(`/library?kind=${k}`)) }))
	);
	let findOpen = $state(false);
	let sortOpen = $state(false);
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
		toasts.push(tr('mediafiles.title.saved', { name }), 'ok');
	}
	function forget(name: string) {
		saved = saved.filter((s) => s.name !== name);
		saveSaved(myId, kind, saved);
	}
	const sameFilter = (a: FilterState, b: FilterState) => a.status === b.status && a.kept === b.kept && a.asker === b.asker && a.series === b.series && (a.codec ?? '') === (b.codec ?? '') && !!a.subs === !!b.subs;

	// Counts stay blank until the Library has loaded, so a 0 only ever means 0.
	const countHint = (n: number) => (loaded ? String(n) : undefined);
	// Add only adds. Everything that looks after the Library is in Tools (people are found by the search, movie series by Group by series, and held files are on Downloads).
	const addItems = $derived<MenuItem[]>([
		...(!partOff('movies') ? ([{ label: tr('library.page.addMovie'), icon: 'film', href: to('/search?kind=movie') }] as MenuItem[]) : []),
		...(!partOff('shows') ? ([{ label: tr('library.page.addShow'), icon: 'tv', href: to('/search?kind=show') }] as MenuItem[]) : []),
		...(!partOff('music') ? ([{ label: tr('library.page.addArtist'), icon: 'music', href: to('/search?kind=music') }] as MenuItem[]) : []),
		...(canManage ? ([{ label: tr('library.page.importFromFolder'), icon: 'folder', href: to('/library/import') }] as MenuItem[]) : [])
	]);
	// Recently deleted lives in Sort & filter: its count is read when the panel opens (at most once a minute).
	let deletedCount = $state<number | null>(null);
	let deletedAt = 0;
	$effect(() => {
		if (!sortOpen || !canManage || Date.now() - deletedAt < 60_000) return;
		deletedAt = Date.now();
		void api.get<{ files: unknown[] }>('/holding').then((r) => (deletedCount = r.files?.length ?? 0), () => {});
	});
	const grouped = $derived(groupSets && kind === 'movie');
	let selecting = $state(false);
	let picked = $state<Set<number>>(new Set());
	let busy = $state(false);
	let deleteIds = $state<number[]>([]);
	let deleteDialog = $state<HTMLDialogElement | null>(null);
	let rules = $state<{ id: number; name: string; kinds: string[] }[]>([]);
	let folders = $state<{ id: number; path: string; kind: string; label: string }[]>([]);
	const pickedShown = $derived(shown.filter((t) => picked.has(t.id)));
	const allPicked = $derived(shown.length > 0 && pickedShown.length === shown.length);
	const deleteWord = $derived(deleteIds.length === 1 ? (titles.find((t) => t.id === deleteIds[0])?.name ?? tr('library.page.oneTitle')) : tr('library.page.nTitles', { n: deleteIds.length }));
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
	function report(action: string, failed: number, total: number, note?: string, vars: Record<string, string> = {}) {
		const base = `library.report.${action}`;
		if (failed) toasts.push(tr(`${base}.${total === 1 ? 'partOne' : 'partMany'}`, { ok: total - failed, total, ...vars }), 'info', { body: tr('library.report.failedBody', { n: failed }) });
		else toasts.push(tr(`${base}.${total === 1 ? 'one' : 'many'}`, { n: total, ...vars }), 'ok', note ? { body: note } : undefined);
	}
	async function setKeep(ids: number[], keep: boolean) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.put(`/titles/${id}`, { keep_up_to_date: keep }));
		busy = false;
		report(keep ? 'keep' : 'unkeep', failed, ids.length);
		await load();
	}
	async function setRules(ids: number[], rule: { id: number; name: string }) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.put(`/titles/${id}`, { pick_rules_id: rule.id }));
		busy = false;
		report('setTo', failed, ids.length, undefined, { rule: rule.name });
		await load();
	}
	async function searchTitles(ids: number[]) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.post(`/titles/${id}/search`, {}));
		busy = false;
		report('searching', failed, ids.length);
	}
	// Find another copy (movies): the selected movies' main files, searched again at their Quality after a review. Protected files are left out.
	const copyPicks = $derived(pickedShown.flatMap((t) => (t.copy && !t.copy.protected ? [{ file_id: t.copy.file_id, version_id: t.copy.version_id }] : [])));
	let copyDialog = $state<HTMLDialogElement | null>(null);
	let copyPreview = $state<CopySearchPreview | null>(null);
	async function reviewCopies() {
		busy = true;
		try {
			copyPreview = await api.post<CopySearchPreview>('/library/copies/search/preview', { selections: copyPicks });
			copyDialog?.showModal();
		} catch (e) {
			toasts.push(tr('library.page.checkFailed'), 'fail', { body: plainReason(message(e)).text });
		} finally {
			busy = false;
		}
	}
	async function findCopies() {
		if (!copyPreview) return;
		busy = true;
		try {
			const r = await api.post<CopySearchResult>('/library/copies/search', { selections: copyPicks, review_token: copyPreview.review_token });
			toasts.push(tr(r.job_ids.length === 1 ? 'library.page.searchingCopyOne' : 'library.page.searchingCopyMany', { n: r.job_ids.length }), 'info');
			copyDialog?.close();
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: plainReason(message(e)).text });
		} finally {
			busy = false;
		}
	}
	// Subtitles: which titles still want some (read at most once a minute); the search covers the shown kind.
	let subsAt = 0;
	let subsSearching = $state(false);
	$effect(() => {
		if (!canManage || !(kind === 'movie' || kind === 'show') || !!partOff('subtitles')) return;
		if (Date.now() - subsAt < 60_000) return;
		subsAt = Date.now();
		void api.get<{ titles: { id: number }[] }>('/subtitles/missing?limit=1').then((r) => (subsTitles = new Set((r.titles ?? []).map((x) => x.id))), () => {});
	});
	async function searchMissingSubs() {
		subsSearching = true;
		try {
			const r = await api.post<{ queued: number }>('/subtitles/missing/search', { kind: kind === 'movie' ? 'movie' : 'episode' });
			toasts.push(tr('shows.sheet.searchingSubs'), 'info', { body: tr('mediafiles.unit.file.' + (r.queued === 1 ? 'one' : 'other'), { n: r.queued }) });
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: plainReason(message(e)).text });
		} finally {
			subsSearching = false;
		}
	}
	async function refreshTitle(t: TitleView) {
		try {
			await api.post(`/titles/${t.id}/refresh`, {});
			toasts.push(tr('library.page.refreshing', { name: t.name }), 'info');
		} catch (e) {
			toasts.push(tr('library.page.refreshFailed'), 'fail', { body: plainReason(message(e)).text });
		}
	}
	// Missing list: one search for every missing title of this kind, after a confirm.
	let missingDialog = $state<HTMLDialogElement | null>(null);
	let searchingAll = $state(false);
	const missingCount = $derived(titles.filter((t) => ofKind(t, kind) && passes(t, { ...noFilter, status: 'missing' }, ctx)).length);
	const missingWhat = $derived(tr(`library.page.searchAllMissing.${kind}`));
	async function searchAllMissing() {
		searchingAll = true;
		try {
			await api.post('/search/missing', kind === 'all' ? undefined : { kind });
			toasts.push(tr(missingCount === 1 ? 'library.page.searchingMissingOne' : 'library.page.searchingMissingMany', { n: missingCount }), 'info');
			missingDialog?.close();
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: plainReason(message(e)).text });
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
		report('removed', failed, ids.length, tr('mediafiles.delete.stay'));
		stopSelecting();
		await load();
	}
	// Move the selected titles' files to another folder of their kind; mixed kinds get no choice.
	async function moveTitles(ids: number[], folder: { id: number; label: string; path: string }) {
		busy = true;
		const failed = await eachTitle(ids, (id) => api.post(`/titles/${id}/move`, { library_folder_id: folder.id, apply: true }));
		busy = false;
		report('moving', failed, ids.length, folder.label || folder.path);
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
		{ label: allPicked ? tr('mediafiles.select.none') : tr('mediafiles.select.all', { n: shown.length }), icon: 'check', onselect: pickAll },
		{ kind: 'separator' },
		{ label: tr('mediafiles.select.keep'), icon: 'bellOn', disabled: busy || !pickedShown.length, onselect: () => setKeep(pickedShown.map((t) => t.id), true) },
		{ label: tr('mediafiles.select.stopKeeping'), icon: 'bell', disabled: busy || !pickedShown.length, onselect: () => setKeep(pickedShown.map((t) => t.id), false) },
		{ label: tr('mediafiles.select.searchNow'), icon: 'search', disabled: busy || !pickedShown.length, onselect: () => searchTitles(pickedShown.map((t) => t.id)) },
		...(kind === 'movie' ? ([{ label: tr('mediafiles.select.findCopy'), icon: 'copy', disabled: busy || !copyPicks.length, onselect: reviewCopies }] as MenuItem[]) : []),
		...(rulesItems.length ? ([{ kind: 'separator' }, { kind: 'heading', label: tr('mediafiles.select.quality') }, ...rulesItems] as MenuItem[]) : []),
		...(moveItems.length > 1 ? ([{ kind: 'separator' }, { kind: 'heading', label: tr('library.page.moveTo') }, ...moveItems] as MenuItem[]) : [])
	]);
	function selectKey(e: KeyboardEvent) {
		if (selecting && e.key === 'Escape' && !deleteDialog?.open) stopSelecting();
	}

	function progressText(t: TitleView): string {
		if (t.kind === 'show') return showProgress(t.show);
		if (t.kind === 'artist') return artistProgress(t.artist);
		return '';
	}
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
		const offs = [live.on('title', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>{kindTitle} · HolyArrs</title></svelte:head>
<svelte:window onkeydown={selectKey} />

<div class="library" class:selecting>
	<h1 class="sr-only">{kindTitle}</h1>
	<LibraryToolbar {phone} {kind} {kindTitle} {loaded} {error} {countText} {shelfItems} sidebarCollapsed={dlTop.collapsed} {sortFilter} {view} {setView} {sizeItems} {posterSize} {density} {listWidth} {filtersOn} bind:sortOpen bind:findOpen {findWords} bind:q {syncUrl} {columns} {libKind} {setColumns} {canManage} shownCount={shown.length} {selecting} {startSelecting} {stopSelecting} {addItems}>
		{#snippet sidebarButton()}<IconButton icon="sidebar" label={tr('library.page.showSidebar')} onclick={() => dlTop.toggle()} data-testid="library-side-btn" />{/snippet}
		{#snippet find()}<DlFind bind:term={() => q, setQ} shown={shown.length} total={kindTotal} searchKey={dlTop.searchKey} placeholder={findWords} note={loaded && !error ? countText : ''} testid="library-find" />{/snippet}
	</LibraryToolbar>
	{#if phone}
		<Sheet bind:open={sortOpen} title={tr('library.page.sortAndFilter')} placement="bottom">{@render sortFilter()}{#snippet footer()}{@render savedFilters(true)}{/snippet}</Sheet>
	{/if}

	{#if filtersOn || q || grouped}
		<div class="chips" role="group" aria-label={tr('library.page.activeFilters')} in:grow out:shrink>
			{#if filter.status !== 'all'}<Chip label={filters.find(([f]) => f === filter.status)?.[1] ?? ''} selected onclick={() => setFilter({ status: 'all' })} onremove={() => setFilter({ status: 'all' })} />{/if}
			{#if filter.kept !== 'any'}<Chip label={filter.kept === 'on' ? tr('mediafiles.sortFilter.kept') : tr('mediafiles.state.notKept')} selected onclick={() => setFilter({ kept: 'any' })} onremove={() => setFilter({ kept: 'any' })} />{/if}
			{#if filter.asker !== 'anyone'}<Chip label={filter.asker === 'me' ? tr('library.page.askedByMe') : tr('library.page.askedBy', { name: askers.find((p) => String(p.id) === filter.asker)?.name ?? tr('library.page.someone') })} selected onclick={() => setFilter({ asker: 'anyone' })} onremove={() => setFilter({ asker: 'anyone' })} />{/if}
			{#if filter.series}<Chip label={tr('library.page.collectionChip', { name: seriesName(filter.series) })} selected onclick={() => setFilter({ series: 0 })} onremove={() => setFilter({ series: 0 })} />{/if}
			{#if filter.codec}<Chip label={codecLabel(filter.codec)} selected onclick={() => setFilter({ codec: '' })} onremove={() => setFilter({ codec: '' })} />{/if}
			{#if filter.subs}<Chip label={tr('library.page.missingSubs')} selected onclick={() => setFilter({ subs: false })} onremove={() => setFilter({ subs: false })} />{/if}
			{#if q}<Chip label={tr('library.page.searchChip', { q })} selected onclick={clearSearch} onremove={clearSearch} />{/if}
			{#if grouped}<Chip label={tr('library.page.grouped')} selected onclick={() => { groupSets = false; syncUrl(); }} onremove={() => { groupSets = false; syncUrl(); }} />{/if}
			<Button variant="plain" size="md" onclick={clearFilters}>{tr('library.page.clear')}</Button>
			{#if canManage && filter.subs && shown.length}<span class="t-miss"><Button size="md" icon="search" busy={subsSearching} onclick={searchMissingSubs} data-testid="search-missing-subtitles">{tr('library.page.searchSubs')}</Button></span>{/if}
			{#if canManage && filter.status === 'missing' && missingCount}<span class="t-miss"><Button size="md" icon="search" disabled={searchingAll} onclick={() => missingDialog?.showModal()} data-testid="search-all-missing">{missingWhat}</Button></span>{/if}
		</div>
	{/if}
	{#if searchError && !error}<p class="search-error" role="alert"><Icon name="alert" size={16} />{plainReason(searchError).text}</p>{/if}

	<LibraryList {titles} {shown} {loaded} {error} {view} {kind} {libKind} {filter} {sort} {grouped} {kindTitle} {q} {filtersOn} {who} {canManage} {selecting} {picked} {lastOpenedId} {chosenCols} {density} {posterSize} {posterDetail} {seriesOfTitle} {seriesName} {phone} bind:listWidth bind:gridEl bind:listEl {progressText} {artistHref} {episodeLabel} {retryLoad} {clearFilters} {togglePick} {startSelecting} {setSort} {searchTitles} {setKeep} {refreshTitle} />

	<LibrarySelectBar {selecting} {phone} {kind} {busy} pickedCount={pickedShown.length} pickedIds={pickedShown.map((t) => t.id)} shownCount={shown.length} {allPicked} copyPicksCount={copyPicks.length} {phoneActs} {moveItems} {rulesItems} {pickAll} {setKeep} {searchTitles} {reviewCopies} {askDelete} {stopSelecting} />
</div>

<LibraryDialogs bind:deleteDialog bind:copyDialog bind:missingDialog bind:saveDialog bind:copyPreview bind:saveName {deleteIds} {deleteWord} {busy} {missingCount} {searchingAll} {confirmDelete} {findCopies} {searchAllMissing} {saveCurrent} />

{#snippet sortFilter()}
	<LibrarySortFilter {kind} {sort} {filter} {counts} {askers} {series} {subsTitles} {loaded} {myId} {canManage} anyPart={!partOff('movies') || !partOff('shows') || !partOff('music')} {deletedCount} bind:groupSets {phone} {view} {sizeItems} foot={panelFoot} {setSort} {setFilter} {syncUrl} onclose={() => (sortOpen = false)} />
{/snippet}

{#snippet panelFoot()}{@render savedFilters(false)}{/snippet}

{#snippet savedFilters(sheet: boolean)}
	<LibrarySavedFilters {sheet} {saved} {filter} {filtersOn} {setFilter} {forget} {clearFilters} openSave={() => { sortOpen = false; saveName = ''; saveDialog?.showModal(); }} />
{/snippet}

<style>
	.library { display: flex; flex-direction: column; gap: 14px; padding-bottom: 48px; min-width: 0;
		/* The sheet Home's tiles and the Requests cards are made of: lit from above, a hairline, one long soft shadow. */
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.library.selecting { padding-bottom: 112px; }
	.chips .t-miss { display: inline-flex; margin-left: auto; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.search-error { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--fail); font: var(--t-callout); }
	@media (max-width: 639px) {
		.chips .t-miss { flex-basis: 100%; }
		.chips .t-miss :global(button) { width: 100%; }
	}
</style>
