// Remembers which shelf kind the Library page showed last, in the browser.

export type ShelfKind = 'movie' | 'show' | 'artist';

// Browser storage key for the last shelf kind.
const KEY = 'holyarrs.library-last-kind';

const all: ShelfKind[] = ['movie', 'show', 'artist'];

// Storage can be blocked, so a bad read falls back to the first allowed kind.
export function lastLibraryKind(allowed: ShelfKind[] = all): ShelfKind {
	const ok = allowed.length ? allowed : all;
	try {
		const v = localStorage.getItem(KEY) as ShelfKind | null;
		if (v && ok.includes(v)) return v;
	} catch {
	}
	return ok[0];
}

// Ignores unknown kinds so a stray value never gets saved.
export function rememberLibraryKind(k: string) {
	if (!all.includes(k as ShelfKind)) return;
	try {
		localStorage.setItem(KEY, k);
	} catch {
	}
}
