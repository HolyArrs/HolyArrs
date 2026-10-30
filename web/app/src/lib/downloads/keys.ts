// The Downloads search shortcuts: Ctrl K (⌘K) and "/" focus the search field.

export interface KeyBits {
	key: string;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
}

/**
 * searchShortcut says whether a key press should focus the Downloads search.
 * `inField`: the press came from a text field ("/" types there instead).
 * `inDialog`: focus sits in, or a modal shows, a dialog, sheet or popover (it keeps its focus).
 */
export function searchShortcut(e: KeyBits, inField: boolean, inDialog: boolean): boolean {
	if (inDialog) return false;
	const mod = e.ctrlKey || e.metaKey;
	if (mod && !e.altKey && (e.key === 'k' || e.key === 'K')) return true;
	return e.key === '/' && !mod && !e.altKey && !inField;
}

/** reviewShown is the "N to review" count the status shows: watchers never get the call. */
export function reviewShown(watch: boolean, toReview: number): number {
	return watch ? 0 : toReview;
}

/** escStep is what Esc does in the Downloads search: clear the words first, then leave the field. */
export function escStep(term: string): 'clear' | 'blur' {
	return term ? 'clear' : 'blur';
}
