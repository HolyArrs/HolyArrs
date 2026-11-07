// The one picture placeholder: an item's initials or
// short name on the picture's own main colour when its record is known
// (so the picture fades in over its own colour), else on a calm tint whose
// hue comes from the name.

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

/** inkOn is the initials' ink on a main colour (#rrggbb): dark on a light
 * colour, light on a dark one; '' for anything that is not a colour. */
export function inkOn(color: string | null | undefined): 'dark' | 'light' | '' {
	const m = color ? /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(color) : null;
	if (!m) return '';
	const [r, g, b] = m.slice(1).map((h) => {
		const c = parseInt(h, 16) / 255;
		return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	});
	return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.36 ? 'dark' : 'light';
}
