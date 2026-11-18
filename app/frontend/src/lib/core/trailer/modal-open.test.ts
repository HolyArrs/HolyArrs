import { afterEach, describe, expect, it, vi } from 'vitest';
import { modalOpen } from './trailer.ts';

// A page whose only open dialog matches when `open` is true.
const page = (open: boolean) => vi.stubGlobal('document', { querySelector: (s: string) => (open && s === 'dialog[open]' ? {} : null) });

describe('modalOpen', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('is false with no page or no open dialog', () => {
		expect(modalOpen()).toBe(false);
		page(false);
		expect(modalOpen()).toBe(false);
	});

	it('is true while a dialog is open, so hover previews stay shut', () => {
		page(true);
		expect(modalOpen()).toBe(true);
	});
});
