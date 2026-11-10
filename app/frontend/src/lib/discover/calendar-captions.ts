// The two caption lines under every calendar card and list row, and the choices for them (?movie=1-6, ?music=1-5, ?channels=all|known).
import type { CalGroup } from './calendar-groups.ts';
import type { CalendarEntry } from '../core/types.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

export type Channels = 'all' | 'known';

/** Which caption each kind shows, and which TV channels are named. */
export interface CaptionLook {
	movie: number;
	music: number;
	channels: Channels;
	/** The viewer's country (US, GB), or 'global' when the browser does not say. */
	region: string;
}

/** A row's words in the set order. `title` is line 1 (show, movie or artist); `lines` sit under it, each only when it has real data; `when` is the time column: an air time ("8 PM"), or "All day". */
export interface RowWords {
	title: string;
	lines: string[];
	when: string;
	quiet: boolean;
}

export const DEFAULT_LOOK: Omit<CaptionLook, 'region'> = { movie: 1, music: 5, channels: 'known' };

/** Names that only repeat the number or the day ("Episode 8827", "19/10/2026", "TBA") say nothing. */
export function fakeEpisodeName(name?: string): boolean {
	const n = (name ?? '').trim();
	if (!n) return true;
	if (/^(episode|ep\.?|chapter|part)\s*#?\d+(\s*\/\s*\d+)?$/i.test(n) || /^\d+\s*\/\s*\d+$/.test(n) || /^(tba|tbd|untitled)$/i.test(n) || /^#?\d+$/.test(n)) return true;
	if (/^\d{1,4}[/.-]\d{1,2}[/.-]\d{1,4}$/.test(n)) return true;
	const months = 'jan(uary)?|feb(ruary)?|mar(ch)?|apr(il)?|may|june?|july?|aug(ust)?|sep(t(ember)?)?|oct(ober)?|nov(ember)?|dec(ember)?';
	const day = '(mon|tue|wed|thu|fri|sat|sun)[a-z]*,?\\s+';
	return new RegExp(`^(${day})?(\\d{1,2}(st|nd|rd|th)?\\s+(${months})\\.?,?(\\s+\\d{4})?|(${months})\\.?\\s+\\d{1,2}(st|nd|rd|th)?,?(\\s+\\d{4})?)$`, 'i').test(n);
}

/** An episode's air time in the viewer's own format: "7 PM", "7:30 PM", "19:00". */
export function clockTime(e: Pick<CalendarEntry, 'kind' | 'date' | 'timed'>, locale?: string): string {
	if (e.kind !== 'episode' || !e.timed) return '';
	const d = new Date(e.date);
	const f = new Intl.DateTimeFormat(locale ?? appLocale(), { hour: 'numeric', minute: '2-digit' });
	const h12 = /h1[12]/.test(f.resolvedOptions().hourCycle ?? '');
	// A 12-hour clock drops ":00" ("7 PM"); a 24-hour clock keeps it ("19:00").
	if (h12 && d.getMinutes() === 0) return new Intl.DateTimeFormat(locale ?? appLocale(), { hour: 'numeric' }).format(d);
	return f.format(d);
}

/** "Season 22, Episode 111"; several: the episode numbers joined by an en dash. */
export function episodeWords(eps: Pick<CalendarEntry, 'season' | 'episode'>[]): string {
	const s = [...eps].sort((a, b) => (a.season ?? 0) - (b.season ?? 0) || (a.episode ?? 0) - (b.episode ?? 0));
	const first = s[0];
	const last = s.at(-1)!;
	if (s.length === 1) return t('discover.calendarCaptions.seasonEpisode', { season: first.season ?? 0, episode: first.episode ?? 0 });
	if (first.season === last.season) return t('discover.calendarCaptions.seasonEpisodes', { season: first.season ?? 0, from: first.episode ?? 0, to: last.episode ?? 0 });
	return t('discover.calendarCaptions.seasonSpan', { season: first.season ?? 0, episode: first.episode ?? 0, season2: last.season ?? 0, episode2: last.episode ?? 0 });
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

/** With 'all' every channel shows. With 'known' only well-known streaming services and the channels of the viewer's own country (read from the browser's language, en-US to US) are named. */
export function channelShown(network: string | undefined, look: Pick<CaptionLook, 'channels' | 'region'>): boolean {
	if (!network) return false;
	if (look.channels === 'all') return true;
	const n = norm(network);
	if (STREAMING.includes(n)) return true;
	return (CHANNELS[look.region] ?? []).includes(n);
}

/** "2h 50m", "2h", "50m". */
export function runLength(min?: number): string {
	if (!min || min <= 0) return '';
	const h = Math.floor(min / 60);
	const m = min % 60;
	return h ? (m ? t('discover.calendarCaptions.hoursMinutes', { h, m }) : t('discover.calendarCaptions.hours', { h })) : t('discover.calendarCaptions.minutes', { m });
}

const shortDay = (iso?: string) => (iso ? new Date(iso.slice(0, 10) + 'T00:00:00').toLocaleDateString(appLocale(), { month: 'short', day: 'numeric' }) : '');
const day = (iso?: string) => (iso ? iso.slice(0, 10) : '');

/** "In cinemas", "In cinemas · At home from Mar 3", "At home", "At home · In cinemas Jul 17". */
export function whereOut(e: CalendarEntry): string {
	if (e.kind === 'cinema') return day(e.home_date) > day(e.date) ? t('discover.calendarCaptions.cinemaThenHome', { date: shortDay(e.home_date) }) : t('discover.calendarCaptions.inCinemas');
	return e.cinema_date && day(e.cinema_date) < day(e.date) ? t('discover.calendarCaptions.homeAfterCinema', { date: shortDay(e.cinema_date) }) : t('discover.calendarCaptions.atHome');
}

const NUMBERS: Record<string, number> = { one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, i: 1, ii: 2, iii: 3, iv: 4, v: 5, vi: 6, vii: 7, viii: 8, ix: 9, x: 10 };
const asNumber = (w: string) => (/^\d+$/.test(w) ? Number(w) : (NUMBERS[w.toLowerCase()] ?? 0));

/** 1st, 2nd, 3rd, 4th, 11th. */
export function ordinal(n: number): string {
	const h = n % 100;
	if (h >= 11 && h <= 13) return t('discover.calendarCaptions.ordinalTh', { n });
	const last = n % 10;
	return last === 1 ? t('discover.calendarCaptions.ordinalSt', { n }) : last === 2 ? t('discover.calendarCaptions.ordinalNd', { n }) : last === 3 ? t('discover.calendarCaptions.ordinalRd', { n }) : t('discover.calendarCaptions.ordinalTh', { n });
}

/** The series a movie belongs to, with its place when the name says it: "Dune · Part 3", "Toy Story · 4th film"; else "Avengers series"; '' outside a series. */
export function seriesLine(e: Pick<CalendarEntry, 'name' | 'collection'>): string {
	if (!e.collection) return '';
	const series = e.collection.replace(/\s+(collection|series|saga|trilogy)$/i, '').trim();
	const part = /\b(part|chapter|volume)\s+([a-z]+|\d+)\b/i.exec(e.name);
	if (part && asNumber(part[2])) return `${series} · ${cap(part[1].toLowerCase())} ${asNumber(part[2])}`;
	const tail = /\s(\d{1,2}|ii|iii|iv|v|vi|vii|viii|ix)(\s*[:\-–].*)?$/i.exec(e.name);
	if (tail && asNumber(tail[1]) > 1) return t('discover.calendarCaptions.nthFilm', { series, nth: ordinal(asNumber(tail[1])) });
	return t('discover.calendarCaptions.series', { series });
}

const people = (xs?: string[]) => (xs ?? []).slice(0, 2).join(' & ');

/** A movie's caption line for the choice picked (1-6). Another known fact stands in for a blank one, never the year. */
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

const kindOf = (type?: string) => {
	const k = (type ?? '').toLowerCase();
	return k === 'single' ? 'single' : k === 'ep' ? 'EP' : 'album';
};

const kindName = (k: string) => (k === 'single' ? t('discover.calendarCaptions.single') : k === 'EP' ? t('discover.calendarCaptions.ep') : t('discover.calendarCaptions.album'));
const cap = (s: string) => (s === 'EP' ? s : s.charAt(0).toUpperCase() + s.slice(1));
const songs = (n?: number) => (n ? (n === 1 ? t('discover.calendarCaptions.songOne') : t('discover.calendarCaptions.songOther', { n })) : '');

/** "44 min", "1 hr 12 min". */
export function albumLength(min?: number): string {
	if (!min || min <= 0) return '';
	if (min < 60) return t('discover.calendarCaptions.albumMinutes', { m: min });
	return min % 60 ? t('discover.calendarCaptions.albumHoursMinutes', { h: Math.floor(min / 60), m: min % 60 }) : t('discover.calendarCaptions.albumHours', { h: Math.floor(min / 60) });
}

/** An album's caption line for the choice picked (1-5). The label is not known yet, so 3 shows the songs. */
export function musicCaption(e: CalendarEntry, look: number): string {
	const kind = kindOf(e.album_type);
	const Kind = cap(kindName(kind));
	const n = songs(e.tracks);
	const kindSongs = [Kind, n].filter(Boolean).join(' · ');
	switch (look) {
		case 2:
			return [Kind, albumLength(e.length_min) || n].filter(Boolean).join(' · ');
		case 4: {
			const g = e.genres?.[0];
			return g ? [t('discover.calendarCaptions.genreKind', { genre: cap(g.toLowerCase()), kind: kindName(kind) }), n].filter(Boolean).join(' · ') : kindSongs;
		}
		case 5: {
			if (kind === 'single') return t('discover.calendarCaptions.newSingle');
			const which = !e.nth ? t('discover.calendarCaptions.newKind', { kind: kindName(kind) }) : e.nth === 1 ? t('discover.calendarCaptions.debutKind', { kind: kindName(kind) }) : t('discover.calendarCaptions.nthKind', { nth: ordinal(e.nth), kind: kindName(kind) });
			return [which, n].filter(Boolean).join(' · ');
		}
		default:
			return kindSongs;
	}
}

/** Builds a List row, or a Cards caption (see cardLines), for one card's dates. */
export function rowWords(g: CalGroup, look: CaptionLook, today: string): RowWords {
	const e = g.lead;
	if (e.kind === 'episode') {
		const name = g.entries.length === 1 && !fakeEpisodeName(e.episode_name) ? (e.episode_name ?? '') : '';
		const chan = channelShown(e.network, look) ? (e.network ?? '') : '';
		const at = clockTime(e);
		return { title: e.name, lines: [name, episodeWords(g.entries), chan].filter(Boolean), when: at || t('discover.calendarCaptions.allDay'), quiet: !at };
	}
	if (e.kind === 'album') {
		const pick = g.entries.length > 1 ? t('discover.calendarCaptions.releases', { n: g.entries.length }) : musicCaption(e, look.music);
		return { title: e.name, lines: [e.album ?? '', pick].filter(Boolean), when: cap(kindName(kindOf(e.album_type))), quiet: false };
	}
	const pick = movieCaption(e, look.movie);
	const genre = e.genres?.[0] ?? '';
	const lines = [pick, pick.includes(genre) ? '' : genre, e.studio ?? ''].filter(Boolean);
	return { title: e.name, lines, when: e.kind === 'cinema' ? t('discover.calendarCaptions.inTheaters') : t('discover.calendarCaptions.streaming'), quiet: false };
}

/** The captions under a card in the same order: the time or short word first, then the lines. The art carries the name, so an album's card names the artist in place of its own name. */
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
