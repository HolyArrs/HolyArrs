export type TipSide = 'below' | 'right';

// A tooltip in the browser's top layer (Popover API), so no list, frame or scroll box can hide it. One shared element, placed from the target's box and kept inside the viewport, styled like the app's other tooltips.
let el: HTMLElement | null = null;

let timer: ReturnType<typeof setTimeout> | undefined;

/** Puts the tip beside `target`, below it or right of it, flipped and clamped to stay on screen. */
export function place(tip: HTMLElement, target: DOMRect, side: TipSide) {
	const pad = 8;
	const vw = document.documentElement.clientWidth;
	const vh = document.documentElement.clientHeight;
	const t = tip.getBoundingClientRect();
	let left: number;
	let top: number;
	if (side === 'right') {
		left = target.right + 8;
		if (left + t.width > vw - pad) left = target.left - 8 - t.width;
		top = target.top + target.height / 2 - t.height / 2;
	} else {
		left = target.left + target.width / 2 - t.width / 2;
		top = target.bottom + 6;
		if (top + t.height > vh - pad) top = target.top - 6 - t.height;
	}
	left = Math.max(pad, Math.min(vw - t.width - pad, left));
	top = Math.max(pad, Math.min(vh - t.height - pad, top));
	tip.style.left = `${Math.round(left)}px`;
	tip.style.top = `${Math.round(top)}px`;
}

export function hideTip() {
	clearTimeout(timer);
	try {
		el?.hidePopover();
	} catch {
		/* Not shown. */
	}
}

/** Shows `text` in the top layer on hover (after a short wait) and on keyboard focus. */
export function tip(node: HTMLElement, opts: string | { text: string; side?: TipSide }) {
	let o = typeof opts === 'string' ? { text: opts, side: 'below' as TipSide } : { side: 'below' as TipSide, ...opts };
	const show = () => {
		if (!o.text || matchMedia('(hover: none)').matches) return;
		const t = ensure();
		t.textContent = o.text;
		try {
			t.showPopover();
		} catch {
			return;
		}
		place(t, node.getBoundingClientRect(), o.side);
	};
	const enter = () => {
		clearTimeout(timer);
		timer = setTimeout(show, 400);
	};
	// focusin bubbles, so a tip on a wrapper also shows when the button inside gets keyboard focus.
	const focus = (e: FocusEvent) => {
		if ((e.target as Element | null)?.matches(':focus-visible')) show();
	};
	node.addEventListener('pointerenter', enter);
	node.addEventListener('pointerleave', hideTip);
	node.addEventListener('pointerdown', hideTip);
	node.addEventListener('focusin', focus);
	node.addEventListener('focusout', hideTip);
	node.addEventListener('keydown', (e) => e.key === 'Escape' && hideTip());
	return {
		update(next: string | { text: string; side?: TipSide }) {
			o = typeof next === 'string' ? { text: next, side: 'below' } : { side: 'below', ...next };
		},
		destroy() {
			hideTip();
			node.removeEventListener('pointerenter', enter);
			node.removeEventListener('pointerleave', hideTip);
			node.removeEventListener('pointerdown', hideTip);
			node.removeEventListener('focusin', focus);
			node.removeEventListener('focusout', hideTip);
		}
	};
}

function ensure(): HTMLElement {
	if (el) return el;
	el = document.createElement('div');
	el.setAttribute('popover', 'manual');
	el.setAttribute('role', 'tooltip');
	el.setAttribute('data-testid', 'dl-tip');
	Object.assign(el.style, {
		position: 'fixed', margin: '0', inset: 'auto', padding: '4px 8px', border: '0', borderRadius: '6px',
		background: 'var(--text)', color: 'var(--bg)', font: 'var(--t-footnote)', whiteSpace: 'nowrap',
		pointerEvents: 'none', maxWidth: 'calc(100vw - 16px)', overflow: 'hidden', textOverflow: 'ellipsis'
	});
	document.body.appendChild(el);
	return el;
}
