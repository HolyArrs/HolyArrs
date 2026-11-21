import { afterEach, describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import { mayOpen, plainMessage, sections, setHeld, visible } from './settings.ts';

describe('plainMessage', () => {
	it('starts server text with a capital and ends it with a stop', () => {
		expect(plainMessage(new ApiError('your account may not do this; ask the owner', 403))).toBe('Your account may not do this; ask the owner.');
	});
	it('keeps text that already ends a sentence', () => {
		expect(plainMessage(new ApiError('HolyArrs is not answering. Check that it is running.', 0))).toBe('HolyArrs is not answering. Check that it is running.');
		expect(plainMessage(new Error('Is it on?'))).toBe('Is it on?');
	});
	it('never shows an empty line', () => {
		expect(plainMessage(new ApiError('  ', 500))).toBe('Something went wrong.');
		expect(plainMessage('odd')).toBe('Something went wrong.');
	});
});

describe('the YouTube section', () => {
	const youtube = () => sections.find((s) => s.href === '/settings/youtube');
	const as = (role: 'owner' | 'admin' | 'member') => (p: string) => role === 'owner' || (p === 'users' && role === 'admin') || (p === 'settings' && role !== 'member');

	afterEach(() => setHeld(() => false));

	it('opens for the owner and admins, not members', () => {
		const s = youtube();
		expect(s).toMatchObject({ group: 'library', icon: 'play', feature: 'youtube', need: 'users' });
		expect(mayOpen(s!, as('owner'))).toBe(true);
		expect(mayOpen(s!, as('admin'))).toBe(true);
		expect(mayOpen(s!, as('member'))).toBe(false);
	});
	it('is hidden while the youtube feature is held', () => {
		setHeld((f) => f === 'youtube');
		expect(mayOpen(youtube()!, as('owner'))).toBe(false);
		expect(visible(sections, as('owner')).some((s) => s.href === '/settings/youtube')).toBe(false);
	});
});
