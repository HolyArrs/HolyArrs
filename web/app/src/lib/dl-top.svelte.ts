// Downloads has no top bar: its toolbar row carries the sidebar button and its own search.
// The layout shares the sidebar state and the shortcut label here.

export const dlTop = $state({
	/** The sidebar is folded, so a "Show sidebar" button belongs in the row (as in the top bar). */
	collapsed: false,
	/** The id of the sidebar element the button controls. */
	controls: 'sidebar',
	/** The shortcut shown in the field (⌘K or Ctrl K). */
	searchKey: 'Ctrl K',
	toggle: (() => {}) as () => void
});
