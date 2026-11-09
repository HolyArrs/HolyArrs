import { isNavigationKey } from './focus.ts';
import { describe, expect, it } from 'vitest';

const k = (key: string, mods: Partial<{ metaKey: boolean; ctrlKey: boolean; altKey: boolean }> = {}) => ({ key, metaKey: false, ctrlKey: false, altKey: false, ...mods });

describe('isNavigationKey', () => {
	it('counts Tab and the palette shortcut anywhere', () => {
		expect(isNavigationKey(k('Tab'), true)).toBe(true);
		expect(isNavigationKey(k('k', { metaKey: true }), true)).toBe(true);
		expect(isNavigationKey(k('K', { ctrlKey: true }), false)).toBe(true);
	});
	it('counts arrows outside a text field only', () => {
		expect(isNavigationKey(k('ArrowDown'), false)).toBe(true);
		expect(isNavigationKey(k('ArrowDown'), true)).toBe(false);
	});
	it('never counts typing', () => {
		expect(isNavigationKey(k('a'), true)).toBe(false);
		expect(isNavigationKey(k('a'), false)).toBe(false);
		expect(isNavigationKey(k('c', { metaKey: true }), false)).toBe(false);
	});
});
