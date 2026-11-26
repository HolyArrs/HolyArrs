// The server speaks English on the wire (pages still match some of its lines).
// serverWords shows a known server line in the picked language at display time;
// messages/en/server-lines.json is built by scripts/server-words.mjs.
import en from './messages/en/server-lines.json';
import enDownloads from './messages/en/downloads.json';
import { appLocale, lang, ltr, t } from './i18n.svelte';

const isPath = (s: string) => /^(\/|[A-Za-z]:[\\/])\S/.test(s);

// The list lines: a lead-in whose one gap holds several lines joined by "; " (lists), and the lines such a list is made of (items). In an item every gap is something a person typed (a Source or Downloader name, tags, a time), so it is shown as typed. These lines are matched by themselves, never as part of the other lines, so nothing else reads differently.
const lists: Record<string, string> = {
	'server.sources.no-source-takes-this-title-s': 'failures',
	'server.downloads.no-downloader-can-take-it-now': 'skipped',
	'server.sources.this-folder-only-takes-sources-tagged': 'sources',
	// An imported Alert's settings HolyArrs has no place for (migrate alertLeft).
	'server.migrate.holyarrs-has-no-place-for': 'left'
};
const items = new Set([
	'server.sources.it-is-paused',
	'server.sources.is-turned-off',
	'server.sources.it-only-has-a-feed',
	'server.sources.search-is-turned-off-for-it',
	'server.downloads.is-resting-until-after-failing',
	'server.sources.its-tags-are-not-on-this',
	'server.sources.its-tags-are-not-in-the',
	'server.sources.it-has-no-tags-and-the',
	'server.migrate.the-user-name-holyarrs-signs-in'
]);

// The row titles and card badges the server puts together (suggest rows, Explore, card strips): their gaps must hold what
// the slot name says (a known genre, a number, a "Jan 2" date), so a plain English title such as "Popular now" is never half-matched.
const shaped = new Set([
	'server.suggest.because-you-asked-for', 'server.suggest.more-from', 'server.suggest.picks-for-you', 'server.suggest.popular', 'server.suggest.popular-s',
	'server.suggest.s', 'server.suggest.this-week-mostly', 'server.discover.trending', 'server.discover.best-new', 'server.discover.top-rated-classics',
	'server.discover.out', 'server.discover.added-d-ago', 'server.discover.season-2', 'server.discover.s-e', 'server.discover.season',
	// A Release's points lines: their gaps are numbers, so no other line ending in "points" is half-matched.
	'server.mediafiles.points-2', 'server.mediafiles.points-needs-at-least', 'server.mediafiles.points-needs-at-least-2',
	'server.mediafiles.below-good-enough-which-needs-points',
	// A torrent's seeders line ("1 seeder, at least 1"): its gaps are numbers too.
	'server.mediafiles.seeder-at-least', 'server.mediafiles.seeder-needs-at-least', 'server.mediafiles.seeders-at-least', 'server.mediafiles.seeders-needs-at-least'
]);
// A ladder step as the Pick rules page names it ("4K Blu-ray (2160p)"), the {step} gap of a Release's reasons (mediafiles StepWord).
const steps = new Map(Object.entries(enDownloads as Record<string, string>).filter(([k]) => k.startsWith('downloads.pr.steps.')).map(([k, v]) => [v, k]));
// The rotating row titles (suggest/titles, music/rowtpl) are shaped lines too.
const rotating = /^server\.(titles|musictitles)\./;
// What a name gap holds: a seed title, a person, a service, an artist, typed as the server wrote it.
const names = new Set(['name', 'name_of_seed', 'person', 'title', 'service', 'artist', 'artist2', 'artists']);
const inCountry = Object.entries(en as Record<string, string>).find(([k, v]) => v === 'in {country}' && rotating.test(k))?.[0];
const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const shortDate = /^(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) (\d{1,2})$/;
const number = /^\d+$/;

