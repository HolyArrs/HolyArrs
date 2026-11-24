// Edit in place for settings lists: while an item is being changed, the
// list leaves it out so the open form is the only copy on screen. The form
// scrolls into view and takes focus; when it closes, focus goes back to the
// Change button of the row that returns.

/** The list without the item being changed. `editingId` is null (or an id
 *  no row has, such as 0 for a new item) when nothing is being changed. */
export function withoutEditing<T>(items: readonly T[], editingId: unknown, idOf: (item: T) => unknown = (x) => (x as { id: unknown }).id): T[] {
	if (editingId == null) return [...items];
	return items.filter((x) => idOf(x) !== editingId);
}

/** Svelte action for the edit form. `returnTo` is the id of the row being
 *  changed (its `data-edit-row` value), or null for a new item. */
export function editForm(node: HTMLElement, returnTo: unknown = null) {
	let back = returnTo;
	const show = () => {
		const reduce = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
		node.scrollIntoView?.({ block: 'start', behavior: reduce ? 'auto' : 'smooth' });
		const first = node.querySelector<HTMLElement>('input:not([type=hidden]):not([disabled]), select:not([disabled]), textarea:not([disabled])');
		first?.focus({ preventScroll: true });
	};
	show();
	return {
		// Change on another row while the form is open: show the form again.
		update(next: unknown) {
			if (next === back) return;
			back = next;
			show();
		},
		destroy() {
			if (back == null) return;
			const id = String(back);
			// The row comes back in the same update; wait for it.
			requestAnimationFrame(() => {
				const active = document.activeElement;
				if (active && active !== document.body) return;
				const row = document.querySelector(`[data-edit-row="${CSS.escape(id)}"]`);
				(row?.querySelector<HTMLElement>('[data-edit-change]') ?? row?.querySelector<HTMLElement>('button'))?.focus();
			});
		}
	};
}
