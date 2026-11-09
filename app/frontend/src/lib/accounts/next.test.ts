import { safeNext } from './next.ts';
import { describe, expect, it } from 'vitest';

describe('sign-in next link', () => {
	it('keeps paths on this site', () => {
		expect(safeNext('/library/12?x=1')).toBe('/library/12?x=1');
		expect(safeNext(null)).toBe('/');
	});
	it('refuses other sites and the sign-in page', () => {
		for (const bad of ['//evil.test', '/\\evil.test', '/\t/evil.test', 'https://evil.test', 'javascript:alert(1)', '/login?next=/x', '']) {
			expect(safeNext(bad)).toBe('/');
		}
	});
});
