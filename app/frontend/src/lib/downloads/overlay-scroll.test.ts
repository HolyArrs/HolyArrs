import { dragTo, overlayScroll, thumb } from './overlay-scroll.ts';
import { afterEach, describe, expect, it, vi } from 'vitest';

// No DOM library in this repo's tests; a tiny fake element is enough for mount and unmount.
class El {
	children: El[] = [];
	parentElement: El | null = null;
	attrs = new Map<string, string>();
	cls = new Set<string>();
	className = '';
	hidden = false;
	clientHeight = 400; clientWidth = 800; scrollHeight = 2000; scrollWidth = 800; scrollTop = 0; scrollLeft = 0;
	offsetParent = null; offsetHeight = 28;
	style = { position: '', setProperty() {}, removeProperty() {} };
	classList = { add: (c: string) => this.cls.add(c), remove: (c: string) => this.cls.delete(c), contains: (c: string) => this.cls.has(c) };
	get firstElementChild() { return this.children[0] ?? null; }
	append(...k: El[]) { for (const c of k) { c.parentElement = this; this.children.push(c); } }
	remove() { const p = this.parentElement; if (p) p.children = p.children.filter((c) => c !== this); this.parentElement = null; }
	setAttribute(k: string, v: string) { this.attrs.set(k, v); }
	removeAttribute(k: string) { this.attrs.delete(k); }
	toggleAttribute(k: string, on: boolean) { if (on) this.attrs.set(k, ''); else this.attrs.delete(k); }
	addEventListener() {}
	removeEventListener() {}
	querySelector() { return null; }
}

function setup(fine: boolean) {
	vi.stubGlobal('matchMedia', () => ({ matches: fine, addEventListener() {}, removeEventListener() {} }));
	vi.stubGlobal('document', { createElement: () => new El() });
	vi.stubGlobal('requestAnimationFrame', () => 1);
	vi.stubGlobal('cancelAnimationFrame', () => {});
	const col = new El(), box = new El();
	col.append(box);
	return { col, box };
}
const rails = (col: El) => col.children.filter((c) => c.className.startsWith('osb-rail'));

afterEach(() => vi.unstubAllGlobals());

describe('thumb', () => {
	it('keeps at least 32px and runs from 0 to track - len', () => {
		const top = thumb(300, 400, 100000, 0);
		expect(top.len).toBe(32);
		expect(top.pos).toBe(0);
		expect(thumb(300, 400, 100000, 100000 - 400).pos).toBe(300 - 32);
		const mid = thumb(300, 400, 800, 200);
		expect(mid.len).toBe(150);
		expect(mid.pos).toBe(75);
	});
	it('clamps overscroll and rounds to whole pixels', () => {
		expect(thumb(300, 400, 2000, -50).pos).toBe(0);
		expect(thumb(300, 400, 2000, 99999).pos).toBe(300 - 60);
		expect(Number.isInteger(thumb(333, 397, 2011, 123).pos)).toBe(true);
	});
	it('hides an axis with no overflow', () => {
		expect(thumb(300, 400, 400, 0).hidden).toBe(true);
		expect(thumb(300, 400, 401, 0).hidden).toBe(true);
		expect(thumb(300, 400, 402, 0).hidden).toBe(false);
	});
	it('maps a drag back to scroll distance', () => {
		const t = thumb(300, 400, 2000, 0);
		expect(dragTo(0, 300 - t.len, 300, t.len, 400, 2000)).toBe(1600);
		expect(dragTo(10, 5, 32, 32, 400, 2000)).toBe(10);
	});
});

describe('overlayScroll action', () => {
	it('mounts two rails outside the scroller and destroy() removes them and the class', () => {
		const { col, box } = setup(true);
		const a = overlayScroll(box as unknown as HTMLElement, { header: '.chead' });
		expect(rails(col)).toHaveLength(2);
		expect(box.children).toHaveLength(0);
		expect(box.cls.has('osb-on')).toBe(true);
		a.destroy();
		expect(rails(col)).toHaveLength(0);
		expect(box.cls.has('osb-on')).toBe(false);
	});
	it('mounts nothing for a coarse pointer', () => {
		const { col, box } = setup(false);
		overlayScroll(box as unknown as HTMLElement);
		expect(rails(col)).toHaveLength(0);
		expect(box.cls.has('osb-on')).toBe(false);
	});
});
