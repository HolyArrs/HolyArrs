<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { flip } from 'svelte/animate';
	import { SvelteMap } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import { bannerHeight, bannerRatio, bannerWidth, posterMinFor, posterRatio, posterSrc, squarePhoneMin, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import { audienceStatus, type audienceOf } from '#lib/ui/title-status.ts';
	import { api } from '../core/api.ts';
	import { date } from '../core/format.ts';
	import { reducedMotion } from '../core/motion.ts';
	import { plainReason } from '../core/plain.ts';
	import { to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import { moveMs, easeOut } from '../core/transitions.ts';
	import type { EpisodeView, TitleView } from '../core/types.ts';
	import BannerArt from './BannerArt.svelte';
	import LibraryLetterRail from './LibraryLetterRail.svelte';
	import LibraryPoster from './LibraryPoster.svelte';
	import { emptyAdds, exceptionChip, firstOfLetter, letterOf, letters, missingEpisodes, posterStatus, type AddKind, type Kind } from './library.ts';
	import { libraryState } from './library-state.ts';
	import { partOff } from './parts.svelte.ts';
	import { buildEntries, cell, grow, isProgressCol, posterExtra, posterFacts, requestedBy, rowCell, scoreText, shrink, tone, type Entry, type View } from './library-view.ts';
	import { needsAttention, barOf, type PosterDetail } from './poster.ts';
	import { columnSort, progressOf, sortName, sortsFor, tableMinWidth, type Column, type FilterState, type LibKind, type SortDir, type SortKey } from './table.ts';
	import { artName, slotKind } from './title-page.ts';
	import { colPx } from './library-view.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		titles: TitleView[];
		shown: TitleView[];
		loaded: boolean;
		error: string;
		view: View;
		kind: Kind;
		libKind: LibKind;
		filter: FilterState;
		sort: { key: SortKey; dir: SortDir };
		grouped: boolean;
		kindTitle: string;
		q: string;
		filtersOn: number;
		who: ReturnType<typeof audienceOf>;
		canManage: boolean;
		selecting: boolean;
		picked: Set<number>;
		lastOpenedId: number | null;
		chosenCols: Column[];
		density: Density;
		posterSize: PosterSize;
		posterDetail: PosterDetail;
		seriesOfTitle: Map<number, number>;
		seriesName: (id: number) => string;
		phone: boolean;
		listWidth: number;
		gridEl?: HTMLElement;
		listEl: HTMLElement | null;
		progressText: (t: TitleView) => string;
		artistHref: (musicbrainz: string) => string;
		episodeLabel: (ep: EpisodeView, order: string) => string;
		retryLoad: () => void;
		clearFilters: () => void;
		togglePick: (id: number) => void;
		startSelecting: (id?: number) => void;
		setSort: (key: SortKey, dir?: SortDir) => void;
		searchTitles: (ids: number[]) => void;
		setKeep: (ids: number[], keep: boolean) => void;
		refreshTitle: (t: TitleView) => void;
	};
	// The empty Library's add buttons, named for the kind each one adds.
	const addWords = $derived<Record<AddKind, string>>({ movie: tr('mediafiles.list.addMovie'), show: tr('mediafiles.list.addShow'), artist: tr('mediafiles.list.addArtist') });
	let { titles, shown, loaded, error, view, kind, libKind, filter, sort, grouped, kindTitle, q, filtersOn, who, canManage, selecting, picked, lastOpenedId, chosenCols, density, posterSize, posterDetail, seriesOfTitle, seriesName, phone, listWidth = $bindable(), gridEl = $bindable(), listEl = $bindable(), progressText, artistHref, episodeLabel, retryLoad, clearFilters, togglePick, startSelecting, setSort, searchTitles, setKeep, refreshTitle }: Props = $props();

	const kindLabel = (k: string) => tr(`mediafiles.kindCap.${k}`);

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
	// The table never needs a sideways scroll: the banner gives way first (down to its floor), then the last columns step out until the rest fits.
	const shownCols = $derived.by(() => {
		if (phone || kind === 'all') return chosenCols;
		let out = chosenCols;
		while (out.length > 0 && colsNeed(out) + Math.min(wantW, bannerFloor[density]) > listWidth) out = out.slice(0, -1);
		return out;
	});
	const hiddenCols = $derived(chosenCols.length - shownCols.length);
	// As many facts as the list is wide enough for: three on a wide screen, down to one. The Rows grid: poster 52, title >= 120, status 168, each fact 150, actions (wider on touch screens), 12 px gaps.
	const actsW = $derived(96 + (touchActs ? 48 : 0));
	const factCols = $derived(chosenCols.filter((c) => c.id !== 'status' && c.id !== 'kept'));
	const rowCols = $derived.by(() => {
		let room = listWidth - 10 - 52 - 120 - 168 - actsW - 12 * 3 - (selecting ? 28 + 12 : 0);
		const out: Column[] = [];
		// All mixes kinds, so its rows show only the score, and only while sorted by it.
		for (const c of kind === 'all' ? factCols.filter((c) => c.id === 'score' && sort.key === 'score') : factCols) {
			if (out.length === 3 || (out.length && room < colPx(c) + 12)) break;
			out.push(c);
			room -= colPx(c) + 12;
		}
		return out;
	});
	const factsTpl = $derived(rowCols.map((c) => `${colPx(c)}px`).join(' ') || '0px');
	const bannerW = $derived(listWidth < 640 ? wantW : Math.round(Math.max(Math.min(wantW, bannerFloor[density]), Math.min(wantW, listWidth - colsNeed(kind === 'all' ? [] : shownCols)))));
	/** A row's height for its title's kind: the banner plus its inset (Music rows are taller). */
	const rowH = (k: string) => bannerHeight(bannerW, k) + ART_GAP;
	// Music's square art packs a little tighter; the All view keeps the 2:3 columns.
	const gridMin = $derived(posterMinFor(posterSize, kind === 'artist' ? 'artist' : 'movie'));
	const colWidths = $derived([`${bannerW}px`, '28px', 'minmax(180px, 1fr)', ...(kind === 'all' ? [] : shownCols.map((c) => c.width))]);
	const gridCols = $derived(colWidths.join(' '));
	// One width for every row (header included), so columns line up.
	const tableMin = $derived(tableMinWidth(colWidths));

	const entries = $derived(buildEntries(shown, grouped, seriesOfTitle, seriesName));
	const rowEntries = $derived(entries.filter((e): e is Extract<Entry, { type: 'row' }> => e.type === 'row'));
	/** The movie series a title is in, as its column prints it. */
	const seriesOf = (t: TitleView) => {
		const id = seriesOfTitle.get(t.id);
		return id ? seriesName(id) : '';
	};

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

	// A to Z, only while sorted by name. Letters follow the sort, which skips a leading The, A or An.
	const jumps = $derived(firstOfLetter(shown.map((t) => ({ id: t.id, name: sortName(t.name) }))));
	const railOn = $derived(sort.key === 'name' && !grouped && shown.length > 12);
	let railFocus = $state('A');
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
	$effect(() => {
		if (!jumps.has(railFocus)) railFocus = letters.find((l) => jumps.has(l)) ?? 'A';
	});

	function titleLink(id: number): string {
		const url = page.shallow?.url ?? page.url;
		return to(`/library/${id}?returnTo=${encodeURIComponent(url.pathname + url.search)}`);
	}
	// Where a title opens: artists go to their music page from every view, the rest to the title page.
	function openLink(t: TitleView): string {
		return t.kind === 'artist' && t.ids?.musicbrainz ? artistHref(t.ids.musicbrainz) : titleLink(t.id);
	}

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

	onMount(() => {
		const onResize = () => {
			viewH = window.innerHeight;
			readTop();
		};
		onResize();
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
	});
</script>

<svelte:window bind:scrollY />

<!-- The rail column is kept while loading too, so the cards do not narrow when it appears. -->
<div class:has-rail={railOn || (!loaded && sort.key === 'name' && !grouped)} class="with-rail">
	<div class="content">
		<PageLoader loading={!loaded} label={tr('mediafiles.list.loading')} onretry={retryLoad}>
		{#if error}
			<LoadError hasData={false} message={plainReason(error).text} onretry={retryLoad} />
		{:else if titles.length === 0}
			{@const [firstAdd, ...moreAdds] = emptyAdds(kind, partOff)}
			<EmptyState icon="library" title={tr('mediafiles.list.nothingYet')} action={firstAdd ? { label: addWords[firstAdd.kind], href: to(firstAdd.href), icon: 'search' } : undefined} />
			<div class="empty-more">
				{#each moreAdds as a (a.kind)}<Button href={to(a.href)}>{addWords[a.kind]}</Button>{/each}
				{#if session.can('settings')}<Button href={to('/settings/library-folders')}>{tr('mediafiles.list.folders')}</Button>{/if}
			</div>
		{:else if shown.length === 0}
			<EmptyState compact icon="search" title={q && !filtersOn ? tr('mediafiles.list.nothingFor', { q }) : tr('mediafiles.list.nothingMatches')} action={filtersOn || q ? { label: tr('mediafiles.saved.clear'), onclick: clearFilters } : undefined} />
		{:else if view === 'posters'}
			<ul class="grid" class:selecting style="--poster-min: {gridMin}px" style:--poster-min-phone={kind === 'artist' ? `${squarePhoneMin[posterSize]}px` : undefined} aria-label={tr('mediafiles.list.titlesOf', { kind: kindTitle })} bind:this={gridEl}>
				{#each shown as t (t.id)}
					{@const p = progressOf(t)}
					{@const ls = who === 'staff' ? libraryState(t) : null}
					<li class="item" style:--poster-ratio={posterRatio(t.kind)} animate:flip={{ duration: moveMs, easing: easeOut }} data-testid="library-card" data-title-id={t.id} data-letter={letterOf(sortName(t.name))} class:selected={picked.has(t.id)} class:last-opened={lastOpenedId === t.id}>
						<LibraryPoster title={t.name} year={t.year} href={openLink(t)} image={posterSrc(t.artwork[0], gridMin)} kind={t.kind === 'artist' ? 'artist' : t.kind === 'show' ? 'show' : 'movie'} vtName={artName(t.id)} status={ls} attention={needsAttention(ls)} bar={barOf(ls, p.have, p.total)} detail={posterDetail} extra={posterExtra(t)} facts={posterFacts(t)} score={sort.key === 'score' ? scoreText(t) : ''} kept={t.keep_up_to_date} {canManage} {selecting} selected={picked.has(t.id)} onsearch={() => searchTitles([t.id])} onkeep={(k) => setKeep([t.id], k)} />
						{#if selecting}
							<button type="button" class="pick" aria-pressed={picked.has(t.id)} aria-label={tr('mediafiles.list.select', { name: t.name })} onclick={() => togglePick(t.id)} data-testid="library-pick"><span class="pick-dot" aria-hidden="true">{#if picked.has(t.id)}<Icon name="check" size={14} stroke={3} />{/if}</span></button>
						{/if}
						{#if lastOpenedId === t.id}<span class="sr-only" data-testid="last-opened">{tr('mediafiles.list.lastOpened')}</span>{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<div class="rows {view}" class:v-rows={view === 'rows'} class:has-select={selecting} style="--acts-w: {actsW}px; --cols: {gridCols}; --table-min: {tableMin}px; --banner-w: {bannerW}px; --row-pad: {20 + touchActs + (view === 'table' && !phone && hiddenCols > 0 ? 16 : 0)}px" bind:this={listEl} bind:clientWidth={listWidth}>
				{#if view === 'rows' && !phone}
					<div class="lh" role="presentation" style="--facts: {factsTpl}">
						{#if selecting}<span></span>{/if}
						<span></span>
						<button type="button" class="h sort" aria-label={sort.key === 'name' ? tr('mediafiles.list.sorted', { what: tr('mediafiles.list.title'), dir: tr(sort.dir === 'asc' ? 'mediafiles.list.lowHigh' : 'mediafiles.list.highLow') }) : tr('mediafiles.list.title')} aria-pressed={sort.key === 'name'} onclick={() => setSort('name')}>{tr('mediafiles.list.title')}{#if sort.key === 'name'}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
						<span class="h">{tr('mediafiles.col.status')}</span>
						{#each rowCols as c (c.id)}
							{@const sk = columnSort[c.id]}
							{#if sk && sortsFor(kind === 'all' ? 'all' : libKind).includes(sk)}
								<button type="button" class="h sort" aria-label={sort.key === sk ? tr('mediafiles.list.sorted', { what: c.label, dir: tr(sort.dir === 'asc' ? 'mediafiles.list.lowHigh' : 'mediafiles.list.highLow') }) : c.label} aria-pressed={sort.key === sk} onclick={() => setSort(sk)}>{c.label}{#if sort.key === sk}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
							{:else}
								<span class="h">{c.label}</span>
							{/if}
						{/each}
						<span></span>
					</div>
				{/if}
				{#if view === 'table' && !phone}
					<div class="row head" role="presentation">
						<span class="h banner-h">{tr('mediafiles.list.banner')}</span>
						<span></span>
						<button type="button" class="h sort" aria-label={sort.key === 'name' ? tr('mediafiles.list.sorted', { what: tr('mediafiles.list.title'), dir: tr(sort.dir === 'asc' ? 'mediafiles.list.lowHigh' : 'mediafiles.list.highLow') }) : tr('mediafiles.list.title')} aria-pressed={sort.key === 'name'} onclick={() => setSort('name')}>{tr('mediafiles.list.title')}{#if sort.key === 'name'}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
						{#if kind !== 'all'}
							{#each shownCols as c (c.id)}
								{@const sk = columnSort[c.id]}
								{#if sk && sortsFor(libKind).includes(sk)}
									<button type="button" class="h sort" class:num={c.numeric} aria-label={sort.key === sk ? tr('mediafiles.list.sorted', { what: c.label, dir: tr(sort.dir === 'asc' ? 'mediafiles.list.lowHigh' : 'mediafiles.list.highLow') }) : c.label} aria-pressed={sort.key === sk} onclick={() => setSort(sk)}>{c.label}{#if sort.key === sk}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
								{:else}
									<span class="h" class:num={c.numeric}>{c.label}</span>
								{/if}
							{/each}
						{/if}
						{#if hiddenCols > 0}<span class="h more" title={tr(hiddenCols === 1 ? 'mediafiles.list.moreColOne' : 'mediafiles.list.moreColMany', { n: hiddenCols })}>+{hiddenCols}</span>{/if}
					</div>
				{/if}
				<div class="body" style:height={virtual ? `${offsets[entries.length]}px` : undefined}>
					<div style:padding-top={virtual ? `${offsets[range.start]}px` : undefined} role="list" aria-label={tr('mediafiles.list.titlesOf', { kind: kindTitle })}>
						{#each slice as e (e.key)}
							{#if e.type === 'head'}
								<div class="series-head" use:measure={e.key} data-key={e.key}>
									<h2>{e.name}</h2>
									<span>{tr('mediafiles.list.inLibraryCount', { n: e.count })}</span>
									{#if e.id}<a href={to(`/library/collections/${e.id}`)}>{tr('mediafiles.list.openCollection')}</a>{/if}
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
									aria-label="{t.name}{t.year ? `, ${t.year}` : ''}{picked.has(t.id) ? `, ${tr('mediafiles.list.selectedWord')}` : ''}"
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
											{#if selecting}<span class="lsel"><input type="checkbox" class="pick-box lpick" checked={picked.has(t.id)} aria-label={tr('mediafiles.list.select', { name: t.name })} tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" /></span>{/if}
											<span class="lp" style:--poster-ratio={posterRatio(t.kind)} class:round={t.kind === 'artist'}>
												<Poster src={t.artwork[0]} name={t.name} />
											</span>
											<span class="lt">
												<a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>
												{#if !phone && t.year}<small>{t.year}</small>{/if}
												{#if phone}{@const fc = rowCols.find((c) => !isProgressCol(c)) ?? rowCols[0]}<span class="lsub">{[ls?.label, fc ? rowCell(t, fc, p, seriesOf(t)) : ''].filter(Boolean).join(' · ')}</span>{/if}
											</span>
											{#if !phone}
												<span class="ls {ls?.tone ?? 'idle'}" data-testid="library-status" data-state={ls?.key}><i class="dot" aria-hidden="true"></i>{ls?.label ?? '—'}</span>
												{#each rowCols as c (c.id)}<span class="lc" title={c.id === 'path' ? cell(t, c, seriesOf(t)) : undefined}>{rowCell(t, c, p, seriesOf(t)) || '—'}</span>{/each}
												<span class="acts">
													{#if canManage && !selecting}
														<IconButton icon="search" size="sm" tipSide="below-end" label={tr('mediafiles.poster.searchFor', { title: t.name })} tabindex={ri === active ? 0 : -1} onclick={() => searchTitles([t.id])} />
														<IconButton icon="refresh" size="sm" tipSide="below-end" label={tr('mediafiles.list.refresh', { name: t.name })} tabindex={ri === active ? 0 : -1} onclick={() => refreshTitle(t)} />
														<IconButton icon="tools" size="sm" tipSide="below-end" label={tr('mediafiles.list.edit', { name: t.name })} tabindex={ri === active ? 0 : -1} href={titleLink(t.id)} />
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
											<span class="state" title={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')}>
												{#if selecting}<input type="checkbox" class="pick-box" checked={picked.has(t.id)} aria-label={tr('mediafiles.list.select', { name: t.name })} tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />
												{:else}<span class="keep" class:on={t.keep_up_to_date} aria-label={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')} role="img"><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={16} /></span>{/if}
											</span>
											<span class="name">
												<a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>
												<span class="sub">
													{#if kind === 'all'}{kindLabel(t.kind)}{#if t.year} · {t.year}{/if}{#if sort.key === 'score' && !phone && scoreText(t)} · ★ {scoreText(t)}{/if}{/if}
													{#if ls && (kind === 'all' || phone || !shownCols.some((c) => c.id === 'status'))}<span class="ls {ls.tone}" data-testid="library-status" data-state={ls.key}><i class="dot" aria-hidden="true"></i>{ls.label}</span>{/if}
												</span>
												{#if phone}<span class="facts">{factCols.slice(0, 3).map((c) => (isProgressCol(c) ? `${p.have}/${p.total}` : cell(t, c, seriesOf(t)))).filter(Boolean).join(' · ')}</span>{/if}
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
														<span class="c keep-c" title={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')}><span class="keep" class:on={t.keep_up_to_date} role="img" aria-label={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')}><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={16} /></span></span>
													{:else if isProgressCol(c)}
														<span class="c prog">
															<span class="meter {tone(t)}"><i style="width: {p.total ? (p.have / p.total) * 100 : 0}%"></i></span>
															<span class="num-t">{p.have}/{p.total}</span>
															<span class="sr-only">{progressText(t)}</span>
														</span>
													{:else}
														<span class="c" class:num={c.numeric} class:path={c.id === 'path'} title={c.id === 'path' ? cell(t, c, seriesOf(t)) : undefined}>{cell(t, c, seriesOf(t)) || '—'}</span>
													{/if}
												{/each}
											{/if}
											<span class="acts">
												{#if canManage && !selecting}
													<IconButton icon="search" size="sm" tipSide="below-end" label={tr('mediafiles.poster.searchFor', { title: t.name })} tabindex={ri === active ? 0 : -1} onclick={() => searchTitles([t.id])} />
													<IconButton icon="refresh" size="sm" tipSide="below-end" label={tr('mediafiles.list.refresh', { name: t.name })} tabindex={ri === active ? 0 : -1} onclick={() => refreshTitle(t)} />
													<IconButton icon="tools" size="sm" tipSide="below-end" label={tr('mediafiles.list.edit', { name: t.name })} tabindex={ri === active ? 0 : -1} href={titleLink(t.id)} />
												{/if}
											</span>
											{#if kind === 'all' || phone}<span class="sr-only">{progressText(t)}</span>{/if}
										</div>
									{:else}
										<div class="ov" class:music={t.kind === 'artist'}>
											<a class="ov-poster" style:--poster-ratio={posterRatio(t.kind)} href={openLink(t)} tabindex="-1" aria-hidden="true"><Poster src={t.artwork[0]} name={t.name} /></a>
											<div class="ov-main">
												<h2><a href={openLink(t)} tabindex="-1" title={t.name}>{t.name}</a>{#if t.year}<span class="yr">{t.year}</span>{/if}<span class="keep ov-keep" class:on={t.keep_up_to_date} role="img" aria-label={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')} title={tr(t.keep_up_to_date ? 'mediafiles.poster.kept' : 'mediafiles.poster.notKept')}><Icon name={t.keep_up_to_date ? 'bellOn' : 'bell'} size={15} /></span></h2>
												{#if t.kind === 'artist'}
													{#if t.artist?.bio}<p class="ov-text ov-bio">{t.artist.bio}</p>{/if}
												{:else}
													<p class="ov-text">{t.overview || tr('mediafiles.list.noDescription')}</p>
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
												{#if ls}<dt class="sr-only">{tr('mediafiles.col.status')}</dt><dd class="ov-state"><span class="ls {ls.tone}"><i class="dot" aria-hidden="true"></i>{ls.label}</span></dd>{/if}
												{#each chosenCols.filter((c) => !isProgressCol(c) && c.id !== 'status' && c.id !== 'kept').slice(0, 4) as c (c.id)}{#if cell(t, c, seriesOf(t))}<dt>{c.label}</dt><dd title={cell(t, c, seriesOf(t))}>{cell(t, c, seriesOf(t))}</dd>{/if}{/each}
											</dl>
											{#if selecting}<input type="checkbox" class="pick-box ov-pick" checked={picked.has(t.id)} aria-label={tr('mediafiles.list.select', { name: t.name })} tabindex="-1" onchange={() => togglePick(t.id)} data-testid="library-pick" />{/if}
										</div>
									{/if}
									{#if fold}
										{@const eps = missingOf[t.id]}
										<div class="fold">
											<Button variant="plain" size="sm" icon="chevronRight" class="fold-btn" aria-expanded={foldOpen === t.id} aria-controls="missing-{t.id}" onclick={() => toggleMissing(t)} data-testid="missing-fold">{tr(t.show?.missing === 1 ? 'mediafiles.list.missingEpOne' : 'mediafiles.list.missingEpMany', { n: t.show?.missing ?? 0 })}</Button>
											{#if foldOpen === t.id}
												<div id="missing-{t.id}" class="fold-body" data-testid="missing-episodes" in:grow out:shrink>
													{#if eps === 'loading' || eps === undefined}<Skeleton lines={2} />
													{:else if eps === 'error'}<p class="fold-note">{tr('mediafiles.list.episodesFailed')} <a href={titleLink(t.id)}>{tr('mediafiles.list.openName', { name: t.name })}</a></p>
													{:else if eps.length === 0}<p class="fold-note">{tr('mediafiles.list.nothingMissing')}</p>
													{:else}
														<ul>
															{#each eps as ep (ep.id)}
																<li data-testid="missing-episode"><span class="ep-code">{episodeLabel(ep, t.show?.episode_order ?? '')}</span> <span class="ep-name">{ep.name || tr('shows.page.nameUnknown')}</span>{#if ep.air_date}<span class="ep-date"> · {date(ep.air_date)}</span>{/if}{#if ep.slots[0] && ep.slots[0].state !== 'wanted'}{' '}{@const es = audienceStatus(slotKind(ep.slots[0].state), who, { tracked: true })}<StatusPill kind={es.kind} label={who === 'staff' ? serverWords(ep.slots[0].state_label) : es.label} />{/if}</li>
															{/each}
														</ul>
													{/if}
												</div>
											{/if}
										</div>
									{/if}
									{#if lastOpenedId === t.id}<span class="sr-only" data-testid="last-opened">{tr('mediafiles.list.lastOpened')}</span>{/if}
									{#if requester && view !== 'rows'}
										<p class="row-notes">
											<span class="requester" data-testid="requested-by">{tr('mediafiles.view.askedBy', { name: requester })}</span>
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
	{#if railOn && loaded && !error}<LibraryLetterRail {jumps} bind:focus={railFocus} onjump={jump} />{/if}
</div>

<style>
	.content { min-width: 0; }
	.with-rail { min-width: 0; }
	.with-rail.has-rail { display: grid; grid-template-columns: minmax(0, 1fr) 26px; gap: 12px; align-items: start; }
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

	.rows { position: relative; isolation: isolate; min-width: 0; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b) 420px); box-shadow: var(--edge), var(--drop); overflow: clip; }
	/* A whisper of grain keeps the flat colour from looking printed (as on Home's tiles). */
	.rows::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; opacity: .05; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E"); }
	.row { display: grid; grid-template-columns: var(--cols); align-items: center; gap: 0 16px; min-height: var(--row-h, 100px); padding: 0 var(--row-pad, 20px) 0 8px; width: 100%; box-sizing: border-box; }
	/* The virtual list moves its own rows (a top spacer); the browser's scroll anchoring must not also correct for them, or the two chase each other. */
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
	:global(.library.selecting) .entry, :global(.library.selecting) .grid > li { scroll-margin-bottom: 120px; }
	/* Every row carries its top line, so a row's height never changes when the virtual window drops the row above it (that would nudge the list 1px). */
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
	@media (max-width: 639px) {
		.with-rail.has-rail { grid-template-columns: minmax(0, 1fr) 20px; gap: 6px; }
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
