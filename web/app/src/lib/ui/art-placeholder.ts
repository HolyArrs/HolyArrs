// The one picture placeholder (Luis, 2026-10-24): an item's initials or
// short name on a calm tint whose hue comes from the name.

/** placeholderText is up to two initials (letters or digits), or the name's
 * first characters when it has no words (a number like "1989"). */
export function placeholderText(name: string): string {
	const words = name
		.replace(/[([{].*$/, '')
		.split(/[\s\-_/&+.,:;!?]+/)
		.filter((w) => /[\p{L}\p{N}]/u.test(w));
	if (!words.length) return '';
	if (words.length === 1 && /^\p{N}+$/u.test(words[0])) return words[0].slice(0, 4);
	return words
		.slice(0, 2)
		.map((w) => [...w.replace(/^[^\p{L}\p{N}]+/u, '')][0]?.toLocaleUpperCase() ?? '')
		.join('');
}

/** placeholderHue is a stable hue (0 to 359) for a name. */
export function placeholderHue(name: string): number {
	let h = 2166136261;
	for (const ch of name.toLowerCase()) {
		h ^= ch.codePointAt(0) ?? 0;
		h = Math.imul(h, 16777619);
	}
	return (h >>> 0) % 360;
}
