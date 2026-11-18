import { describe, expect, it, vi } from 'vitest';
import type { TitleView } from '../core/types.ts';
import type { Column } from './table.ts';
import { buildEntries, cell, drawRange, firstOnScreen, holdFocus, holder, placeKeeper, posterCols, posterRowGuess, posterRowKey, rowCell, tone } from './library-view.ts';
import { fakeWindow } from './fake-window.ts';

const t = (id: number) => ({ id, kind: 'movie', slots: [] }) as unknown as TitleView;
const col = (id: string) => ({ id, width: '150px' }) as unknown as Column;

describe('buildEntries', () => {
	it('groups by series, sorts headings, loose last', () => {
		const m = new Map([[1, 20], [2, 10]]);
		const name = (id: number) => (id === 10 ? 'Alien' : 'Zorro');
		const e = buildEntries([t(1), t(2), t(3)], true, m, name);
		expect(e.map((x) => x.key)).toEqual(['h10', 'r2', 'h20', 'r1', 'h0', 'r3']);
		expect(e[4]).toMatchObject({ name: 'Not in a series', count: 1 });
		expect(new Set(e.map((x) => x.key)).size).toBe(e.length);
		expect(buildEntries([t(1)], false, m, name)).toEqual([{ type: 'row', key: 'r1', t: t(1) }]);
		expect(buildEntries([t(1)], true, m, name).some((x) => x.key === 'h0')).toBe(false);
	});
});

describe('rowCell / cell', () => {
	it('maps column ids to text', () => {
		const m = { id: 1, kind: 'movie', slots: [], year: 1999, keep_up_to_date: true, runtime_minutes: 125, digital_date: '2999-01-01' } as unknown as TitleView;
		expect(cell(m, col('year'), '')).toBe('1999');
		expect(cell(m, col('runtime'), '')).toBe('2h 5m');
		expect(cell(m, col('kept'), '')).toBe('Yes');
		expect(cell(m, col('release'), '')).toMatch(/^Home release/);
		expect(cell({ ...m, digital_date: '2000-01-01' } as TitleView, col('release'), '')).toBe('Jan 1, 2000');
		expect(cell({ ...m, digital_date: undefined, cinema_date: '2999-01-01' } as TitleView, col('release'), '')).toMatch(/^Cinemas/);
		expect(cell(m, col('series'), 'Matrix')).toBe('Matrix');
		expect(cell(m, col('nope'), '')).toBe('');
		expect(cell({ ...m, score: 7.4, votes: 900 } as TitleView, col('score'), '')).toBe('★ 7.4');
		expect(cell({ ...m, score: 8, votes: 900 } as TitleView, col('score'), '')).toBe('★ 8.0');
		expect(cell({ ...m, score: 9.1, votes: 3 } as TitleView, col('score'), '')).toBe('');
		expect(cell(m, col('score'), '')).toBe('');
		expect(rowCell(m, col('episodes'), { have: 3, total: 10 }, '')).toBe('3/10');
		expect(rowCell(m, col('episodes'), { have: 0, total: 0 }, '')).toBe('');
	});
});

describe('tone', () => {
	const show = (in_library: number, episodes: number, missing: number, keep = true) =>
		({ kind: 'show', slots: [], keep_up_to_date: keep, show: { in_library, episodes, not_out_yet: 0, missing } }) as unknown as TitleView;
	it('picks the bar state', () => {
		expect(tone(show(0, 0, 0))).toBe('later');
		expect(tone(show(5, 5, 0))).toBe('full');
		expect(tone(show(0, 5, 5, false))).toBe('none');
		expect(tone(show(3, 5, 0))).toBe('later');
		expect(tone(show(3, 5, 2))).toBe('gap');
	});
});

