/** Marks a sideways-scrolling row with data-fade-left / data-fade-right on the sides that still have more to show (physical sides, so right-to-left rows work). CSS draws the fade from them. */
export function edgeFade(node: HTMLElement) {
	function update() {
		const max = node.scrollWidth - node.clientWidth;
		// scrollLeft is 0 at the start edge and runs negative in right-to-left rows.
		const x = Math.abs(node.scrollLeft);
		const rtl = getComputedStyle(node).direction === 'rtl';
		const more = max > 2;
		const atStart = x <= 2;
		const atEnd = x >= max - 2;
		const left = more && (rtl ? !atEnd : !atStart);
		const right = more && (rtl ? !atStart : !atEnd);
		node.toggleAttribute('data-fade-left', left);
		node.toggleAttribute('data-fade-right', right);
	}
	update();
	node.addEventListener('scroll', update, { passive: true });
	const ro = typeof ResizeObserver === 'function' ? new ResizeObserver(update) : null;
	ro?.observe(node);
	for (const c of Array.from(node.children)) ro?.observe(c);
	const mo = typeof MutationObserver === 'function' ? new MutationObserver(update) : null;
	mo?.observe(node, { childList: true, subtree: true, characterData: true });
	return {
		destroy() {
			node.removeEventListener('scroll', update);
			ro?.disconnect();
			mo?.disconnect();
		}
	};
}
