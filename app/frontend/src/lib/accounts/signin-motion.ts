// Phones: lift the card above the on-screen keyboard (--kb on the card's parent).
export function liftAboveKeyboard(panel: () => HTMLElement | undefined): () => void {
	const vv = window.visualViewport;
	const lift = () => {
		const el = panel();
		if (!vv || !el) return;
		let kb = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
		// Never lift the card's top past 16 px of the visible area.
		kb = Math.min(kb, Math.max(0, el.offsetTop - 16));
		el.parentElement?.style.setProperty('--kb', `${kb}px`);
	};
	vv?.addEventListener('resize', lift);
	vv?.addEventListener('scroll', lift);
	return () => {
		vv?.removeEventListener('resize', lift);
		vv?.removeEventListener('scroll', lift);
	};
}
