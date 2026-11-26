import { beforeEach, describe, expect, it, vi } from 'vitest';
import { previewDelay, previewPref, setPreviewPref, SLOW_PREVIEW_DELAY } from './trailer.ts';

describe('hover preview choice (#541)', () => {
	beforeEach(() => {
		const m = new Map<string, string>();
		vi.stubGlobal('localStorage', { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k), get length() { return m.size; } });
	});

	it.each([
		['on', 400, 400],
		['on', 140, 140],
		['slow', 400, SLOW_PREVIEW_DELAY],
		['slow', 1500, 1500],
		['off', 400, null]
	] as const)('%s with a %i ms base waits %s', (pref, base, want) => {
		expect(previewDelay(base, pref)).toBe(want);
	});

	it('defaults to on and keeps nothing stored for it', () => {
		expect(previewPref()).toBe('on');
		setPreviewPref('off');
		expect(previewPref()).toBe('off');
		setPreviewPref('on');
		expect(localStorage.length).toBe(0);
	});

	it('reads a junk stored value as on', () => {
		localStorage.setItem('holyarrs.preview.hover', 'sometimes');
		expect(previewPref()).toBe('on');
	});
});