describe('posters drawn a row at a time', () => {
	it('counts the columns the way the grid fills them', () => {
		expect(posterCols(1180, 172, 18)).toBe(6);
		expect(posterCols(1311, 172, 18)).toBe(6);
		expect(posterCols(1312, 172, 18)).toBe(7);
		expect(posterCols(100, 172, 18)).toBe(1);
	});
	it('guesses a row from its tallest art, its caption and any lines under it', () => {
		const one = posterRowGuess([{ square: false, lines: 0 }], 180, 26);
		expect(one).toBeCloseTo(270 + 6 + 3 + 18.9 + 26, 5);
		expect(posterRowGuess([{ square: true, lines: 0 }, { square: false, lines: 0 }], 180, 26)).toBeCloseTo(one, 5);
		expect(posterRowGuess([{ square: false, lines: 2 }], 180, 26)).toBeCloseTo(one + 2 * (3 + 15.6), 5);
	});
	it('draws only the rows near the screen, wherever the list is scrolled to', () => {
		// 1,700 posters in rows of 6, each row 350 px.
		const rows = Math.ceil(1700 / 6);
		const offsets = Array.from({ length: rows + 1 }, (_, i) => i * 350);
		const top = drawRange(offsets, 0, 900);
		expect(top.start).toBe(0);
		expect((top.end - top.start) * 6).toBeLessThan(60);
		// A jump to the letter T (about poster 1,243) draws a screenful there, not every poster before it.
		const at = Math.floor(1243 / 6);
		const t = drawRange(offsets, offsets[at], 900);
		expect(t.start).toBeLessThanOrEqual(at);
		expect(t.end).toBeGreaterThan(at + 2);
		expect((t.end - t.start) * 6).toBeLessThan(60);
		// The end of the list.
		const end = drawRange(offsets, offsets[rows] - 900, 900);
		expect(end.end).toBe(rows);
		expect((end.end - end.start) * 6).toBeLessThan(60);
	});
});

describe('keeping a title in place', () => {
	const card = (id: number, at: number, top: number, h = 300) => ({ dataset: { titleId: String(id) }, getAttribute: () => String(at + 1), getBoundingClientRect: () => ({ top, height: h, bottom: top + h }) }) as unknown as Element;
	it('notes the first card at least half on screen', () => {
		expect(firstOnScreen([card(1, 0, -400), card(2, 1, -120), card(3, 2, 200)], 900)).toEqual({ id: 2, at: 1, top: -120 });
		expect(firstOnScreen([card(1, 0, -400)], 900)).toBeNull();
		// Cards drawn only below the screen are not on it.
		expect(firstOnScreen([card(1, 0, 1000)], 900)).toBeNull();
	});
	it('moves the page by how far the title drifted, and is done once it stays put', () => {
		let y = 1000;
		const by = (d: number) => (y += d);
		const step = holder(200, { y: () => y, by });
		expect(step(219, 0)).toBe(false);
		expect(y).toBe(1019);
		expect(step(200, 0)).toBe(false);
		expect(step(200.4, 0)).toBe(true);
	});
	it('goes to the guessed row while the title is not drawn yet', () => {
		let y = 1000;
		const step = holder(150, { y: () => y, by: (d) => (y += d) });
		expect(step(null, 5150)).toBe(false);
		expect(y).toBe(6000);
	});
	it('lets go when someone else moves the page, or the page cannot move further', () => {
		let y = 1000;
		const step = holder(200, { y: () => y, by: (d) => (y += d) });
		y = 3000;
		expect(step(500, 0)).toBe(true);
		const stuck = holder(200, { y: () => 1000, by: () => {} });
		expect(stuck(260, 0)).toBe(true);
	});
	it('keys a measured row by its columns, details and width, so a narrower grid or other details start from a fresh guess', () => {
		const k = posterRowKey(6, 'status', 380.4, 7);
		expect(posterRowKey(6, 'status', 380.2, 7)).toBe(k);
		expect(posterRowKey(6, 'status', 362, 7)).not.toBe(k);
		expect(posterRowKey(6, 'title', 380.4, 7)).not.toBe(k);
		expect(posterRowKey(5, 'status', 380.4, 7)).not.toBe(k);
	});
});

