// Explore's genre pages: the rows, hero and art of one genre, and the way back to where the person was.
import { discoverCard, type ExploreCard, type ExploreRow } from '#lib/ui/title-cards.ts';
import { whenIdle } from '#lib/ui/lazy-art.ts';
import { readAnchor, readOffsets, type ExploreReturn } from './explore-return.ts';
import { serverRow } from './explore-server.ts';
import { genrePath, genresFor, parseGenre, type GenreAnswer } from './genres.ts';
import { parseType } from './explore-type.ts';
import { browsePath, browseSectionPath, tileHue, type BrowseAnswer, type BrowseSectionAnswer } from './search-browse.ts';
import { untrack } from 'svelte';
import { page } from '$app/state';
import { goto } from '$app/navigation';
import { api } from '#lib/core/api.ts';
import { fitTile } from '#lib/core/tmdb-image.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createExploreGenre(ctx: any) {
	const genre = $derived(parseGenre((page.shallow?.url ?? page.url).searchParams.get('genre'), ctx.type));
	const genres = $derived(genresFor(ctx.type));
	let musicGenres = $state<{ key: string; name: string; href: string; art: string; images?: string[]; hue: number }[]>([]);
	let genreRows = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let genreHero = $state<ExploreCard[]>([]);
	let genreLoaded = $state<number | null>(null);
	let genreFailed = $state(false);
	let genreAsk = 0;
	async function loadGenre(id: number, again = 0) {
		const n = again ? genreAsk : ++genreAsk;
		genreFailed = false;
		try {
			const a = await api.get<GenreAnswer>(genrePath(ctx.type, id), ctx.gone.signal);
			if (n !== genreAsk) return;
			const got = a.rows.map(serverRow);
			// Top on your services, in the genre, is the second row.
			const marker = { id: 'top_services', type: 'top_services', title: t('discover.exploreGenre.trendingOnStreaming'), cards: [], keep: true, wide: false };
			genreRows = [...got.slice(0, 1), marker, ...got.slice(1)];
			genreHero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'hero'), wide: d.backdrop ?? null }));
			if (!again) ctx.heroAt = 0;
			genreLoaded = id;
			// A cold page answers with its first rows; the lower ones follow on the next ask.
			if (a.partial && again < 6) setTimeout(() => n === genreAsk && void loadGenre(id, again + 1), 1000);
		} catch {
			if (n === genreAsk && !again) genreFailed = true;
		}
	}
	$effect(() => {
		const g = genre;
		if (g && genreLoaded !== g.id) void loadGenre(g.id);
	});
	// Back from a genre page lands on the tab and the spot it was opened from: the address (tab included) maps to where the page stood when a genre was opened from it.
	const spots = new Map<string, ExploreReturn>();
	function rememberSpot() {
		spots.set(new URL((page.shallow?.url ?? page.url).href).href, {
			y: Math.round(scrollY),
			rows: readOffsets(document),
			anchor: readAnchor(document, document.querySelector('main')?.getBoundingClientRect().top ?? 0)
		});
	}
	$effect(() => {
		const u = page.shallow?.url ?? page.url;
		untrack(() => {
			const t = parseType(u.searchParams.get('type'), ctx.musicKept);
			if (t !== ctx.type) {
				ctx.type = t;
				try {
					localStorage.setItem(ctx.TYPE_KEY, t);
				} catch {
					/* The choice is a convenience. */
				}
				void ctx.loadHead();
			}
			const spot = spots.get(u.href);
			if (spot && !u.searchParams.has('genre')) {
				spots.delete(u.href);
				ctx.back = spot;
			}
		});
	});
	/** Opens a genre page (a new history entry, so Back returns); null clears it. */
	function pickGenre(id: number | null, close?: () => void) {
		close?.();
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (id !== null) rememberSpot();
		if (id === null) url.searchParams.delete('genre');
		else url.searchParams.set('genre', String(id));
		genreLoaded = null;
		void goto(url, { shallow: true, reset: false });
		scrollTo({ top: 0, behavior: 'instant' });
	}
	const genreOn = $derived(!!genre && ctx.type !== 'music');

	// The Genres picker for Movies and TV: the Music tile look, each genre over the backdrop Search's Browse grid uses for it (read when the picker first opens).
	let browseArt = $state<Record<string, Record<string, string>>>({});
	async function loadBrowseArt(t: typeof ctx.type) {
		if (t !== 'movies' && t !== 'shows') return;
		if (browseArt[t]) return;
		browseArt = { ...browseArt, [t]: {} };
		try {
			// The same tiles Search's Browse grid shows: its first answer, then its Genres section. Worlds is read too: it holds the pictures the combined TV genres fall back on.
			const [a, more, worlds] = await Promise.all([api.get<BrowseAnswer>(browsePath(t)), api.get<BrowseSectionAnswer>(browseSectionPath(t, 'genres')).catch(() => null), api.get<BrowseSectionAnswer>(browseSectionPath(t, 'worlds')).catch(() => null)]);
			const art: Record<string, string> = {};
			for (const x of [...(a.tiles ?? []), ...(more?.tiles ?? []), ...(worlds?.tiles ?? [])]) {
				if (x.mood || !x.backdrop) continue;
				const src = fitTile(x.backdrop, 130, false);
				art[`id:${x.genre}`] ??= src;
				art[`name:${x.genre_name.toLowerCase()}`] ??= src;
				art[`name:${x.name.toLowerCase()}`] ??= src;
			}
			browseArt = { ...browseArt, [t]: art };
			// Fetched ahead into the browser's cache, so the tiles are filled the first time the picker opens.
			for (const src of new Set(Object.values(art))) new Image().src = src;
		} catch {
			// No art: the tiles keep their colour.
		}
	}
	// The picker's art is read ahead (once the head is in, when the browser is idle), not on first open.
	$effect(() => {
		const t = ctx.type;
		if (!ctx.headLoaded || (t !== 'movies' && t !== 'shows')) return;
		return whenIdle(() => void loadBrowseArt(t), 300);
	});
	// Genres Browse has no tile of their own (TMDB's combined TV genres) borrow a picture from the closest Browse tiles, tried in order.
	const genreArtAlias: Record<string, string[]> = {
		'action & adventure': ['action', 'adventure', 'treasure hunts'],
		'sci-fi & fantasy': ['sci-fi', 'science fiction', 'fantasy', 'space', 'fantasy worlds', 'parallel worlds'],
		'science fiction': ['sci-fi', 'space'],
		fantasy: ['fantasy worlds', 'medieval'],
		'war & politics': ['war', 'politics', 'world war ii', 'cold war'],
		war: ['world war ii', 'cold war'],
		'tv movie': ['drama'],
		history: ['victorian era', 'ancient worlds', 'medieval'],
	};
	function genreArt(art: Record<string, string> | undefined, id: number, name: string): string {
		if (!art) return '';
		const n = name.toLowerCase();
		for (const k of [`id:${id}`, `name:${n}`, ...(genreArtAlias[n] ?? []).map((a) => `name:${a}`)]) if (art[k]) return art[k];
		return '';
	}
	const genreTiles = $derived(genres.map((g) => ({ key: String(g.id), name: g.name, href: '', art: genreArt(browseArt[ctx.type], g.id, g.en ?? g.name), hue: 0, tint: tileHue(g.en ?? g.name) })));

	return {
		get genreOn() {
			return genreOn;
		},
		get musicGenres() {
			return musicGenres;
		},
		set musicGenres(v) {
			musicGenres = v;
		},
		get genreHero() {
			return genreHero;
		},
		set genreHero(v) {
			genreHero = v;
		},
		get genreLoaded() {
			return genreLoaded;
		},
		set genreLoaded(v) {
			genreLoaded = v;
		},
		get genre() {
			return genre;
		},
		get genreRows() {
			return genreRows;
		},
		set genreRows(v) {
			genreRows = v;
		},
		get genres() {
			return genres;
		},
		loadBrowseArt,
		get genreTiles() {
			return genreTiles;
		},
		pickGenre,
		get genreFailed() {
			return genreFailed;
		},
		set genreFailed(v) {
			genreFailed = v;
		},
		loadGenre,
	};
}
