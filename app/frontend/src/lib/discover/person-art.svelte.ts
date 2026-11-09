// A person page's art: a film shows only once its picture has loaded, and is dropped when it will not.
import { type PersonMovie } from '#lib/ui/people.ts';

export function createPersonArt() {
	// A credit shows only with a real picture (poster or backdrop). Pictures that fail or hang are dropped too.
	const ART_WAIT_MS = 8000;
	let artState = $state<Record<string, 'ok' | 'bad'>>({});
	const probing = new Set<string>();
	const artOf = (m: PersonMovie) => m.backdrop || m.poster || '';
	function probe(m: PersonMovie) {
		const k = m.ids.tmdb, src = artOf(m);
		if (!src || probing.has(k)) return;
		probing.add(k);
		const img = new Image();
		let settled = false;
		const end = (ok: boolean) => { if (settled) return; settled = true; clearTimeout(t); artState = { ...artState, [k]: ok ? 'ok' : 'bad' }; };
		const t = setTimeout(() => end(false), ART_WAIT_MS);
		img.onload = () => end(img.naturalWidth > 0);
		img.onerror = () => end(false);
		img.src = src;
	}
	const withArt = (list: PersonMovie[] | undefined) => (list ?? []).filter((m) => artOf(m) && artState[m.ids.tmdb] !== 'bad');

	return {
		probe,
		withArt,
	};
}