describe('placeKeeper', () => {
	it('keeps the card on screen still when a row above it grows (a download starts for a title above)', () => {
		const w = fakeWindow();
		let y = 5000;
		let shift = 0;
		const el = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 5200 + shift - y, height: 300, bottom: 5500 + shift - y }) };
		const grid = { children: [el], querySelector: () => el } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		vi.stubGlobal('scrollY', 0);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 0);
		keeper.note();
		shift = 19;
		keeper.keep();
		expect(y).toBe(5019);
		expect(el.getBoundingClientRect().top).toBe(200);
		w.frame();
		w.frame();
		expect(w.pending()).toBe(0);
		vi.unstubAllGlobals();
	});
	it('lets go of a title that left the list or moved far (a new sort)', () => {
		const el = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 300, height: 300, bottom: 600 }) };
		vi.stubGlobal('innerHeight', 900);
		const by = vi.fn();
		vi.stubGlobal('scrollBy', by);
		const keeper = placeKeeper(() => ({ children: [el] }) as unknown as HTMLElement, () => null);
		keeper.note();
		keeper.keep();
		expect(by).not.toHaveBeenCalled();
		vi.unstubAllGlobals();
	});
});

describe('holdFocus', () => {
	const setup = (focusedId: string) => {
		const li = { dataset: { titleId: focusedId } };
		const active = { closest: () => li };
		const grid = { contains: () => true, focus: vi.fn() };
		vi.stubGlobal('document', { activeElement: active });
		return grid;
	};
	it('moves focus to the grid when the focused card is about to stop being drawn', () => {
		const grid = setup('9');
		holdFocus(grid as unknown as HTMLElement, [{ id: 1 }, { id: 2 }]);
		expect(grid.focus).toHaveBeenCalledWith({ preventScroll: true });
		vi.unstubAllGlobals();
	});
	it('leaves focus alone while the focused card stays drawn', () => {
		const grid = setup('2');
		holdFocus(grid as unknown as HTMLElement, [{ id: 1 }, { id: 2 }]);
		expect(grid.focus).not.toHaveBeenCalled();
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper across a window resize', () => {
	it('puts back the place seen before the browser re-laid the cards out at the new width', () => {
		const w = fakeWindow();
		let y = 5000;
		let shift = 0;
		const el = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 5200 + shift - y, height: 300, bottom: 5500 + shift - y }) };
		const grid = { children: [el], querySelector: () => el } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 0);
		// Seen while scrolling: the title 200 px from the top.
		keeper.mark();
		// The window narrows: the cards above grow taller before the columns change.
		shift = 37;
		keeper.note();
		keeper.keep();
		expect(el.getBoundingClientRect().top).toBe(200);
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper never lands on a wrong place', () => {
	it('does not give up at the end of the page while the title is not drawn yet (a guess can overshoot)', () => {
		let y = 0;
		const step = holder(150, { y: () => y, by: (d) => (y = Math.max(0, y + d)) });
		expect(step(null, -5000)).toBe(false);
		expect(step(null, 5150)).toBe(false);
		expect(y).toBe(5000);
	});
	it('forgets a remembered place whose title is no longer drawn (the list was scrolled far away since)', () => {
		const w = fakeWindow();
		let y = 0;
		const far = { dataset: { titleId: '1' }, getAttribute: () => '1', getBoundingClientRect: () => ({ top: 87 - y, height: 300, bottom: 387 - y }) };
		const here = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 5200 - y, height: 300, bottom: 5500 - y }) };
		let drawn = [far];
		const grid = { get children() { return drawn; }, querySelector: (s: string) => drawn.find((e) => s.includes(`"${e.dataset.titleId}"`)) ?? null } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, (p) => (p.id === 1 ? 87 - y : 5200 - y));
		keeper.mark();
		// Jumped far down without a mark in between: only the new cards are drawn.
		y = 5000;
		drawn = [here];
		keeper.note();
		keeper.keep();
		expect(y).toBe(5000);
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper after a jump', () => {
	it('does not go back to a place noted before the page scrolled, even while its old cards are still drawn', () => {
		const w = fakeWindow();
		let y = 0;
		const first = { dataset: { titleId: '1' }, getAttribute: () => '1', getBoundingClientRect: () => ({ top: 87 - y, height: 300, bottom: 387 - y }) };
		const grid = { children: [first], querySelector: () => first } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		const by = vi.fn(({ top }: { top: number }) => (y += top));
		vi.stubGlobal('scrollBy', by);
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, (p) => 87 - y + p.at);
		keeper.mark();
		// A to Z jumps to T; the rows change in the same update, before the old cards are replaced.
		y = 118481;
		keeper.note();
		keeper.keep();
		expect(by).not.toHaveBeenCalled();
		expect(y).toBe(118481);
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper when the update has already run its after-step', () => {
	it('still keeps (and so lets go of) a noted place, right after the update, so it never waits for a later change', async () => {
		const w = fakeWindow();
		let y = 0;
		let shift = 0;
		const el = { dataset: { titleId: '67' }, getAttribute: () => '1', getBoundingClientRect: () => ({ top: 87 + shift - y, height: 300, bottom: 387 + shift - y }) };
		const grid = { children: [el], querySelector: () => el } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 87 - y);
		// The rows change; only the before-step runs for it.
		keeper.note();
		shift = 5;
		await Promise.resolve();
		expect(y).toBe(5);
		w.frame();
		w.frame();
		w.frame();
		// Settled and let go: a later change notes afresh instead of reusing the old place.
		expect(w.pending()).toBe(0);
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper while the window is being resized', () => {
	it('does not let a scroll the browser makes during the resize replace the place seen before it', () => {
		const w = fakeWindow();
		let y = 5000;
		let width = 1000;
		const a = { dataset: { titleId: '8' }, getAttribute: () => '8', getBoundingClientRect: () => ({ top: 5120 - y, height: 380, bottom: 5500 - y }) };
		const b = { dataset: { titleId: '4' }, getAttribute: () => '4', getBoundingClientRect: () => ({ top: 4734 - y, height: 380, bottom: 5114 - y }) };
		let drawn = [a];
		const grid = { get children() { return drawn; }, querySelector: (s: string) => [a, b].find((e) => s.includes(`"${e.dataset.titleId}"`)) ?? null, get clientWidth() { return width; } } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 0);
		keeper.mark();
		// The window widens: the browser moves the page, and the row above now shows at the top.
		width = 1300;
		y = 4614;
		drawn = [b, a];
		keeper.mark();
		keeper.note();
		keeper.keep();
		expect(a.getBoundingClientRect().top).toBe(120);
		vi.unstubAllGlobals();
	});
});

