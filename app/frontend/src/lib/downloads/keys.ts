// The Downloads search shortcuts: Ctrl K (⌘K) and "/" focus the search field.

export interface KeyBits {
	key: string;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
}

/** Whether a key press should focus the Downloads search. `inField`: the press came from a text field, where "/" types instead. `inDialog`: focus sits in a dialog, sheet or popover, or a modal shows. */
export function searchShortcut(e: KeyBits, inField: boolean, inDialog: boolean): boolean {
	if (inDialog) return false;
	const mod = e.ctrlKey || e.metaKey;
	if (mod && !e.altKey && (e.key === 'k' || e.key === 'K')) return true;
	return e.key === '/' && !mod && !e.altKey && !inField;
}

/** The "N to review" count in the status; watchers never get the call. */
export function reviewShown(watch: boolean, toReview: number): number {
	return watch ? 0 : toReview;
}

/** Esc in the search: clear the words first, then leave the field. */
export function escStep(term: string): 'clear' | 'blur' {
	return term ? 'clear' : 'blur';
}

/** Esc on the list: clear a selection first, then close the open panel. */
export function listEsc(picked: number, open: boolean): 'clear' | 'close' | 'none' {
	if (picked > 0) return 'clear';
	return open ? 'close' : 'none';
}
