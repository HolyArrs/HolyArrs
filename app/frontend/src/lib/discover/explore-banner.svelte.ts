// Explore's banner: the favourite genre pick, and the Music banner.
import { discoverCard, ownedMark, ownedTitle, cardIdentity, type ExploreCard, type ownedIndex } from '#lib/ui/title-cards.ts';
import { serverRow } from './explore-server.ts';
import { genrePath, type GenreAnswer, GENRES } from './genres.ts';
import { fetchMusicGenre, musicImage, type MusicCardData, coverFor } from '#lib/music/index.ts';
import { api } from '#lib/core/api.ts';
import type { TitleView } from '#lib/core/types.ts';
import type { audienceOf } from '#lib/ui/title-status.ts';
import type { ExploreType } from './explore-type.ts';

export function createExploreBanner(ctx: {
	type: ExploreType;
	titles: TitleView[];
	headLoaded: boolean;
	gone: AbortController;
	live: (c: ExploreCard) => ExploreCard;
	owned: ReturnType<typeof ownedIndex>;
	who: ReturnType<typeof audienceOf>;
	musicGenres: { key: string; name: string; href: string; art: string; images?: string[]; hue: number }[];
	genreOn: boolean;
}) {
	const bannerLayout: number = 2;
	const bannerNorm = (n: string) => n.toLowerCase().replace(/\s*&\s*/g, ' & ');
	/** The household's favourite genre: the most common among Library titles (asked for ones included). */
	const favourite = $derived.by(() => {
		if (ctx.type === 'music') return null;
		const tally = new Map<string, { n: number; kind: 'movies' | 'shows'; id: number; name: string }>();
		for (const t of ctx.titles) {
			const k = t.kind === 'movie' ? 'movies' : t.kind === 'show' ? 'shows' : null;
			if (!k || (ctx.type !== 'all' && ctx.type !== k)) continue;
			const list = GENRES[k === 'movies' ? 'movie' : 'show'];
			for (const gn of t.genres ?? []) {
				const g = list.find((x) => bannerNorm(x.name) === bannerNorm(gn) || bannerNorm(x.name).split(' & ').includes(bannerNorm(gn)));
				if (!g) continue;
				const key = `${k}:${g.id}`;
				const e = tally.get(key) ?? { n: 0, kind: k, id: g.id, name: g.name };
				e.n++;
				tally.set(key, e);
			}
		}
		// Movies and shows count as one genre by name: the larger side opens its page.
		const byName = new Map<string, { n: number; best: { n: number; kind: 'movies' | 'shows'; id: number; name: string } }>();
		for (const e of tally.values()) {
			const cur = byName.get(e.name) ?? { n: 0, best: e };
			cur.n += e.n;
			if (e.n > cur.best.n) cur.best = e;
			byName.set(e.name, cur);
		}
		const top = [...byName.values()].sort((a, b) => b.n - a.n)[0];
		return top && top.n >= 2 ? { ...top.best, count: top.n } : null;
	});
	let bannerCards = $state<ExploreCard[]>([]);
	let bannerFor = '';
	$effect(() => {
		const f = favourite;
		if (bannerLayout === 0 || !f || !ctx.headLoaded) return;
		const key = `${f.kind}:${f.id}`;
		if (bannerFor === key) return;
		bannerFor = key;
		bannerCards = [];
		// The app's own genre answer (the genre page's), never an outside call.
		api.get<GenreAnswer>(genrePath(f.kind, f.id), ctx.gone.signal).then(
			(a) => {
				if (bannerFor !== key) return;
				const got = [...(a.hero ?? []).map((d) => ({ ...discoverCard(d, 'banner'), wide: d.backdrop ?? null })), ...a.rows.flatMap((r) => serverRow(r).cards)];
				const seen = new Set<string>();
				bannerCards = got.filter((c) => (seen.has(cardIdentity(c)) ? false : (seen.add(cardIdentity(c)), true)));
			},
			() => {}
		);
	});
	const bannerPicks = $derived(bannerCards.map(ctx.live).filter((c) => (c.kind === 'movie' || c.kind === 'show') && c.image && !ownedMark(c, ownedTitle(c, ctx.owned), ctx.who) && c.titleId === undefined).slice(0, 7));
	// Music: the household's top music genre (first of the genre tiles the music rows answer, built from the Library's artists) and its albums and artists not in the Library yet, from the app's own genre page answer.
	let musicPicks = $state<MusicCardData[]>([]);
	/** The picks are on their way: the banner keeps its place (placeholders) so the rows below never jump. */
	let musicPicksPending = $state(false);
	let musicPicksFor = '';
	const musicFav = $derived(ctx.type === 'music' ? (ctx.musicGenres[0] ?? null) : null);
	/** The music rows' first answer is in (musicGenres says all it will). */
	let musicReady = $state(false);
	/** Until the music rows answer, the "Your genre" banner's place is held (the same banner with placeholders, so the exact height) when the last visit showed it, so the rows never jump down when it arrives. Remembered per browser. */
	let musicBannerHint = $state(true);
	/** The last banner's genre name: the held banner's name wraps the same, so the same height. */
	let musicBannerName = $state('');
	try {
		const kept = localStorage.getItem('holyarrs-music-banner');
		musicBannerHint = kept !== '0';
		musicBannerName = kept && kept !== '0' && kept !== '1' ? kept : '';
	} catch {
		/* Storage off: hold the place. */
	}
	$effect(() => {
		if (ctx.type !== 'music') musicReady = false;
	});
	const musicBannerShown = $derived(ctx.type === 'music' && !!musicFav && (musicPicksPending || musicPicks.length >= 4));
	const musicBannerHold = $derived(ctx.type === 'music' && !musicFav && !musicReady && musicBannerHint);
	$effect(() => {
		if (ctx.type !== 'music' || !musicReady || musicPicksPending) return;
		try {
			localStorage.setItem('holyarrs-music-banner', musicBannerShown && musicFav ? musicFav.name : '0');
		} catch {
			/* Storage off. */
		}
	});
	$effect(() => {
		const g = musicFav;
		if (!g) return;
		const slug = g.href.split('/music/genre/')[1]?.split('?')[0] ?? '';
		if (!slug || musicPicksFor === slug) return;
		musicPicksFor = slug;
		musicPicks = [];
		musicPicksPending = true;
		void fetchMusicGenre(decodeURIComponent(slug), 'US', api, ctx.gone.signal).then(
			(a) => {
				if (musicPicksFor !== slug) return;
				const seen = new Set<string>();
				musicPicks = [...(a.hero ? [a.hero] : []), ...a.rows.flatMap((r) => r.items ?? [])]
					.filter((c) => (c.kind === 'album' || c.kind === 'artist') && c.image && !c.title_id && !seen.has(c.key) && (seen.add(c.key), true))
					.slice(0, 7)
					// The rows' own address (versioned, card size), so each cover downloads once.
					.map((c) => ({ ...c, image: coverFor(musicImage(c), 150).src ?? c.image }));
			},
			() => {}
		).finally(() => musicPicksFor === slug && (musicPicksPending = false));
	});
	const bannerOn = $derived(bannerLayout > 0 && !ctx.genreOn && ctx.type !== 'music' && !!favourite && bannerPicks.length >= 5);

	return {
		get musicBannerHold() {
			return musicBannerHold;
		},
		get musicBannerName() {
			return musicBannerName;
		},
		set musicBannerName(v) {
			musicBannerName = v;
		},
		get musicBannerShown() {
			return musicBannerShown;
		},
		get musicFav() {
			return musicFav;
		},
		get musicPicks() {
			return musicPicks;
		},
		set musicPicks(v) {
			musicPicks = v;
		},
		get musicPicksPending() {
			return musicPicksPending;
		},
		set musicPicksPending(v) {
			musicPicksPending = v;
		},
		get musicReady() {
			return musicReady;
		},
		set musicReady(v) {
			musicReady = v;
		},
		get bannerOn() {
			return bannerOn;
		},
		get favourite() {
			return favourite;
		},
		get bannerLayout() {
			return bannerLayout;
		},
		get bannerPicks() {
			return bannerPicks;
		},
	};
}
