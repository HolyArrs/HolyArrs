
export type ShelfKind = 'movie' | 'show' | 'artist';
const KEY = 'holyarrs.library-last-kind';
const all: ShelfKind[] = ['movie', 'show', 'artist'];

export function lastLibraryKind(allowed: ShelfKind[] = all): ShelfKind {
	const ok = allowed.length ? allowed : all;
	try {
		const v = localStorage.getItem(KEY) as ShelfKind | null;
		if (v && ok.includes(v)) return v;
	} catch {
	}
	return ok[0];
}

export function rememberLibraryKind(k: string) {
	if (!all.includes(k as ShelfKind)) return;
	try {
		localStorage.setItem(KEY, k);
	} catch {
	}
}
