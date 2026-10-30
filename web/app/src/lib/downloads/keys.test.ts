import { describe, expect, it } from 'vitest';
import { escStep, reviewShown, searchShortcut } from './keys.ts';

const k = (key: string, mods: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean }> = {}) => ({ key, ctrlKey: false, metaKey: false, altKey: false, ...mods });

describe('search shortcuts', () => {
	it('Ctrl K and ⌘K focus the search, even from a text field', () => {
		expect(searchShortcut(k('k', { ctrlKey: true }), false, false)).toBe(true);
		expect(searchShortcut(k('K', { metaKey: true }), true, false)).toBe(true);
	});
	it('Ctrl K never steals focus out of an open dialog or sheet', () => {
		expect(searchShortcut(k('k', { ctrlKey: true }), false, true)).toBe(false);
		expect(searchShortcut(k('k', { metaKey: true }), true, true)).toBe(false);
	});
	it('"/" focuses the search only outside text fields and dialogs', () => {
		expect(searchShortcut(k('/'), false, false)).toBe(true);
		expect(searchShortcut(k('/'), true, false)).toBe(false);
		expect(searchShortcut(k('/'), false, true)).toBe(false);
	});
	it('ignores other keys and Alt combinations', () => {
		expect(searchShortcut(k('/', { ctrlKey: true }), false, false)).toBe(false);
		expect(searchShortcut(k('k', { ctrlKey: true, altKey: true }), false, false)).toBe(false);
		expect(searchShortcut(k('k'), false, false)).toBe(false);
		expect(searchShortcut(k('a', { ctrlKey: true }), false, false)).toBe(false);
	});
});

describe('review call', () => {
	it('watchers never see the review count', () => {
		expect(reviewShown(true, 3)).toBe(0);
		expect(reviewShown(false, 3)).toBe(3);
		expect(reviewShown(false, 0)).toBe(0);
	});
});

describe('search Esc', () => {
	it('clears the words first, then leaves the field', () => {
		expect(escStep('dune')).toBe('clear');
		expect(escStep('')).toBe('blur');
	});
});
