// The server speaks English on the wire (pages still match some of its lines).
// serverWords shows a known server line in the picked language at display time;
// messages/en/server-lines.json is built by scripts/server-words.mjs.
import en from './messages/en/server-lines.json';
import { appLocale, lang, ltr, t } from './i18n.svelte';

const isPath = (s: string) => /^(\/|[A-Za-z]:[\\/])\S/.test(s);

// The list lines: a lead-in whose one gap holds several lines joined by "; " (lists), and the lines such a list is made of (items). In an item every gap is something a person typed (a Source or Downloader name, tags, a time), so it is shown as typed. These lines are matched by themselves, never as part of the other lines, so nothing else reads differently.
const lists: Record<string, string> = {
	'server.sources.no-source-takes-this-title-s': 'failures',
	'server.downloads.no-downloader-can-take-it-now': 'skipped',
	'server.sources.this-folder-only-takes-sources-tagged': 'sources'
};
const items = new Set([
	'server.sources.it-is-paused',
	'server.sources.is-turned-off',
	'server.sources.it-only-has-a-feed',
	'server.sources.search-is-turned-off-for-it',
	'server.downloads.is-resting-until-after-failing',
	'server.sources.its-tags-are-not-on-this',
	'server.sources.its-tags-are-not-in-the',
	'server.sources.it-has-no-tags-and-the'
]);

// The row titles and card badges the server puts together (suggest rows, Explore, card strips): their gaps must hold what
// the slot name says (a known genre, a number, a "Jan 2" date), so a plain English title such as "Popular now" is never half-matched.
const shaped = new Set([
	'server.suggest.because-you-asked-for', 'server.suggest.more-from', 'server.suggest.picks-for-you', 'server.suggest.popular', 'server.suggest.popular-s',
	'server.suggest.s', 'server.suggest.this-week-mostly', 'server.discover.trending', 'server.discover.best-new', 'server.discover.top-rated-classics',
	'server.discover.out', 'server.discover.added-d-ago', 'server.discover.season-2', 'server.discover.s-e', 'server.discover.season'
]);
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
		if (shaped.has(tp.key)) {
			if (!tp.slots.every((s, i) => (vars[s] = shapedSlot(s, m[i + 1])!) !== undefined)) continue;
			// A title starts with a capital even when a lower-case genre comes first in the picked language.
			const line = t(tp.key, vars);
			return line.charAt(0).toLocaleUpperCase(appLocale()) + line.slice(1);
		}
		// A path keeps its slashes in place inside a right-to-left line.
		tp.slots.forEach((s, i) => (vars[s] = isPath(m[i + 1]) ? ltr(m[i + 1]) : look(m[i + 1], depth + 1)));
		return t(tp.key, vars);
	}
	return text;
}

// One gap of a shaped line in the app's language, or undefined when it does not hold what its name says.
function shapedSlot(slot: string, v: string): string | undefined {
	if (slot === 'name') return v;
	if (slot === 'n' || slot === 'season' || slot === 'episode' || slot === 'decade') return number.test(v) ? v : undefined;
	if (slot === 'date' || slot === 'day') return shortDate.test(v) ? dayMonth(v) : slot === 'day' ? look(v, 2) : undefined;
	if (slot === 'genre') {
		const key = exact.get(v) ?? lower.get(v);
		if (!key) return undefined;
		const word = t(key);
		// English writes a genre in lower case inside a title; German keeps its nouns capitalised.
		return v === v.toLowerCase() && lang.value !== 'de' ? word.toLocaleLowerCase(appLocale()) : word;
	}
	return look(v, 2);
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
