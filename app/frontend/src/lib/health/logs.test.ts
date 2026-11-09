import { describe, expect, it } from 'vitest';
import { emptyFilter, levelTone, logQuery, pageWords } from './logs.ts';

describe('logs', () => {
	it('builds the query from the filled fields only', () => {
		expect(logQuery(emptyFilter())).toBe('limit=100');
		expect(logQuery({ level: 'error', q: ' qbit ', from: '2026-10-01', to: '2026-10-02', offset: 100, limit: 50 })).toBe(
			'level=error&q=qbit&from=2026-10-01&to=2026-10-02&offset=100&limit=50'
		);
	});
	it('colours levels', () => {
		expect(levelTone('error')).toBe('fail');
		expect(levelTone('warning')).toBe('wait');
		expect(levelTone('info')).toBe('');
	});
	it('says which lines are shown', () => {
		expect(pageWords({ total: 0, offset: 0 }, 0)).toBe('No lines match.');
		expect(pageWords({ total: 250, offset: 100 }, 100)).toBe('Lines 101 to 200 of 250, newest first.');
	});
});
