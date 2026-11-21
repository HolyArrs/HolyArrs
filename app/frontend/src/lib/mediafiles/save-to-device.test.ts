import { afterEach, describe, expect, it } from 'vitest';
import { session } from '#lib/core/session.svelte.ts';
import { saveHref } from './save-to-device.ts';

describe('Save to device', () => {
	afterEach(() => {
		Object.assign(session.s, { role: 'member', perms: [], held: [], maxRating: '' });
	});
	it('links the file for a person with save_files', () => {
		Object.assign(session.s, { role: 'member', perms: ['ask', 'save_files'] });
		expect(saveHref(42)).toMatch(/\/api\/v1\/files\/42\/download$/);
	});
	it('links it for the owner, who has every permission', () => {
		Object.assign(session.s, { role: 'owner', perms: [] });
		expect(saveHref(7)).toMatch(/\/api\/v1\/files\/7\/download$/);
	});
	it('offers nothing without the permission, without a file, to a kid or while held back', () => {
		Object.assign(session.s, { role: 'member', perms: ['ask'] });
		expect(saveHref(42)).toBe('');
		Object.assign(session.s, { perms: ['ask', 'save_files'] });
		expect(saveHref(undefined)).toBe('');
		expect(saveHref(0)).toBe('');
		Object.assign(session.s, { maxRating: 'PG' });
		expect(saveHref(42)).toBe('');
		Object.assign(session.s, { maxRating: '', held: ['youtube'] });
		expect(saveHref(42)).toBe('');
	});
});
