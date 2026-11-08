import { cleanDisplayName, displayNameError, roleWord, shownName } from './profile.ts';
import { describe, expect, it } from 'vitest';

describe('display name', () => {
	it('trims and falls back to the username', () => {
		expect(cleanDisplayName('  Ana B.  ')).toBe('Ana B.');
		expect(shownName('  ', 'ana')).toBe('ana');
		expect(shownName('Ana', 'ana')).toBe('Ana');
	});
	it('refuses long names and control characters', () => {
		expect(displayNameError('x'.repeat(64))).toBe('');
		expect(displayNameError('x'.repeat(65))).toMatch(/64/);
		expect(displayNameError('a\u0007b')).toMatch(/plain/);
		expect(displayNameError('')).toBe('');
	});
	it('names roles in plain words', () => {
		expect(roleWord('owner')).toBe('Owner');
		expect(roleWord('member')).toBe('Member');
	});
});
