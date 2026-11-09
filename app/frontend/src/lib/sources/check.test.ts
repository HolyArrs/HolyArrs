import { describe, expect, it } from 'vitest';
import { checkCount, checkSummary, checkTone, checkWord, type SourceCheck } from './check.ts';

const row = (o: Partial<SourceCheck>): SourceCheck => ({ id: 1, name: 'A', answered: false, paused: false, count: 0, ms: 0, health: { state: 'working', text: '' }, ...o });

describe('source check', () => {
	it('shows each row as one word with its tone', () => {
		expect(checkWord(row({ answered: true }))).toBe('Connected');
		expect(checkTone(row({ answered: true }))).toBe('ok');
		expect(checkWord(row({}))).toBe('Not connected');
		expect(checkTone(row({}))).toBe('fail');
		expect(checkWord(row({ paused: true, health: { state: 'unreachable', text: '' } }))).toBe('Paused');
	});
	it('shows how many results came back', () => {
		expect(checkCount(row({ answered: true, count: 1 }))).toBe('1 result');
		expect(checkCount(row({ answered: true, count: 12 }))).toBe('12 results');
		expect(checkCount(row({ count: 3 }))).toBe('');
	});
	it('sums up a whole test search', () => {
		expect(checkSummary([])).toBe('No enabled Sources to ask.');
		expect(checkSummary([row({ answered: true }), row({}), row({ paused: true })])).toBe('1 of 3 connected, 1 failed, 1 paused.');
	});
});
