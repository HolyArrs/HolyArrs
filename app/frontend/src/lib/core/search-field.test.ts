import { afterEach, describe, expect, it, vi } from 'vitest';
import { focusOnOpen, registerField } from './search-field.svelte.ts';

// A visible input stand-in that records focus calls.
function fakeField() {
	return { offsetParent: {}, getClientRects: () => [1], focus: vi.fn(), select: vi.fn() } as unknown as HTMLInputElement;
}

function touch(on: boolean) {
	vi.stubGlobal('matchMedia', (q: string) => ({ matches: on && q.includes('coarse') }));
}

describe('focusOnOpen', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('focuses the field on a computer', () => {
		touch(false);
		const el = fakeField();
		const undo = registerField(el);
		expect(focusOnOpen()).toBe(true);
		expect(el.focus).toHaveBeenCalled();
		undo();
	});

	it('leaves the field alone on a phone so the keyboard stays down', () => {
		touch(true);
		const el = fakeField();
		const undo = registerField(el);
		expect(focusOnOpen()).toBe(false);
		expect(el.focus).not.toHaveBeenCalled();
		undo();
	});
});
