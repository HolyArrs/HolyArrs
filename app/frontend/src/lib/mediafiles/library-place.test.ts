import { describe, expect, it, vi } from 'vitest';
import { fakeWindow } from './fake-window.ts';
import { anchorOf, lander, listWaits, placeGate, placeTop, reading, untilSettled } from './library-view.ts';
import { noFilter } from './table.ts';

const MODIFIERS = ['Shift', 'Control', 'Alt', 'AltGraph', 'Meta', 'CapsLock'];

describe('untilSettled', () => {
	it('steps at once and then once a frame until the step says it is done', () => {
		const w = fakeWindow();
		let steps = 0;
		let ended = 0;
		untilSettled(() => ++steps === 3, 4000, () => ended++, w as unknown as typeof globalThis);
		expect(steps).toBe(1);
		w.frame();
		w.frame();
		expect(steps).toBe(3);
		w.frame();
		expect(steps).toBe(3);
		expect(ended).toBe(1);
		expect(w.listening()).toBe(0);
	});
	it.each(['wheel', 'touchstart', 'keydown', 'pointerdown'])('stops on the first %s, so it never fights the person', (e) => {
		const w = fakeWindow();
		let steps = 0;
		untilSettled(() => (steps++, false), 4000, () => {}, w as unknown as typeof globalThis);
		w.send(e);
		w.frame();
		expect(steps).toBe(1);
		expect(w.pending()).toBe(0);
		expect(w.listening()).toBe(0);
	});
	it('stop cancels the waiting frame (the page is left)', () => {
		const w = fakeWindow();
		let steps = 0;
		const stop = untilSettled(() => (steps++, false), 4000, () => {}, w as unknown as typeof globalThis);
		stop();
		w.frame();
		expect(steps).toBe(1);
		expect(w.pending()).toBe(0);
	});
	it('gives up when the time is over', () => {
		const w = fakeWindow();
		let steps = 0;
		untilSettled(() => (steps++, false), 0, () => {}, w as unknown as typeof globalThis);
		w.frame();
		expect(steps).toBe(1);
	});
	it.each(MODIFIERS)('keeps going while %s alone is pressed or held (Alt held from Alt+Left going back)', (key) => {
		const w = fakeWindow();
		let steps = 0;
		untilSettled(() => (steps++, false), 4000, () => {}, w as unknown as typeof globalThis);
		w.send('keydown', { key });
		w.send('keydown', { key, repeat: true });
		w.frame();
		expect(steps).toBe(2);
		expect(w.listening()).toBeGreaterThan(0);
	});
	it('keeps going through a held key repeating, and stops at a real key', () => {
		const w = fakeWindow();
		let steps = 0;
		untilSettled(() => (steps++, false), 4000, () => {}, w as unknown as typeof globalThis);
		w.send('keydown', { key: 'ArrowLeft', repeat: true });
		w.frame();
		expect(steps).toBe(2);
		w.send('keydown', { key: 'ArrowDown' });
		w.frame();
		expect(steps).toBe(2);
		expect(w.listening()).toBe(0);
	});
});

describe('lander', () => {
	it('is done within a pixel of the place on a page that kept its height a frame, or at the end of a page that has not grown for half a second', () => {
		let now = 0;
		const there = lander(5000, 500, () => now);
		expect(there(5000.6, 9000)).toBe(false);
		expect(there(5000.6, 9000)).toBe(true);
		const short = lander(5000, 500, () => now);
		expect(short(4990, 9000)).toBe(false);
		expect(short(4990, 9000)).toBe(false);
		// The saved place is past the end (a title was removed, a filter is narrower): the end is as close as it gets, once the page stays that long.
		const end = lander(9500, 500, () => now);
		expect(end(9000, 9000)).toBe(false);
		now = 499;
		expect(end(9000, 9000)).toBe(false);
		now = 500;
		expect(end(9000, 9000)).toBe(true);
		// The page is still growing: the wait starts again.
		const grows = lander(9500, 500, () => now);
		expect(grows(8000, 8000)).toBe(false);
		now = 900;
		expect(grows(9000, 9000)).toBe(false);
		now = 1300;
		expect(grows(9000, 9000)).toBe(false);
	});
});

