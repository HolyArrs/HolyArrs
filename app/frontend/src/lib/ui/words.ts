// Splits one translated line into plain text and marked parts, so a whole sentence stays one key.
// Markers: <b>…</b>, <strong>…</strong>, <em>…</em>, <code>…</code>, <mono>…</mono> (monospace), and links <a>…</a>, <a2>…</a2>, <a3>…</a3>.

export type WordPart = { tag: string | null; text: string };

const MARK = /<(b|strong|em|code|mono|a\d?)>([\s\S]*?)<\/\1>/g;

export function splitWords(line: string): WordPart[] {
	const out: WordPart[] = [];
	let at = 0;
	for (const m of line.matchAll(MARK)) {
		if (m.index > at) out.push({ tag: null, text: line.slice(at, m.index) });
		out.push({ tag: m[1], text: m[2] });
		at = m.index + m[0].length;
	}
	if (at < line.length) out.push({ tag: null, text: line.slice(at) });
	return out;
}

/** Fills {name} placeholders after splitting, so a value can never add markers. */
export function fillWords(text: string, vars?: Record<string, string | number>): string {
	return vars ? text.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m)) : text;
}
