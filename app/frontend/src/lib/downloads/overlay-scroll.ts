// Overlay pill scrollbar: a Svelte action for one scroll box. Native scrolling stays untouched; the rails only mirror scrollTop and scrollLeft and let you drag or page. Mounted only for a fine pointer on wide screens; phones keep the native bar.
import './overlay-scroll.css';

export type OverlayScrollOpts = { header?: string };

type Axis = 'v' | 'h';

export const OSB_QUERY = '(min-width: 641px) and (pointer: fine)';
const MIN = 32;

/** Thumb length and offset in whole pixels for one axis. */
export function thumb(track: number, client: number, total: number, at: number) {
	if (total <= client + 1 || track <= 0) return { hidden: true, len: 0, pos: 0 };
	const len = Math.min(track, Math.max(MIN, Math.round((track * client) / total)));
	const room = total - client;
	const pos = Math.round(((track - len) * Math.min(Math.max(at, 0), room)) / room);
	return { hidden: false, len, pos };
}

/** Scroll value for a thumb dragged `d` pixels from where it started. */
export function dragTo(start: number, d: number, track: number, len: number, client: number, total: number) {
	const free = track - len;
	return free > 0 ? start + (d * (total - client)) / free : start;
}

export function overlayScroll(node: HTMLElement, opts: OverlayScrollOpts = {}) {
	let header = opts.header ?? '';
	const host = node.parentElement;
	const mq = typeof matchMedia === 'function' ? matchMedia(OSB_QUERY) : null;
	let on = false, raf = 0, lastH = -1;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let rails: Record<Axis, HTMLElement> | null = null;
	let ro: ResizeObserver | null = null, mo: MutationObserver | null = null;
	const reduce = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

	const wake = () => {
		if (!host) return;
		host.setAttribute('data-osb-active', '');
		clearTimeout(timer);
		timer = setTimeout(() => host.removeAttribute('data-osb-active'), 900);
	};
	const parts = () => [node.firstElementChild, header ? node.querySelector(header) : null, node.querySelector('.selbar')].filter(Boolean) as Element[];
	const headH = () => {
		let h = 0;
		const hd = header ? node.querySelector<HTMLElement>(header) : null;
		if (hd && hd.offsetParent !== null) h += hd.offsetHeight;
		const sb = node.querySelector<HTMLElement>('.selbar');
		if (sb && sb.offsetParent !== null) h += sb.offsetHeight;
		return h;
	};
	const draw = () => {
		raf = 0;
		if (!rails || !host) return;
		// The rails belong to the list, not the card: they start below the toolbar rows and header above the scroller and end above whatever sits under it (the detail pane). All reads come first, then the writes.
		const hr = host.getBoundingClientRect(), nr = node.getBoundingClientRect();
		const top = Math.max(0, Math.round(nr.top - hr.top)) + headH();
		const below = Math.max(0, Math.round(hr.bottom - nr.bottom));
		const v = thumb(rails.v.clientHeight, node.clientHeight, node.scrollHeight, node.scrollTop);
		const h = thumb(rails.h.clientWidth, node.clientWidth, node.scrollWidth, node.scrollLeft);
		for (const [ax, t] of [['v', v], ['h', h]] as const) {
			const r = rails[ax], th = r.firstElementChild as HTMLElement;
			r.style.setProperty('--osb-top', `${top}px`);
			r.style.setProperty('--osb-bottom', `${below}px`);
			r.hidden = t.hidden;
			th.style.setProperty('--osb-len', `${t.len}px`);
			th.style.setProperty('--osb-pos', `${t.pos}px`);
		}
		host.toggleAttribute('data-osb-both', !v.hidden && !h.hidden);
		// The rail's own size moves when the other rail or the header offset changes; redraw once more.
		if (rails.v.clientHeight !== lastH) { lastH = rails.v.clientHeight; queue(); }
	};
	const queue = () => { if (!raf) raf = requestAnimationFrame(draw); };
	const bump = () => { queue(); wake(); };

	const sizes = (ax: Axis) => ax === 'v'
		? { track: rails!.v.clientHeight, client: node.clientHeight, total: node.scrollHeight, at: node.scrollTop }
		: { track: rails!.h.clientWidth, client: node.clientWidth, total: node.scrollWidth, at: node.scrollLeft };
	const set = (ax: Axis, val: number) => { if (ax === 'v') node.scrollTop = val; else node.scrollLeft = val; };

	const make = (ax: Axis) => {
		const r = document.createElement('div');
		r.className = `osb-rail osb-${ax}`;
		r.setAttribute('aria-hidden', 'true');
		const th = document.createElement('div');
		th.className = 'osb-thumb';
		r.append(th);
		th.addEventListener('pointerdown', (e) => {
			if (e.button !== 0) return;
			e.preventDefault(); e.stopPropagation();
			th.setPointerCapture?.(e.pointerId);
			const s = sizes(ax), t = thumb(s.track, s.client, s.total, s.at);
			const p0 = ax === 'v' ? e.clientY : e.clientX;
			r.setAttribute('data-drag', ''); host?.setAttribute('data-osb-drag', '');
			const move = (m: PointerEvent) => set(ax, dragTo(s.at, (ax === 'v' ? m.clientY : m.clientX) - p0, s.track, t.len, s.client, s.total));
			const end = () => {
				r.removeAttribute('data-drag'); host?.removeAttribute('data-osb-drag');
				th.removeEventListener('pointermove', move); th.removeEventListener('pointerup', end); th.removeEventListener('lostpointercapture', end);
			};
			th.addEventListener('pointermove', move); th.addEventListener('pointerup', end); th.addEventListener('lostpointercapture', end);
		});
		r.addEventListener('pointerdown', (e) => {
			if (e.button !== 0 || e.target !== r) return;
			e.preventDefault();
			const s = sizes(ax), t = thumb(s.track, s.client, s.total, s.at);
			const box = r.getBoundingClientRect();
			const at = ax === 'v' ? e.clientY - box.top : e.clientX - box.left;
			if (e.shiftKey) { set(ax, ((at - t.len / 2) / Math.max(1, s.track - t.len)) * (s.total - s.client)); return; }
			const step = (s.client - 40) * (at < t.pos ? -1 : 1);
			node.scrollBy({ [ax === 'v' ? 'top' : 'left']: step, behavior: reduce() ? 'auto' : 'smooth' });
		});
		if (ax === 'h') r.addEventListener('wheel', (e) => {
			if (!e.deltaY || Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
			e.preventDefault(); node.scrollLeft += e.deltaY;
		}, { passive: false });
		return r;
	};

	const observe = () => { if (ro) for (const el of parts()) ro.observe(el); };
	const mount = () => {
		if (on || !host) return;
		on = true;
		node.classList.add('osb-on');
		rails = { v: make('v'), h: make('h') };
		host.append(rails.v, rails.h);
		node.addEventListener('scroll', bump, { passive: true });
		node.addEventListener('focusin', wake);
		if (typeof ResizeObserver === 'function') { ro = new ResizeObserver(bump); ro.observe(node); observe(); }
		if (typeof MutationObserver === 'function') {
			// The selection bar and the table come and go as direct children; rows sit a level down.
			mo = new MutationObserver(() => { observe(); bump(); });
			mo.observe(node, { childList: true });
			if (node.firstElementChild) mo.observe(node.firstElementChild, { childList: true });
		}
		queue();
	};
	const unmount = () => {
		if (!on) return;
		on = false;
		node.classList.remove('osb-on');
		node.removeEventListener('scroll', bump);
		node.removeEventListener('focusin', wake);
		ro?.disconnect(); mo?.disconnect(); ro = mo = null;
		rails?.v.remove(); rails?.h.remove(); rails = null;
		if (raf) cancelAnimationFrame(raf);
		raf = 0; lastH = -1;
		clearTimeout(timer);
		for (const a of ['data-osb-active', 'data-osb-both', 'data-osb-drag']) host?.removeAttribute(a);
	};
	const check = () => (mq?.matches ? mount() : unmount());
	mq?.addEventListener?.('change', check);
	check();

	return {
		update(o: OverlayScrollOpts = {}) { header = o.header ?? ''; observe(); queue(); },
		destroy() { mq?.removeEventListener?.('change', check); unmount(); }
	};
}
