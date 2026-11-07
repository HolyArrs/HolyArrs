// The two caption lines under every calendar card and list row, and the
// choices for them (?movie=1-6, ?music=1-5, ?channels=all|known).
import type { CalendarEntry } from '../core/types.ts';
import type { CalGroup } from './calendar-groups.ts';

export type Channels = 'all' | 'known';
/** Which caption each kind shows: movie 1-6, music 1-5, and which TV channels are named. */
export interface CaptionLook {
	movie: number;
	music: number;
	channels: Channels;
	/** The viewer's country (US, GB…), or 'global' when the browser does not say. */
	region: string;
}

export const MOVIE_LOOKS = [
	{ value: 1, label: 'Genre · length' },
	{ value: 2, label: 'Director' },
	{ value: 3, label: 'Where it is out' },
	{ value: 4, label: 'Star · genre' },
	{ value: 5, label: 'Part of a set' },
	{ value: 6, label: 'Director · length' }
];
export const MUSIC_LOOKS = [
	{ value: 1, label: 'Kind · songs' },
	{ value: 2, label: 'Kind · length' },
	{ value: 3, label: 'Kind · label' },
	{ value: 4, label: 'Genre · songs' },
	{ value: 5, label: 'Which release' }
];
export const DEFAULT_LOOK: Omit<CaptionLook, 'region'> = { movie: 1, music: 5, channels: 'known' };

/** lookFrom reads the choices from the address, then from what was kept, else the defaults. */
export function lookFrom(q: Pick<URLSearchParams, 'get'>, kept: Partial<CaptionLook> | null, region: string): CaptionLook {
	const num = (v: string | null, max: number) => {
		const n = Number(v);
		return v && Number.isInteger(n) && n >= 1 && n <= max ? n : null;
	};
	const ch = (v: string | null | undefined): Channels | null => (v === 'all' || v === 'known' ? v : null);
	return {
		movie: num(q.get('movie'), 6) ?? num(String(kept?.movie ?? ''), 6) ?? DEFAULT_LOOK.movie,
		music: num(q.get('music'), 5) ?? num(String(kept?.music ?? ''), 5) ?? DEFAULT_LOOK.music,
		channels: ch(q.get('channels')) ?? ch(kept?.channels) ?? DEFAULT_LOOK.channels,
		region
	};
}

// ---- Episodes ----

/**
 * fakeEpisodeName: names that only repeat the number or the day ("Episode 8827", "Episode 170 / 8805",
 * "19/10/2026", "2026-10-19", "October 19, 2026", "TBA") say nothing.
 */
