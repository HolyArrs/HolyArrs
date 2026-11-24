// Every primary button inside the setup wizard card gets the sign-in page's
// pixel trail (hover, press, busy), as Create account has. Buttons come and go
// with the steps, so new ones are picked up as they appear.
import { pixelTrail } from '#lib/accounts/index.ts';

export function trailButtons(node: HTMLElement) {
	const on = new Map<HTMLElement, ReturnType<typeof pixelTrail>>();
	const busyOf = (b: HTMLElement) => b.getAttribute('aria-busy') === 'true';
	function scan() {
		for (const [b, t] of on) {
			if (!node.contains(b)) {
				t.destroy();
				on.delete(b);
			} else t.update({ busy: busyOf(b) });
		}
		for (const b of node.querySelectorAll<HTMLElement>('.ui-btn.primary')) {
			// A button that already has its own trail (the sign-in form's) keeps it.
			// The bridge card's button is drawn as a secondary on the glass, so it gets no primary motion.
			if (b.closest('.ui-settings')) continue;
			if (on.has(b) || b.querySelector(':scope > canvas.px-trail')) continue;
			b.classList.add('has-trail');
			on.set(b, pixelTrail(b, { busy: busyOf(b) }));
		}
	}
	scan();
	const mo = new MutationObserver(scan);
	mo.observe(node, { childList: true, subtree: true, attributes: true, attributeFilter: ['aria-busy'] });
	return {
		destroy() {
			mo.disconnect();
			for (const t of on.values()) t.destroy();
			on.clear();
		}
	};
}
