// The server speaks English on the wire (pages still match some of its lines).
// serverWords shows a known server line in the picked language at display time;
// messages/en/server-lines.json is built by scripts/server-words.mjs.
import en from './messages/en/server-lines.json';
import { lang, t } from './i18n.svelte';

const exact = new Map<string, string>();
const templates: { key: string; re: RegExp; slots: string[] }[] = [];
for (const [key, line] of Object.entries(en as Record<string, string>)) {
	if (!/\{\w+\}/.test(line)) {
		if (!exact.has(line)) exact.set(line, key);
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
	templates.push({ key, re: new RegExp(`^${re}$`, 's'), slots });
}
// Longer fixed text first, so the most specific line wins.
const fixed = (r: RegExp) => r.source.replace(/\(\.\+\?\)/g, '').length;
templates.sort((a, b) => fixed(b.re) - fixed(a.re));

/** A server line in the app's language; unknown text (and null or undefined) comes back unchanged. */
export function serverWords<T extends string | null | undefined>(text: T): T;
export function serverWords(text: string | null | undefined): string | null | undefined {
	if (!text) return text;
	if (lang.value === 'en') return text;
	if (cache.lang !== lang.words) cache.lang = lang.words, cache.lines.clear();
	let line = cache.lines.get(text);
	if (line === undefined) cache.lines.set(text, (line = look(text, 0)));
	return line;
}

const cache = { lang: null as Record<string, string> | null, lines: new Map<string, string>() };

function look(text: string, depth: number): string {
	const key = exact.get(text);
	if (key) return t(key);
	if (depth > 1) return text;
	for (const tp of templates) {
		const m = tp.re.exec(text);
		if (!m) continue;
		const vars: Record<string, string> = {};
		tp.slots.forEach((s, i) => (vars[s] = look(m[i + 1], depth + 1)));
		return t(tp.key, vars);
	}
	return text;
}
