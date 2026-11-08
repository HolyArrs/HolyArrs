import { contrast, deepen, dominant, FALLBACK, hexRgb, paintWindow, rgbHsl, tintOf, unpaintWindow } from './album-tint.ts';
import { describe, expect, it } from 'vitest';

const covers = ['#3fae2a', '#d62839', '#2a5fe0', '#111111', '#f5f5f5', '#ffd400', '#e0e0ff', '#8a2be2', '#00ffff', '#7a5c3a'];

describe('album tint', () => {
	it('keeps white text at 7:1 and the soft text at 4.5:1 on every cover colour', () => {
		for (const c of covers) {
			const t = tintOf(c);
			expect(t.contrast, c).toBeGreaterThanOrEqual(7);
			expect(t.softContrast, c).toBeGreaterThanOrEqual(4.5);
			expect(contrast([255, 255, 255], hexRgb(t.band)!), c).toBeGreaterThanOrEqual(7);
		}
	});

	it('keeps the cover hue: green stays green, red red, blue blue', () => {
		const hue = (c: string) => rgbHsl(hexRgb(deepen(c))!)[0];
		expect(hue('#3fae2a')).toBeGreaterThan(80);
		expect(hue('#3fae2a')).toBeLessThan(140);
		expect(hue('#d62839') > 340 || hue('#d62839') < 15).toBe(true);
		expect(hue('#2a5fe0')).toBeGreaterThan(200);
		expect(hue('#2a5fe0')).toBeLessThan(240);
	});

	it('turns a grey or dark cover near black, and a missing one into the fallback', () => {
		expect(rgbHsl(hexRgb(deepen('#f5f5f5'))!)[2]).toBeLessThan(0.15);
		expect(rgbHsl(hexRgb(deepen('#111111'))!)[2]).toBeLessThan(0.15);
		expect(deepen(null)).toBe(FALLBACK);
		expect(deepen('nope')).toBe(FALLBACK);
	});

	it('picks the main colour from pixels, not the white margin', () => {
		const px: number[] = [];
		for (let i = 0; i < 60; i++) px.push(255, 255, 255, 255);
		for (let i = 0; i < 40; i++) px.push(200, 30, 40, 255);
		const [h] = rgbHsl(hexRgb(dominant(px)!)!);
		expect(h > 340 || h < 15).toBe(true);
		expect(dominant([])).toBeNull();
	});
});

describe('paintWindow / unpaintWindow', () => {
	const fakeRoot = () => {
		const props = new Map<string, string>();
		return { props, el: { dataset: {} as Record<string, string>, style: { setProperty: (k: string, v: string) => props.set(k, v), removeProperty: (k: string) => props.delete(k) } } as unknown as HTMLElement };
	};
	it('sets the tint, then fades back and clears everything', async () => {
		const { props, el } = fakeRoot();
		const t = tintOf('#3a7a2a');
		paintWindow(t, el);
		expect(el.dataset.tint).toBe('album');
		expect(props.get('--tint-bg')).toBe(t.bg);
		expect(props.get('--tint-bg-2')).toBe(t.band);
		unpaintWindow(el, 5);
		expect(el.dataset.tint).toBe('leaving');
		await new Promise((r) => setTimeout(r, 20));
		expect(el.dataset.tint).toBeUndefined();
		expect(props.size).toBe(0);
	});
	it('a new album during the fade keeps its colour', async () => {
		const { props, el } = fakeRoot();
		paintWindow(tintOf('#a02020'), el);
		unpaintWindow(el, 5);
		const next = tintOf('#2040a0');
		paintWindow(next, el);
		await new Promise((r) => setTimeout(r, 20));
		expect(el.dataset.tint).toBe('album');
		expect(props.get('--tint-bg')).toBe(next.bg);
	});
});
