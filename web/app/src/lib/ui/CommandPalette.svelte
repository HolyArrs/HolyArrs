<script lang="ts">
	import Icon from '../Icon.svelte';
	import Poster from '../Poster.svelte';
	import Sheet from './Sheet.svelte';
	import Skeleton from './Skeleton.svelte';
	import { api } from '../api.ts';
	import { partsNow } from '../parts.svelte.ts';
	import { remember } from '../title-details.ts';
	import { filterDestinations, type Destination } from './nav.ts';
	import {
		artistHits,
		libraryByTmdb,
		libraryHits,
		movieHits,
		options as flatten,
		searchPath,
		sectionLabels,
		showHits,
		type Hit,
		type Option,
		type Section,
		type SectionId
	} from '../instant-search.ts';
	import type { LookupArtist, LookupShow, Movie, TitleView } from '../types.ts';

	// ⌘K / Ctrl+K and the top bar search: places to go, then instant title
	// results (Library, Movies, TV shows, Music) as you type. Arrow keys move,
	// Enter opens, Escape closes. See all opens /search?q=….
	let {
		open = $bindable(false),
		items,
		go,
		onsearch,
		initial = ''
	}: { open?: boolean; items: Destination[]; go: (href: string) => void; onsearch: (q: string) => void; initial?: string } = $props();

	const id = $props.id();
	let q = $state('');
	let active = $state(0);
	let input = $state<HTMLInputElement>();
	let sections = $state<Section[]>([]);
	const typed = $derived(q.trim());
	const places = $derived(filterDestinations(items, q).slice(0, typed ? 4 : 8));
	const list = $derived(flatten(places.length, sections, q));
	const partOn = (name: string) => partsNow.answer?.parts.find((p) => p.part === name)?.on !== false;

	$effect(() => {
		if (open) {
			q = initial;
			active = 0;
			queueMicrotask(() => {
				input?.focus();
				if (input && initial) input.setSelectionRange(initial.length, initial.length);
			});
		}
	});

	// Debounced lookups; a newer query aborts the older requests.
	let timer: ReturnType<typeof setTimeout> | undefined;
	let musicTimer: ReturnType<typeof setTimeout> | undefined;
	let ctrl: AbortController | undefined;
	$effect(() => {
		const term = typed;
		active = 0;
		clearTimeout(timer);
		clearTimeout(musicTimer);
		ctrl?.abort();
		if (!open || term.length < 2) {
			sections = [];
			return;
		}
		const ids: SectionId[] = ['library', ...(partOn('movies') ? (['movies'] as const) : []), ...(partOn('shows') ? (['shows'] as const) : []), ...(partOn('music') && term.length >= 3 ? (['music'] as const) : [])];
		sections = ids.map((s) => ({ id: s, label: sectionLabels[s], hits: [], state: 'loading' }));
		const c = (ctrl = new AbortController());
		timer = setTimeout(() => void lookup(term, c, ids), 250);
		// MusicBrainz answers once a second, so music waits for a pause in typing.
		if (ids.includes('music')) musicTimer = setTimeout(() => void music(term, c), 650);
		return () => {
			clearTimeout(timer);
			clearTimeout(musicTimer);
		};
	});

	function put(sid: SectionId, change: Partial<Section>) {
		sections = sections.map((s) => (s.id === sid ? { ...s, ...change } : s));
	}
	const enc = (t: string) => encodeURIComponent(t);

	async function lookup(term: string, c: AbortController, ids: SectionId[]) {
		let lib = new Map<string, number>();
		const library = api
			.get<{ titles: TitleView[] }>(`/titles?q=${enc(term)}`, c.signal)
			.then((r) => {
				lib = libraryByTmdb(r.titles);
				put('library', { hits: libraryHits(r.titles), state: 'done', more: `/library?q=${enc(term)}` });
			})
			.catch(() => !c.signal.aborted && put('library', { state: 'error' }));
		const movies = ids.includes('movies')
			? api.get<{ movies: Movie[] }>(`/metadata/search?q=${enc(term)}`, c.signal).catch((e) => (c.signal.aborted ? null : e instanceof Error ? e : null))
			: null;
		const shows = ids.includes('shows') ? api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${enc(term)}`, c.signal).catch((e) => (c.signal.aborted ? null : e instanceof Error ? e : null)) : null;
		await library;
		if (c.signal.aborted) return;
		if (movies) {
			const r = await movies;
			if (c.signal.aborted) return;
			if (r && !(r instanceof Error)) put('movies', { hits: movieHits(r.movies, lib), state: 'done', more: searchPath(term) });
			else put('movies', { state: 'error' });
		}
		if (shows) {
			const r = await shows;
			if (c.signal.aborted) return;
			if (r && !(r instanceof Error)) put('shows', { hits: showHits(r.shows, lib), state: 'done', more: searchPath(term, 'show') });
			else put('shows', { state: 'error' });
		}
	}

	async function music(term: string, c: AbortController) {
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${enc(term)}`, c.signal);
			if (!c.signal.aborted) put('music', { hits: artistHits(r.artists), state: 'done', more: searchPath(term, 'music') });
		} catch {
			if (!c.signal.aborted) put('music', { state: 'error' });
		}
	}

	function choose(o: Option | undefined) {
		if (!o) return;
		open = false;
		if (o.kind === 'place') go(places[o.index].href);
		else if (o.kind === 'hit') {
			if (o.hit.detail) remember(o.hit.detail.kind, o.hit.detail.tmdb, { name: o.hit.name, year: o.hit.year, poster: o.hit.poster });
			go(o.hit.href);
		} else if (o.kind === 'more') go(o.href);
		else onsearch(typed);
	}
	function key(e: KeyboardEvent) {
		const n = list.length;
		if (e.key === 'ArrowDown') { e.preventDefault(); active = n ? (active + 1) % n : 0; }
		else if (e.key === 'ArrowUp') { e.preventDefault(); active = n ? (active - 1 + n) % n : 0; }
		else if (e.key === 'Home' && e.ctrlKey) { e.preventDefault(); active = 0; }
		else if (e.key === 'End' && e.ctrlKey) { e.preventDefault(); active = Math.max(0, n - 1); }
		else if (e.key === 'Enter') { e.preventDefault(); if (n) choose(list[active]); else if (typed) onsearch(typed); }
	}
	$effect(() => {
		// Keep the active row in view while arrowing through a long list.
		void active;
		document.getElementById(`${id}-o${active}`)?.scrollIntoView({ block: 'nearest' });
	});

	const indexOf = (o: (x: Option) => boolean) => list.findIndex(o);
	const hitIndex = (sid: SectionId, h: Hit) => indexOf((x) => x.kind === 'hit' && x.section === sid && x.hit.key === h.key);
	const moreIndex = (sid: SectionId) => indexOf((x) => x.kind === 'more' && x.section === sid);
	const allIndex = $derived(indexOf((x) => x.kind === 'all'));
	const loading = $derived(sections.some((s) => s.state === 'loading'));
	const nothing = $derived(typed.length >= 2 && !loading && !places.length && sections.every((s) => s.state !== 'loading' && !s.hits.length));
