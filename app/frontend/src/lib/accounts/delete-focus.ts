// Where focus goes when the delete ask in a People row closes: back to that
// person's Delete button on Keep, or to the People heading after a delete.
type Focusable = { focus(): void; setAttribute(k: string, v: string): void; hasAttribute(k: string): boolean };

export function focusAfterAsk(root: { querySelector(s: string): unknown }, userId: number, deleted: boolean): void {
	if (!deleted) {
		(root.querySelector(`[data-testid="user-delete"][data-user="${userId}"]`) as Focusable | null)?.focus();
		return;
	}
	const h = root.querySelector('#people-card h2') as Focusable | null;
	if (!h) return;
	if (!h.hasAttribute('tabindex')) h.setAttribute('tabindex', '-1');
	h.focus();
}
