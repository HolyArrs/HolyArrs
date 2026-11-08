// Search's entrance and exit: the tiles landing one by one and the way back out.
import { ENTER_DELAY, ENTER_SWEEP, passAt } from './MosaicReveal.svelte';
import { goto } from '$app/navigation';
import { to } from '#lib/core/paths.ts';
import { restoreReturn } from '#lib/core/search-field.svelte.ts';

export function createSearchEntry(ctx: { revealing: boolean; calm: boolean; restored: boolean }) {
	// Search is a layer: Close, Escape on an empty field or Back returns to the page you came from, at its scroll. Typing replaces the address, so only links inside Search add history entries (counted in depth).
	let cameFrom = '';
	let depth = 0;
	let leaving = $state(false);
	let covering = $state(false);
	const LAND_MS = 700;
	function exit() {
		if (leaving) return;
		ctx.revealing = false;
		leaving = true;
		// The tiles drop out quickly, last visible row first; the band then sweeps back up (ease-in). The page stays put and the pixels cover it, bottom up.
		covering = true;
	}
	function left() {
		if (cameFrom) {
			restoreReturn(location.pathname);
			history.go(-(depth + 1));
		} else void goto(to('/'));
	}
	const t0 = typeof performance === 'undefined' ? 0 : performance.now();
	// Each tile enters behind the band: rows about 70 ms apart, about 25 ms left to right within a row, starting as the band passes it, bounded so the last lands by about 1.1 s.
	function landing(node: HTMLElement, at: { row: number; col: number }) {
		if (ctx.calm || ctx.restored) return;
		const elapsed = performance.now() - t0;
		// The first screen only: tiles that arrive after the entry just appear.
		if (elapsed > ENTER_DELAY + ENTER_SWEEP) return;
		const top = document.querySelector('.ui-topbar')?.getBoundingClientRect().bottom ?? 0;
		const r = node.getBoundingClientRect();
		if (r.top > innerHeight) return;
		const f = (r.top - top) / Math.max(1, innerHeight - top);
		// The pixels themselves reveal the tile (it is already there under the mosaic); as the band passes its row the tile only settles a hair, so the page comes through the pixelation instead of after it.
		const start = Math.min(ENTER_DELAY + ENTER_SWEEP - LAND_MS, passAt(Math.max(0, f)) + at.col * 18);
		const delay = Math.max(0, start - elapsed);
		node.style.willChange = 'transform';
		const a = node.animate([{ transform: 'scale(1.025)' }, { transform: 'none' }], { duration: LAND_MS, delay, easing: 'cubic-bezier(.22,1,.36,1)', fill: 'backwards' });
		a.onfinish = a.oncancel = () => (node.style.willChange = '');
	}

	return {
		exit,
		get cameFrom() {
			return cameFrom;
		},
		set cameFrom(v) {
			cameFrom = v;
		},
		get depth() {
			return depth;
		},
		set depth(v) {
			depth = v;
		},
		get covering() {
			return covering;
		},
		set covering(v) {
			covering = v;
		},
		left,
		landing,
	};
}
