import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { KEY, defaults, load, visible } from './columns.ts';
import { createColumns } from './columns-grid.svelte.ts';
import type { DownloadRow } from '../core/types.ts';

function fakeStorage(seed: Record<string, string> = {}) {
	const m = new Map(Object.entries(seed));
	return {
		getItem: (k: string) => m.get(k) ?? null,
		setItem: (k: string, v: string) => void m.set(k, String(v)),
		removeItem: (k: string) => void m.delete(k)
	};
}

const rows = (n: number) => Array.from({ length: n }, (_, i) => ({ id: i }) as unknown as DownloadRow);
const key = (k: string, extra: Record<string, unknown> = {}) => ({ key: k, shiftKey: false, preventDefault() {}, stopPropagation() {}, ...extra }) as any;

// Vitest runs in node, where Svelte compiles for the server: runes become plain values and $effect does nothing. That is enough here, since the logic under test is state, deriveds and the save calls. The grid's measuring effects need a real browser (the e2e run covers them).
function make(opts: { done?: boolean; list?: DownloadRow[] } = {}) {
	return createColumns({ isDone: () => !!opts.done, list: () => opts.list ?? [] });
}
const saved = () => load(localStorage.getItem(KEY), null);

describe('createColumns', () => {
	beforeEach(() => {
		vi.stubGlobal('localStorage', fakeStorage());
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('starts from the default columns and widths', () => {
		const c = make();
		expect(c.vis).toEqual(visible(defaults()));
		expect(c.customised).toBe(false);
		expect(c.layout.widths).toEqual(defaults().widths);
	});

	it('reads a saved layout from this browser', () => {
		const l = defaults();
		l.hidden = [...l.hidden, 'size'];
		vi.stubGlobal('localStorage', fakeStorage({ [KEY]: JSON.stringify({ v: 3, order: l.order, hidden: l.hidden, widths: l.widths, titleSet: false }) }));
		const c = make();
		expect(c.vis).not.toContain('size');
		expect(c.customised).toBe(true);
	});

	it('flip shows a hidden column, hides it again and saves each time', () => {
		const c = make();
		c.flip('swarm');
		expect(c.vis).toContain('swarm');
		expect(saved().hidden).not.toContain('swarm');
		c.flip('swarm');
		expect(c.vis).not.toContain('swarm');
		expect(saved().hidden).toContain('swarm');
	});

	it('finished rows show their own short set until the layout is customised', () => {
		expect(make({ done: true }).vis).toEqual(['poster', 'title', 'proto', 'status', 'size', 'added', 'requested']);
		const c = make({ done: true });
		c.flip('swarm');
		expect(c.vis).toContain('swarm');
		expect(c.vis).not.toContain('added');
	});

	it('resize widens a column; the keyboard nudge saves it', () => {
		const c = make();
		c.resize('size', 60, 30);
		expect(c.layout.widths.size).toBe(90);
		expect(saved().widths.size).toBe(60); // Not saved until the drag ends.
		c.nudge(key('ArrowRight', { currentTarget: { parentElement: { getBoundingClientRect: () => ({ width: 90 }) } } }), 'size');
		expect(c.layout.widths.size).toBe(100);
		expect(saved().widths.size).toBe(100);
	});

	it('resize stops at the column minimum and maximum', () => {
		const c = make();
		c.resize('status', 120, -500);
		expect(c.layout.widths.status).toBe(40);
		c.resize('status', 120, 99999);
		expect(c.layout.widths.status).toBe(3000);
	});

	it('Poster, Type and Downloader snap to their own sizes', () => {
		const c = make();
		c.resize('poster', 32, 20);
		expect(c.posterW).toBe(46);
		c.resize('kind', 24, 40);
		expect(c.kindW).toBe(60);
		expect(c.kindWide).toBe(true);
		c.resize('proto', 24, 100);
		expect(c.srcW).toBe(92);
		expect(c.srcName).toBe(true);
		c.fitOne('poster');
		expect(c.posterW).toBe(62);
		c.fitOne('poster');
		expect(c.posterW).toBe(32);
	});

	it('moving a column in the picker changes the order and saves it', () => {
		const c = make();
		c.handleKey(key(' '), 'size');
		expect(c.held).toBe('size');
		const from = c.layout.order.indexOf('size');
		c.handleKey(key('ArrowUp'), 'size');
		expect(c.layout.order.indexOf('size')).toBe(from - 1);
		c.handleKey(key('Enter'), 'size');
		expect(c.held).toBeNull();
		expect(saved().order).toEqual(c.layout.order);
	});

	it('Escape puts a picked-up column back where it was', () => {
		const c = make();
		const before = [...c.layout.order];
		c.handleKey(key(' '), 'size');
		c.handleKey(key('ArrowUp'), 'size');
		c.handleKey(key('Escape'), 'size');
		expect(c.layout.order).toEqual(before);
		expect(c.held).toBeNull();
	});

	it('sortBy saves the sort; resetAll clears the saved layout', () => {
		const c = make();
		c.sortBy('size');
		expect(c.layout.sort?.id).toBe('size');
		expect(saved().sort?.id).toBe('size');
		c.flip('swarm');
		c.resetAll();
		expect(localStorage.getItem(KEY)).toBeNull();
		expect(c.layout.widths).toEqual(defaults().widths);
		expect(c.vis).toEqual(visible(defaults()));
	});

	it('survives a storage that refuses to write', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => null,
			setItem: () => {
				throw new Error('private window');
			},
			removeItem: () => {}
		});
		const c = make();
		expect(() => c.flip('swarm')).not.toThrow();
		expect(c.vis).toContain('swarm');
	});

	it('draws every row of a short list', () => {
		const c = make({ list: rows(100) });
		expect(c.windowed).toBe(false);
		expect(c.shownRows).toHaveLength(100);
	});

	it('draws only the rows on screen plus a margin for a long list', () => {
		const c = make({ list: rows(300) });
		expect(c.windowed).toBe(true);
		c.vH = 900;
		c.vTop = 0;
		expect(c.span).toEqual({ a: 0, b: 30 });
		expect(c.shownRows).toHaveLength(30);
		c.vTop = 4500;
		expect(c.span).toEqual({ a: 90, b: 130 });
		expect((c.shownRows[0] as any).id).toBe(90);
	});

	it('a phone row is taller than a desktop row by default', () => {
		const c = make({ list: rows(10) });
		expect(c.rh).toBe(45);
		c.phone = true;
		expect(c.rh).toBe(65);
	});

	it('a phone gets its fixed card cells even when the layout hides them', () => {
		const c = make();
		expect(c.cells.filter((x) => x.ph)).toEqual([]);
		c.flip('size');
		expect(c.cells.filter((x) => x.ph).map((x) => x.id)).toEqual(['size']);
	});

	it('names a heading with its column and sort', () => {
		const c = make();
		expect(c.headName('proto', undefined)).toBe('Downloader');
		expect(c.headName('swarm', undefined)).toBe('Seeds, peers and ratio');
		expect(c.headName('size', { desc: true })).toBe('Size, sorted high to low');
		expect(c.headName('size', { desc: false })).toBe('Size, sorted low to high');
	});
});