export function fakeEpisodeName(name?: string): boolean {
	const n = (name ?? '').trim();
	if (!n) return true;
	if (/^(episode|ep\.?|chapter|part)\s*#?\d+(\s*\/\s*\d+)?$/i.test(n) || /^\d+\s*\/\s*\d+$/.test(n) || /^(tba|tbd|untitled)$/i.test(n) || /^#?\d+$/.test(n)) return true;
	if (/^\d{1,4}[/.-]\d{1,2}[/.-]\d{1,4}$/.test(n)) return true;
	const months = 'jan(uary)?|feb(ruary)?|mar(ch)?|apr(il)?|may|june?|july?|aug(ust)?|sep(t(ember)?)?|oct(ober)?|nov(ember)?|dec(ember)?';
	const day = '(mon|tue|wed|thu|fri|sat|sun)[a-z]*,?\\s+';
	return new RegExp(`^(${day})?(\\d{1,2}(st|nd|rd|th)?\\s+(${months})\\.?,?(\\s+\\d{4})?|(${months})\\.?\\s+\\d{1,2}(st|nd|rd|th)?,?(\\s+\\d{4})?)$`, 'i').test(n);
}

/** clockTime is an episode's air time in the viewer's own format: "7 PM", "7:30 PM", "19:00". */
export function clockTime(e: Pick<CalendarEntry, 'kind' | 'date' | 'timed'>, locale?: string): string {
	if (e.kind !== 'episode' || !e.timed) return '';
	const d = new Date(e.date);
	const f = new Intl.DateTimeFormat(locale, { hour: 'numeric', minute: '2-digit' });
	const h12 = /h1[12]/.test(f.resolvedOptions().hourCycle ?? '');
	// A 12-hour clock drops ":00" ("7 PM"); a 24-hour clock keeps it ("19:00").
	if (h12 && d.getMinutes() === 0) return new Intl.DateTimeFormat(locale, { hour: 'numeric' }).format(d);
	return f.format(d);
}

/** episodeWords: "Season 22, Episode 111"; several: "Season 39, Episodes 194–196". */
export function episodeWords(eps: Pick<CalendarEntry, 'season' | 'episode'>[]): string {
	const s = [...eps].sort((a, b) => (a.season ?? 0) - (b.season ?? 0) || (a.episode ?? 0) - (b.episode ?? 0));
	const first = s[0];
	const last = s.at(-1)!;
	if (s.length === 1) return `Season ${first.season ?? 0}, Episode ${first.episode ?? 0}`;
	if (first.season === last.season) return `Season ${first.season ?? 0}, Episodes ${first.episode ?? 0}–${last.episode ?? 0}`;
	return `Season ${first.season ?? 0}, Episode ${first.episode ?? 0} – Season ${last.season ?? 0}, Episode ${last.episode ?? 0}`;
}

const STREAMING = [
	'netflix', 'disney+', 'apple tv+', 'apple tv', 'prime video', 'amazon', 'amazon prime video', 'max', 'hbo max', 'hulu', 'paramount+',
	'paramount+ with showtime', 'peacock', 'youtube', 'youtube premium', 'crunchyroll', 'starz', 'mgm+', 'amc+', 'bet+', 'britbox', 'shudder', 'tubi', 'roku channel', 'the roku channel'
];
/** The best-known channels of a country; a viewer's own country's channels are named. */
const CHANNELS: Record<string, string[]> = {
	US: ['abc', 'cbs', 'nbc', 'fox', 'the cw', 'pbs', 'hbo', 'amc', 'fx', 'fxx', 'showtime', 'usa network', 'tnt', 'tbs', 'comedy central', 'adult swim', 'cartoon network', 'nickelodeon', 'disney channel', 'bravo', 'syfy', 'a&e', 'history', 'discovery', 'national geographic', 'mtv', 'freeform', 'hallmark channel', 'lifetime', 'bet', 'e!', 'espn', 'cnn', 'food network', 'hgtv', 'tlc', 'cinemax', 'epix', 'ifc', 'sundancetv', 'bbc america'],
	GB: ['bbc one', 'bbc two', 'bbc three', 'bbc four', 'bbc iplayer', 'itv', 'itv1', 'itv2', 'itvx', 'channel 4', 'e4', 'channel 5', 'sky one', 'sky atlantic', 'sky max', 'sky showcase', 'dave', 'gold', 'u&dave', 'all 4'],
	IE: ['rté one', 'rte one', 'rté2', 'virgin media one', 'tg4'],
	AU: ['abc tv', 'abc (au)', 'abc', 'seven network', 'nine network', 'network 10', 'sbs', 'stan', 'foxtel', 'binge', 'showcase'],
	NZ: ['tvnz 1', 'tvnz 2', 'three', 'neon', 'sky'],
	CA: ['cbc', 'ctv', 'global', 'citytv', 'crave', 'showcase', 'aptn'],
	ZA: ['m-net', 'sabc 1', 'sabc 2', 'sabc 3', 'e.tv', 'showmax', 'kyknet', 'mzansi magic'],
	DE: ['das erste', 'ard', 'zdf', 'rtl', 'prosieben', 'sat.1', 'vox', 'kabel eins', 'sky deutschland', 'joyn', 'rtl+', 'zdfneo', 'arte'],
	AT: ['orf 1', 'orf eins', 'orf 2', 'servustv'],
	FR: ['tf1', 'france 2', 'france 3', 'canal+', 'm6', 'arte', 'france 5'],
	NL: ['npo 1', 'npo 2', 'npo 3', 'rtl 4', 'sbs6', 'videoland'],
	BE: ['één', 'vtm', 'la une', 'rtl-tvi'],
	ES: ['la 1', 'antena 3', 'telecinco', 'cuatro', 'la sexta', 'movistar plus+', 'atresplayer'],
	IT: ['rai 1', 'rai 2', 'rai 3', 'canale 5', 'italia 1', 'sky italia', 'la7'],
	SE: ['svt1', 'svt2', 'tv4', 'kanal 5', 'viaplay'],
	NO: ['nrk1', 'tv 2', 'tvnorge', 'viaplay'],
	DK: ['dr1', 'tv 2', 'viaplay'],
	JP: ['nhk', 'tokyo mx', 'fuji tv', 'tbs', 'tv tokyo', 'nippon tv', 'tv asahi', 'mbs'],
	KR: ['kbs2', 'mbc', 'sbs', 'tvn', 'jtbc', 'ena'],
	IN: ['sony liv', 'zee5', 'jiocinema', 'star plus', 'colors tv']
};
const norm = (s: string) => s.trim().toLowerCase();

/**
 * channelShown: Every channel shows as today with 'all'. With 'known', only the
 * well-known streaming services and the channels of the viewer's own country
 * (read from the browser's language, en-US → US) are named; a viewer whose
 * browser names no country sees the streaming services only.
 */
export function channelShown(network: string | undefined, look: Pick<CaptionLook, 'channels' | 'region'>): boolean {
	if (!network) return false;
	if (look.channels === 'all') return true;
	const n = norm(network);
	if (STREAMING.includes(n)) return true;
	return (CHANNELS[look.region] ?? []).includes(n);
}

// ---- Movies ----

/** runLength: "2h 50m", "2h", "50m". */
export function runLength(min?: number): string {
	if (!min || min <= 0) return '';
	const h = Math.floor(min / 60);
	const m = min % 60;
	return h ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`;
}

const shortDay = (iso?: string) => (iso ? new Date(iso.slice(0, 10) + 'T00:00:00').toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '');
const day = (iso?: string) => (iso ? iso.slice(0, 10) : '');

/** whereOut: "In cinemas", "In cinemas · At home from Mar 3", "At home", "At home · In cinemas Jul 17". */
export function whereOut(e: CalendarEntry): string {
	if (e.kind === 'cinema') return day(e.home_date) > day(e.date) ? `In cinemas · At home from ${shortDay(e.home_date)}` : 'In cinemas';
	return e.cinema_date && day(e.cinema_date) < day(e.date) ? `At home · In cinemas ${shortDay(e.cinema_date)}` : 'At home';
}

const NUMBERS: Record<string, number> = { one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, i: 1, ii: 2, iii: 3, iv: 4, v: 5, vi: 6, vii: 7, viii: 8, ix: 9, x: 10 };
const asNumber = (w: string) => (/^\d+$/.test(w) ? Number(w) : (NUMBERS[w.toLowerCase()] ?? 0));

/** ordinal: 1st, 2nd, 3rd, 4th, 11th… */
export function ordinal(n: number): string {
	const t = n % 100;
	if (t >= 11 && t <= 13) return `${n}th`;
	return `${n}${{ 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] ?? 'th'}`;
}

/**
 * seriesLine: the series a movie belongs to, and its place when the name says it:
 * "Dune · Part 3", "Toy Story · 4th film"; else "Avengers series"; '' outside a series.
 */
export function seriesLine(e: Pick<CalendarEntry, 'name' | 'collection'>): string {
	if (!e.collection) return '';
	const series = e.collection.replace(/\s+(collection|series|saga|trilogy)$/i, '').trim();
	const part = /\b(part|chapter|volume)\s+([a-z]+|\d+)\b/i.exec(e.name);
	if (part && asNumber(part[2])) return `${series} · ${cap(part[1].toLowerCase())} ${asNumber(part[2])}`;
	const tail = /\s(\d{1,2}|ii|iii|iv|v|vi|vii|viii|ix)(\s*[:\-–].*)?$/i.exec(e.name);
	if (tail && asNumber(tail[1]) > 1) return `${series} · ${ordinal(asNumber(tail[1]))} film`;
	return `${series} series`;
}

const people = (xs?: string[]) => (xs ?? []).slice(0, 2).join(' & ');

/** movieCaption is a movie's caption line for the choice picked (1-6); another known fact stands in for a blank one, never the year. */
export function movieCaption(e: CalendarEntry, look: number): string {
	const genre = e.genres?.[0] ?? '';
	const len = runLength(e.runtime);
	const genreLen = [genre, len].filter(Boolean).join(' · ');
	const director = people(e.directors);
	const pick: Record<number, string> = {
		1: genreLen,
		2: director,
		3: whereOut(e),
		4: e.cast?.[0] ? [e.cast[0], genre].filter(Boolean).join(' · ') : '',
		5: seriesLine(e),
		6: director ? [director, len].filter(Boolean).join(' · ') : ''
	};
	return pick[look] || genreLen || director;
}

// ---- Music ----

const kindOf = (t?: string) => {
	const k = (t ?? '').toLowerCase();
	return k === 'single' ? 'single' : k === 'ep' ? 'EP' : 'album';
};
const cap = (s: string) => (s === 'EP' ? s : s.charAt(0).toUpperCase() + s.slice(1));
const songs = (n?: number) => (n ? `${n} ${n === 1 ? 'song' : 'songs'}` : '');
/** albumLength: "44 min", "1 hr 12 min". */
export function albumLength(min?: number): string {
	if (!min || min <= 0) return '';
	return min < 60 ? `${min} min` : `${Math.floor(min / 60)} hr${min % 60 ? ` ${min % 60} min` : ''}`;
}

/** musicCaption is an album's caption line for the choice picked (1-5). The label is not known yet: 3 shows the songs. */
export function musicCaption(e: CalendarEntry, look: number): string {
	const kind = kindOf(e.album_type);
	const Kind = cap(kind);
	const n = songs(e.tracks);
	const kindSongs = [Kind, n].filter(Boolean).join(' · ');
	switch (look) {
		case 2:
			return [Kind, albumLength(e.length_min) || n].filter(Boolean).join(' · ');
		case 4: {
			const g = e.genres?.[0];
			return g ? [`${cap(g.toLowerCase())} ${kind === 'EP' ? 'EP' : kind}`, n].filter(Boolean).join(' · ') : kindSongs;
		}
		case 5: {
			if (kind === 'single') return 'New single';
			const which = !e.nth ? `New ${kind}` : e.nth === 1 ? `Debut ${kind}` : `${ordinal(e.nth)} ${kind}`;
			return [which, n].filter(Boolean).join(' · ');
		}
		default:
			return kindSongs;
	}
}

/**
 * A row's words, in the set order. `title` is line 1 (the show, the movie, the
 * artist); `lines` are the lines under it, each only when it has real data;
 * `when` is the time column: an air time ("8 PM"), "All day" when an episode
 * has no time (`quiet`), or for movies and albums Cinema, At home or Out now
 * (an album still to come says its kind: Album, EP, Single).
 */
export interface RowWords {
	title: string;
	lines: string[];
	when: string;
	quiet: boolean;
}

/** rowWords builds a List row (or a Cards caption, see cardLines) for a card's worth of dates. */
export function rowWords(g: CalGroup, look: CaptionLook, today: string): RowWords {
	const e = g.lead;
	if (e.kind === 'episode') {
		const name = g.entries.length === 1 && !fakeEpisodeName(e.episode_name) ? (e.episode_name ?? '') : '';
		const chan = channelShown(e.network, look) ? (e.network ?? '') : '';
		const at = clockTime(e);
		return { title: e.name, lines: [name, episodeWords(g.entries), chan].filter(Boolean), when: at || 'All day', quiet: !at };
	}
	if (e.kind === 'album') {
		const pick = g.entries.length > 1 ? `${g.entries.length} releases` : musicCaption(e, look.music);
		return { title: e.name, lines: [e.album ?? '', pick].filter(Boolean), when: cap(kindOf(e.album_type)), quiet: false };
	}
	const pick = movieCaption(e, look.movie);
	const genre = e.genres?.[0] ?? '';
	const lines = [pick, pick.includes(genre) ? '' : genre, e.studio ?? ''].filter(Boolean);
	return { title: e.name, lines, when: e.kind === 'cinema' ? 'In theaters' : 'Streaming', quiet: false };
}

/**
 * cardLines are the captions under a card, in the same order: the time or the
 * short word first, then the lines. The art carries the show's, movie's or
 * album's name, so an album's card names the artist in place of its own name.
 */
export function cardLines(g: CalGroup, look: CaptionLook, today: string): string[] {
	const w = rowWords(g, look, today);
	// An episode card: the time, then one line "Season 4, Episode 3 · Its name" (two lines on every card, all alike).
	if (g.lead.kind === 'episode') {
		const e = g.lead;
		const name = g.entries.length === 1 && !fakeEpisodeName(e.episode_name) ? (e.episode_name ?? '') : '';
		return [w.when, [episodeWords(g.entries), name].filter(Boolean).join(' · ')];
	}
	return [w.when, ...(g.lead.kind === 'album' ? [w.title, ...w.lines.slice(1)] : w.lines)];
}