type Template = { key: string; re: RegExp; slots: string[] };
const exact = new Map<string, string>();
// The same lines by their lower-case text: a genre inside a title is lower case ("Popular drama").
const lower = new Map<string, string>();
const templates: Template[] = [];
const listLines: Template[] = [];
const itemLines: Template[] = [];
for (const [key, line] of Object.entries(en as Record<string, string>)) {
	if (!/\{\w+\}/.test(line)) {
		if (!exact.has(line)) exact.set(line, key);
		if (!lower.has(line.toLowerCase())) lower.set(line.toLowerCase(), key);
		continue;
	}
	const slots: string[] = [];
	const re = line
		.split(/(\{\w+\})/)
		.map((p) => {
			const m = p.match(/^\{(\w+)\}$/);
			if (!m) return p.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
			slots.push(m[1]);
			return '(.+?)';
		})
		.join('');
	(key in lists ? listLines : items.has(key) ? itemLines : templates).push({ key, re: new RegExp(`^${re}$`, 's'), slots });
}
// Longer fixed text first, so the most specific line wins.
const fixed = (r: RegExp) => r.source.replace(/\(\.\+\?\)/g, '').length;
for (const list of [templates, listLines, itemLines]) list.sort((a, b) => fixed(b.re) - fixed(a.re));

/** A server line in the app's language; unknown text (and null or undefined) comes back unchanged. */
export function serverWords<T extends string | null | undefined>(text: T): T;
export function serverWords(text: string | null | undefined): string | null | undefined {
	if (!text) return text;
	if (lang.value === 'en') return text;
	if (cache.lang !== lang.words) cache.lang = lang.words, cache.lines.clear();
	let line = cache.lines.get(text);
	if (line === undefined) cache.lines.set(text, (line = lookCapped(text)));
	return line;
}

const cache = { lang: null as Record<string, string> | null, lines: new Map<string, string>() };

// A History row starts its sentence with a capital letter; the stored line starts in lower case.
function lookCapped(text: string): string {
	const line = look(text, 0);
	if (line !== text || !/^\p{Lu}/u.test(text)) return line;
	const low = look(text[0].toLowerCase() + text.slice(1), 0);
	return low === text[0].toLowerCase() + text.slice(1) ? text : low[0].toUpperCase() + low.slice(1);
}

/** One line of a list ("Nyaa is paused") in the app's language, its gaps as typed; unknown text comes back unchanged. */
export function listItem(text: string): string {
	for (const tp of itemLines) {
		const m = tp.re.exec(text);
		if (!m) continue;
		const vars: Record<string, string> = {};
		tp.slots.forEach((s, i) => (vars[s] = m[i + 1]));
		return t(tp.key, vars);
	}
	return exact.has(text) ? t(exact.get(text)!) : text;
}

// A list line: its list is shown line by line, its other gaps (tags, a protocol) as typed.
function listLine(text: string): string | null {
	for (const tp of listLines) {
		const m = tp.re.exec(text);
		if (!m) continue;
		const vars: Record<string, string> = {};
		tp.slots.forEach((s, i) => (vars[s] = s === lists[tp.key] ? m[i + 1].split('; ').map(listItem).join('; ') : m[i + 1]));
		return t(tp.key, vars);
	}
	return null;
}

function look(text: string, depth: number): string {
	const key = exact.get(text);
	if (key) return t(key);
	if (depth > 1) return text;
	const list = listLine(text);
	if (list !== null) return list;
	for (const tp of templates) {
		const m = tp.re.exec(text);
		if (!m) continue;
		const vars: Record<string, string> = {};
		if (shaped.has(tp.key) || rotating.test(tp.key)) {
			if (!tp.slots.every((s, i) => (vars[s] = shapedSlot(s, m[i + 1])!) !== undefined)) continue;
			// A title starts with a capital even when a lower-case genre comes first in the picked language.
			const line = t(tp.key, vars);
			return line.charAt(0).toLocaleUpperCase(appLocale()) + line.slice(1);
		}
		// A path keeps its slashes in place inside a right-to-left line.
		tp.slots.forEach((s, i) => {
			const v = m[i + 1];
			const step = /^step\d?$/.test(s) ? steps.get(v) : undefined;
			vars[s] = step ? t(step) : isPath(v) ? ltr(v) : look(v, depth + 1);
		});
		return t(tp.key, vars);
	}
	return text;
}

