import { tick } from 'svelte';
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

/** The entrance stagger: numbers the panel's parts in order (at most 9), so each fades in 70 ms after the one before. */
export function staggerParts(panel: HTMLElement) {
	const parts = panel.querySelectorAll<HTMLElement>('.title, .lead, .provs, .or, .field, .submit, .step .link, .help');
	parts.forEach((el, i) => el.style.setProperty('--i', String(Math.min(i + 1, 9))));
}

/** Focuses the name (or password) field once it is there; false while it is not. Phones get no focus on load, so the keyboard does not cover the art and the card. */
export function focusFirstField(panel: HTMLElement): boolean {
	const field = panel.querySelector<HTMLInputElement>('#name') ?? panel.querySelector<HTMLInputElement>('#password');
	if (!field) return false;
	if (matchMedia('(pointer: fine)').matches && (!document.activeElement || document.activeElement === document.body)) field.focus({ preventScroll: true });
	return true;
}

/** After a view change the pressed button is gone, so focus would drop to the page: move it to the new step's first control. Touch screens are left alone (focusing a field there pops the keyboard). */
export function focusStepIn(panel: () => HTMLElement | undefined) {
	const touch = !matchMedia('(pointer: fine)').matches && document.documentElement.dataset.input !== 'keyboard';
	void tick().then(() => {
		// Touch (and steps with no field): the heading, so screen readers read the new step and no keyboard pops up.
		const h = panel()?.querySelector<HTMLElement>('.step h1');
		const heading = () => {
			if (!h) return false;
			if (!h.hasAttribute('tabindex')) h.tabIndex = -1;
			h.focus({ preventScroll: true });
			return true;
		};
		if (touch) {
			const a = document.activeElement;
			if (!a || a === document.body || !a.isConnected) heading();
			return;
		}
		for (const s of ['.digit', '#otp', '#addr', '#name', '#password', '.acct-pick']) {
			const el = panel()?.querySelector<HTMLElement>(s);
			if (el) return el.focus({ preventScroll: true });
		}
		if (!heading()) panel()?.querySelector<HTMLElement>('.step button')?.focus({ preventScroll: true });
	});
}
