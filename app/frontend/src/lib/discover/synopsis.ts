// A short synopsis of whole sentences up to about `max` characters. A first sentence that is already too long is cut at a word and ends in an ellipsis, so no "Show more" is needed.
export function shortSynopsis(text: string | null | undefined, max = 160): string {
	const t = (text ?? '').replace(/\s+/g, ' ').trim();
	if (t.length <= max) return t;
	const sentences = t.match(/[^.!?]+[.!?]+["')\]]*\s*/g) ?? [t];
	let out = '';
	for (const s of sentences) {
		if ((out + s).trim().length > max) break;
		out += s;
	}
	out = out.trim();
	if (out) return out;
	const cut = t.slice(0, max);
	const at = cut.lastIndexOf(' ');
	return (at > max * 0.6 ? cut.slice(0, at) : cut).replace(/[\s,;:–-]+$/, '') + '…';
}