describe('holder settles only on the drawn title', () => {
	it('is not done while only the guessed row is in place and the title is not drawn yet', () => {
		let y = 1000;
		const step = holder(120, { y: () => y, by: (d) => (y += d) });
		expect(step(null, 120.2)).toBe(false);
		expect(step(null, 120.2)).toBe(false);
		expect(step(null, 120.2)).toBe(false);
		expect(step(120.3, 0)).toBe(false);
		expect(step(120.3, 0)).toBe(true);
	});
});

describe('placeKeeper on a short window', () => {
	it('keeps the place seen before a resize that moves the cards further than one screen', () => {
		const w = fakeWindow();
		let y = 5000;
		let shift = 0;
		const el = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 5120 + shift - y, height: 300, bottom: 5420 + shift - y }) };
		const grid = { children: [el], querySelector: () => el, clientWidth: 1000 } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 720);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 0);
		keeper.mark();
		shift = -760;
		keeper.note();
		keeper.keep();
		expect(el.getBoundingClientRect().top).toBe(120);
		vi.unstubAllGlobals();
	});
});

// A poster grid of 2000 titles, 400 px a row, 100 px below the top of the page; `cols` columns.
function posterGrid(cols: number) {
	const g = { cols, y: 0 };
	const rowOf = (i: number) => 100 + Math.floor(i / g.cols) * 400 - g.y;
	const cards = Array.from({ length: 2000 }, (_, i) => ({ dataset: { titleId: String(i) }, getAttribute: () => String(i + 1), getBoundingClientRect: () => ({ top: rowOf(i), height: 380, bottom: rowOf(i) + 380 }) }));
	const grid = { children: cards, querySelector: (s: string) => cards[Number(/"(\d+)"/.exec(s)?.[1])] ?? null } as unknown as HTMLElement;
	return { g, grid, rowOf, topOf: (id: number) => rowOf(id) };
}

describe('placeKeeper over several column changes', () => {
	it('keeps the same title in the top row each time, instead of creeping to the start of its row', () => {
		const w = fakeWindow();
		const { g, grid, rowOf, topOf } = posterGrid(6);
		// Title 1236 starts its row, 150 px from the top.
		g.y = 100 + 206 * 400 - 150;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => g.y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (g.y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, (p) => rowOf(p.id));
		keeper.mark();
		for (const cols of [7, 8, 5]) {
			keeper.note();
			g.cols = cols;
			keeper.keep();
			for (let i = 0; i < 4; i++) w.frame();
			expect(w.pending()).toBe(0);
			expect(topOf(1236), `${cols} columns`).toBe(150);
		}
		vi.unstubAllGlobals();
	});
});

