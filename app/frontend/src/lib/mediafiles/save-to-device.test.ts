import { afterEach, describe, expect, it } from 'vitest';
import { session } from '#lib/core/session.svelte.ts';
import { saveHref } from './save-to-device.ts';

describe('save to device', () => {
	afterEach(() => Object.assign(session.s, { role: 'member', perms: [], maxRating: '' }));

	it('links the file for someone given save_files, and for the owner', () => {
		Object.assign(session.s, { role: 'member', perms: ['ask', 'save_files'], maxRating: '' });
		expect(saveHref(7, 42)).toBe('/api/v1/titles/7/files/42/save');
		Object.assign(session.s, { role: 'owner', perms: [] });
		expect(saveHref(7, 42)).toBe('/api/v1/titles/7/files/42/save');
	});

	it('offers nothing without the permission, to a Kid, or without a file', () => {
		Object.assign(session.s, { role: 'admin', perms: ['ask', 'manage_library', 'settings'], maxRating: '' });
		expect(saveHref(7, 42)).toBe('');
		Object.assign(session.s, { role: 'member', perms: ['ask', 'save_files'], maxRating: 'PG' });
		expect(saveHref(7, 42)).toBe('');
		Object.assign(session.s, { maxRating: '' });
		expect(saveHref(7, undefined)).toBe('');
		expect(saveHref(undefined, 42)).toBe('');
	});
});