// One gap of a shaped line in the app's language, or undefined when it does not hold what its name says.
function shapedSlot(slot: string, v: string): string | undefined {
	if (names.has(slot)) return v;
	if (slot === 'n' || slot === 'min' || slot === 'season' || slot === 'episode' || slot === 'decade' || slot === 'count' || slot === 'year') return number.test(v) ? v : undefined;
	if (slot === 'day' && WEEKDAYS.includes(v)) return weekday(v);
	if (slot === 'date' || slot === 'day') return shortDate.test(v) ? dayMonth(v) : slot === 'day' ? look(v, 2) : undefined;
	if (slot === 'country') return country(v);
	// A music style ("Dance", "K-pop") reads as written unless it is a known genre: one to three words, no sentence marks.
	if (/^style\d?$/.test(slot)) {
		if (exact.has(v) || lower.has(v.toLowerCase())) return shapedSlot('genre', v);
		return /^[\p{L}\p{N}&'’+-]+(?: [\p{L}\p{N}&'’+-]+){0,2}$/u.test(v) ? v : undefined;
	}
	// A music place is "worldwide" or "in <country>" (rowtpl place).
	if (slot === 'place' || slot === 'neighbor') {
		if (v === 'worldwide') return look(v, 2);
		const c = v.startsWith('in ') ? country(v.slice(3)) : undefined;
		return c && inCountry ? t(inCountry, { country: c }) : undefined;
	}
	if (slot === 'genre' || slot === 'genre2') {
		const key = exact.get(v) ?? lower.get(v.toLowerCase());
		if (!key) return undefined;
		const word = t(key);
		// English writes a genre in lower case inside a title; German keeps its nouns capitalised.
		return v === v.toLowerCase() && lang.value !== 'de' ? word.toLocaleLowerCase(appLocale()) : word;
	}
	// Any other gap (a mood, a season, a place) must be a known line, so a title is never half English.
	if (!exact.has(v) && lower.has(v.toLowerCase())) return shapedSlot('genre', v);
	const line = look(v, 2);
	return line !== v || exact.has(v) ? line : undefined;
}

// "Friday" as the app's language writes a weekday (1 January 2023 was a Sunday).
function weekday(v: string): string {
	return new Intl.DateTimeFormat(appLocale(), { weekday: 'long' }).format(new Date(2023, 0, 1 + WEEKDAYS.indexOf(v)));
}

// A country's English name ("Germany", as titles write it) in the app's language, or undefined when it is not one.
let countries: Map<string, string> | undefined;
function country(v: string): string | undefined {
	if (!countries) {
		countries = new Map();
		const en = new Intl.DisplayNames('en', { type: 'region' });
		for (let a = 65; a <= 90; a++)
			for (let b = 65; b <= 90; b++) {
				const code = String.fromCharCode(a, b);
				const name = en.of(code);
				if (name && name !== code && !countries.has(name)) countries.set(name, code);
			}
	}
	// The server writes a few names its own way ("the US", "the Netherlands", "Turkey").
	const code = countries.get(v) ?? countries.get(v.replace(/^the /, '')) ?? { US: 'US', UK: 'GB', Turkey: 'TR' }[v.replace(/^the /, '')];
	return code ? new Intl.DisplayNames(appLocale() ?? 'en', { type: 'region' }).of(code) : undefined;
}

// "Nov 11" as the app's language writes a day and month. The year is the one that puts the day nearest today.
function dayMonth(v: string): string {
	const [, mon, day] = shortDate.exec(v)!;
	const now = new Date();
	let at = new Date(now.getFullYear(), MONTHS.indexOf(mon), Number(day));
	const half = 182 * 864e5;
	if (at.getTime() - now.getTime() > half) at = new Date(now.getFullYear() - 1, at.getMonth(), at.getDate());
	else if (now.getTime() - at.getTime() > half) at = new Date(now.getFullYear() + 1, at.getMonth(), at.getDate());
	return new Intl.DateTimeFormat(appLocale(), { month: 'short', day: 'numeric' }).format(at);
}