describe('placeKeeper when the page is moved during a hold', () => {
	it('does not restart a hold while the scrollbar is held, so a drag made since its last frame is not undone', () => {
		const w = fakeWindow();
		const { g, grid, rowOf, topOf } = posterGrid(6);
		g.y = 100 + 206 * 400 - 150;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => g.y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (g.y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, (p) => rowOf(p.id));
		keeper.mark();
		// A live update above: the hold starts and is still running.
		keeper.note();
		keeper.keep();
		expect(w.pending()).toBe(1);
		// The person presses the scrollbar (the browser sends pointerdown) and drags; a second live update comes before the page has said it scrolled.
		w.send('pointerdown');
		g.y += 300;
		const moved = g.y;
		keeper.note();
		keeper.keep();
		for (let i = 0; i < 4; i++) w.frame();
		expect(g.y).toBe(moved);
		expect(topOf(1236)).toBe(-150);
		// Let go of the scrollbar: the next change keeps the place again.
		w.send('pointerup');
		keeper.mark();
		keeper.note();
		g.y -= 40;
		keeper.keep();
		expect(topOf(1236)).toBe(-150);
		keeper.stop();
		vi.unstubAllGlobals();
	});
	it('still goes back to the title when the browser itself pulled the page to its new end (a phone turned: the rows got fewer)', () => {
		const w = fakeWindow();
		const { g, grid, rowOf, topOf } = posterGrid(2);
		const room = () => 100 + Math.ceil(2000 / g.cols) * 400 - 900;
		g.y = 100 + 618 * 400 - 150;
		vi.stubGlobal('innerHeight', 900);
		Object.defineProperty(globalThis, 'scrollY', { get: () => g.y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (g.y = Math.max(0, Math.min(room(), g.y + top))));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, (p) => rowOf(p.id));
		keeper.mark();
		// Turning the phone lays the cards out in one column for a moment: the hold follows the title far down.
		keeper.note();
		g.cols = 1;
		keeper.keep();
		expect(topOf(1236)).toBe(150);
		// Then three columns: the page gets shorter and the browser pulls it to the new end, before the next change is noted.
		g.cols = 3;
		g.y = Math.min(g.y, room());
		keeper.note();
		keeper.keep();
		for (let i = 0; i < 4; i++) w.frame();
		expect(topOf(1236)).toBe(150);
		vi.unstubAllGlobals();
	});
});

describe('the poster grid for screen readers', () => {
	it('says it is a list, since a list without bullets loses that in Safari (and with it the "n of m" of each card)', async () => {
		const { readFileSync } = await import('node:fs');
		const src = readFileSync(new URL('./LibraryList.svelte', import.meta.url), 'utf8');
		expect(src).toMatch(/<ul class="grid"[^>]*\brole="list"/);
	});
});

describe('placeKeeper when the after-step of a resize runs first', () => {
	it('still puts back the place seen before the resize', async () => {
		const w = fakeWindow();
		let y = 5000;
		let shift = 0;
		let width = 1000;
		const el = { dataset: { titleId: '42' }, getAttribute: () => '100', getBoundingClientRect: () => ({ top: 5120 + shift - y, height: 300, bottom: 5420 + shift - y }) };
		const grid = { children: [el], querySelector: () => el, get clientWidth() { return width; } } as unknown as HTMLElement;
		vi.stubGlobal('innerHeight', 720);
		Object.defineProperty(globalThis, 'scrollY', { get: () => y, configurable: true });
		vi.stubGlobal('scrollBy', ({ top }: { top: number }) => (y += top));
		for (const k of ['requestAnimationFrame', 'cancelAnimationFrame', 'addEventListener', 'removeEventListener'] as const) vi.stubGlobal(k, w[k]);
		const keeper = placeKeeper(() => grid, () => 0);
		keeper.mark();
		width = 700;
		shift = -60;
		keeper.keep();
		keeper.note();
		await Promise.resolve();
		expect(el.getBoundingClientRect().top).toBe(120);
		vi.unstubAllGlobals();
	});
});
