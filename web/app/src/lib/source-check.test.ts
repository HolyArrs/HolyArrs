import { describe, expect, it } from 'vitest';
import { checkCount, checkSummary, checkTone, checkWord, type SourceCheck } from './source-check.ts';

const row = (o: Partial<SourceCheck>): SourceCheck => ({ id: 1, name: 'A', answered: false, paused: false, count: 0, ms: 0, health: { state: 'working', text: '' }, ...o });

describe('source check', () => {
	it('says each row in one word with the health tones', () => {
		expect(checkWord(row({ answered: true }))).toBe('Answered');
		expect(checkTone(row({ answered: true }))).toBe('ok');
		expect(checkWord(row({}))).toBe('Failed');
		expect(checkTone(row({}))).toBe('fail');
		expect(checkWord(row({ paused: true, health: { state: 'unreachable', text: '' } }))).toBe('Paused');
	});
	it('counts results', () => {
		expect(checkCount(row({ answered: true, count: 1 }))).toBe('1 result');
		expect(checkCount(row({ answered: true, count: 12 }))).toBe('12 results');
		expect(checkCount(row({ count: 3 }))).toBe('');
	});
	it('sums up a run', () => {
		expect(checkSummary([])).toBe('No enabled Sources to ask.');
		expect(checkSummary([row({ answered: true }), row({}), row({ paused: true })])).toBe('1 of 3 answered, 1 failed, 1 paused.');
	});
});
