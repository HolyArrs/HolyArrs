import { describe, expect, it } from 'vitest';
import { scrimFrames } from './theater.svelte.ts';

// Closing: the shrink fades the scrim out, then the player fades. That last fade
// started the scrim back at full (86% black) and showed a black blink.
describe('scrimFrames', () => {
	it('never brings the scrim back after the shrink faded it', () => {
		const shrink = scrimFrames(0, 1);
		const fadeOut = scrimFrames(0, shrink.at(-1)!.opacity);
		expect(fadeOut.every((f) => f.opacity === 0)).toBe(true);
	});
	it('fades in and out from where it stands', () => {
		expect(scrimFrames(1, 0)).toEqual([{ opacity: 0 }, { opacity: 1 }]);
		expect(scrimFrames(0, 1)).toEqual([{ opacity: 1 }, { opacity: 0 }]);
	});
});