describe('restoring a place on a slow network', () => {
	it('waits while the data a filter or grouping needs is still on its way', () => {
		const none = { subs: false, sets: false };
		expect(listWaits({ ...noFilter, subs: true }, false, 'posters', { ...none, subs: true })).toBe(true);
		expect(listWaits({ ...noFilter, subs: true }, false, 'posters', { ...none, sets: true })).toBe(false);
		expect(listWaits({ ...noFilter, series: 4 }, false, 'posters', { ...none, sets: true })).toBe(true);
		expect(listWaits(noFilter, true, 'rows', { ...none, sets: true })).toBe(true);
		expect(listWaits(noFilter, true, 'table', { ...none, sets: true })).toBe(true);
		expect(listWaits(noFilter, false, 'rows', { subs: true, sets: true })).toBe(false);
		expect(listWaits({ ...noFilter, subs: true, series: 4 }, true, 'rows', none)).toBe(false);
	});
	it('does not wait for the series when only grouping asks for them in Posters, which are never grouped', () => {
		expect(listWaits(noFilter, true, 'posters', { subs: false, sets: true })).toBe(false);
		expect(listWaits({ ...noFilter, series: 4 }, true, 'posters', { subs: false, sets: true })).toBe(true);
	});
	it('gives the place once the list is whole', () => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, '/library?subs=missing|posters', w as unknown as typeof globalThis);
		expect(gate.take('/library?subs=missing|posters', true)).toBeNull();
		expect(gate.take('/library?subs=missing|posters', false)).toEqual({ x: 0, y: 9000 });
		// Once only, and it stops listening.
		expect(gate.take('/library?subs=missing|posters', false)).toBeNull();
		expect(w.listening()).toBe(0);
	});
	it.each(['wheel', 'touchstart', 'keydown', 'pointerdown'])('drops the place at the first %s while it waits', (e) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		expect(gate.take('k', true)).toBeNull();
		w.send(e);
		expect(gate.take('k', false)).toBeNull();
		expect(w.listening()).toBe(0);
	});
	it('drops the place when another list is shown than the one it was saved on (filter cleared, other kind or view)', () => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, '/library?kind=movie&subs=missing|posters', w as unknown as typeof globalThis);
		expect(gate.take('/library?kind=movie&subs=missing|posters', true)).toBeNull();
		expect(gate.take('/library?kind=movie|posters', false)).toBeNull();
		// Coming back to the first list later does not bring it back.
		expect(gate.take('/library?kind=movie&subs=missing|posters', false)).toBeNull();
		expect(w.listening()).toBe(0);
		const view = placeGate({ x: 0, y: 9000 }, '/library?kind=movie|posters', fakeWindow() as unknown as typeof globalThis);
		expect(view.take('/library?kind=movie|table', true)).toBeNull();
		expect(view.take('/library?kind=movie|posters', false)).toBeNull();
	});
	it('stop drops the place and stops listening (the page is left)', () => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		gate.stop();
		expect(gate.take('k', false)).toBeNull();
		expect(w.listening()).toBe(0);
		expect(w.pending()).toBe(0);
	});
	it.each(MODIFIERS)('keeps the place while %s alone is pressed or held', (key) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('keydown', { key });
		w.send('keydown', { key, repeat: true });
		expect(gate.take('k', false)).toEqual({ x: 0, y: 9000 });
		expect(w.listening()).toBe(0);
	});
	it('keeps the place through a held key repeating', () => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('keydown', { key: 'ArrowLeft', repeat: true });
		expect(gate.take('k', false)).toEqual({ x: 0, y: 9000 });
	});
	it.each(['ArrowDown', 'PageDown', ' '])('drops the place at a real key (%j)', (key) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('keydown', { key });
		expect(gate.take('k', false)).toBeNull();
		expect(w.listening()).toBe(0);
	});
	it('drops a place still held 8 s after the titles arrived, so a very slow list never pulls the page long after', () => {
		vi.useFakeTimers();
		try {
			const w = fakeWindow();
			const late = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
			// A big library can take longer than 8 s to load: that time does not count.
			expect(late.take('k', true, false)).toBeNull();
			vi.advanceTimersByTime(20000);
			expect(late.peek()).toEqual({ x: 0, y: 9000 });
			// The titles are here, the subtitles or series list is not: from now on 8 s.
			expect(late.take('k', true, true)).toBeNull();
			vi.advanceTimersByTime(7999);
			expect(late.take('k', true, true)).toBeNull();
			expect(late.peek()).toEqual({ x: 0, y: 9000 });
			vi.advanceTimersByTime(1);
			expect(late.take('k', false)).toBeNull();
			expect(w.listening()).toBe(0);
			// One that is given in time leaves no timer behind.
			const soon = placeGate({ x: 0, y: 9000 }, 'k', fakeWindow() as unknown as typeof globalThis);
			expect(soon.take('k', false)).toEqual({ x: 0, y: 9000 });
			expect(vi.getTimerCount()).toBe(0);
		} finally {
			vi.useRealTimers();
		}
	});
	it('peek shows the held place without giving it, so leaving again before the list is whole saves it rather than the top', () => {
		const gate = placeGate({ x: 0, y: 9000 }, 'k', fakeWindow() as unknown as typeof globalThis);
		expect(gate.peek()).toEqual({ x: 0, y: 9000 });
		expect(gate.peek()).toEqual({ x: 0, y: 9000 });
		expect(gate.take('k', false)).toEqual({ x: 0, y: 9000 });
		expect(gate.peek()).toBeNull();
		const dropped = placeGate({ x: 0, y: 9000 }, 'k', fakeWindow() as unknown as typeof globalThis);
		dropped.stop();
		expect(dropped.peek()).toBeNull();
	});
	it('drops the place when the page was moved more than half a screen with none of the watched events (a scrollbar drag, find in page)', () => {
		const w = fakeWindow();
		w.scrollY = 120;
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.frame();
		w.scrollY = 120 + 451;
		expect(gate.take('k', false)).toBeNull();
		// A small shift (the page settling as it loads) keeps it.
		const w2 = fakeWindow();
		const kept = placeGate({ x: 0, y: 9000 }, 'k', w2 as unknown as typeof globalThis);
		w2.frame();
		w2.scrollY = 300;
		expect(kept.take('k', false)).toEqual({ x: 0, y: 9000 });
	});
	it('peek also lets go of a place the page was moved away from, so leaving then saves where the person is', () => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.frame();
		w.scrollY = 451;
		expect(gate.peek()).toBeNull();
		expect(gate.take('k', false)).toBeNull();
		expect(w.listening()).toBe(0);
	});
	it.each([3, 4])('keeps the place through the mouse\'s back or forward button (button %i)', (button) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('pointerdown', { button });
		expect(gate.take('k', false)).toEqual({ x: 0, y: 9000 });
	});
	it.each([0, 1, 2])('drops the place at a mouse press with button %i', (button) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('pointerdown', { button });
		expect(gate.take('k', false)).toBeNull();
	});
	it.each([
		['Alt+ArrowLeft', { key: 'ArrowLeft', altKey: true }],
		['Alt+ArrowRight', { key: 'ArrowRight', altKey: true }],
		['Cmd+[', { key: '[', metaKey: true }],
		['Cmd+]', { key: ']', metaKey: true }],
		['Cmd+ArrowLeft', { key: 'ArrowLeft', metaKey: true }],
		['Cmd+ArrowRight', { key: 'ArrowRight', metaKey: true }]
	])('keeps the place through %s, the browser\'s back or forward keys', (_, init) => {
		const w = fakeWindow();
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		w.send('keydown', init);
		expect(gate.take('k', false)).toEqual({ x: 0, y: 9000 });
		// The same keys alone move the page.
		const w2 = fakeWindow();
		const plain = placeGate({ x: 0, y: 9000 }, 'k', w2 as unknown as typeof globalThis);
		w2.send('keydown', { key: init.key });
		expect(plain.take('k', false)).toBeNull();
	});
	it('the keys and buttons that only go back or forward do not stop a running restore either', () => {
		const w = fakeWindow();
		let steps = 0;
		untilSettled(() => (steps++, false), 4000, () => {}, w as unknown as typeof globalThis);
		w.send('pointerdown', { button: 3 });
		w.send('keydown', { key: 'ArrowRight', altKey: true });
		w.send('keydown', { key: ']', metaKey: true });
		w.frame();
		expect(steps).toBe(2);
		w.send('pointerdown', { button: 0 });
		w.frame();
		expect(steps).toBe(2);
	});
	it('a read that fails still ends the wait, so the place is put back', async () => {
		const w = fakeWindow();
		const waiting = { subs: false, sets: false };
		const gate = placeGate({ x: 0, y: 9000 }, 'k', w as unknown as typeof globalThis);
		let fail = (_: Error) => {};
		const subs = reading(waiting, 'subs', new Promise((_, no) => (fail = no))).catch(() => {});
		const sets = reading(waiting, 'sets', Promise.reject(new Error('offline'))).catch(() => {});
		expect(waiting).toEqual({ subs: true, sets: true });
		expect(gate.take('k', listWaits({ ...noFilter, subs: true, series: 4 }, false, 'posters', waiting))).toBeNull();
		await sets;
		expect(waiting.sets).toBe(false);
		expect(gate.take('k', listWaits({ ...noFilter, subs: true, series: 4 }, false, 'posters', waiting))).toBeNull();
		fail(new Error('offline'));
		await subs;
		expect(waiting).toEqual({ subs: false, sets: false });
		expect(gate.take('k', listWaits({ ...noFilter, subs: true, series: 4 }, false, 'posters', waiting))).toEqual({ x: 0, y: 9000 });
	});
	it('a read that answers ends the wait and passes its answer on', async () => {
		const waiting = { subs: false, sets: false };
		await expect(reading(waiting, 'sets', Promise.resolve(7))).resolves.toBe(7);
		expect(waiting.sets).toBe(false);
	});
	it('does not give up while the list is short for a moment, and lands once the data arrives (delayed subtitles or series)', () => {
		const w = fakeWindow();
		let now = 0;
		let y = 0;
		let room = 2000;
		const at = lander(9000, 500, () => now);
		let done = false;
		untilSettled(() => ((y = Math.min(9000, room)), at(y, room)), 4000, () => (done = true), w as unknown as typeof globalThis);
		for (now = 16; now <= 400; now += 16) w.frame();
		expect(done).toBe(false);
		// The delayed list arrives: the page grows past the place.
		room = 20000;
		w.frame();
		w.frame();
		expect(done).toBe(true);
		expect(y).toBe(9000);
	});
	it('puts a place saved at another list width (a turned phone, a resized window, the sidebar shown or hidden) back by its title, at the height it had', () => {
		const rows = new Map([[42, 30000]]);
		const rowTop = (id: number) => rows.get(id) ?? null;
		const place = { y: 48455, w: 1192, a: { id: 42, top: 249 } };
		// The same list width: the same spot.
		expect(placeTop(place, 1192, rowTop)).toBe(48455);
		// Another width: the title's row, as high on screen as the title was.
		expect(placeTop(place, 912, rowTop)).toBe(29751);
		// The rows are measured as they come into view: the place follows them.
		rows.set(42, 30250);
		expect(placeTop(place, 912, rowTop)).toBe(30001);
		// No title kept with the place, or it is not in this list, or a place saved before the width was kept: the same spot.
		expect(placeTop({ y: 48455, w: 1192 }, 912, rowTop)).toBe(48455);
		expect(placeTop({ ...place, a: { id: 7, top: 249 } }, 912, rowTop)).toBe(48455);
		expect(placeTop({ y: 48455, a: place.a }, 912, rowTop)).toBe(48455);
		rows.set(42, 50);
		expect(placeTop({ ...place, y: 900 }, 912, rowTop)).toBe(0);
		// A screen that is now shorter (a phone turned on its side): no lower than its middle, so the title is not below the fold.
		rows.set(42, 30000);
		expect(placeTop({ ...place, a: { id: 42, top: 600 } }, 912, rowTop, 390)).toBe(29805);
		expect(placeTop({ ...place, a: { id: 42, top: 600 } }, 912, rowTop, 1400)).toBe(29400);
		expect(placeTop({ ...place, a: { id: 42, top: -30 } }, 912, rowTop, 390)).toBe(30030);
	});
	it('keeps a place by the title being opened when it is drawn, else by the first title on screen', () => {
		const card = (id: number, top: number) => ({ dataset: { titleId: String(id) }, getAttribute: () => '1', getBoundingClientRect: () => ({ top, height: 300 }) }) as unknown as Element;
		const cards = [card(5, -400), card(6, 80), card(7, 420)];
		// Opening a title: that title, where it is.
		expect(anchorOf(cards, 900, 7)).toEqual({ id: 7, top: 420 });
		// Leaving for another page (an older opened title is not on screen): the first title on screen.
		expect(anchorOf(cards, 900, null)).toEqual({ id: 6, top: 80 });
		expect(anchorOf(cards, 900, 99)).toEqual({ id: 6, top: 80 });
		expect(anchorOf([], 900, null)).toBeNull();
	});
	it('lands on a place that moves while the rows are measured', () => {
		const at = lander(29880, 500, () => 0);
		expect(at(30130, 80000, 30130)).toBe(false);
		expect(at(30130, 80000, 30130)).toBe(true);
	});
	it('is not done on the frame the list first appears, while its rows are still laid out again at the measured width', () => {
		const at = lander(48455, 500, () => 0);
		// The place is reached on a page laid out from a guessed width...
		expect(at(48455, 83606)).toBe(false);
		// ...which the measured width then shortens (and something else moved the page meanwhile).
		expect(at(46826, 80758)).toBe(false);
		// Put back on the settled page: done.
		expect(at(48455, 80758)).toBe(true);
	});
});