</script>

<Sheet bind:open title="Search and go" hideTitle size="lg" placement="full">
	<!-- Only while open, so the closed palette adds no hidden copy of every place. -->
	{#if open}
	<div class="ui-palette">
		<div class="bar">
			<Icon name="search" size={18} />
			<input
				bind:this={input}
				bind:value={q}
				type="text"
				role="combobox"
				aria-label="Search titles or go to a place"
				aria-expanded={list.length > 0}
				aria-controls="{id}-list"
				aria-activedescendant={list.length ? `${id}-o${active}` : undefined}
				aria-autocomplete="list"
				placeholder="Search titles or go to…"
				autocomplete="off"
				spellcheck="false"
				onkeydown={key}
			/>
			<kbd>esc</kbd>
			<button class="cancel" type="button" onclick={() => (open = false)}>Cancel</button>
		</div>
		<div class="results" id="{id}-list" role="listbox" aria-label="Results" aria-busy={loading}>
			{#if places.length}
				<div role="group" aria-labelledby="{id}-g-places">
					{#if typed}<p class="sh" id="{id}-g-places">Go to</p>{:else}<span class="sr-only" id="{id}-g-places">Places</span>{/if}
					{#each places as d, i (d.href + d.label)}
						<div id="{id}-o{i}" role="option" tabindex="-1" aria-selected={i === active} class="row" class:active={i === active} onpointermove={() => (active = i)} onclick={() => choose(list[i])} onkeydown={key}>
							<span class="ic"><Icon name={d.icon} size={17} /></span><span class="grow">{d.label}</span><span class="sec">{d.section}</span>
						</div>
					{/each}
				</div>
			{/if}
			{#each sections as s (s.id)}
				<div role="group" aria-labelledby="{id}-g-{s.id}" data-testid="palette-{s.id}">
					<p class="sh" id="{id}-g-{s.id}">{s.label}</p>
					{#if s.state === 'loading'}
						<div class="skel" aria-hidden="true"><Skeleton lines={2} /></div>
					{:else if s.state === 'error'}
						<p class="note">Could not search {s.label.toLowerCase()} right now.</p>
					{:else if !s.hits.length}
						<p class="note">No {s.id === 'library' ? 'titles in your Library' : s.label.toLowerCase()} match “{typed}”.</p>
					{/if}
					{#each s.hits as h (h.key)}
						{@const i = hitIndex(s.id, h)}
						<div id="{id}-o{i}" role="option" tabindex="-1" aria-selected={i === active} class="row hit" class:active={i === active} onpointermove={() => (active = i)} onclick={() => choose(list[i])} onkeydown={key}>
							<span class="art" class:square={h.kindWord === 'Artist'}><Poster src={h.poster} name={h.name} /></span>
							<span class="grow"><span class="name">{h.name}</span><span class="sub">{[h.year, h.kindWord].filter(Boolean).join(' · ')}</span></span>
							{#if h.inLibrary}<span class="pill"><i aria-hidden="true"></i>In Library</span>{/if}
						</div>
					{/each}
					{#if moreIndex(s.id) >= 0}
						{@const i = moreIndex(s.id)}
						<div id="{id}-o{i}" role="option" tabindex="-1" aria-selected={i === active} class="row more" class:active={i === active} onpointermove={() => (active = i)} onclick={() => choose(list[i])} onkeydown={key}>
							<span class="grow">More {s.id === 'library' ? 'in your Library' : s.label.toLowerCase()}</span><Icon name="chevronRight" size={15} />
						</div>
					{/if}
				</div>
			{/each}
			{#if allIndex >= 0}
				<div id="{id}-o{allIndex}" role="option" tabindex="-1" aria-selected={allIndex === active} class="row all" class:active={allIndex === active} onpointermove={() => (active = allIndex)} onclick={() => choose(list[allIndex])} onkeydown={key}>
					<span class="ic"><Icon name="search" size={17} /></span><span class="grow">See all results for “{typed}”</span><Icon name="arrow" size={15} />
				</div>
			{/if}
		</div>
		{#if !typed}<p class="hint">Type a title, or a place like Downloads or Settings.</p>{:else if nothing}<p class="hint" role="status">Nothing matches “{typed}”. Try another spelling, or see all results.</p>{/if}
	</div>
	{/if}
</Sheet>

<style>
	.ui-palette { margin: -8px -22px -22px; display: flex; flex-direction: column; min-height: 0; }
	.bar { display: flex; align-items: center; gap: 10px; padding: 0 16px; height: 56px; flex: none; border-bottom: 1px solid var(--line); color: var(--text-muted); }
	input { flex: 1; min-width: 0; height: 100%; border: 0; background: transparent; color: var(--text); font: var(--t-body); font-size: 17px; outline: none; }
	/* The whole bar is the field: the input itself never draws a ring or tint;
	   with the keyboard the bar gets an accent underline. */
	.bar input { border: 0; background: transparent !important; box-shadow: none !important; }
	:global(:root[data-input='keyboard']) .bar:focus-within { box-shadow: inset 0 -2px 0 var(--accent); }
	input::placeholder { color: var(--text-muted); }
	kbd { padding: 2px 6px; border-radius: 5px; background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); }
	.cancel { display: none; min-height: 44px; padding: 0 4px; color: var(--accent-text); font: var(--t-body); font-weight: 500; }
	.results { padding: 6px; max-height: min(64dvh, 620px); overflow: auto; overscroll-behavior: contain; }
	.sh { padding: 10px 10px 4px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.row { display: flex; align-items: center; gap: 10px; min-height: 42px; padding: 0 10px; border-radius: 9px; cursor: pointer; font: var(--t-callout); font-size: 15px; }
	.row.active { background: var(--surface-3); }
	.row.all.active, .row.more.active { background: var(--accent-soft); }
	.hit { min-height: 56px; padding-block: 6px; }
	.art { width: 32px; flex: none; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.ic { width: 22px; display: grid; place-items: center; color: var(--text-muted); }
	.grow { flex: 1; min-width: 0; display: flex; flex-direction: column; overflow: hidden; }
	.grow, .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.row:not(.hit) .grow { display: block; }
	.name { font-weight: 500; }
	.sub { color: var(--text-muted); font: var(--t-footnote); }
	.sec { color: var(--text-muted); font: var(--t-footnote); }
	.more { color: var(--accent-text); font-weight: 500; min-height: 36px; }
	.all { margin-top: 4px; border-top: 1px solid var(--line); border-radius: 0 0 9px 9px; color: var(--accent-text); font-weight: 600; }
	.pill { display: inline-flex; align-items: center; gap: 6px; height: 22px; padding: 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--ok-text); font: var(--t-footnote); font-weight: 600; white-space: nowrap; }
	.pill i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
	.skel { padding: 6px 10px 10px; }
	.note { padding: 4px 10px 8px; color: var(--text-muted); font: var(--t-callout); }
	.hint { padding: 12px 18px 20px; color: var(--text-muted); font: var(--t-callout); }
	@media (max-width: 767px) {
		.results { max-height: none; flex: 1; }
		kbd { display: none; }
		.cancel { display: inline-flex; align-items: center; }
		.ui-palette { flex: 1; }
	}
</style>
