// The Library is always one kind: Movies, TV shows or Music. A plain /library
// link opens the kind this person used last (Movies the first time).

export type ShelfKind = 'movie' | 'show' | 'artist';
const KEY = 'holyarrs.library-last-kind';
const all: ShelfKind[] = ['movie', 'show', 'artist'];

/** lastLibraryKind is the kind used last, if it is still one of `allowed`; else the first allowed one. */
export function lastLibraryKind(allowed: ShelfKind[] = all): ShelfKind {
	const ok = allowed.length ? allowed : all;
	try {
		const v = localStorage.getItem(KEY) as ShelfKind | null;
		if (v && ok.includes(v)) return v;
	} catch {
		// storage blocked: fall through
	}
	return ok[0];
}

/** rememberLibraryKind notes the kind on screen for the next plain /library link. */
export function rememberLibraryKind(k: string) {
	if (!all.includes(k as ShelfKind)) return;
	try {
		localStorage.setItem(KEY, k);
	} catch {
		// storage blocked: nothing to remember
	}
}
