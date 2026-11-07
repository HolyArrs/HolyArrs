// Letters, hue and text colour for the fallback art shown when a title has no picture.


// Up to two initials; a lone number is kept whole so years stay readable.
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

// FNV-1a hash so the same name always gets the same colour.
export function placeholderHue(name: string): number {
	let h = 2166136261;
	for (const ch of name.toLowerCase()) {
		h ^= ch.codePointAt(0) ?? 0;
		h = Math.imul(h, 16777619);
	}
	return (h >>> 0) % 360;
}

// Picks dark or light text by luminance; 0.36 is where contrast with white starts to lose.
export function inkOn(color: string | null | undefined): 'dark' | 'light' | '' {
	const m = color ? /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(color) : null;
	if (!m) return '';
	const [r, g, b] = m.slice(1).map((h) => {
		const c = parseInt(h, 16) / 255;
		return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	});
	return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.36 ? 'dark' : 'light';
}
