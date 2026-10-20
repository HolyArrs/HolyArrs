// The one search field lives in the top bar (a phone-only copy sits on the
// Search page, where the top bar is hidden). Its words are shared here so the
// Search page can follow them as you type on any page.

type Hooks = { submit?: () => void; blur?: () => void };

export const searchField = $state({ term: '' });

/** The Search page's own reactions (Enter, leaving the field); set while it is open. */
export const searchHooks: Hooks = {};

const fields = new Set<HTMLInputElement>();

/** registerField records a field element; the returned function forgets it. */
export function registerField(el: HTMLInputElement): () => void {
	fields.add(el);
	return () => fields.delete(el);
}

/** focusField focuses (and selects) the field that is on screen; false when none is. */
export function focusField(select = false): boolean {
	for (const el of fields) {
		if (el.offsetParent === null && el.getClientRects().length === 0) continue;
		el.focus();
		if (select) el.select();
		return true;
	}
	return false;
}
