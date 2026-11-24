import { describe, expect, it } from 'vitest';
import { cleanupLine, type CleanupCheck } from './streaming-cleanup.ts';

const base: CleanupCheck = { running: false, checked: 0, total: 0, failed: 0, region: 'US', found: [] };
const one = { id: 1, kind: 'movie' as const, name: 'Dune', tmdb_id: '1', bytes: 2048, files: 1, service: 'Netflix' };

describe('cleanupLine', () => {
	it.each([
		{ name: 'never run', c: null, has: '' },
		{ name: 'not started', c: base, has: '' },
		{ name: 'running', c: { ...base, running: true, checked: 3, total: 10 }, has: '3' },
		{ name: 'nothing found', c: { ...base, total: 5, finished_at: 'x' }, has: '5' },
		{ name: 'found', c: { ...base, total: 5, found: [one], finished_at: 'x' }, has: '2.0 KB' },
		{ name: 'some reads failed', c: { ...base, total: 5, failed: 2, finished_at: 'x' }, has: '2' }
	])('$name', ({ c, has }) => {
		const line = cleanupLine(c);
		if (has) expect(line).toContain(has);
		else expect(line).toBe('');
	});
});
