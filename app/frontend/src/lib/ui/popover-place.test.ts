import { describe, expect, it } from 'vitest';
import { placePanel, placeSide } from './popover-place.ts';

const view = { width: 1440, height: 900 };
const btn = (left: number, top: number, w = 100, h = 36) => ({ left, top, right: left + w, bottom: top + h });

describe('placePanel', () => {
	it('opens below, lined up with the start edge', () => {
		const p = placePanel(btn(100, 100), 220, 300, view, { align: 'start', placement: 'below' });
		expect(p).toMatchObject({ left: 100, top: 142, up: false, maxHeight: undefined, originX: 'left' });
	});
	it('end follows the reading direction: right edge in ltr, left edge in rtl', () => {
		expect(placePanel(btn(500, 100), 220, 300, view, { align: 'end' }).left).toBe(380);
		const r = placePanel(btn(500, 100), 220, 300, view, { align: 'end', rtl: true });
		expect(r.left).toBe(500);
		expect(r.originX).toBe('left');
		expect(placePanel(btn(500, 100), 220, 300, view, { align: 'start', rtl: true }).left).toBe(380);
	});
	it('flips up when there is more room above', () => {
		const p = placePanel(btn(100, 800), 220, 300, view, {});
		expect(p.up).toBe(true);
		expect(p.bottom).toBe(900 - 800 + 6);
	});
	it('caps its height to the room on the side it opens when neither side fits', () => {
		const p = placePanel(btn(100, 300), 220, 900, view, {});
		expect(p.up).toBe(false);
		expect(p.maxHeight).toBe(900 - 336 - 6 - 8);
	});
	it('stays 8px inside the viewport', () => {
		expect(placePanel(btn(1400, 100, 30), 220, 100, view, { align: 'start' }).left).toBe(1440 - 8 - 220);
		expect(placePanel(btn(0, 100, 30), 220, 100, view, { align: 'end' }).left).toBe(8);
	});
});

describe('placeSide', () => {
	const row = btn(10, 590, 240, 36);
	it('opens beside the row, 12px past its end, top aligned 8px above it', () => {
		expect(placeSide(row, 320, 200, view, false)).toMatchObject({ left: 262, top: 582, originX: 'left' });
	});
	it('opens on the left in right-to-left', () => {
		const r = placeSide(btn(1190, 590, 240, 36), 320, 200, view, true);
		expect(r).toMatchObject({ left: 1190 - 12 - 320, top: 582, originX: 'right' });
	});
	it('slides up to stay 8px above the bottom', () => {
		expect(placeSide(btn(10, 800, 240, 36), 320, 200, view, false).top).toBe(900 - 8 - 200);
	});
	it('caps its height when taller than the viewport', () => {
		const p = placeSide(row, 320, 2000, view, false);
		expect(p.top).toBe(8);
		expect(p.maxHeight).toBe(900 - 16);
	});
});
