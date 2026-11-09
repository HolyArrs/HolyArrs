// Downloads has no top bar: its toolbar row carries the sidebar button and the search. The layout shares the sidebar state and shortcut label here.

export const dlTop = $state({
	/** The sidebar is folded, so the row needs a "Show sidebar" button. */
	collapsed: false,
	/** The shortcut shown in the field (⌘K or Ctrl K). */
	searchKey: 'Ctrl K',
	toggle: (() => {}) as () => void
});
