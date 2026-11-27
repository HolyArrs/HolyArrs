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
		{ key: 'wait', type: 'number', default: '30' },
		{ key: 'name', type: 'text' }
	];
	it('flags an empty number field so Save stays off', () => {
		expect(badNumbers(fields, { port: '', limit: '5', wait: '30', name: '' })).toEqual(['port']);
	});
	it('flags a cleared number field that has no default (#922)', () => {
		expect(badNumbers(fields, { port: '1', wait: '30', limit: '' })).toEqual(['limit']);
	});
	it('flags a whitespace-only number field with no default (#922)', () => {
		expect(badNumbers(fields, { port: '1', wait: '30', limit: '   ' })).toEqual(['limit']);
	});
	it('flags a number field the view never sent a value for (#922)', () => {
		expect(badNumbers(fields, { port: '1', wait: '30' })).toEqual(['limit']);
	});
	it('flags every empty number field at once (#922)', () => {
		expect(badNumbers(fields, { port: '', limit: '', wait: '', name: '' })).toEqual(['port', 'limit', 'wait']);
	});
	it('accepts number fields that hold a whole number', () => {
		expect(badNumbers(fields, { port: '1', limit: '0', wait: '30', name: '' })).toEqual([]);
	});
	it('still flags bad text in a field with no default', () => {
		expect(badNumbers(fields, { port: '1', wait: '30', limit: '-3', name: '' })).toEqual(['limit']);
	});
	it('ignores empty text fields', () => {
		expect(badNumbers(fields, { port: '0', wait: '30', limit: '5', name: '' })).toEqual([]);
	});
	it('ignores other field types left empty', () => {
		const extra = [
			{ key: 'token', type: 'secret' },
			{ key: 'on', type: 'toggle' },
			{ key: 'mode', type: 'select', options: ['a'] }
		];
		expect(badNumbers(extra, { token: '', on: '', mode: '' })).toEqual([]);
	});
});
