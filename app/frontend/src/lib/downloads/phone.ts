// Phone helpers for the Downloads list: the second line under a row's title and the long press that starts picking rows.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { DownloadRow, EpisodeLine } from '../core/types.ts';

type SubRow = Pick<DownloadRow, 'kind' | 'year' | 'covers' | 'album' | 'title' | 'name'> & { episodes?: Pick<EpisodeLine, 'season' | 'episode' | 'label' | 'name'>[] };

const pad = (n: number) => String(n).padStart(2, '0');

/** What a phone row shows under its title: the episode ("S04E01 · Name"), the season for a pack ("Season 3"), the album for music, the year for a movie. */
export function subLine(r: SubRow): string {
	if (r.kind === 'show') {
		const eps = r.episodes ?? [];
		if (eps.length === 1) {
			const e = eps[0];
			const code = `S${pad(e.season)}E${pad(e.episode)}`;
			return e.name ? `${code} · ${e.name}` : code;
		}
		if (eps.length > 1) {
			const seasons = [...new Set(eps.map((e) => e.season))].sort((a, b) => a - b);
			if (seasons.length === 1) return t('downloads.phone.season', { n: seasons[0] });
			return t('downloads.phone.seasons', { from: seasons[0], to: seasons[seasons.length - 1] });
		}
		return r.covers || '';
	}
	if (r.kind === 'artist') return r.album || r.covers || '';
	if (r.kind === 'movie' || r.title) return r.year ? String(r.year) : '';
	return '';
}

/** The row title without the year, which the line under it carries. */
export function phoneTitle(r: Pick<DownloadRow, 'title' | 'name' | 'kind'>): string {
	return r.title || r.name;
}

export const HOLD_MS = 450;
const SLOP = 10;

/** The long-press state machine, kept apart from the DOM so it can be tested. */
export function pressTimer(onlong: () => void, ms = HOLD_MS, timers: { set: (f: () => void, ms: number) => ReturnType<typeof setTimeout>; clear: (t: ReturnType<typeof setTimeout> | undefined) => void } = { set: (f, ms) => setTimeout(f, ms), clear: (t) => clearTimeout(t) }) {
	let t: ReturnType<typeof setTimeout> | undefined;
	let x = 0;
	let y = 0;
	let fired = false;
	return {
		down(px: number, py: number) {
			fired = false;
			x = px;
			y = py;
			timers.clear(t);
			t = timers.set(() => {
				t = undefined;
				fired = true;
				onlong();
			}, ms);
		},
		move(px: number, py: number) {
			if (t !== undefined && Math.hypot(px - x, py - y) > SLOP) this.cancel();
		},
		cancel() {
			timers.clear(t);
			t = undefined;
		},
		/** True once after a press that fired, so the click that follows is swallowed. */
		takeFired() {
			const f = fired;
			fired = false;
			return f;
		},
		get holding() {
			return t !== undefined;
		}
	};
}

/** Svelte action: holding a row (touch or pen) for HOLD_MS calls onlong, marks the row with data-pressing while held, and swallows the click after it. */
export function longPress(node: HTMLElement, onlong: () => void) {
	let cb = onlong;
	let touched = false;
	const p = pressTimer(() => {
		node.removeAttribute('data-pressing');
		navigator.vibrate?.(12);
		cb();
	});
	const down = (e: PointerEvent) => {
		if (e.pointerType === 'mouse') {
			touched = false;
			return;
		}
		if (!e.isPrimary || e.button > 0 || !matchMedia('(max-width: 640px)').matches) return;
		if ((e.target as Element | null)?.closest?.('input, button, a, select, textarea')) return;
		touched = true;
		node.setAttribute('data-pressing', '');
		p.down(e.clientX, e.clientY);
	};
	const move = (e: PointerEvent) => {
		p.move(e.clientX, e.clientY);
		if (!p.holding) node.removeAttribute('data-pressing');
	};
	const stop = () => {
		p.cancel();
		node.removeAttribute('data-pressing');
	};
	const click = (e: MouseEvent) => {
		if (p.takeFired()) {
			e.stopImmediatePropagation();
			e.preventDefault();
		}
	};
	const menu = (e: Event) => {
		if (touched) e.preventDefault();
	};
	node.addEventListener('pointerdown', down);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', stop);
	node.addEventListener('pointercancel', stop);
	node.addEventListener('pointerleave', stop);
	node.addEventListener('click', click, true);
	node.addEventListener('contextmenu', menu);
	return {
		update(next: () => void) {
			cb = next;
		},
		destroy() {
			p.cancel();
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', stop);
			node.removeEventListener('pointercancel', stop);
			node.removeEventListener('pointerleave', stop);
			node.removeEventListener('click', click, true);
			node.removeEventListener('contextmenu', menu);
		}
	};
}
