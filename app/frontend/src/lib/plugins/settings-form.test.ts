import { describe, expect, it } from 'vitest';
import { badNumbers, wholeNumber } from './settings-form.ts';

describe('wholeNumber (#922)', () => {
	const cases: [string, unknown, boolean][] = [
		['empty', '', false],
		['spaces only', '   ', false],
		['missing', undefined, false],
		['null', null, false],
		['zero', '0', true],
		['positive', '42', true],
		['padded', ' 7 ', true],
		['negative', '-1', false],
		['decimal', '1.5', false],
		['letters', 'abc', false],
		['mixed', '12a', false]
	];
	it.each(cases)('%s', (_n, v, want) => expect(wholeNumber(v)).toBe(want));
});

describe('badNumbers', () => {
	const fields = [
		{ key: 'port', type: 'number', default: '8765' },
		{ key: 'limit', type: 'number' },
		{ key: 'name', type: 'text' }
	];
	it('flags an empty number field so Save stays off', () => {
		expect(badNumbers(fields, { port: '', name: '' })).toEqual(['port']);
	});
	it('lets a number field with no default stay empty', () => {
		expect(badNumbers(fields, { port: '1', limit: '', name: '' })).toEqual([]);
	});
	it('still flags bad text in a field with no default', () => {
		expect(badNumbers(fields, { port: '1', limit: '-3', name: '' })).toEqual(['limit']);
	});
	it('ignores empty text fields', () => {
		expect(badNumbers(fields, { port: '0', name: '' })).toEqual([]);
	});
});
