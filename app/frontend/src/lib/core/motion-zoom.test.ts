import { describe, expect, it } from 'vitest';
import { flip, radiusFor, shift } from './motion.ts';

const card = { left: 300, top: 200, width: 300, height: 200 };
const page = { left: 240, top: 64, width: 1200, height: 836 };

describe('settings motion', () => {
	it('flip maps the page onto the card from its top-left corner', () => {
		expect(flip(card, page)).toBe('translate(60px, 136px) scale(0.25, 0.239)');
		expect(flip(page, page)).toBe('translate(0px, 0px) scale(1, 1)');
	});
	it('shift moves without scaling', () => {
		expect(shift(page, card)).toBe('translate(-60px, -136px)');
	});
	it('radiusFor stays soft at both ends', () => {
		expect(radiusFor(card, page)).toBe(32);
		expect(radiusFor(page, page)).toBe(16);
		expect(radiusFor({ ...card, width: 0 }, page)).toBeLessThanOrEqual(72);
	});
});
