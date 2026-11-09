import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import { plainMessage } from './settings.ts';

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
