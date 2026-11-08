import { pausesPortals } from './signin-portals.ts';
import { describe, it, expect } from 'vitest';

describe('pausesPortals', () => {
	it('pauses on a trusted printable or editing key in a field', () => {
		expect(pausesPortals({ isTrusted: true, key: 'a' }, true)).toBe(true);
		expect(pausesPortals({ isTrusted: true, key: 'Backspace' }, true)).toBe(true);
	});
	it('ignores synthetic events, focus keys, shortcuts and keys outside fields', () => {
		expect(pausesPortals({ isTrusted: false, key: 'a' }, true)).toBe(false);
		expect(pausesPortals({ isTrusted: true }, true)).toBe(false);
		expect(pausesPortals({ isTrusted: true, key: 'Tab' }, true)).toBe(false);
		expect(pausesPortals({ isTrusted: true, key: 'v', metaKey: true }, true)).toBe(false);
		expect(pausesPortals({ isTrusted: true, key: 'a' }, false)).toBe(false);
	});
});
