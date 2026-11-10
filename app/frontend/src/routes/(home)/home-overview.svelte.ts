// Home's overview: the picture tiles, the stats and what is coming up.
import type { IconName } from '#lib/ui/Icon.svelte';
import { askArtist, askDetails, wantDetails } from '#lib/requests/index.ts';
import { arrivedTitle, type HomeCard } from './home.ts';
import { localDay } from './home-line.ts';
import { SvelteSet } from 'svelte/reactivity';
import type { Ask } from '#lib/core/types.ts';

export type Place = { key: string; shape: string; pic?: { c: HomeCard; label: string }; stat?: string };

export type UpLine = { key: string; c: HomeCard; sub: string; when: string; now: boolean };

import type { HomeContext } from './home-context.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createHomeOverview(ctx: Pick<HomeContext, 'manager' | 'kid' | 'home' | 'now' | 'rows' | 'owner' | 'canAsk' | 'libCards' | 'showDisk' | 'fetchingAll' | 'waiting'>) {
	let board = $state<'home' | 'system'>('home');
	const showSystem = $derived(ctx.manager && !ctx.kid);
	const shown = $derived(showSystem ? board : 'home');
	// Pictures for the big, tall and wide tiles: new seasons, arrivals, coming soon and follows, taken in turn, each title once. A picture that cannot load drops out, and the next one (or a count tile) takes its place.
	const badPic = new SvelteSet<number>();
	const badLogo = new SvelteSet<number>();
	const pics = $derived.by(() => {
		const lists: [HomeCard[], (c: HomeCard) => string][] = [
			[ctx.home?.new_seasons ?? [], () => t('home.newSeason')],
			// "Just arrived" only while it is: an older arrival says how old it is.
			[ctx.home?.arrived ?? [], (c) => arrivedTitle(c.at ? Date.parse(c.at) : undefined, ctx.now.getTime())],
			[ctx.home?.soon ?? [], () => t('home.comingSoon2')],
			[ctx.home?.following ?? [], () => t('home.following')]
		];
		const seen = new Set<number>();
		const out: { c: HomeCard; label: string }[] = [];
		for (let i = 0; out.length < 3 && lists.some(([l]) => i < l.length); i++) {
			for (const [l, label] of lists) {
				const c = l[i];
				if (c && (c.backdrop || c.poster) && !seen.has(c.title_id) && !badPic.has(c.title_id) && out.length < 3) {
					seen.add(c.title_id);
					out.push({ c, label: label(c) });
				}
			}
		}
		return out;
	});
	const showOverview = $derived(!ctx.rows.includes('welcome'));
	// The count tiles, in order. The first four are always there for the person they are for (Needs attention, asks, Library, disk), so they never change places while the page loads; the rest fill the picture places that have no picture. Explore, New & Popular and Search always can, so the grid never has a hole.
	const recentArrived = $derived((ctx.home?.arrived ?? []).filter((c) => c.at && ctx.now.getTime() - Date.parse(c.at) < 14 * 86400000));
	const recentNew = $derived(recentArrived.length + (ctx.home?.new_seasons?.length ?? 0));
	const statPool = $derived(
		[
			ctx.owner ? 'att' : '',
			ctx.canAsk ? 'ask' : '',
			ctx.libCards.length ? 'lib' : '',
			ctx.showDisk ? 'disk' : '',
			ctx.fetchingAll.length ? 'dl' : '',
			ctx.owner && ctx.home?.soon?.length && !pics.some((p) => p.label === t('home.comingSoon2')) ? 'soon' : '',
			recentNew ? 'new' : '',
			'explore',
			'popular',
			ctx.manager ? 'calendar' : '',
			'search'
		].filter(Boolean)
	);
	// The grid's shape for how many tiles there are: every shape fills whole rows at four columns and at two.
	const SHAPES: Record<number, string[]> = {
		7: ['big', '', '', 'tall', '', 'wide', 'fill'],
		6: ['big', '', '', 'wide', 'wide', 'wide'],
		5: ['big', 'wide', 'wide', 'wide', 'wide'],
		4: ['big', '', '', 'wide'],
		3: ['big', 'wide', 'wide'],
		2: ['big', 'big'],
		1: ['banner']
	};
	
	const places = $derived.by((): Place[] => {
		const shapes = SHAPES[Math.min(7, pics.length + statPool.length)] ?? [];
		const p = [...pics];
		const st = [...statPool];
		const out: Place[] = shapes.map((shape) => ({ key: '', shape }));
		const roomy = (shape: string) => shape === 'big' || shape === 'tall' || shape === 'wide' || shape === 'banner';
		// Pictures take the roomy places, the first count tiles take the small ones, and what is left fills the rest.
		for (const pl of out) if (roomy(pl.shape) && p.length) pl.pic = p.shift();
		for (const pl of out) if (!roomy(pl.shape) && st.length) pl.stat = st.shift();
		for (const pl of out) {
			if (!pl.pic && !pl.stat) {
				if (st.length) pl.stat = st.shift();
				else pl.pic = p.shift();
			}
			pl.key = pl.pic ? `p${pl.pic.c.title_id}` : `s${pl.stat}`;
		}
		return out;
	});
	const libTotal = $derived(ctx.libCards.reduce((t, c) => t + c.big, 0));
	// Under the tiles Home has one picture row (Recently added) and one list, Coming up. The other rows went: New season, Coming soon and Waiting for approval repeated a tile above; Your asks is the asks tile and the Requests page; what you follow shows up in the row and the list when something new happens. Coming up shows what airs next, one line each with its date; a new season that is already out leads, two at most.
	const onTiles = $derived(new Set(showOverview ? places.flatMap((pl) => (pl.pic ? [pl.pic.c.title_id] : [])) : []));
	const below = (cards: HomeCard[] | undefined) => (cards ?? []).filter((c) => !onTiles.has(c.title_id));
	
	const comingUp = $derived.by((): UpLine[] => {
		const today = localDay(ctx.now);
		const seen = new Set<number>(onTiles);
		const out: UpLine[] = [];
		const add = (c: HomeCard, max: number) => {
			if (seen.has(c.title_id) || out.length >= max) return;
			seen.add(c.title_id);
			const [sub, tail] = (c.line ?? '').split(' · ');
			const outNow = /out now/i.test(tail ?? '');
			const isToday = c.at?.slice(0, 10) === today;
			out.push({ key: c.key, c, sub: sub ?? '', when: outNow ? t('home.outNow') : isToday ? t('home.today') : (tail ?? ''), now: outNow || isToday });
		};
		for (const c of (ctx.home?.new_seasons ?? []).filter((x) => /out now/i.test(x.line ?? ''))) add(c, 2);
		for (const c of ctx.home?.soon ?? []) add(c, 6);
		for (const c of ctx.home?.new_seasons ?? []) add(c, 6);
		return out;
	});
	// The list sits where the first of the two rows it replaces was.
	const upAt = $derived(ctx.rows.find((r) => r === 'new_season' || r === 'soon'));
	const badThumb = new SvelteSet<string>();
	const kindIcon = (k: string): IconName => (k === 'movie' ? 'film' : k === 'artist' ? 'music' : 'tv');
	// The Library is never a mixed list: every link goes to one kind. A card's own kind when the link is about a card; for the Library total, the biggest part of it.
	const libHref = $derived([...ctx.libCards].sort((x, y) => y.big - x.big)[0]?.href ?? '/library?kind=movie');
	const kindHref = (c?: HomeCard) => (c ? `/library?kind=${c.kind}` : libHref);
	// An ask's picture. Its own poster first; when the ask was saved without one, or it cannot be shown, the picture the Requests page uses: the title's poster from this install's own details, or the artist's wide photo.
	const badArt = new SvelteSet<number>();
	const noArt = new SvelteSet<number>();
	function askPic(a: Ask): string | undefined {
		if (noArt.has(a.id)) return undefined;
		if (a.poster && !badArt.has(a.id)) return a.poster;
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] || undefined;
		wantDetails(a);
		return askDetails(a)?.poster || undefined;
	}
	function artFailed(a: Ask, pic: string) {
		if (pic === a.poster) badArt.add(a.id);
		else noArt.add(a.id);
	}
	// An artist's own photo can be an empty file: it is tried once here, so the row can fall back before it shows a gap.
	const tried = new Set<number>();
	$effect(() => {
		for (const a of ctx.waiting.slice(0, 20)) {
			if (a.kind !== 'artist' || !a.poster || tried.has(a.id)) continue;
			tried.add(a.id);
			const probe = new Image();
			probe.onerror = () => badArt.add(a.id);
			probe.src = a.poster;
		}
	});

	return {
		askPic,
		get showSystem() { return showSystem; },
		get showOverview() { return showOverview; },
		get board() { return board; },
		set board(v) { board = v; },
		get shown() { return shown; },
		get badPic() { return badPic; },
		get badLogo() { return badLogo; },
		artFailed,
		get libHref() { return libHref; },
		get libTotal() { return libTotal; },
		kindHref,
		get recentArrived() { return recentArrived; },
		get recentNew() { return recentNew; },
		get places() { return places; },
		below,
		get upAt() { return upAt; },
		get comingUp() { return comingUp; },
		get badThumb() { return badThumb; },
		kindIcon,
	};
}
